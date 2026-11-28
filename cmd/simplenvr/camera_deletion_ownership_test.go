package main

import (
 "context"
 "errors"
 "sync"
 "testing"
 "time"
)

type cameraOwnershipWaitContext struct {context.Context;waiting chan struct{};once sync.Once}
func (ctx *cameraOwnershipWaitContext) Done() <-chan struct{} {ctx.once.Do(func(){close(ctx.waiting)});return ctx.Context.Done()}
func cameraOwnershipResult(t *testing.T,result <-chan error) error {
 t.Helper()
 select {case err:=<-result:return err;case <-time.After(3*time.Second):t.Fatal("camera ownership operation did not join");return nil}
}
func cameraOwnershipDeps(remove func(context.Context,string,func() error) error,persist func() error) cameraDeletionDeps {
 return cameraDeletionDeps{validate:func(string,string) error{return nil},remove:remove,persist:persist}
}

func TestCameraDeletionJoinsWorkerBeforeMediaAdmission(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 workerCtx,cancel:=context.WithCancel(context.Background())
 finishCtx,cancelFinish:=context.WithCancel(context.Background())
 cam.recordCancel=cancel;cam.IsRecording=true;cam.recordWG.Add(1)
 owned,finish,removed:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once
 defer func(){cancel();cancelFinish();once.Do(func(){close(finish)});cam.recordWG.Wait()}()
 go func(){
  defer cam.recordWG.Done()
  lease,err:=func()(*mediaMutationLease,error){<-workerCtx.Done();return recordingMediaOwnership.acquire(finishCtx)}()
  if err!=nil {if finishCtx.Err()==nil {t.Error(err)};return}
  defer lease.release()
  close(owned);<-finish
 }()
 result:=make(chan error,1)
 go func(){result<-deleteManagedCameras(context.Background(),n,[]string{cam.ID},cameraOwnershipDeps(func(context.Context,string,func() error) error {close(removed);return nil},func() error{return nil}))}()
 awaitShutdownStep(t,owned,"finishing worker could not acquire ownership during join")
 select {case <-removed:t.Fatal("camera removed before worker join");default:}
 requireRegistryAvailable(t,n)
 once.Do(func(){close(finish)})
 if err:=cameraOwnershipResult(t,result);err!=nil {t.Fatal(err)}
 awaitShutdownStep(t,removed,"camera removal never ran")
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("camera or worker leaked lease")}
}

func TestCameraDeletionCanceledOwnershipWaitPreservesStoppedRegistration(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 ctx:=&cameraOwnershipWaitContext{Context:parent,waiting:make(chan struct{})}
 saves,removes:=0,0
 result:=make(chan error,1)
 go func(){result<-deleteManagedCameras(ctx,n,[]string{cam.ID},cameraOwnershipDeps(func(context.Context,string,func() error) error {removes++;return nil},func() error{saves++;return nil}))}()
 awaitShutdownStep(t,ctx.waiting,"camera did not wait for media ownership")
 requireRegistryAvailable(t,n)
 n.mu.RLock();stopped:=!cam.IsRecording && cam.recordCancel==nil;n.mu.RUnlock()
 if !stopped || saves!=1 {t.Fatal("stopped intent not persisted before admission")}
 cancel()
 if err:=cameraOwnershipResult(t,result);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 n.mu.RLock();registered:=n.cameras[cam.ID]==cam;n.mu.RUnlock()
 if !registered || removes!=0 || saves!=1 || !owner.held() {t.Fatal("canceled waiter changed registration/files/owner")}
}

func TestCameraDeletionRevalidatesAfterOwnershipWait(t *testing.T) {
 for _,change:=range []string{"replace","id","path","remove","shutdown"} {t.Run(change,func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);id:=cam.ID
  owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
  ctx:=&cameraOwnershipWaitContext{Context:context.Background(),waiting:make(chan struct{})}
  saves,removes:=0,0
  result:=make(chan error,1)
  go func(){result<-deleteManagedCameras(ctx,n,[]string{id},cameraOwnershipDeps(func(context.Context,string,func() error) error {removes++;return nil},func() error{saves++;return nil}))}()
  awaitShutdownStep(t,ctx.waiting,"camera did not reach admission")
  n.mu.Lock()
  switch change {
  case "replace":n.cameras[id]=&Camera{ID:id}
  case "id":cam.ID="changed"
  case "path":cam.RecordDir="changed"
  case "remove":delete(n.cameras,id)
  case "shutdown":n.shuttingDown.Store(true)
  }
  n.mu.Unlock()
  owner.release()
  err=cameraOwnershipResult(t,result)
  expected:=errRecordingStateChanged;if change=="shutdown" {expected=errRecorderShuttingDown}
  if !errors.Is(err,expected) || saves!=1 || removes!=0 {t.Fatal("stale admitted deletion was used",err,saves,removes)}
  if len(recordingMediaOwnership.token)!=0 {t.Fatal("revalidation refusal leaked lease")}
 })}
}

func TestCameraDeletionOwnershipSpansRemovalAndFinalSave(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.RetentionDays=1
 target:=snapshotRetentionCleanup(n)[0]
 entered,release,finalSave,finishSave:=make(chan struct{}),make(chan struct{}),make(chan struct{}),make(chan struct{})
 var releaseOnce,saveOnce sync.Once
 defer releaseOnce.Do(func(){close(release)});defer saveOnce.Do(func(){close(finishSave)})
 saves:=0
 result:=make(chan error,1)
 go func(){result<-deleteManagedCameras(context.Background(),n,[]string{cam.ID},cameraOwnershipDeps(func(context.Context,string,func() error) error {close(entered);<-release;return nil},func() error{saves++;if saves==2 {close(finalSave);<-finishSave};return nil}))}()
 awaitShutdownStep(t,entered,"removal not entered")
 requireRegistryAvailable(t,n)
 if len(recordingMediaOwnership.token)!=1 {t.Fatal("removal has no media lease")}
 retentionCtx:=&cameraOwnershipWaitContext{Context:context.Background(),waiting:make(chan struct{})}
 retentionResult:=make(chan bool,1)
 go func(){retentionResult<-cleanupRetentionSnapshotWith(retentionCtx,n,target,func(context.Context,string,time.Duration){t.Error("retention ran against deleted registration")})}()
 awaitShutdownStep(t,retentionCtx.waiting,"retention did not wait on shared media ownership")
 releaseOnce.Do(func(){close(release)})
 awaitShutdownStep(t,finalSave,"final save not entered")
 requireRegistryAvailable(t,n)
 if len(recordingMediaOwnership.token)!=1 {t.Fatal("final save lost media ownership")}
 select {case <-retentionResult:t.Fatal("retention admitted before final save completed");default:}
 saveOnce.Do(func(){close(finishSave)})
 if err:=cameraOwnershipResult(t,result);err!=nil {t.Fatal(err)}
 select {case invoked:=<-retentionResult:if invoked {t.Fatal("retention accepted removed camera")};case <-time.After(3*time.Second):t.Fatal("retention waiter did not join")}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("completed camera deletion leaked lease")}
}

func TestCameraDeletionSaveFailurePrecedesOwnershipWait(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
 result:=make(chan error,1)
 go func(){result<-deleteManagedCameras(context.Background(),n,[]string{cam.ID},cameraOwnershipDeps(func(context.Context,string,func() error) error {t.Error("removal despite failed save");return nil},func() error{return errors.New("injected save failure")}))}()
 if err:=cameraOwnershipResult(t,result);!errors.Is(err,errCameraDeletePersistence) {t.Fatal(err)}
 if !owner.held() {t.Fatal("failed save disturbed unrelated owner")}
}

func TestCameraDeletionTerminalErrorsReleaseOwnership(t *testing.T) {
 for _,phase:=range []string{"remove","final-save"} {t.Run(phase,func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous)
  saves:=0;injected:=errors.New("injected failure")
  err:=deleteManagedCameras(context.Background(),n,[]string{cam.ID},cameraOwnershipDeps(func(context.Context,string,func() error) error {if phase=="remove" {return injected};return nil},func() error{saves++;if phase=="final-save" && saves==2 {return injected};return nil}))
  if !errors.Is(err,injected) {t.Fatal("error lost",err)}
  if phase=="final-save" && !errors.Is(err,errCameraDeletePersistence) {t.Fatal("persistence precedence lost",err)}
  n.mu.RLock();registered:=n.cameras[cam.ID]==cam;n.mu.RUnlock()
  if registered!=(phase=="remove") {t.Fatal("pretend rollback or early registry removal")}
  if len(recordingMediaOwnership.token)!=0 {t.Fatal("terminal error leaked ownership")}
 })}
}
