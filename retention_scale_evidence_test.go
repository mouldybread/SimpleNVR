package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func TestRetention34CameraWorkIgnoresSkippedContents(t *testing.T) {
 lease:=publishedEngineLease(t);root:=t.TempDir();cutoff:=scanTime("20250101","120000")
 const cameras,retainedDays=34,363
 dirs:=make([]string,cameras);before:=make([]publishedDeletionStats,cameras)
 for c:=range dirs {
  dir:=filepath.Join(root,fmt.Sprintf("cam%02d",c));dirs[c]=dir
  for d:=1;d<=retainedDays;d++ {
   start:=stampedDayStart(cutoff).AddDate(0,0,d)
   stampedPutFile(t,stampedDayDir(t,dir,start),start,start.Add(time.Minute),"first",true)
  }
  old:=cutoff.Add(-48*time.Hour)
  scanFixture(t,dir,old,old.Add(time.Minute),"expired_partial")
  scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff,"equal")
  scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff.Add(time.Second),"after")
  work:=newPublishedDeletionWork()
  result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,work)
  if err!=nil || result.Count!=1 || result.Bytes!=7 || work.Stats.DayFolders!=365 || work.Stats.DaysSkipped!=363 || work.Stats.DaysOpened!=2 || work.Stats.EntriesRead!=368 || work.Stats.EntriesProcessed!=368 || work.Stats.Unlinks!=1 {t.Fatal(c,result,work.Stats,err)}
  if work.Stats.RootReads==0 || work.Stats.DayReads==0 {t.Fatal("root/day enumeration not observed",work.Stats)}
  before[c]=work.Stats
 }
 // Restore the same expired candidate after the first pass, then double only
 // skipped-day contents. Both passes perform real deletion with identical scope.
 for c,dir:=range dirs {
  old:=cutoff.Add(-48*time.Hour)
  scanFixture(t,dir,old,old.Add(time.Minute),"expired_partial")
  for d:=1;d<=retainedDays;d++ {
   start:=stampedDayStart(cutoff).AddDate(0,0,d)
   stampedPutFile(t,stampedDayDir(t,dir,start),start,start.Add(time.Minute),"second",true)
  }
  work:=newPublishedDeletionWork()
  result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,work)
  if err!=nil || result.Count!=1 || work.Stats!=before[c] {t.Fatal("retained content changed work",c,result,before[c],work.Stats,err)}
  assertPublicationBytes(t,filepath.Join(dir,stampedDayName(cutoff),stampedFileName(cutoff.Add(-time.Hour),cutoff,"equal")),"fixture")
 }
 // The remaining physical fixture has 24,752 files, not 1.79 million.
 if cameras*(retainedDays*2+2)<=20000 {t.Fatal("fixture does not exceed 20,000 files")}
}

func TestDeletionExactDayFolderBounds(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=filepath.Join(t.TempDir(),"camera")
 if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 base:=scanTime("20240101","000000")
 for i:=0;i<400;i++ {if err:=os.Mkdir(filepath.Join(dir,stampedDayName(base.AddDate(0,0,i))),0700);err!=nil {t.Fatal(err)}}
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,base.AddDate(0,0,-1),publishedEngineGuard,nil)
 if err!=nil || result.Count!=0 {t.Fatal("400 retained days refused",result,err)}
 admitted:=errors.New("first admitted unlink")
 calls:=0
 err=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int)error{calls++;return admitted})
 if !errors.Is(err,admitted) || calls!=1 {t.Fatal("400-day camera plan not admitted",err,calls)}
 budget:=newOrphanInspectionBudget(context.Background())
 if retained,err:=budget.inspect(dir);err!=nil || retained {t.Fatal("400-day orphan inspection refused",retained,err)}
 if err:=os.Mkdir(filepath.Join(dir,stampedDayName(base.AddDate(0,0,400))),0700);err!=nil {t.Fatal(err)}
 calls=0
 result,err=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,func(int,string,int)error{calls++;return nil})
 if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || calls!=0 {t.Fatal(result,err,calls)}
 err=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int)error{calls++;return nil})
 if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal(err,calls)}
 if retained,err:=newOrphanInspectionBudget(context.Background()).inspect(dir);!retained || !errors.Is(err,errOrphanInspectionLimit) {t.Fatal(retained,err)}
}

func TestDeletionExactStampedDayEntryBounds(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();start:=scanTime("20261110","100000")
 day:=stampedDayDir(t,dir,start)
 for i:=0;i<8192;i++ {stampedPutFile(t,day,start,start.Add(time.Minute),fmt.Sprintf("clip%05d",i),true)}
 never:=func(string,time.Time)bool{return false}
 work:=newPublishedDeletionWork()
 result,err:=deletePublishedLeasedWithWork(context.Background(),lease,dir,never,publishedEngineGuard,unix.Unlinkat,work)
 if err!=nil || result.Count!=0 || work.Stats.EntriesRead!=8193 {t.Fatal("8192-entry inspection refused",result,work.Stats,err)}
 admitted:=errors.New("first admitted unlink");calls:=0
 err=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int)error{calls++;return admitted})
 if !errors.Is(err,admitted) || calls!=1 {t.Fatal("8192-entry camera plan not admitted",err,calls)}
 if retained,err:=newOrphanInspectionBudget(context.Background()).inspect(dir);err!=nil || retained {t.Fatal(retained,err)}
 stampedPutFile(t,day,start,start.Add(time.Minute),"overflow",true)
 calls=0
 result,err=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,func(int,string,int)error{calls++;return nil})
 if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || calls!=0 {t.Fatal(result,err,calls)}
 err=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int)error{calls++;return nil})
 if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal(err,calls)}
 if retained,err:=newOrphanInspectionBudget(context.Background()).inspect(dir);!retained || !errors.Is(err,errOrphanInspectionLimit) {t.Fatal(retained,err)}
}
