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

func farFutureDiskUsageWork() *diskUsageWork {
 work:=newDiskUsageWork()
 work.now=func() time.Time {return time.Now().Add(240*time.Hour)}
 return work
}

func TestDiskUsageCountsPublishedRootAndStagingOnly(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");outside:=t.TempDir()
 scanFixture(t,dir,scanTime("20261110","100000"),scanTime("20261110","101000"),"a")
 scanFixture(t,dir,scanTime("20261110","101000"),scanTime("20261110","102000"),"b")
 putCleanupFixture(t,filepath.Join(dir,"legacy.mp4"),"legacy")
 putCleanupFixture(t,filepath.Join(dir,"buffer","buffer_20261110_100000_ab.mp4"),"abc")
 putCleanupFixture(t,filepath.Join(dir,"temp","event-a","pre.mp4"),"wxyz")
 putCleanupFixture(t,filepath.Join(dir,"capture.mp4.tmp"),"ignored-12345")
 putCleanupFixture(t,filepath.Join(dir,"20261110","capture.tmp"),"ignored-12345")
 putCleanupFixture(t,filepath.Join(dir,"unknown","stray.mp4"),"stray")
 target:=filepath.Join(outside,"big.mp4");putCleanupFixture(t,target,"0123456789")
 if err:=os.Symlink(target,filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(dir,"20261111"));err!=nil {t.Fatal(err)}
 total,err:=computeDiskUsage(context.Background(),nil,dir,nil)
 if err!=nil || total!=27 {t.Fatal("want 14 published + 6 root + 3 buffer + 4 temp",total,err)}
}

func TestDiskUsageClosedDayCache(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");cache:=newDiskUsageCache()
 var files []string
 for _,date:=range []string{"20200101","20200102","20200103"} {
  files=append(files,scanFixture(t,dir,scanTime(date,"100000"),scanTime(date,"101000"),"a"))
 }
 first:=farFutureDiskUsageWork()
 total,err:=computeDiskUsage(context.Background(),cache,dir,first)
 if err!=nil || total!=21 || first.Stats.CacheMisses!=3 || first.Stats.CacheStores!=3 || first.Stats.DaysOpened!=3 || cache.size(dir)!=3 {t.Fatal(total,first.Stats,err)}
 second:=farFutureDiskUsageWork()
 total,err=computeDiskUsage(context.Background(),cache,dir,second)
 if err!=nil || total!=21 || second.Stats.CacheHits!=3 || second.Stats.CacheMisses!=0 || second.Stats.DaysOpened!=0 || second.Stats.DayReads!=0 || second.Stats.MetadataCalls!=3 {t.Fatal("cached render read day contents",total,second.Stats,err)}
 // A new file in an old day changes the directory's mtime and ctime. The pause keeps
 // the change out of the previous timestamp tick.
 time.Sleep(50*time.Millisecond)
 putCleanupFixture(t,filepath.Join(filepath.Dir(files[1]),"20200102_120000_E20200102_121000_extra.mp4"),"12345")
 third:=farFutureDiskUsageWork()
 total,err=computeDiskUsage(context.Background(),cache,dir,third)
 if err!=nil || total!=26 || third.Stats.CacheHits!=2 || third.Stats.CacheMisses!=1 || third.Stats.DaysOpened!=1 || third.Stats.CacheStores!=1 {t.Fatal("new file not seen",total,third.Stats,err)}
 // A retention-style removal is seen the same way.
 time.Sleep(50*time.Millisecond)
 if err:=os.Remove(files[0]);err!=nil {t.Fatal(err)}
 fourth:=farFutureDiskUsageWork()
 total,err=computeDiskUsage(context.Background(),cache,dir,fourth)
 if err!=nil || total!=19 || fourth.Stats.CacheHits!=2 || fourth.Stats.CacheMisses!=1 {t.Fatal("removal not seen",total,fourth.Stats,err)}
 // A vanished day is dropped from the cache.
 time.Sleep(50*time.Millisecond)
 if err:=os.RemoveAll(filepath.Dir(files[2]));err!=nil {t.Fatal(err)}
 fifth:=farFutureDiskUsageWork()
 total,err=computeDiskUsage(context.Background(),cache,dir,fifth)
 if err!=nil || total!=12 || cache.size(dir)!=2 {t.Fatal("vanished day retained",total,cache.size(dir),err)}
}

func TestDiskUsageRacyAndOpenDaysAreNotCached(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");cache:=newDiskUsageCache()
 old:=scanFixture(t,dir,scanTime("20200101","100000"),scanTime("20200101","101000"),"a")
 info,err:=os.Stat(filepath.Dir(old))
 if err!=nil {t.Fatal(err)}
 racy:=newDiskUsageWork()
 racy.now=func() time.Time {return info.ModTime().Add(time.Second)}
 total,err:=computeDiskUsage(context.Background(),cache,dir,racy)
 if err!=nil || total!=7 || racy.Stats.CacheStores!=0 || cache.size(dir)!=0 {t.Fatal("day whose timestamps are within the margin was cached",total,racy.Stats,err)}
 closed:=farFutureDiskUsageWork()
 total,err=computeDiskUsage(context.Background(),cache,dir,closed)
 if err!=nil || total!=7 || closed.Stats.CacheStores!=1 {t.Fatal("closed day with old timestamps was not cached",total,closed.Stats,err)}
 // Today's folder can still receive files, so it is not cached even when the scan
 // clock is a day later and its timestamps are old enough.
 today:=time.Now().UTC()
 openDir:=filepath.Join(t.TempDir(),"camera")
 scanFixture(t,openDir,today,today.Add(time.Minute),"a")
 open:=newDiskUsageWork()
 open.now=func() time.Time {return time.Now().Add(24*time.Hour)}
 total,err=computeDiskUsage(context.Background(),cache,openDir,open)
 if err!=nil || total!=7 || open.Stats.CacheStores!=0 {t.Fatal("open day was cached",total,open.Stats,err)}
}

func TestDiskUsageBoundsAreIncompleteNotSmaller(t *testing.T) {
 t.Run("day-folders",func(t *testing.T) {
  dir:=t.TempDir();start:=scanTime("20240101","000000")
  for i:=0;i<maxStampedScanDays;i++ {
   if err:=os.Mkdir(filepath.Join(dir,stampedDayName(start.AddDate(0,0,i))),0700);err!=nil {t.Fatal(err)}
  }
  total,err:=computeDiskUsage(context.Background(),nil,dir,nil)
  if err!=nil || total!=0 {t.Fatal("400 day folders refused",total,err)}
  if err:=os.Mkdir(filepath.Join(dir,stampedDayName(start.AddDate(0,0,maxStampedScanDays))),0700);err!=nil {t.Fatal(err)}
  if _,err:=computeDiskUsage(context.Background(),nil,dir,nil);!errors.Is(err,errDiskUsageIncomplete) {t.Fatal("401 day folders accepted",err)}
 })
 t.Run("day-entries",func(t *testing.T) {
  dir:=t.TempDir();day:=filepath.Join(dir,"20261110")
  if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
  for i:=0;i<diskUsageEntryLimit;i++ {
   if err:=os.WriteFile(filepath.Join(day,fmt.Sprintf("n%05d.txt",i)),nil,0600);err!=nil {t.Fatal(err)}
  }
  total,err:=computeDiskUsage(context.Background(),nil,dir,nil)
  if err!=nil || total!=0 {t.Fatal("8192 entries refused",total,err)}
  if err:=os.WriteFile(filepath.Join(day,"overflow.txt"),nil,0600);err!=nil {t.Fatal(err)}
  if _,err:=computeDiskUsage(context.Background(),nil,dir,nil);!errors.Is(err,errDiskUsageIncomplete) {t.Fatal("8193 entries accepted",err)}
 })
}

func TestDiskUsageFailuresAreIncompleteOrUnavailable(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera")
 scanFixture(t,dir,scanTime("20261110","100000"),scanTime("20261110","101000"),"a")
 work:=newDiskUsageWork();reads:=0
 work.readHook=func(file *os.File,n int)([]string,error) {
  reads++
  if reads>=3 {return nil,os.ErrPermission}
  return file.Readdirnames(n)
 }
 total,err:=computeDiskUsage(context.Background(),nil,dir,work)
 if !errors.Is(err,errDiskUsageIncomplete) || total!=0 {t.Fatal("unreadable day under-counted",total,err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=computeDiskUsage(ctx,nil,dir,nil);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if got:=diskUsageText(ctx,nil,dir);got!=diskUsageUnavailable {t.Fatal(got)}
}

func TestDiskUsageMissingDirectoryAndWrapper(t *testing.T) {
 missing:=filepath.Join(t.TempDir(),"never-created")
 total,err:=computeDiskUsage(context.Background(),nil,missing,nil)
 if err!=nil || total!=0 {t.Fatal(total,err)}
 if got:=calculateDiskUsage(missing);got!="0 B" {t.Fatal(got)}
 dir:=filepath.Join(t.TempDir(),"camera")
 scanFixture(t,dir,scanTime("20261110","100000"),scanTime("20261110","101000"),"a")
 scanFixture(t,dir,scanTime("20261110","101000"),scanTime("20261110","102000"),"b")
 if got:=calculateDiskUsage(dir);got!="14 B" {t.Fatal(got)}
}
