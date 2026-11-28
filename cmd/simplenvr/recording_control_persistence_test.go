package main

import (
 "context"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
)

func TestFailedModeReplacementPersistsActualStoppedModeAndRestartState(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.RecordDir="/recordings/test"
 worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();<-ctx.Done()}
 if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string) error {return nil});err!=nil {t.Fatal(err)}
 defer n.cancelAndWaitRecorders()
 path:=filepath.Join(t.TempDir(),"state.json");if err:=n.saveAt(path);err!=nil {t.Fatal(err)}
 checks,saves:=0,0
 check:=func(string) error {checks++;if checks==2 {return errStorageReserve};return nil}
 persist:=func() error {saves++;requireRegistryAvailable(t,n);return n.saveAt(path)}
 err:=n.transitionRecordingAndSave(cam.ID,"mode",ModeEventBased,worker,check,persist)
 if !errors.Is(err,errStorageReserve) || saves!=1 || checks!=2 {t.Fatal("failed replacement not saved",err,saves,checks)}
 restored:=&NVR{};if err:=restored.loadAt(path);err!=nil {t.Fatal(err)}
 saved:=restored.cameras[cam.ID]
 if saved==nil || saved.IsRecording || saved.RecordingMode!=ModeEventBased || cam.recordCancel!=nil {t.Fatal("stale restart intent persisted")}
 starts:=0
 if err:=restored.resumeRecordingsWithAdmission(func(_ context.Context,cam *Camera,_ RecordingMode){starts++;cam.recordWG.Done()},func(string) error {return nil});err!=nil {t.Fatal(err)}
 if starts!=0 {t.Fatal("failed replacement restarted unexpectedly")}
}

func TestRecordingPreflightRefusalsDoNotPersistOrDisturbOldWorker(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();<-ctx.Done()}
 if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string) error {return nil});err!=nil {t.Fatal(err)}
 defer n.cancelAndWaitRecorders();saves:=0
 err:=n.transitionRecordingAndSave(cam.ID,"mode",ModeEventBased,worker,func(string) error {return errStorageReserve},func() error {saves++;return nil})
 if !errors.Is(err,errStorageReserve) || saves!=0 || !cam.IsRecording || cam.recordCancel==nil || cam.RecordingMode!=ModeContinuous {t.Fatal("preflight refusal changed running state",err,saves)}
}

func TestRecordingPersistenceFailurePrecedesAdmissionStatusWithoutRollback(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.IsRecording=true;checks:=0
 err:=n.transitionRecordingAndSave(cam.ID,"mode",ModeEventBased,func(_ context.Context,cam *Camera,_ RecordingMode){t.Error("unexpected replacement worker");cam.recordWG.Done()},func(string) error {checks++;if checks==2 {return errStorageReserve};return nil},func() error {return os.ErrPermission})
 if !errors.Is(err,errRecordingPersistence) || !errors.Is(err,errStorageReserve) || !errors.Is(err,os.ErrPermission) {t.Fatal("combined failure lost",err)}
 if cam.IsRecording || cam.RecordingMode!=ModeEventBased {t.Fatal("failure falsely rolled back mode or restarted worker")}
 w:=httptest.NewRecorder();recordingError(w,err)
 if w.Code!=500 {t.Fatal("persistence failure masked by admission status",w.Code)}
}

func TestRecordingControlsHoldSerializationThroughPersistenceNotRegistry(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);saves:=0
 worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();<-ctx.Done()}
 defer n.cancelAndWaitRecorders()
 persist:=func() error {
  saves++;requireRegistryAvailable(t,n)
  if recordingControlMu.TryLock() {recordingControlMu.Unlock();t.Fatal("control serialization released before save")}
  return nil
 }
 for _,action:=range []string{"start","stop"} {
  if err:=n.transitionRecordingAndSave(cam.ID,action,"",worker,func(string) error {return nil},persist);err!=nil {t.Fatal(err)}
 }
 if saves!=2 || cam.IsRecording || cam.recordCancel!=nil {t.Fatal("control outcome not saved",saves)}
}

func TestRecordingSaveWrapperRejectsMissingPersistenceAndInvalidControls(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);starts,saves:=0,0
 worker:=func(_ context.Context,cam *Camera,_ RecordingMode){starts++;cam.recordWG.Done()}
 if err:=n.transitionRecordingAndSave(cam.ID,"start","",worker,func(string) error {return nil},nil);!errors.Is(err,errRecordingPersistence) {t.Fatal("missing persistence accepted",err)}
 persist:=func() error {saves++;return nil}
 for _,item:=range []struct{id,action string;mode RecordingMode}{
  {"missing","start",""},{cam.ID,"invalid",""},{cam.ID,"mode","invalid"},
 } {if err:=n.transitionRecordingAndSave(item.id,item.action,item.mode,worker,func(string) error {return nil},persist);err==nil {t.Fatal("invalid control accepted")}}
 n.shuttingDown.Store(true)
 if err:=n.transitionRecordingAndSave(cam.ID,"start","",worker,func(string) error {return nil},persist);!errors.Is(err,errRecorderShuttingDown) {t.Fatal("shutdown admission accepted",err)}
 if starts!=0 || saves!=0 {t.Fatal("refusal launched or saved state",starts,saves)}
}

func TestModeReplacementShutdownKeepsRestoredIntentAndSavesChangedMode(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.IsRecording=true;checks,saves:=0,0
 check:=func(string) error {checks++;if checks==2 {n.shuttingDown.Store(true)};return nil}
 worker:=func(_ context.Context,cam *Camera,_ RecordingMode){t.Error("worker started during shutdown");cam.recordWG.Done()}
 err:=n.transitionRecordingAndSave(cam.ID,"mode",ModeEventBased,worker,check,func() error {saves++;return nil})
 if !errors.Is(err,errRecorderShuttingDown) || saves!=1 || !cam.IsRecording || cam.recordCancel!=nil || cam.RecordingMode!=ModeEventBased {t.Fatal("shutdown intent semantics changed or mode not saved",err,saves)}
}
