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

func TestRetentionExpiryUsesStampedEndNotStart(t *testing.T) {
 cutoff:=scanTime("20261110","120000")
 selectFile:=publishedExpiredBefore(cutoff)
 for _,tc:=range []struct{name string;start,end time.Time;want bool}{
  {"ended-before",scanTime("20261110","100000"),scanTime("20261110","115959"),true},
  {"end-equals-cutoff",scanTime("20261110","110000"),cutoff,false},
  {"starts-before-ends-after",scanTime("20261109","230000"),scanTime("20261110","120001"),false},
  {"entirely-old",scanTime("20261101","000000"),scanTime("20261101","010000"),true},
 } {if got:=selectFile(stampedFileName(tc.start,tc.end,"x"),time.Time{});got!=tc.want {t.Errorf("%s: got %v want %v",tc.name,got,tc.want)}}
}

func TestPublishedEngineDeletesExpiredStampedFilesInDayFolders(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();outside:=t.TempDir()
 cutoff:=scanTime("20261110","120000")
 expired:=scanFixture(t,dir,scanTime("20261101","100000"),scanTime("20261101","110000"),"old")
 just:=scanFixture(t,dir,scanTime("20261110","105959"),scanTime("20261110","115959"),"just")
 equal:=scanFixture(t,dir,scanTime("20261110","110000"),cutoff,"equal")
 spans:=scanFixture(t,dir,scanTime("20261109","230000"),scanTime("20261110","130000"),"span")
 mixed:=scanFixture(t,dir,scanTime("20261105","100000"),scanTime("20261105","110000"),"mixed")
 tmp:=filepath.Join(filepath.Dir(mixed),"active.tmp");putCleanupFixture(t,tmp,"retain")
 linked:=scanFixture(t,outside,scanTime("20261102","100000"),scanTime("20261102","110000"),"outside")
 if err:=os.Symlink(filepath.Dir(linked),filepath.Join(dir,"20261102"));err!=nil {t.Fatal(err)}
 putCleanupFixture(t,filepath.Join(dir,"temp","event","input.mp4"),"input")
 result,err:=deletePublishedLeased(context.Background(),lease,dir,publishedExpiredBefore(cutoff),publishedEngineGuard)
 if err!=nil || result.Count!=3 || result.Bytes!=21 || result.Failures!=0 {t.Fatal(result,err)}
 for _,path:=range []string{expired,just,mixed} {if _,err:=os.Lstat(path);!os.IsNotExist(err) {t.Fatal("expired file retained",path,err)}}
 if _,err:=os.Lstat(filepath.Dir(expired));!os.IsNotExist(err) {t.Fatal("emptied day folder retained",err)}
 for _,path:=range []string{equal,spans,tmp,linked,filepath.Join(dir,"20261102"),filepath.Join(dir,"temp","event","input.mp4"),filepath.Dir(mixed)} {if _,err:=os.Lstat(path);err!=nil {t.Fatal("retained entry lost",path,err)}}
 if !lease.held() {t.Fatal("borrower released caller lease")}
}

func TestPublishedEngineDayFolderRefusesMissingReleasedAndCanceledOwnership(t *testing.T) {
 dir:=t.TempDir();file:=scanFixture(t,dir,scanTime("20261101","100000"),scanTime("20261101","110000"),"old")
 result,err:=deletePublishedLeased(context.Background(),nil,dir,publishedEngineAll,publishedEngineGuard)
 if err==nil || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err)}
 released,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)}
 released.release()
 result,err=deletePublishedLeased(context.Background(),released,dir,publishedEngineAll,publishedEngineGuard)
 if err==nil || result.Count!=0 {t.Fatal("released lease accepted",result,err)}
 lease:=publishedEngineLease(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
 result,err=deletePublishedLeased(ctx,lease,dir,publishedEngineAll,publishedEngineGuard)
 if !errors.Is(err,context.Canceled) || result.Count!=0 {t.Fatal(result,err)}
 if _,err:=os.Lstat(file);err!=nil {t.Fatal("refused work deleted a file",err)}
}

func TestPublishedEngineDayFolderGuardFailureStopsAndKeepsFolder(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir()
 a:=scanFixture(t,dir,scanTime("20261101","100000"),scanTime("20261101","110000"),"a")
 b:=scanFixture(t,dir,scanTime("20261101","100000"),scanTime("20261101","110000"),"b")
 failed:=false
 guard:=func() error {if failed {return errRecordingStateChanged};return nil}
 result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,guard,func(fd int,name string,flags int) error {err:=unix.Unlinkat(fd,name,flags);failed=true;return err})
 if !errors.Is(err,errRecordingStateChanged) || result.Count!=1 || result.Failures!=1 {t.Fatal(result,err)}
 if _,err:=os.Lstat(a);!os.IsNotExist(err) {t.Fatal("first file retained",err)}
 if got,err:=os.ReadFile(b);err!=nil || string(got)!="fixture" {t.Fatal("second file lost",err)}
 if _,err:=os.Lstat(filepath.Dir(b));err!=nil {t.Fatal("day folder removed after failure",err)}
}

func TestPublishedEngineDayFolderLateEntryKeepsFolder(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir()
 file:=scanFixture(t,dir,scanTime("20261101","100000"),scanTime("20261101","110000"),"old")
 late:=filepath.Join(filepath.Dir(file),"late.tmp")
 result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,func(fd int,name string,flags int) error {
  if flags==unix.AT_REMOVEDIR {putCleanupFixture(t,late,"late")}
  return unix.Unlinkat(fd,name,flags)
 })
 if err!=nil || result.Count!=1 {t.Fatal(result,err)}
 if got,err:=os.ReadFile(late);err!=nil || string(got)!="late" {t.Fatal("late entry removed",err)}
}

func TestPublishedEngineDayFolderBounds(t *testing.T) {
 for _,kind:=range []string{"days","entries"} {t.Run(kind,func(t *testing.T){
  lease:=publishedEngineLease(t);dir:=t.TempDir();calls:=0
  if kind=="days" {
   base:=scanTime("20240101","000000")
   for i:=0;i<=maxStampedScanDays;i++ {if err:=os.MkdirAll(filepath.Join(dir,stampedDayName(base.AddDate(0,0,i))),0755);err!=nil {t.Fatal(err)}}
  } else {
   day:=filepath.Join(dir,"20261101");if err:=os.MkdirAll(day,0755);err!=nil {t.Fatal(err)}
   for i:=0;i<=publishedDeletionEntryLimit;i++ {file,err:=os.Create(filepath.Join(day,fmt.Sprintf("%05d.mp4",i)));if err!=nil {t.Fatal(err)};file.Close()}
  }
  result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,func(int,string,int) error {calls++;return nil})
  if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || calls!=0 {t.Fatal(result,err,calls)}
 })}
}

func cameraDayFixture(t *testing.T) (string,string,string) {
 t.Helper()
 dir:=filepath.Join(t.TempDir(),"camera");if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 a:=scanFixture(t,dir,scanTime("20261110","100000"),scanTime("20261110","101000"),"a")
 b:=scanFixture(t,dir,scanTime("20261110","101000"),scanTime("20261110","102000"),"b")
 return dir,a,b
}

func TestCameraDeletionRemovesStampedDayFoldersWithoutRecursion(t *testing.T) {
 dir,_,_:=cameraDayFixture(t)
 scanFixture(t,dir,scanTime("20261111","100000"),scanTime("20261111","101000"),"c")
 if err:=os.Mkdir(filepath.Join(dir,"buffer"),0700);err!=nil {t.Fatal(err)}
 removed:=map[string]bool{}
 err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(fd int,name string,flags int) error {
  if flags==unix.AT_REMOVEDIR {removed[name]=true}
  return unix.Unlinkat(fd,name,flags)
 })
 if err!=nil || !removed["20261110"] || !removed["20261111"] || !removed["buffer"] {t.Fatal(err,removed)}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("camera directory not removed",err)}
}

func TestCameraDeletionDayFolderEvidenceBlocksAllUnlinking(t *testing.T) {
 for _,kind:=range []string{"tmp","nested","symlink-member","fifo","symlink-day"} {t.Run(kind,func(t *testing.T){
  dir,a,_:=cameraDayFixture(t);outside:=t.TempDir();target:=filepath.Join(outside,"keep.mp4");putCleanupFixture(t,target,"outside")
  day:=filepath.Dir(a)
  switch kind {
  case "tmp":putCleanupFixture(t,filepath.Join(day,"active.tmp"),"retain")
  case "nested":putCleanupFixture(t,filepath.Join(day,"nested","keep.mp4"),"retain")
  case "symlink-member":if err:=os.Symlink(target,filepath.Join(day,"link.mp4"));err!=nil {t.Fatal(err)}
  case "fifo":if err:=unix.Mkfifo(filepath.Join(day,"fifo.mp4"),0600);err!=nil {t.Fatal(err)}
  case "symlink-day":if err:=os.Symlink(outside,filepath.Join(dir,"20261112"));err!=nil {t.Fatal(err)}
  }
  calls:=0
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int) error {calls++;return nil})
  if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal("evidence did not block all unlinking",err,calls)}
  if got,err:=os.ReadFile(a);err!=nil || string(got)!="fixture" {t.Fatal("day file changed",err)}
  if got,err:=os.ReadFile(target);err!=nil || string(got)!="outside" {t.Fatal("outside changed",err)}
 })}
}

func TestCameraDeletionDayFolderBounds(t *testing.T) {
 for _,kind:=range []string{"days","entries"} {t.Run(kind,func(t *testing.T){
  dir:=filepath.Join(t.TempDir(),"camera");if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
  if kind=="days" {
   base:=scanTime("20240101","000000")
   for i:=0;i<=maxStampedScanDays;i++ {if err:=os.Mkdir(filepath.Join(dir,stampedDayName(base.AddDate(0,0,i))),0700);err!=nil {t.Fatal(err)}}
  } else {
   day:=filepath.Join(dir,"20261101");if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
   for i:=0;i<=cameraDeleteEntryLimit;i++ {file,err:=os.Create(filepath.Join(day,fmt.Sprintf("%05d.mp4",i)));if err!=nil {t.Fatal(err)};file.Close()}
  }
  calls:=0
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int) error {calls++;return nil})
  if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal("bound did not fail before unlink",err,calls)}
 })}
}

func TestCameraDeletionDayFolderLateEntryAndGuardFailure(t *testing.T) {
 t.Run("late-entry",func(t *testing.T){
  dir,a,_:=cameraDayFixture(t);day:=filepath.Dir(a);late:=filepath.Join(day,"late.tmp")
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(fd int,name string,flags int) error {
   if flags==unix.AT_REMOVEDIR && name=="20261110" {putCleanupFixture(t,late,"late evidence")}
   return unix.Unlinkat(fd,name,flags)
  })
  if err==nil {t.Fatal("late entry ignored")}
  if got,err:=os.ReadFile(late);err!=nil || string(got)!="late evidence" {t.Fatal("late evidence removed",err)}
 })
 t.Run("guard-failure",func(t *testing.T){
  dir,a,b:=cameraDayFixture(t);failed:=false
  guard:=func() error {if failed {return errRecordingStateChanged};return nil}
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,guard,func(fd int,name string,flags int) error {err:=unix.Unlinkat(fd,name,flags);failed=true;return err})
  if !errors.Is(err,errRecordingStateChanged) {t.Fatal(err)}
  _,errA:=os.Lstat(a);_,errB:=os.Lstat(b)
  if os.IsNotExist(errA)==os.IsNotExist(errB) {t.Fatal("expected exactly one day file removed",errA,errB)}
  if _,err:=os.Lstat(filepath.Dir(a));err!=nil {t.Fatal("day folder removed after failure",err)}
 })
}

func TestManagedCameraDeletionRemovesDayFoldersUnderLease(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 dir,_,_:=cameraDayFixture(t);cam.RecordDir=dir
 leaseHeld:=false
 deps:=deletionDeps(n,func() error {return nil},func(ctx context.Context,path string,guard func() error) error {
  leaseHeld=len(recordingMediaOwnership.token)!=0
  return removeCameraDirectory(ctx,path,guard)
 })
 if err:=deleteManagedCameras(context.Background(),n,[]string{cam.ID},deps);err!=nil {t.Fatal(err)}
 if !leaseHeld {t.Fatal("removal ran without the media lease")}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("camera directory not removed",err)}
 if n.cameras[cam.ID]!=nil || len(recordingMediaOwnership.token)!=0 {t.Fatal("registry or lease state wrong")}
}
