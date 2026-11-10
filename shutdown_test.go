package main

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func awaitShutdownStep(t *testing.T,ch <-chan struct{},message string) {
 t.Helper();select {case <-ch:case <-time.After(3*time.Second):t.Fatal(message)}
}

func TestShutdownJoinsRecordersDrainsMutationsAndPreservesRestartIntent(t *testing.T) {
 active:=&Camera{ID:"active",Name:"Active",RecordDir:"/recordings/active",RecordingMode:ModeContinuous}
 paused:=&Camera{ID:"paused",Name:"Paused",RecordDir:"/recordings/paused",RecordingMode:ModeContinuous}
 n:=&NVR{cameras:map[string]*Camera{active.ID:active,paused.ID:paused},cameraOrder:[]string{active.ID,paused.ID}}
 started,canceled,releaseChild:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var releaseChildOnce sync.Once
 defer releaseChildOnce.Do(func(){close(releaseChild)})
 worker:=func(ctx context.Context,cam *Camera,mode RecordingMode){defer cam.recordWG.Done();close(started);<-ctx.Done();close(canceled);<-releaseChild}
 if err:=n.transitionRecording(active.ID,"start","",worker);err!=nil {t.Fatal(err)}
 awaitShutdownStep(t,started,"worker not started")
 mutationEntered,releaseMutation,requestDone:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var releaseMutationOnce sync.Once
 defer releaseMutationOnce.Do(func(){close(releaseMutation)})
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(mutationEntered);<-releaseMutation;n.mu.Lock();paused.RetentionDays=14;n.mu.Unlock();w.WriteHeader(204)}))
 go func(){defer close(requestDone);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("POST","/api/cameras/update-retention",nil))}()
 awaitShutdownStep(t,mutationEntered,"mutation not admitted")
 path:=filepath.Join(t.TempDir(),"cameras.json")
 var persisted atomic.Bool
 done:=make(chan error,1)
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 go func(){done<-shutdownApplication(ctx,n,gate,&http.Server{},func() error {err:=n.saveAt(path);persisted.Store(true);return err})}()
 awaitShutdownStep(t,canceled,"recorder not canceled")
 if persisted.Load() {t.Fatal("persisted before worker joined")}
 w:=httptest.NewRecorder();gate.ServeHTTP(w,httptest.NewRequest("GET","/",nil))
 if w.Code!=503 {t.Fatal("shutdown gate accepted a new request")}
 if err:=n.transitionRecording(active.ID,"start","",worker);!errors.Is(err,errRecorderShuttingDown) {t.Fatalf("late start accepted: %v",err)}
 releaseChildOnce.Do(func(){close(releaseChild)})
 if persisted.Load() {t.Fatal("persisted before admitted mutation drained")}
 releaseMutationOnce.Do(func(){close(releaseMutation)})
 awaitShutdownStep(t,requestDone,"admitted mutation did not drain")
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("shutdown did not complete")}
 reloaded:=&NVR{}
 if err:=reloaded.loadAt(path);err!=nil {t.Fatal(err)}
 if !reloaded.cameras["active"].IsRecording || reloaded.cameras["paused"].IsRecording || reloaded.cameras["paused"].RetentionDays!=14 {t.Fatal("recording intent or drained mutation was lost")}
 resumed:=make(chan string,2)
 resumeWorker:=func(ctx context.Context,cam *Camera,mode RecordingMode){defer cam.recordWG.Done();resumed<-cam.ID;<-ctx.Done()}
 if err:=reloaded.resumeRecordings(resumeWorker);err!=nil {t.Fatal(err)}
 select {case id:=<-resumed:if id!="active" {t.Fatal("paused camera resumed")};case <-time.After(3*time.Second):t.Fatal("active camera not resumed")}
 if err:=reloaded.transitionRecording("active","stop","",resumeWorker);err!=nil {t.Fatal(err)}
 if reloaded.cameras["active"].IsRecording {t.Fatal("explicit API stop did not clear intent")}
}

func TestModeChangeInterruptedByShutdownDoesNotLosePreviousIntent(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 started,canceled,release:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 var starts atomic.Int32
 worker:=func(ctx context.Context,cam *Camera,mode RecordingMode){defer cam.recordWG.Done();starts.Add(1);close(started);<-ctx.Done();close(canceled);<-release}
 if err:=n.transitionRecording(cam.ID,"start","",worker);err!=nil {t.Fatal(err)}
 awaitShutdownStep(t,started,"worker not started")
 done:=make(chan error,1)
 go func(){done<-n.transitionRecording(cam.ID,"mode",ModeEventBased,worker)}()
 awaitShutdownStep(t,canceled,"mode change did not cancel old worker")
 n.shuttingDown.Store(true)
 once.Do(func(){close(release)})
 select {case err:=<-done:if !errors.Is(err,errRecorderShuttingDown) {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("mode transition stuck")}
 if starts.Load()!=1 || cam.RecordingMode!=ModeContinuous || !cam.IsRecording || cam.recordCancel!=nil {t.Fatal("shutdown started a replacement or erased previous restart intent")}
}

func TestShutdownCancellationReturnsWithoutClaimingPersistenceCompleted(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{}}
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){}))
 entered,release,finished:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 done:=make(chan error,1)
 go func(){done<-shutdownApplication(ctx,n,gate,&http.Server{},func() error {close(entered);<-release;close(finished);return nil})}()
 awaitShutdownStep(t,entered,"persistence callback not reached")
 cancel()
 select {case err:=<-done:if !errors.Is(err,context.Canceled) {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("canceled shutdown blocked")}
 once.Do(func(){close(release)})
 awaitShutdownStep(t,finished,"test persistence worker leaked")
}
