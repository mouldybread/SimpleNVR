package main

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func TestRetentionDayClassificationUTC(t *testing.T) {
 base:=scanTime("20261110","000000")
 for _,cutoff:=range []time.Time{base,base.Add(12*time.Hour),base.In(time.FixedZone("east",8*3600))} {
  for day,want:=range map[string]retentionDayClass{"20261108":retentionOldDay,"20261109":retentionBoundaryDay,"20261110":retentionBoundaryDay,"20261111":retentionRetainedDay} {
   if got:=classifyRetentionDay(day,cutoff);got!=want {t.Fatal(day,cutoff,got,want)}
  }
 }
}

func TestRetentionDaySelectionStrictCutoff(t *testing.T) {
 for _,hour:=range []time.Duration{0,12*time.Hour} {
  t.Run(fmt.Sprint(hour),func(t *testing.T) {
   lease:=publishedEngineLease(t);dir:=t.TempDir()
   cutoff:=scanTime("20261110","000000").Add(hour)
   old:=scanFixture(t,dir,cutoff.Add(-25*time.Hour),cutoff.Add(-2*time.Hour),"old")
   before:=scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff.Add(-time.Second),"before")
   equal:=scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff,"equal")
   after:=scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff.Add(time.Second),"after")
   newerStart:=stampedDayStart(cutoff).AddDate(0,0,1)
   newer:=scanFixture(t,dir,newerStart,newerStart.Add(time.Hour),"newer")
   work:=newPublishedDeletionWork()
   result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,work)
   if err!=nil || result.Count!=2 || result.Bytes!=14 || result.Failures!=0 || work.Stats.DaysSkipped!=1 || work.Stats.Unlinks!=2 {t.Fatal(result,work.Stats,err)}
   for _,path:=range []string{old,before} {if _,err:=os.Stat(path);!os.IsNotExist(err) {t.Fatal(path,err)}}
   for _,path:=range []string{equal,after,newer} {if _,err:=os.Stat(path);err!=nil {t.Fatal(path,err)}}
  })
 }
}

func TestRetentionNewerDayPreservesExceptionsUntilDayAgesIn(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();cutoff:=scanTime("20261110","120000")
 future:=filepath.Join(dir,"20261111");if err:=os.Mkdir(future,0700);err!=nil {t.Fatal(err)}
 oldStart:=scanTime("20261101","000000")
 misplaced:=filepath.Join(future,stampedFileName(oldStart,oldStart.Add(time.Hour),"misplaced"))
 prefix:=filepath.Join(future,"20261101_000000_legacy.mp4")
 fallback:=filepath.Join(future,"unparseable_long_name.mp4")
 for _,path:=range []string{misplaced,prefix,fallback} {
  if err:=os.WriteFile(path,[]byte("x"),0600);err!=nil {t.Fatal(err)}
  if err:=os.Chtimes(path,oldStart,oldStart);err!=nil {t.Fatal(err)}
 }
 first:=newPublishedDeletionWork()
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,first)
 if err!=nil || result.Count!=0 || first.Stats.DaysSkipped!=1 || first.Stats.DayReads!=0 {t.Fatal(result,first.Stats,err)}
 for _,path:=range []string{misplaced,prefix,fallback} {if _,err:=os.Stat(path);err!=nil {t.Fatal(path,err)}}
 result,err=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,scanTime("20261111","000000"),publishedEngineGuard,nil)
 if err!=nil || result.Count!=3 {t.Fatal("fallback not applied when day ages in",result,err)}
}

func TestRetentionSkipDoesNotChangeGenericOrDeleteAll(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();cutoff:=scanTime("20261110","120000")
 future:=filepath.Join(dir,"20261111");if err:=os.Mkdir(future,0700);err!=nil {t.Fatal(err)}
 old:=scanTime("20261101","000000")
 expired:=filepath.Join(future,stampedFileName(old,old.Add(time.Hour),"misplaced"))
 if err:=os.WriteFile(expired,[]byte("x"),0600);err!=nil {t.Fatal(err)}
 kept:=scanFixture(t,dir,scanTime("20261111","010000"),scanTime("20261111","020000"),"kept")
 result,err:=deletePublishedLeased(context.Background(),lease,dir,publishedExpiredBefore(cutoff),publishedEngineGuard)
 if err!=nil || result.Count!=1 {t.Fatal("generic selector unexpectedly skipped newer day",result,err)}
 if _,err:=os.Stat(kept);err!=nil {t.Fatal(err)}
 result,err=deleteAllPublishedLeased(context.Background(),lease,dir,publishedEngineGuard)
 if err!=nil || result.Count!=1 {t.Fatal("delete-all failed to inspect newer day",result,err)}
}

func TestRetentionSkippedContentsDoNotIncreaseWork(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();cutoff:=scanTime("20261110","120000")
 start:=scanTime("20261111","000000");day:=stampedDayDir(t,dir,start)
 stampedPutFile(t,day,start,start.Add(time.Minute),"first",true)
 first:=newPublishedDeletionWork()
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,first)
 if err!=nil || result.Count!=0 {t.Fatal(result,err)}
 for i:=0;i<100;i++ {stampedPutFile(t,day,start,start.Add(time.Minute),fmt.Sprint(i),true)}
 second:=newPublishedDeletionWork()
 result,err=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,second)
 if err!=nil || result.Count!=0 || second.Stats!=first.Stats || second.Stats.DaysOpened!=0 || second.Stats.DayReads!=0 {t.Fatal(result,first.Stats,second.Stats,err)}
}

func TestRetentionSkippedDaysStillSpendDayBudget(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();start:=scanTime("20240101","000000")
 for i:=0;i<=maxStampedScanDays;i++ {if err:=os.Mkdir(filepath.Join(dir,stampedDayName(start.AddDate(0,0,i))),0700);err!=nil {t.Fatal(err)}}
 work:=newPublishedDeletionWork()
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,start.AddDate(0,0,-1),publishedEngineGuard,work)
 if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || work.Stats.DayFolders!=401 || work.Stats.DaysSkipped!=400 || work.Stats.DaysOpened!=0 {t.Fatal(result,work.Stats,err)}
}

func TestPublishedWorkReadFailureCountsWholeBatch(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();work:=newPublishedDeletionWork();failure:=errors.New("read failed")
 work.readHook=func(_ *os.File,_ int)([]string,error){return []string{"a.tmp","b.tmp","c.tmp"},failure}
 result,err:=deletePublishedLeasedWithWork(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,unix.Unlinkat,work)
 if !errors.Is(err,failure) || result.Count!=0 || result.Failures!=1 || work.Stats.RootReads!=1 || work.Stats.EntriesRead!=3 || work.Stats.EntriesProcessed!=3 {t.Fatal(result,work.Stats,err)}
 // EOF with an empty batch is also an observed read, but no processed name.
 empty:=newPublishedDeletionWork();empty.readHook=func(*os.File,int)([]string,error){return nil,io.EOF}
 result,err=deletePublishedLeasedWithWork(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,unix.Unlinkat,empty)
 if err!=nil || result.Count!=0 || empty.Stats.RootReads!=1 || empty.Stats.EntriesRead!=0 {t.Fatal(result,empty.Stats,err)}
}
