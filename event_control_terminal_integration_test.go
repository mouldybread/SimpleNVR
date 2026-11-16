package main

import (
 "context"
 "errors"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

// Real admitted job, creator-bound capture, concat, publication and checkpoints.
// Child scripts supply synthetic bytes/probe outcomes, not valid camera media.
func terminalInteractionJob(t *testing.T) (*NVR,*BufferManager,*bufferEvent,func()) {
 t.Helper()
 installRecoveryContractTools(t,0,0,false)
 bm:=bufferFixture(t)
 cam:=bm.camera
 cam.RecordingMode=ModeEventBased
 cam.RetentionDays=1
 source:=filepath.Join(cam.RecordDir,"selected-source.mp4")
 if err:=os.WriteFile(source,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bm.registerCompleted(source,now.Add(-2*time.Second),now.Add(-time.Second))
 if err:=bm.TriggerEvent(bufferTestEvent("terminal-interaction"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan
 var releaseOnce sync.Once
 release:=func(){releaseOnce.Do(func(){bm.releaseJob(job)})}
 t.Cleanup(release)
 bm.handleEvent(job)
 if job.recoveryFailed || job.recovery.State!="published" || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal || !job.recoveryBorrow.lease.held() {t.Fatal("real publication did not transfer terminal ownership")}
 if len(job.preFiles)!=1 || job.inventory==nil || job.recoveryBorrow.output==nil {t.Fatal("fixture lacks pinned/published evidence")}
 cam.IsRecording=true
 cam.recordCancel=bm.cancel
 cam.bufferManager=bm
 n:=&NVR{cameras:map[string]*Camera{cam.ID:cam},cameraOrder:[]string{cam.ID}}
 return n,bm,job,release
}

func terminalInteractionTreeEqual(t *testing.T,before,after map[string]string,checkpoint string) {
 t.Helper()
 delete(before,checkpoint)
 delete(after,checkpoint)
 if !reflect.DeepEqual(before,after) {t.Fatal("non-checkpoint recording evidence changed")}
}

func TestEventTerminalReleaseSerializesRealRecordingControls(t *testing.T) {
 actions:=[]string{"stop","disable","replace","replacement-admission-fails","delete-blocked","delete-stop-save-fails","shutdown"}
 for _,action:=range actions {for _,failCheckpoint:=range []bool{false,true} {
  t.Run(fmt.Sprintf("%s/checkpoint-fails=%t",action,failCheckpoint),func(t *testing.T){
   n,bm,job,releaseJob:=terminalInteractionJob(t)
   cam:=bm.camera
   inventory:=job.inventory
   lease:=job.recoveryBorrow.lease
   before:=recoveryContractTree(t,cam.RecordDir)
   checkpoint:=filepath.Join("temp",filepath.Base(job.dir),ownedCheckpointName)
   previous:=readRecoveryTest(t,job.dir)
   previousBytes,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil {t.Fatal(err)}
   entered,allow,exited:=make(chan struct{}),make(chan struct{}),make(chan struct{})
   var unblock sync.Once
   var terminalDone atomic.Bool
   original:=job.recoveryWriter
   injected:=errors.New("injected terminal checkpoint failure")
   job.recoveryWriter=func(dir string,data []byte)error{
    close(entered)
    <-allow
    if failCheckpoint {return injected}
    return original(dir,data)
   }
   cam.recordWG.Add(1)
   go func(){
    defer close(exited)
    defer cam.recordWG.Done()
    <-bm.ctx.Done()
    releaseJob()
    terminalDone.Store(true)
   }()
   operationDone:=make(chan error,1)
   controlExited:=make(chan struct{})
   var saves,removes,starts,admissions atomic.Int32
   var early atomic.Bool
   persist:=func()error{
    saves.Add(1)
    if !terminalDone.Load() {early.Store(true)}
    if action=="delete-stop-save-fails" {return injected}
    return nil
   }
   replacementStarted:=make(chan struct{})
   worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){
    defer cam.recordWG.Done()
    starts.Add(1)
    if !terminalDone.Load() {early.Store(true)}
    close(replacementStarted)
    <-ctx.Done()
   }
   check:=func(string)error{
    count:=admissions.Add(1)
    if action=="replacement-admission-fails" && count==2 {return errStorageUnavailable}
    return nil
   }
   ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
   t.Cleanup(func(){
    cancel();bm.cancel();unblock.Do(func(){close(allow)})
    awaitShutdownStep(t,exited,"terminal worker cleanup did not join")
    awaitShutdownStep(t,controlExited,"control cleanup did not join")
    // A replacement worker, if any, owns the same camera WaitGroup.
    n.mu.RLock();stop:=cam.recordCancel;n.mu.RUnlock()
    if stop!=nil {stop()}
    cam.recordWG.Wait()
   })
   go func(){
    defer close(controlExited)
    switch action {
    case "stop":operationDone<-n.transitionRecordingAndSave(cam.ID,"stop","",worker,check,persist)
    case "disable":operationDone<-n.transitionRecordingAndSave(cam.ID,"mode",ModeNoRecording,worker,check,persist)
    case "replace","replacement-admission-fails":operationDone<-n.transitionRecordingAndSave(cam.ID,"mode",ModeContinuous,worker,check,persist)
    case "delete-blocked","delete-stop-save-fails":
     deps:=cameraDeletionDeps{validate:func(string,string)error{return nil},persist:persist,
      remove:func(ctx context.Context,dir string,guard func()error)error{
       removes.Add(1)
       if !terminalDone.Load() {early.Store(true)}
       return removeCameraDirectory(ctx,dir,guard)
      }}
     operationDone<-deleteManagedCameras(ctx,n,[]string{cam.ID},deps)
    case "shutdown":operationDone<-shutdownApplication(ctx,n,newShutdownGate(http.NewServeMux()),nil,persist)
    }
   }()
   awaitShutdownStep(t,entered,"control did not cancel the worker into terminal retention")
   requireRegistryAvailable(t,n)
   if !lease.held() || len(bm.slots)!=1 || saves.Load()!=0 || removes.Load()!=0 || starts.Load()!=0 {t.Fatal("control crossed blocked terminal retention")}
   select {case err:=<-operationDone:t.Fatalf("control completed before terminal retention: %v",err);default:}
   unblock.Do(func(){close(allow)})
   got:=cameraOwnershipResult(t,operationDone)
   switch action {
   case "replacement-admission-fails":if !errors.Is(got,errStorageUnavailable) {t.Fatal(got)}
   case "delete-blocked":if !errors.Is(got,errCameraDeleteBlocked) {t.Fatal(got)}
   case "delete-stop-save-fails":if !errors.Is(got,errCameraDeletePersistence) || !errors.Is(got,injected) {t.Fatal(got)}
   default:if got!=nil {t.Fatal(got)}
   }
   awaitShutdownStep(t,exited,"control did not join the original worker")
   if action=="replace" {awaitShutdownStep(t,replacementStarted,"replacement not started")}
   if early.Load() || saves.Load()!=1 {t.Fatal("persistence/replacement/removal crossed terminal ownership",saves.Load())}
   wantRemoves:=int32(0);if action=="delete-blocked" {wantRemoves=1}
   wantStarts:=int32(0);if action=="replace" {wantStarts=1}
   if removes.Load()!=wantRemoves || starts.Load()!=wantStarts {t.Fatal("unexpected removal or replacement",removes.Load(),starts.Load())}
   if job.inventory!=nil || job.recoveryBorrow!=nil || !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("terminal resources leaked")}
   assertJobInventoryClosed(t,inventory)
   if job.recoveryFailed!=failCheckpoint {t.Fatal("checkpoint failure latch changed")}
   after:=readRecoveryTest(t,job.dir)
   if after.State!=previous.State || after.Partial!=previous.Partial {t.Fatal("processing state rewritten by control")}
   if failCheckpoint {
    assertPublicationBytes(t,filepath.Join(job.dir,ownedCheckpointName),string(previousBytes))
    if after.Revision!=previous.Revision {t.Fatal("failed retention replaced prior checkpoint")}
   } else if after.Revision!=previous.Revision+1 {t.Fatal("terminal checkpoint not persisted exactly once")}
   terminalInteractionTreeEqual(t,before,recoveryContractTree(t,cam.RecordDir),checkpoint)
   n.mu.RLock()
   registered:=n.cameras[cam.ID]==cam
   active:=cam.IsRecording
   mode:=cam.RecordingMode
   n.mu.RUnlock()
   if !registered || active!=(action=="replace" || action=="shutdown") {t.Fatal("registration/restart intent changed unexpectedly")}
   if action=="disable" && mode!=ModeNoRecording {t.Fatal("disable mode lost")}
   if (action=="replace" || action=="replacement-admission-fails") && mode!=ModeContinuous {t.Fatal("replacement mode lost")}
   if len(recordingMediaOwnership.token)!=0 {t.Fatal("control left media ownership held")}
  })
 }}
}

func TestEventTerminalBlockedShutdownDoesNotPersistAfterFailure(t *testing.T) {
 for _,kind:=range []string{"cancel","already-expired-deadline"} {t.Run(kind,func(t *testing.T){
  n,bm,job,releaseJob:=terminalInteractionJob(t)
  cam:=bm.camera
  entered,allow,exited:=make(chan struct{}),make(chan struct{}),make(chan struct{})
  var unblock sync.Once
  original:=job.recoveryWriter
  job.recoveryWriter=func(dir string,data []byte)error{close(entered);<-allow;return original(dir,data)}
  cam.recordWG.Add(1)
  go func(){defer close(exited);defer cam.recordWG.Done();<-bm.ctx.Done();releaseJob()}()
  t.Cleanup(func(){bm.cancel();unblock.Do(func(){close(allow)});awaitShutdownStep(t,exited,"shutdown worker cleanup did not join")})
  var saves atomic.Int32
  persist:=func()error{saves.Add(1);return nil}
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  ctx:=parent
  want:=context.Canceled
  if kind=="already-expired-deadline" {
   var stop context.CancelFunc
   ctx,stop=context.WithDeadline(parent,time.Now().Add(-time.Second));defer stop()
   want=context.DeadlineExceeded
  }
  done:=make(chan error,1)
  go func(){done<-shutdownApplication(ctx,n,newShutdownGate(http.NewServeMux()),nil,persist)}()
  awaitShutdownStep(t,entered,"shutdown did not reach blocked terminal retention")
  cancel()
  if err:=cameraOwnershipResult(t,done);!errors.Is(err,want) {t.Fatal(err)}
  if saves.Load()!=0 || !job.recoveryBorrow.lease.held() {t.Fatal("failed shutdown saved early or discarded terminal owner")}
  unblock.Do(func(){close(allow)})
  awaitShutdownStep(t,exited,"released terminal worker did not exit")
  // The failed shutdown caller is already joined; it must not leave a save coordinator.
  if saves.Load()!=0 || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 || len(recordingMediaOwnership.token)!=0 {t.Fatal("late persistence or terminal ownership leak")}
  if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal("shutdown failure destroyed processing evidence")}
 })}
}
