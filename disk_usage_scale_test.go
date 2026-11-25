package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func (s *diskUsageStats) add(o diskUsageStats) {
 s.RootReads+=o.RootReads;s.DayReads+=o.DayReads;s.StagingReads+=o.StagingReads;s.EntriesRead+=o.EntriesRead
 s.MetadataCalls+=o.MetadataCalls;s.DaysListed+=o.DaysListed;s.DaysOpened+=o.DaysOpened
 s.CacheHits+=o.CacheHits;s.CacheMisses+=o.CacheMisses;s.CacheStores+=o.CacheStores
}

// After every kind of change, the cached result must equal both a fresh walk and an
// independently computed expected total. Pauses keep each change out of the previous
// timestamp tick of the directory that the cache remembers.
func TestDiskUsageCachedMatchesFreshWalk(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");outside:=t.TempDir();cache:=newDiskUsageCache()
 scanFixture(t,dir,scanTime("20200101","100000"),scanTime("20200101","101000"),"a")
 d2a:=scanFixture(t,dir,scanTime("20200102","100000"),scanTime("20200102","101000"),"a")
 scanFixture(t,dir,scanTime("20200102","110000"),scanTime("20200102","111000"),"b")
 scanFixture(t,dir,scanTime("20200103","100000"),scanTime("20200103","101000"),"a")
 if err:=os.Mkdir(filepath.Join(dir,"20200104"),0700);err!=nil {t.Fatal(err)}
 putCleanupFixture(t,filepath.Join(dir,"legacy.mp4"),"legacy")
 putCleanupFixture(t,filepath.Join(dir,"buffer","b.mp4"),"abc")
 putCleanupFixture(t,filepath.Join(dir,"temp","e","p.mp4"),"wxyz")
 putCleanupFixture(t,filepath.Join(outside,"big.mp4"),"0123456789")
 ctx:=context.Background()
 check:=func(step string,want int64) {
  t.Helper()
  work:=farFutureDiskUsageWork()
  cached,err:=computeDiskUsage(ctx,cache,dir,work)
  if err!=nil {t.Fatal(step,"cached",err)}
  fresh,err:=computeDiskUsage(ctx,nil,dir,farFutureDiskUsageWork())
  if err!=nil {t.Fatal(step,"fresh",err)}
  if cached!=want || fresh!=want {t.Fatal(step,"cached",cached,"fresh",fresh,"want",want)}
 }
 change:=func(mutate func()) {time.Sleep(50*time.Millisecond);mutate()}
 check("baseline",41)
 repeat:=farFutureDiskUsageWork()
 if total,err:=computeDiskUsage(ctx,cache,dir,repeat);err!=nil || total!=41 || repeat.Stats.CacheHits==0 {t.Fatal("repeat was not served from the cache",total,repeat.Stats,err)}
 change(func(){scanFixture(t,dir,scanTime("20200101","120000"),scanTime("20200101","121000"),"b")})
 check("new file in an old day",48)
 change(func(){if err:=os.Remove(d2a);err!=nil {t.Fatal(err)}})
 check("retention removed a file",41)
 change(func(){if err:=os.RemoveAll(filepath.Join(dir,"20200103"));err!=nil {t.Fatal(err)}})
 check("retention removed a whole day",34)
 change(func(){
  day:=filepath.Join(dir,"20200102")
  if err:=os.Rename(day,filepath.Join(outside,"old-0102"));err!=nil {t.Fatal(err)}
  scanFixture(t,dir,scanTime("20200102","140000"),scanTime("20200102","141000"),"c")
  scanFixture(t,dir,scanTime("20200102","150000"),scanTime("20200102","151000"),"d")
 })
 check("day replaced by a different directory",41)
 change(func(){
  day:=filepath.Join(dir,"20200101")
  if err:=os.RemoveAll(day);err!=nil {t.Fatal(err)}
  if err:=os.Symlink(outside,day);err!=nil {t.Fatal(err)}
 })
 check("day replaced by a symlink",27)
 change(func(){scanFixture(t,dir,scanTime("20200102","160000"),scanTime("20200102","161000"),"x_partial")})
 check("partial file",34)
 change(func(){putCleanupFixture(t,filepath.Join(dir,"legacy2.mp4"),"12345")})
 check("loose root file",39)
 change(func(){putCleanupFixture(t,filepath.Join(dir,"temp","e","q.mp4"),"12")})
 check("staging file added",41)
 change(func(){if err:=os.Remove(filepath.Join(dir,"buffer","b.mp4"));err!=nil {t.Fatal(err)}})
 check("staging file removed",38)
 change(func(){if err:=os.Mkdir(filepath.Join(dir,"20200105"),0700);err!=nil {t.Fatal(err)}})
 check("empty day folder",38)
}

// A change in the same timestamp tick as the scan can look unchanged, so such a day
// is never cached and the next render reads it again.
func TestDiskUsageSameTickChangeIsNeverServedFromCache(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");cache:=newDiskUsageCache()
 first:=scanFixture(t,dir,scanTime("20200101","100000"),scanTime("20200101","101000"),"a")
 info,err:=os.Stat(filepath.Dir(first))
 if err!=nil {t.Fatal(err)}
 clock:=func() time.Time {return info.ModTime().Add(time.Second)}
 one:=newDiskUsageWork();one.now=clock
 total,err:=computeDiskUsage(context.Background(),cache,dir,one)
 if err!=nil || total!=7 || one.Stats.CacheStores!=0 {t.Fatal(total,one.Stats,err)}
 scanFixture(t,dir,scanTime("20200101","110000"),scanTime("20200101","111000"),"b")
 two:=newDiskUsageWork();two.now=clock
 total,err=computeDiskUsage(context.Background(),cache,dir,two)
 if err!=nil || total!=14 || two.Stats.CacheHits!=0 || two.Stats.CacheStores!=0 {t.Fatal("same-tick change was not seen",total,two.Stats,err)}
}

// 34 cameras with a year of day folders. Counters, not timing: once the closed days
// are cached, a render costs one stat per day folder and reads no day contents.
func TestDiskUsage34CamerasYearOfDayFolders(t *testing.T) {
 const cameras,days=34,365
 root:=t.TempDir();cache:=newDiskUsageCache();base:=scanTime("20200101","100000")
 dirs:=make([]string,cameras)
 for c:=range dirs {
  dirs[c]=filepath.Join(root,fmt.Sprintf("cam%02d",c))
  for d:=0;d<days;d++ {
   start:=base.AddDate(0,0,d);day:=filepath.Join(dirs[c],stampedDayName(start))
   if err:=os.MkdirAll(day,0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(day,stampedFileName(start,start.Add(time.Minute),"a")),[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
  }
 }
 pass:=func() (diskUsageStats,int64) {
  var sum diskUsageStats;var total int64
  for _,dir:=range dirs {
   work:=farFutureDiskUsageWork()
   got,err:=computeDiskUsage(context.Background(),cache,dir,work)
   if err!=nil {t.Fatal(dir,err)}
   total+=got;sum.add(work.Stats)
  }
  return sum,total
 }
 first,total:=pass()
 if total!=cameras*days*7 || first.CacheMisses!=cameras*days || first.CacheStores!=cameras*days || first.DaysOpened!=cameras*days {t.Fatal("first render",total,first)}
 second,total:=pass()
 if total!=cameras*days*7 || second.CacheHits!=cameras*days || second.CacheMisses!=0 || second.DaysOpened!=0 || second.DayReads!=0 || second.StagingReads!=0 || second.MetadataCalls!=cameras*days || second.EntriesRead!=cameras*days {t.Fatal("cached render did more than one stat per day folder",total,second)}
 // Only a changed day is read again: add a file to the newest day of every camera.
 time.Sleep(50*time.Millisecond)
 last:=base.AddDate(0,0,days-1)
 for _,dir:=range dirs {
  extra:=filepath.Join(dir,stampedDayName(last),stampedFileName(last.Add(2*time.Hour),last.Add(2*time.Hour+time.Minute),"extra"))
  if err:=os.WriteFile(extra,[]byte("12345"),0600);err!=nil {t.Fatal(err)}
 }
 third,total:=pass()
 if total!=cameras*days*7+cameras*5 || third.CacheHits!=cameras*(days-1) || third.CacheMisses!=cameras || third.DaysOpened!=cameras || third.DayReads==0 {t.Fatal("only the changed day should be read",total,third)}
}

// Staging is walked live under the existing event staging entry and depth limits,
// and exceeding either is incomplete, never a smaller figure.
func TestDiskUsageStagingBoundsAreIncompleteNotSmaller(t *testing.T) {
 t.Run("entries",func(t *testing.T) {
  dir:=t.TempDir();staging:=filepath.Join(dir,"temp")
  if err:=os.Mkdir(staging,0700);err!=nil {t.Fatal(err)}
  for i:=0;i<eventStagingScanEntries;i++ {
   if err:=os.WriteFile(filepath.Join(staging,fmt.Sprintf("n%05d.mp4",i)),nil,0600);err!=nil {t.Fatal(err)}
  }
  total,err:=computeDiskUsage(context.Background(),nil,dir,nil)
  if err!=nil || total!=0 {t.Fatal("staging at the entry limit refused",total,err)}
  if err:=os.WriteFile(filepath.Join(staging,"overflow.mp4"),nil,0600);err!=nil {t.Fatal(err)}
  if _,err:=computeDiskUsage(context.Background(),nil,dir,nil);!errors.Is(err,errDiskUsageIncomplete) {t.Fatal("staging over the entry limit accepted",err)}
 })
 t.Run("depth",func(t *testing.T) {
  chain:=func(levels int) string {
   dir:=t.TempDir();parts:=[]string{dir,"temp"}
   for i:=0;i<levels;i++ {parts=append(parts,"d")}
   bottom:=filepath.Join(parts...)
   if err:=os.MkdirAll(bottom,0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(bottom,"x.mp4"),[]byte("12345"),0600);err!=nil {t.Fatal(err)}
   return dir
  }
  total,err:=computeDiskUsage(context.Background(),nil,chain(eventStagingScanDepth),nil)
  if err!=nil || total!=5 {t.Fatal("staging at the depth limit refused",total,err)}
  if _,err:=computeDiskUsage(context.Background(),nil,chain(eventStagingScanDepth+1),nil);!errors.Is(err,errDiskUsageIncomplete) {t.Fatal("staging over the depth limit accepted",err)}
 })
}
