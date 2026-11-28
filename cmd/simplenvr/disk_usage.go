package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "sync"
 "time"

 "golang.org/x/sys/unix"
)

const diskUsageUnavailable="Unavailable"
const diskUsageEntryLimit=8192
const diskUsageReadBatch=64

// A render waits at most this long in total; slower cameras show unavailable while
// their inspection keeps running in the background and fills the cache.
const diskUsageRenderDeadline=10*time.Second

// One inspection is abandoned after this long, independently of any request.
const diskUsageComputeTimeout=2*time.Minute

// At most this many cameras are inspected at once across all renders.
const diskUsageSlots=4

// A cached day must be older than the scan by more than the filesystem timestamp
// granularity, so a change in the same tick as the scan can never be cached.
const diskUsageTimestampMargin=2*time.Second

// A day can still receive published files until its start day plus the maximum
// stamped span; this margin is deliberately larger than the finalize grace.
const diskUsageFinalizeMargin=time.Minute

var errDiskUsageIncomplete=errors.New("disk usage inspection incomplete")
var errDiskUsageVanished=errors.New("disk usage day vanished or was replaced")

// Counters belong to one call and are never global. EntriesRead counts every name
// returned by directory reads. MetadataCalls counts every stat, including the
// per-entry stats of the camera root.
type diskUsageStats struct {
 RootReads,DayReads,StagingReads,EntriesRead int
 MetadataCalls,DaysListed,DaysOpened,CacheHits,CacheMisses,CacheStores int
}

type diskUsageWork struct {
 Stats diskUsageStats
 now func() time.Time
 readHook func(*os.File,int)([]string,error)
 statatHook func(int,string,*unix.Stat_t,int) error
 fstatHook func(int,*unix.Stat_t) error
}

func newDiskUsageWork() *diskUsageWork {
 return &diskUsageWork{
  now:time.Now,
  readHook:func(file *os.File,n int)([]string,error){return file.Readdirnames(n)},
  statatHook:unix.Fstatat,
  fstatHook:unix.Fstat,
 }
}

func (w *diskUsageWork) read(dir *os.File,reads *int) ([]string,error) {
 *reads++
 names,err:=w.readHook(dir,diskUsageReadBatch)
 w.Stats.EntriesRead+=len(names)
 return names,err
}
func (w *diskUsageWork) statat(fd int,name string,out *unix.Stat_t,flags int) error {
 w.Stats.MetadataCalls++
 return w.statatHook(fd,name,out,flags)
}
func (w *diskUsageWork) fstat(fd int,out *unix.Stat_t) error {
 w.Stats.MetadataCalls++
 return w.fstatHook(fd,out)
}

// A cached closed day is valid only while the directory keeps the same identity,
// mtime and ctime. Published media is immutable, so file contents are not rechecked.
type diskUsageDay struct {
 dev,ino uint64
 mtim,ctim unix.Timespec
 bytes int64
}

func (entry diskUsageDay) matches(stat *unix.Stat_t) bool {
 return stat.Mode&unix.S_IFMT==unix.S_IFDIR && entry.dev==uint64(stat.Dev) && entry.ino==uint64(stat.Ino) && entry.mtim==stat.Mtim && entry.ctim==stat.Ctim
}

// The cache is keyed by camera record directory. Every eviction bumps the
// directory's generation; an inspection that began before the eviction cannot
// store into, or prune, the newer state.
type diskUsageCache struct {
 mu sync.Mutex
 dirs map[string]map[string]diskUsageDay
 gens map[string]uint64
}

func newDiskUsageCache() *diskUsageCache {
 return &diskUsageCache{dirs:make(map[string]map[string]diskUsageDay),gens:make(map[string]uint64)}
}

var recordingDiskUsageCache=newDiskUsageCache()

func (c *diskUsageCache) generation(dir string) uint64 {
 c.mu.Lock();defer c.mu.Unlock()
 return c.gens[dir]
}

func (c *diskUsageCache) lookup(dir,day string,stat *unix.Stat_t) (int64,bool) {
 c.mu.Lock();defer c.mu.Unlock()
 entry,ok:=c.dirs[dir][day]
 if !ok || !entry.matches(stat) {return 0,false}
 return entry.bytes,true
}

func (c *diskUsageCache) store(dir string,gen uint64,day string,stat *unix.Stat_t,total int64) {
 c.mu.Lock();defer c.mu.Unlock()
 if c.gens[dir]!=gen {return}
 days:=c.dirs[dir]
 if days==nil {days=make(map[string]diskUsageDay);c.dirs[dir]=days}
 days[day]=diskUsageDay{dev:uint64(stat.Dev),ino:uint64(stat.Ino),mtim:stat.Mtim,ctim:stat.Ctim,bytes:total}
}

// retain drops cached days that no longer exist in the camera directory.
func (c *diskUsageCache) retain(dir string,gen uint64,present map[string]bool) {
 c.mu.Lock();defer c.mu.Unlock()
 if c.gens[dir]!=gen {return}
 for day:=range c.dirs[dir] {
  if !present[day] {delete(c.dirs[dir],day)}
 }
}

func (c *diskUsageCache) evict(dir string) {
 c.mu.Lock();defer c.mu.Unlock()
 c.gens[dir]++
 delete(c.dirs,dir)
}

// pruneExcept evicts every cached directory that is not currently registered, which
// also covers a camera whose record directory changed.
func (c *diskUsageCache) pruneExcept(active map[string]bool) {
 c.mu.Lock();defer c.mu.Unlock()
 for dir:=range c.dirs {
  if !active[dir] {
   c.gens[dir]++
   delete(c.dirs,dir)
  }
 }
}

func (c *diskUsageCache) size(dir string) int {
 c.mu.Lock();defer c.mu.Unlock()
 return len(c.dirs[dir])
}

func sameDiskUsageStamp(a,b *unix.Stat_t) bool {
 return a.Dev==b.Dev && a.Ino==b.Ino && a.Mtim==b.Mtim && a.Ctim==b.Ctim
}

// A day is cached only when it can no longer receive published files and its
// timestamps are old enough to be trusted.
func diskUsageCacheable(day string,scanStart time.Time,stat *unix.Stat_t) bool {
 start,err:=time.ParseInLocation(stampDayLayout,day,time.UTC)
 if err!=nil {return false}
 if !scanStart.After(start.AddDate(0,0,1).Add(maxStampedSpan+diskUsageFinalizeMargin)) {return false}
 for _,stamp:=range []unix.Timespec{stat.Mtim,stat.Ctim} {
  if scanStart.Sub(time.Unix(int64(stamp.Sec),int64(stamp.Nsec)))<=diskUsageTimestampMargin {return false}
 }
 return true
}

// Sums .mp4 regular files of one day folder with descriptor-relative no-follow
// stats. It returns the directory's state after the scan so the caller can tell
// whether it changed while being read.
func (w *diskUsageWork) sumDay(ctx context.Context,root *os.File,name string,listed *unix.Stat_t) (int64,unix.Stat_t,error) {
 var after unix.Stat_t
 day,err:=openCameraDeleteDirectory(int(root.Fd()),name)
 if err!=nil {
  if errors.Is(err,os.ErrNotExist) || errors.Is(err,unix.ELOOP) || errors.Is(err,unix.ENOTDIR) {return 0,after,errDiskUsageVanished}
  return 0,after,errDiskUsageIncomplete
 }
 defer day.Close()
 w.Stats.DaysOpened++
 var opened unix.Stat_t
 if err:=w.fstat(int(day.Fd()),&opened);err!=nil {return 0,after,errDiskUsageIncomplete}
 if opened.Dev!=listed.Dev || opened.Ino!=listed.Ino {return 0,after,errDiskUsageVanished}
 var total int64
 seen:=0
 for {
  if err:=ctx.Err();err!=nil {return 0,after,err}
  names,readErr:=w.read(day,&w.Stats.DayReads)
  for _,entry:=range names {
   seen++
   if seen>diskUsageEntryLimit {return 0,after,errDiskUsageIncomplete}
   if filepath.Ext(entry)!=".mp4" {continue}
   var stat unix.Stat_t
   if err:=w.statat(int(day.Fd()),entry,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
    if errors.Is(err,os.ErrNotExist) {continue}
    return 0,after,errDiskUsageIncomplete
   }
   if stat.Mode&unix.S_IFMT==unix.S_IFREG {total+=stat.Size}
  }
  if readErr==io.EOF {break}
  if readErr!=nil {return 0,after,errDiskUsageIncomplete}
 }
 if err:=w.fstat(int(day.Fd()),&after);err!=nil {return 0,after,errDiskUsageIncomplete}
 return total,after,nil
}

// Sums .mp4 regular files under buffer/ or temp/ live and bounded. Staging churns,
// so it is never cached. Symlinks are not followed.
func (w *diskUsageWork) sumStaging(ctx context.Context,parent int,name string,depth int,budget *int) (int64,error) {
 dir,err:=openCameraDeleteDirectory(parent,name)
 if err!=nil {
  if errors.Is(err,os.ErrNotExist) || errors.Is(err,unix.ELOOP) || errors.Is(err,unix.ENOTDIR) {return 0,nil}
  return 0,errDiskUsageIncomplete
 }
 defer dir.Close()
 var total int64
 for {
  if err:=ctx.Err();err!=nil {return 0,err}
  names,readErr:=w.read(dir,&w.Stats.StagingReads)
  for _,entry:=range names {
   *budget--
   if *budget<0 {return 0,errDiskUsageIncomplete}
   var stat unix.Stat_t
   if err:=w.statat(int(dir.Fd()),entry,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
    if errors.Is(err,os.ErrNotExist) {continue}
    return 0,errDiskUsageIncomplete
   }
   switch stat.Mode&unix.S_IFMT {
   case unix.S_IFDIR:
    if depth<=0 {return 0,errDiskUsageIncomplete}
    sub,err:=w.sumStaging(ctx,int(dir.Fd()),entry,depth-1,budget)
    if err!=nil {return 0,err}
    total+=sub
   case unix.S_IFREG:
    if filepath.Ext(entry)==".mp4" {total+=stat.Size}
   }
  }
  if readErr==io.EOF {return total,nil}
  if readErr!=nil {return 0,errDiskUsageIncomplete}
 }
}

// computeDiskUsage returns the total size of .mp4 regular files that belong to one
// camera: loose files in the camera root, valid UTC day folders (closed days are
// served from the cache when their identity and timestamps are unchanged), and
// buffer/ and temp/ staging. Anything it cannot inspect completely is an error,
// never a smaller number. A missing camera directory has no recordings yet.
// Bounds: 8,192 entries per directory and 400 day folders.
func computeDiskUsage(ctx context.Context,cache *diskUsageCache,dir string,work *diskUsageWork) (int64,error) {
 if work==nil {work=newDiskUsageWork()}
 if cache==nil {cache=newDiskUsageCache()}
 gen:=cache.generation(dir)
 scanStart:=work.now()
 if err:=ctx.Err();err!=nil {return 0,err}
 root,err:=openCameraDeleteDirectory(unix.AT_FDCWD,dir)
 if err!=nil {
  if errors.Is(err,os.ErrNotExist) {return 0,nil}
  return 0,errDiskUsageIncomplete
 }
 defer root.Close()
 type dayListing struct {
  name string
  stat unix.Stat_t
 }
 var total int64
 var days []dayListing
 var staging []string
 seen:=0
 for {
  if err:=ctx.Err();err!=nil {return 0,err}
  names,readErr:=work.read(root,&work.Stats.RootReads)
  for _,name:=range names {
   seen++
   if seen>diskUsageEntryLimit {return 0,errDiskUsageIncomplete}
   isDay:=isStampedDayName(name)
   isStaging:=name=="buffer" || name=="temp"
   if !isDay && !isStaging && filepath.Ext(name)!=".mp4" {continue}
   var stat unix.Stat_t
   if err:=work.statat(int(root.Fd()),name,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
    if errors.Is(err,os.ErrNotExist) {continue}
    return 0,errDiskUsageIncomplete
   }
   switch {
   case isDay:
    // Symlinks and non-directories named like a day are never followed.
    if stat.Mode&unix.S_IFMT!=unix.S_IFDIR {continue}
    work.Stats.DaysListed++
    if len(days)>=maxStampedScanDays {return 0,errDiskUsageIncomplete}
    days=append(days,dayListing{name:name,stat:stat})
   case isStaging:
    if stat.Mode&unix.S_IFMT==unix.S_IFDIR {staging=append(staging,name)}
   default:
    if stat.Mode&unix.S_IFMT==unix.S_IFREG {total+=stat.Size}
   }
  }
  if readErr==io.EOF {break}
  if readErr!=nil {return 0,errDiskUsageIncomplete}
 }
 present:=make(map[string]bool,len(days))
 for _,listing:=range days {
  if err:=ctx.Err();err!=nil {return 0,err}
  present[listing.name]=true
  if cached,ok:=cache.lookup(dir,listing.name,&listing.stat);ok {
   work.Stats.CacheHits++
   total+=cached
   continue
  }
  work.Stats.CacheMisses++
  dayTotal,after,err:=work.sumDay(ctx,root,listing.name,&listing.stat)
  if errors.Is(err,errDiskUsageVanished) {continue}
  if err!=nil {return 0,err}
  total+=dayTotal
  if sameDiskUsageStamp(&listing.stat,&after) && diskUsageCacheable(listing.name,scanStart,&after) {
   cache.store(dir,gen,listing.name,&after,dayTotal)
   work.Stats.CacheStores++
  }
 }
 budget:=eventStagingScanEntries
 for _,name:=range staging {
  sub,err:=work.sumStaging(ctx,int(root.Fd()),name,eventStagingScanDepth,&budget)
  if err!=nil {return 0,err}
  total+=sub
 }
 cache.retain(dir,gen,present)
 return total,nil
}

func diskUsageText(ctx context.Context,cache *diskUsageCache,dir string) string {
 total,err:=computeDiskUsage(ctx,cache,dir,nil)
 if err!=nil {return diskUsageUnavailable}
 return formatBytes(total)
}

// Direct, single-call wrapper kept for callers outside the camera list render.
func calculateDiskUsage(recordDir string) string {
 return diskUsageText(context.Background(),recordingDiskUsageCache,recordDir)
}

// One inspection of one camera directory. Waiters share its result.
type diskUsageCall struct {
 done chan struct{}
 total int64
 err error
}

func (call *diskUsageCall) text() string {
 if call.err!=nil {return diskUsageUnavailable}
 return formatBytes(call.total)
}

// wait returns the shared result, or unavailable when the caller's own context ends
// first. The inspection itself is not cancelled by a caller giving up.
func (call *diskUsageCall) wait(ctx context.Context) string {
 select {
 case <-call.done:
  return call.text()
 default:
 }
 select {
 case <-call.done:
  return call.text()
 case <-ctx.Done():
  return diskUsageUnavailable
 }
}

// The tracker runs at most one inspection per camera directory at a time and at most
// len(slots) overall. Inspections run detached from requests with their own timeout
// so a slow first render still warms the cache for the next one.
type diskUsageTracker struct {
 mu sync.Mutex
 calls map[string]*diskUsageCall
 slots chan struct{}
 cache *diskUsageCache
 compute func(context.Context,*diskUsageCache,string)(int64,error)
 timeout time.Duration
}

func newDiskUsageTracker(cache *diskUsageCache,slots int,timeout time.Duration,compute func(context.Context,*diskUsageCache,string)(int64,error)) *diskUsageTracker {
 if slots<1 {slots=1}
 if compute==nil {
  compute=func(ctx context.Context,cache *diskUsageCache,dir string)(int64,error){return computeDiskUsage(ctx,cache,dir,nil)}
 }
 return &diskUsageTracker{calls:make(map[string]*diskUsageCall),slots:make(chan struct{},slots),cache:cache,compute:compute,timeout:timeout}
}

var recordingDiskUsage=newDiskUsageTracker(recordingDiskUsageCache,diskUsageSlots,diskUsageComputeTimeout,nil)

// start returns the inspection in flight for dir, or begins one without blocking.
func (t *diskUsageTracker) start(dir string) *diskUsageCall {
 t.mu.Lock();defer t.mu.Unlock()
 if call:=t.calls[dir];call!=nil {return call}
 call:=&diskUsageCall{done:make(chan struct{})}
 t.calls[dir]=call
 go t.run(dir,call)
 return call
}

func (t *diskUsageTracker) run(dir string,call *diskUsageCall) {
 t.slots<-struct{}{}
 ctx,cancel:=context.WithTimeout(context.Background(),t.timeout)
 total,err:=t.compute(ctx,t.cache,dir)
 cancel()
 <-t.slots
 t.mu.Lock()
 call.total,call.err=total,err
 delete(t.calls,dir)
 t.mu.Unlock()
 close(call.done)
}

// render returns one display string per directory, in order. It first drops cached
// directories that are no longer registered, starts every inspection so slow cameras
// keep working after the deadline, then waits for each up to the render deadline.
func (t *diskUsageTracker) render(ctx context.Context,dirs []string) []string {
 active:=make(map[string]bool,len(dirs))
 for _,dir:=range dirs {active[dir]=true}
 t.cache.pruneExcept(active)
 calls:=make([]*diskUsageCall,len(dirs))
 for i,dir:=range dirs {calls[i]=t.start(dir)}
 ctx,cancel:=context.WithTimeout(ctx,diskUsageRenderDeadline)
 defer cancel()
 out:=make([]string,len(dirs))
 for i,call:=range calls {out[i]=call.wait(ctx)}
 return out
}

// Called after a camera is removed from the registry.
func evictCameraDiskUsage(dir string) {
 recordingDiskUsage.cache.evict(dir)
}
