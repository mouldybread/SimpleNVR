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

type diskUsageCache struct {
 mu sync.Mutex
 dirs map[string]map[string]diskUsageDay
}

func newDiskUsageCache() *diskUsageCache {
 return &diskUsageCache{dirs:make(map[string]map[string]diskUsageDay)}
}

// The production cache is keyed by camera record directory. Eviction on camera
// deletion is wired in a later slice; until then entries are bounded by cameras x 400.
var recordingDiskUsageCache=newDiskUsageCache()

func (c *diskUsageCache) lookup(dir,day string,stat *unix.Stat_t) (int64,bool) {
 c.mu.Lock();defer c.mu.Unlock()
 entry,ok:=c.dirs[dir][day]
 if !ok || !entry.matches(stat) {return 0,false}
 return entry.bytes,true
}

func (c *diskUsageCache) store(dir,day string,stat *unix.Stat_t,total int64) {
 c.mu.Lock();defer c.mu.Unlock()
 days:=c.dirs[dir]
 if days==nil {days=make(map[string]diskUsageDay);c.dirs[dir]=days}
 days[day]=diskUsageDay{dev:uint64(stat.Dev),ino:uint64(stat.Ino),mtim:stat.Mtim,ctim:stat.Ctim,bytes:total}
}

// retain drops cached days that no longer exist in the camera directory.
func (c *diskUsageCache) retain(dir string,present map[string]bool) {
 c.mu.Lock();defer c.mu.Unlock()
 for day:=range c.dirs[dir] {
  if !present[day] {delete(c.dirs[dir],day)}
 }
}

func (c *diskUsageCache) evict(dir string) {
 c.mu.Lock();defer c.mu.Unlock()
 delete(c.dirs,dir)
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
   cache.store(dir,listing.name,&after,dayTotal)
   work.Stats.CacheStores++
  }
 }
 budget:=eventStagingScanEntries
 for _,name:=range staging {
  sub,err:=work.sumStaging(ctx,int(root.Fd()),name,eventStagingScanDepth,&budget)
  if err!=nil {return 0,err}
  total+=sub
 }
 cache.retain(dir,present)
 return total,nil
}

func diskUsageText(ctx context.Context,cache *diskUsageCache,dir string) string {
 total,err:=computeDiskUsage(ctx,cache,dir,nil)
 if err!=nil {return diskUsageUnavailable}
 return formatBytes(total)
}

// Compatibility wrapper for the camera list: an incomplete inspection is shown as
// unavailable instead of a silently smaller number.
func calculateDiskUsage(recordDir string) string {
 return diskUsageText(context.Background(),recordingDiskUsageCache,recordDir)
}
