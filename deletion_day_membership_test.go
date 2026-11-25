package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func TestCameraDayMembershipRefusesBeforeAnyUnlink(t *testing.T) {
 for _,kind:=range []string{"malformed","wrong-day","overlong","nested","tmp"} {
  t.Run(kind,func(t *testing.T) {
   dir,a,_:=cameraDayFixture(t);day:=filepath.Dir(a)
   name:="malformed.mp4"
   switch kind {
   case "wrong-day":name="20261111_100000_E20261111_101000_wrong.mp4"
   case "overlong":name="20261110_100000_E20261112_101000_long.mp4"
   case "nested":name="nested"
   case "tmp":name="capture.mp4.tmp"
   }
   if kind=="nested" {if err:=os.Mkdir(filepath.Join(day,name),0700);err!=nil {t.Fatal(err)}} else {putCleanupFixture(t,filepath.Join(day,name),"keep")}
   calls:=0
   err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int)error{calls++;return nil})
   if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal(err,calls)}
   assertPublicationBytes(t,a,"fixture")
  })
 }
}

func orphanDayPair(t *testing.T)(root,dir,a,b string) {
 t.Helper();root=t.TempDir();dir=filepath.Join(root,"orphan")
 a=scanFixture(t,dir,scanTime("20261110","100000"),scanTime("20261110","101000"),"a")
 b=scanFixture(t,dir,scanTime("20261110","101000"),scanTime("20261110","102000"),"b")
 return
}

func TestOrphanDaysDeleteOneLevelAndPreserveRegisteredHidden(t *testing.T) {
 root,dir,_,_:=orphanDayPair(t)
 registered:=scanFixture(t,filepath.Join(root,"registered"),scanTime("20261110","100000"),scanTime("20261110","101000"),"keep")
 hidden:=scanFixture(t,filepath.Join(root,".private"),scanTime("20261110","100000"),scanTime("20261110","101000"),"keep")
 result,err:=cleanupOrphanedAt(root,map[string]bool{"registered":true})
 if err!=nil || result.Count!=2 || result.Bytes!=14 || result.Failures!=0 {t.Fatal(result,err)}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("empty orphan tree retained",err)}
 assertPublicationBytes(t,registered,"fixture");assertPublicationBytes(t,hidden,"fixture")
}

func TestOrphanDayEvidencePreservesWholeCamera(t *testing.T) {
 for _,kind:=range []string{"root-tmp","day-tmp","nested","symlink","fifo","malformed","wrong-day","day-link","temp","buffer"} {
  t.Run(kind,func(t *testing.T) {
   root,dir,a,b:=orphanDayPair(t);day:=filepath.Dir(a);outside:=t.TempDir()
   target:=filepath.Join(outside,"keep");putCleanupFixture(t,target,"outside")
   switch kind {
   case "root-tmp":putCleanupFixture(t,filepath.Join(dir,"capture.tmp"),"")
   case "day-tmp":putCleanupFixture(t,filepath.Join(day,"capture.tmp"),"")
   case "nested":putCleanupFixture(t,filepath.Join(day,"nested","keep.mp4"),"evidence")
   case "symlink":if err:=os.Symlink(target,filepath.Join(day,"20261110_110000_E20261110_111000_link.mp4"));err!=nil {t.Fatal(err)}
   case "fifo":if err:=unix.Mkfifo(filepath.Join(day,"20261110_110000_E20261110_111000_fifo.mp4"),0600);err!=nil {t.Fatal(err)}
   case "malformed":putCleanupFixture(t,filepath.Join(day,"bad.mp4"),"keep")
   case "wrong-day":putCleanupFixture(t,filepath.Join(day,"20261111_110000_E20261111_111000_wrong.mp4"),"keep")
   case "day-link":if err:=os.Symlink(outside,filepath.Join(dir,"20261111"));err!=nil {t.Fatal(err)}
   case "temp","buffer":putCleanupFixture(t,filepath.Join(dir,kind,"opaque"),"keep")
   }
   result,err:=cleanupOrphanedAt(root,nil)
   if err!=nil || result.Count!=0 || result.SkippedRecoveryDirs!=1 || result.Failures!=0 {t.Fatal(result,err)}
   assertPublicationBytes(t,a,"fixture");assertPublicationBytes(t,b,"fixture");assertPublicationBytes(t,target,"outside")
  })
 }
}

func TestOrphanDayBoundsChargeSharedBudget(t *testing.T) {
 t.Run("400-and-401",func(t *testing.T) {
  dir:=t.TempDir();start:=scanTime("20240101","000000")
  for i:=0;i<orphanDayFolderLimit;i++ {if err:=os.Mkdir(filepath.Join(dir,stampedDayName(start.AddDate(0,0,i))),0700);err!=nil {t.Fatal(err)}}
  budget:=newOrphanInspectionBudget(context.Background())
  retained,err:=budget.inspect(dir)
  if err!=nil || retained || budget.reads!=401 || budget.entries!=400 {t.Fatal(retained,err,budget.reads,budget.entries)}
  if err:=os.Mkdir(filepath.Join(dir,stampedDayName(start.AddDate(0,0,400))),0700);err!=nil {t.Fatal(err)}
  retained,err=newOrphanInspectionBudget(context.Background()).inspect(dir)
  if !retained || !errors.Is(err,errOrphanInspectionLimit) {t.Fatal(retained,err)}
 })
 for _,kind:=range []string{"folder","pass-entries","pass-reads"} {
  t.Run(kind,func(t *testing.T) {
   root,_,a,b:=orphanDayPair(t);budget:=newOrphanInspectionBudget(context.Background())
   switch kind {case "folder":budget.perDirectory=1;case "pass-entries":budget.maxEntries=3;case "pass-reads":budget.maxReads=3}
   result,err:=cleanupOrphanedWithIO(root,nil,orphanBudgetOps(budget))
   if err!=nil || !result.LimitReached || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err)}
   assertPublicationBytes(t,a,"fixture");assertPublicationBytes(t,b,"fixture")
  })
 }
}

func TestOrphanDayPartialOutcomes(t *testing.T) {
 for _,kind:=range []string{"new-evidence","cancel","limit","unlink-failure","read-failure","late-rmdir"} {
  t.Run(kind,func(t *testing.T) {
   root,_,a,b:=orphanDayPair(t);day:=filepath.Dir(a)
   ctx,cancel:=context.WithCancel(context.Background());defer cancel()
   budget:=newOrphanInspectionBudget(ctx);if kind=="limit" {budget.maxReads=7}
   ops:=orphanBudgetOps(budget)
   if kind=="read-failure" {
    read:=ops.readDir
    ops.readDir=func(path string)([]os.DirEntry,error){if path==day {return nil,os.ErrPermission};return read(path)}
   }
   calls:=0
   ops.remove=func(path string)error {
    calls++
    if kind=="unlink-failure" {return os.ErrPermission}
    if kind=="late-rmdir" && path==day {putCleanupFixture(t,filepath.Join(day,"late.tmp"),"keep")}
    err:=os.Remove(path)
    if err==nil && calls==1 {
     if kind=="new-evidence" {putCleanupFixture(t,filepath.Join(day,"late.tmp"),"keep")}
     if kind=="cancel" {cancel()}
    }
    return err
   }
   result,err:=cleanupOrphanedWithIO(root,nil,ops)
   if err!=nil {t.Fatal(err)}
   want:=1;if kind=="unlink-failure" || kind=="read-failure" {want=0};if kind=="late-rmdir" {want=2}
   if result.Count!=want || result.Bytes!=int64(want*7) {t.Fatal(kind,result)}
   if kind=="cancel" && (!result.Interrupted || result.Failures!=1) {t.Fatal(result)}
   if kind=="limit" && (!result.LimitReached || result.Failures!=1) {t.Fatal(result)}
   if kind=="new-evidence" && (result.SkippedRecoveryDirs!=1 || result.Failures!=0) {t.Fatal(result)}
   if (kind=="unlink-failure" || kind=="read-failure" || kind=="late-rmdir") && result.Failures!=1 {t.Fatal(result)}
   if want<2 {assertPublicationBytes(t,b,"fixture")}
   if want==0 {assertPublicationBytes(t,a,"fixture")}
   if _,err:=os.Lstat(day);err!=nil {t.Fatal("day removed after incomplete work",err)}
  })
 }
}

func TestOrphanDayCancellationBeforeInspection(t *testing.T) {
 root,_,a,b:=orphanDayPair(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
 budget:=newOrphanInspectionBudget(ctx)
 result,err:=cleanupOrphanedWithIO(root,nil,orphanBudgetOps(budget))
 if err!=nil || !result.Interrupted || result.Count!=0 || budget.reads!=0 {t.Fatal(result,err,budget.reads)}
 assertPublicationBytes(t,a,"fixture");assertPublicationBytes(t,b,"fixture")
}

func TestCameraDirectRootLegacyFixtureStillDeletes(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera")
 putCleanupFixture(t,filepath.Join(dir,"legacy.mp4"),"legacy")
 if err:=removeCameraDirectory(context.Background(),dir,deletionGuard);err!=nil {t.Fatal(fmt.Sprint(err))}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal(err)}
}
