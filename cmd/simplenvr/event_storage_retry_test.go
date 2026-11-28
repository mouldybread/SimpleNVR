package main

import (
 "context"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func awaitEventSetupStep(t *testing.T,ch <-chan struct{}) {
 t.Helper()
 select {case <-ch:case <-time.After(3*time.Second):t.Fatal("event setup worker did not reach expected step")}
}

func TestEventStorageSetupRetryPreservesIntentAndRejectsCapture(t *testing.T) {
 n,cam:=lifecycleFixture(ModeEventBased);cam.Name="front";cam.RecordDir="/recordings/test"
 waiting,release,ready:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 attempts:=0
 worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){
  recordCameraEventBasedWithSetup(ctx,n,cam,func(string)error{attempts++;if attempts==1 {return os.ErrPermission};return nil},func(ctx context.Context)bool{
   close(waiting);select {case <-ctx.Done():return false;case <-release:return true}
  },func(bm *BufferManager){close(ready);<-bm.ctx.Done()})
 }
 defer n.cancelAndWaitRecorders()
 if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string)error{return nil});err!=nil {t.Fatal(err)}
 awaitEventSetupStep(t,waiting)
 n.mu.RLock();intent,manager:=cam.IsRecording,cam.bufferManager;n.mu.RUnlock()
 if !intent || manager!=nil {t.Fatal("setup failure changed intent or published manager")}
 path:=filepath.Join(t.TempDir(),"cameras.json")
 if err:=n.saveAt(path);err!=nil {t.Fatal(err)}
 restored:=&NVR{};if err:=restored.loadAt(path);err!=nil {t.Fatal(err)}
 if !restored.cameras[cam.ID].IsRecording {t.Fatal("unrelated save persisted an unintended stop")}
 body:=`{"camera_name":"front","label":"motion","start_time":"2026-11-17T00:00:00Z","duration_seconds":5}`
 w:=httptest.NewRecorder();saves:=0
 triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(body)),n,func()error{saves++;return nil})
 if w.Code!=503 || saves!=0 || len(n.events)!=0 {t.Fatal("unavailable capture became metadata-only success",w.Code,saves)}
 close(release);awaitEventSetupStep(t,ready)
 n.mu.RLock();intent,manager=cam.IsRecording,cam.bufferManager;n.mu.RUnlock()
 if !intent || manager==nil {t.Fatal("successful retry did not publish active manager")}
 if err:=n.transitionRecordingWithAdmission(cam.ID,"stop","",worker,func(string)error{return nil});err!=nil {t.Fatal(err)}
 if attempts!=3 || cam.IsRecording || cam.recordCancel!=nil || cam.bufferManager!=nil {t.Fatal("retry or stop lifecycle incorrect",attempts)}
}

func TestEventStorageSetupRetryJoinsOnStopAndShutdown(t *testing.T) {
 for _,shutdown:=range []bool{false,true} {
  name:="stop";if shutdown {name="shutdown"}
  t.Run(name,func(t *testing.T){
   n,cam:=lifecycleFixture(ModeEventBased)
   waiting,returned:=make(chan struct{}),make(chan struct{})
   worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){
    defer close(returned)
    recordCameraEventBasedWithSetup(ctx,n,cam,func(string)error{return os.ErrPermission},func(ctx context.Context)bool{close(waiting);<-ctx.Done();return false},func(*BufferManager){t.Error("recording entered after failed setup")})
   }
   defer n.cancelAndWaitRecorders()
   if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string)error{return nil});err!=nil {t.Fatal(err)}
   awaitEventSetupStep(t,waiting)
   if shutdown {n.shuttingDown.Store(true);n.cancelAndWaitRecorders()} else if err:=n.transitionRecordingWithAdmission(cam.ID,"stop","",worker,func(string)error{return nil});err!=nil {t.Fatal(err)}
   awaitEventSetupStep(t,returned)
   if cam.IsRecording!=shutdown || cam.bufferManager!=nil || cam.recordCancel!=nil {t.Fatal("stop/shutdown intent or join incorrect")}
  })
 }
}

func TestEventStorageSetupCanceledBeforeEntryDoesNotInitialize(t *testing.T) {
 n,cam:=lifecycleFixture(ModeEventBased);ctx,cancel:=context.WithCancel(context.Background());cancel()
 cam.recordWG.Add(1)
 recordCameraEventBasedWithSetup(ctx,n,cam,func(string)error{t.Error("setup after cancellation");return errors.New("unexpected")},func(context.Context)bool{t.Error("retry after cancellation");return false},func(*BufferManager){t.Error("run after cancellation")})
 cam.recordWG.Wait()
 if cam.bufferManager!=nil {t.Fatal("canceled setup exposed manager")}
}
