package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

type retentionEngineWaitContext struct {context.Context;waiting chan struct{};once sync.Once}
func (ctx *retentionEngineWaitContext) Done() <-chan struct{} {ctx.once.Do(func(){close(ctx.waiting)});return ctx.Context.Done()}

func TestRetentionEngineSelectionPreservesFilenameAndMtimeRules(t *testing.T) {
 cutoff:=time.Date(2020,1,1,0,0,0,0,time.UTC)
 selectFile:=publishedExpiredBefore(cutoff)
 for _,tc:=range []struct{name string;modified time.Time;want bool}{
  {"20000101_000000_saved.mp4",cutoff.Add(time.Hour),true},
  {"99990101_000000_saved.mp4",cutoff.Add(-time.Hour),false},
  {"20200101_000000_equal.mp4",cutoff.Add(-time.Hour),false},
  {"20000101_000000_partial.mp4",cutoff,true},
  {"invalid_timestamp_name.mp4",cutoff.Add(-time.Hour),true},
  {"invalid_timestamp_name.mp4",cutoff,false},
  {"old.mp4",cutoff.Add(-time.Hour),false},
 } {if got:=selectFile(tc.name,tc.modified);got!=tc.want {t.Errorf("%s: got %v want %v",tc.name,got,tc.want)}}
}

func TestManagedRetentionProductionUsesEngineWithoutNestedAdmission(t *testing.T) {
 n,cam,expired,target:=retentionSnapshotFixture(t)
 retained:=[]string{"99990101_000000_future.mp4","old.mp4","active.mp4.tmp","temp/event/input.mp4","buffer/pre.mp4"}
 for _,name:=range retained {putCleanupFixture(t,filepath.Join(cam.RecordDir,name),"retain")}
 partial:=filepath.Join(cam.RecordDir,"20000101_000000_partial.mp4");putCleanupFixture(t,partial,"partial")
 fallback:=filepath.Join(cam.RecordDir,"invalid_timestamp_name.mp4");putCleanupFixture(t,fallback,"fallback")
 outside:=filepath.Join(t.TempDir(),"outside.mp4");putCleanupFixture(t,outside,"outside")
 link:=filepath.Join(cam.RecordDir,"20000101_000000_link.mp4");if err:=os.Symlink(outside,link);err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 done:=make(chan bool,1);go func(){done<-cleanupRetentionSnapshot(ctx,n,target)}()
 select {case invoked:=<-done:if !invoked {t.Fatal("valid managed pass refused")};case <-time.After(3*time.Second):cancel();t.Fatal("managed engine may have reacquired media ownership")}
 for _,path:=range []string{expired,partial,fallback} {if _,err:=os.Stat(path);!os.IsNotExist(err) {t.Fatal("expired output retained",path,err)}}
 for _,name:=range retained {if got,err:=os.ReadFile(filepath.Join(cam.RecordDir,name));err!=nil || string(got)!="retain" {t.Fatal("retained evidence changed",name,err)}}
 if _,err:=os.Lstat(link);err!=nil {t.Fatal("symlink removed",err)}
 if got,err:=os.ReadFile(outside);err!=nil || string(got)!="outside" {t.Fatal("outside target changed",err)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("managed retention leaked lease")}
}

func TestRetentionEngineRechecksGuardAfterAdmission(t *testing.T) {
 n,cam,file,target:=retentionSnapshotFixture(t)
 var observed error
 invoked:=admitRetentionDeletion(context.Background(),n,target,recordingMediaOwnership,func(ctx context.Context,lease *mediaMutationLease,dir string,duration time.Duration,guard func() error) error {
  n.mu.Lock();cam.RecordDir="changed";n.mu.Unlock()
  observed=cleanupExpiredPublishedLeased(ctx,lease,dir,duration,guard);return observed
 })
 if !invoked || !errors.Is(observed,errRecordingStateChanged) {t.Fatal("guard not propagated or invocation semantics changed",invoked,observed)}
 if _,err:=os.Stat(file);err!=nil {t.Fatal("stale target deleted",err)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("engine refusal leaked ownership")}
}

func TestRetentionEngineBorrowedLocksAndExplicitDeletionOrdering(t *testing.T) {
 n,cam,_,target:=retentionSnapshotFixture(t)
 entered,release,finished:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 go func(){defer close(finished);admitRetentionDeletion(context.Background(),n,target,recordingMediaOwnership,func(_ context.Context,lease *mediaMutationLease,_ string,_ time.Duration,guard func() error) error {
  if !lease.held() {t.Error("missing borrowed lease")};if err:=guard();err!=nil {t.Error(err)}
  close(entered);<-release;return errors.New("injected retention failure")
 })}()
 awaitShutdownStep(t,entered,"retention callback not entered")
 requireRegistryAvailable(t,n)
 if retentionPolicyMu.TryLock() {retentionPolicyMu.Unlock();t.Fatal("retention lost policy synchronization")}
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 waiting:=&retentionEngineWaitContext{Context:ctx,waiting:make(chan struct{})}
 result:=make(chan publishedAdmissionOutcome,1)
 go func(){value,err:=deleteRegisteredPublished(waiting,n,publishedDeletionTarget{cam.ID,cam.RecordDir,cam},deleteAllPublishedLeased);result<-publishedAdmissionOutcome{value,err}}()
 awaitShutdownStep(t,waiting.waiting,"explicit deletion did not wait for retention ownership")
 once.Do(func(){close(release)})
 awaitShutdownStep(t,finished,"retention callback did not join")
 outcome:=publishedAdmissionResult(t,result)
 if outcome.err!=nil || outcome.result.Count!=1 {t.Fatal(outcome)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("failure or following deletion leaked lease")}
}

func TestRetentionPolicyIncreaseWhileWaitingProtectsOutput(t *testing.T) {
 n,_,file,target:=retentionSnapshotFixture(t)
 owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 ctx:=&retentionEngineWaitContext{Context:parent,waiting:make(chan struct{})}
 done:=make(chan bool,1);go func(){done<-cleanupRetentionSnapshot(ctx,n,target)}()
 awaitShutdownStep(t,ctx.waiting,"retention did not wait on shared gate")
 requireRegistryAvailable(t,n)
 if !retentionPolicyMu.TryLock() {t.Fatal("media wait held policy lock")};retentionPolicyMu.Unlock()
 if err:=setCameraRetention(n,target.id,30);err!=nil {t.Fatal(err)}
 owner.release()
 select {case invoked:=<-done:if invoked {t.Fatal("stale shorter policy admitted")};case <-time.After(3*time.Second):t.Fatal("retention did not join")}
 if _,err:=os.Stat(file);err!=nil {t.Fatal("protected output lost",err)}
}

func TestRetentionEngineRejectsInvalidDurationAndCanceledWork(t *testing.T) {
 dir:=t.TempDir();path:=filepath.Join(dir,"20000101_000000_saved.mp4");putCleanupFixture(t,path,"retain")
 if err:=cleanupExpiredPublishedLeased(context.Background(),nil,dir,0,publishedEngineGuard);!errors.Is(err,errCameraRetention) {t.Fatal(err)}
 lease:=publishedEngineLease(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=cleanupExpiredPublishedLeased(ctx,lease,dir,time.Hour,publishedEngineGuard);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if got,err:=os.ReadFile(path);err!=nil || string(got)!="retain" {t.Fatal("refused work changed bytes",err)}
 if !lease.held() {t.Fatal("borrower released caller lease")}
}
