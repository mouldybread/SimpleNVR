package main

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func orphanManyDays(t *testing.T,days,files int)(root,dir string,paths []string) {
 t.Helper();root=t.TempDir();dir=filepath.Join(root,"orphan");base:=scanTime("20261001","100000")
 for d:=0;d<days;d++ {
  for f:=0;f<files;f++ {
   start:=base.AddDate(0,0,d).Add(time.Duration(f)*time.Minute)
   paths=append(paths,scanFixture(t,dir,start,start.Add(time.Minute),fmt.Sprintf("clip%03d",f)))
  }
 }
 return
}

// A year of day folders must finish inside one pass budget. Per-file whole-camera
// inspection would have exhausted the 262,144-entry pass limit long before this.
func TestOrphanYearOfDaysCompletesWithinPassBudget(t *testing.T) {
 const days,files=365,3
 root,dir,_:=orphanManyDays(t,days,files)
 budget:=newOrphanInspectionBudget(context.Background())
 result,err:=cleanupOrphanedWithIO(root,nil,orphanBudgetOps(budget))
 if err!=nil || result.Count!=days*files || result.Bytes!=int64(days*files*7) || result.Failures!=0 || result.LimitReached || result.Interrupted {t.Fatal(result,err)}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("orphan camera was not removed",err)}
 if budget.reads>1500 || budget.entries>20000 {t.Fatal("evidence rechecks grew with removals",budget.reads,budget.entries)}
}

func TestOrphanDayBatchRecheckStopsOnLateEvidence(t *testing.T) {
 root,_,paths:=orphanManyDays(t,1,70)
 day:=filepath.Dir(paths[0])
 ops:=orphanBudgetOps(newOrphanInspectionBudget(context.Background()));calls:=0
 ops.remove=func(path string)error {
  err:=os.Remove(path);calls++
  if err==nil && calls==1 {putCleanupFixture(t,filepath.Join(day,"late.tmp"),"keep")}
  return err
 }
 result,err:=cleanupOrphanedWithIO(root,nil,ops)
 if err!=nil || result.Count!=orphanDayRecheckBatch || result.Bytes!=int64(orphanDayRecheckBatch*7) || result.SkippedRecoveryDirs!=1 || result.Failures!=0 {t.Fatal(result,err)}
 if _,err:=os.Lstat(paths[orphanDayRecheckBatch-1]);!os.IsNotExist(err) {t.Fatal("batch member retained",err)}
 assertPublicationBytes(t,paths[orphanDayRecheckBatch],"fixture");assertPublicationBytes(t,paths[69],"fixture")
 assertPublicationBytes(t,filepath.Join(day,"late.tmp"),"keep")
}

func TestOrphanRootEvidenceRecheckBetweenDays(t *testing.T) {
 root,dir,paths:=orphanManyDays(t,20,1)
 ops:=orphanBudgetOps(newOrphanInspectionBudget(context.Background()));calls:=0
 ops.remove=func(path string)error {
  err:=os.Remove(path);calls++
  if err==nil && calls==3 {putCleanupFixture(t,filepath.Join(dir,"capture.tmp"),"")}
  return err
 }
 result,err:=cleanupOrphanedWithIO(root,nil,ops)
 if err!=nil || result.Count!=orphanRootRecheckDays || result.SkippedRecoveryDirs!=1 || result.Failures!=0 {t.Fatal(result,err)}
 if _,err:=os.Lstat(paths[orphanRootRecheckDays-1]);!os.IsNotExist(err) {t.Fatal("processed day retained",err)}
 assertPublicationBytes(t,paths[orphanRootRecheckDays],"fixture");assertPublicationBytes(t,paths[19],"fixture")
 if _,err:=os.Lstat(filepath.Join(dir,"capture.tmp"));err!=nil {t.Fatal(err)}
}
