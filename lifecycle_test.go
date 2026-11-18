package main

import (
 "context"
 "errors"
 "net/http/httptest"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func lifecycleFixture(mode RecordingMode) (*NVR,*Camera) {
 cam:=&Camera{ID:"test",Name:"Test",RecordingMode:mode}
 return &NVR{cameras:map[string]*Camera{cam.ID:cam},cameraOrder:[]string{cam.ID}},cam
}

func TestDisabledStartDoesNotLeakWaitGroup(t *testing.T) {
 n,cam:=lifecycleFixture(ModeNoRecording)
 if err:=n.transitionRecording(cam.ID,"start","",nil);!errors.Is(err,errRecordingDisabled) {t.Fatalf("unexpected result: %v",err)}
 if cam.IsRecording || cam.recordCancel!=nil {t.Fatal("disabled start changed runtime state")}
 done:=make(chan struct{})
 go func(){cam.recordWG.Wait();close(done)}()
 select {case <-done:case <-time.After(time.Second):t.Fatal("WaitGroup leaked")}
 if err:=n.transitionRecording(cam.ID,"stop","",nil);err!=nil {t.Fatal(err)}
}

func TestInvalidAndMissingControlsDoNotStartWorker(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 if err:=n.transitionRecording("missing","start","",nil);!errors.Is(err,errCameraMissing) {t.Fatal(err)}
 if err:=n.transitionRecording(cam.ID,"mode","invalid",nil);!errors.Is(err,errRecordingMode) {t.Fatal(err)}
 if cam.IsRecording || cam.recordCancel!=nil {t.Fatal("invalid control changed state")}
}

func TestConcurrentControlsNeverOverlapTopLevelWorkers(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 var active,maximum,starts atomic.Int32
 worker:=func(ctx context.Context,cam *Camera,mode RecordingMode){
  defer cam.recordWG.Done()
  count:=active.Add(1)
  for old:=maximum.Load();count>old;old=maximum.Load(){if maximum.CompareAndSwap(old,count){break}}
  starts.Add(1)
  <-ctx.Done()
  active.Add(-1)
 }
 if err:=n.transitionRecording(cam.ID,"start","",worker);err!=nil {t.Fatal(err)}
 var wg sync.WaitGroup
 for i:=0;i<60;i++ {
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   var err error
   switch i%3 {
   case 0:err=n.transitionRecording(cam.ID,"start","",worker)
   case 1:err=n.transitionRecording(cam.ID,"mode",ModeEventBased,worker)
   case 2:err=n.transitionRecording(cam.ID,"stop","",worker)
   }
   if err!=nil {t.Error(err)}
  }(i)
 }
 done:=make(chan struct{})
 go func(){wg.Wait();close(done)}()
 select {case <-done:case <-time.After(5*time.Second):t.Fatal("concurrent controls deadlocked")}
 if err:=n.transitionRecording(cam.ID,"stop","",worker);err!=nil {t.Fatal(err)}
 if maximum.Load()!=1 || active.Load()!=0 || starts.Load()==0 {t.Fatalf("active=%d max=%d starts=%d",active.Load(),maximum.Load(),starts.Load())}
}

func TestModeChangeWaitsForWorkerBeforeReplacingIt(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 started:=make(chan struct{})
 canceled:=make(chan struct{})
 release:=make(chan struct{})
 var starts atomic.Int32
 worker:=func(ctx context.Context,cam *Camera,mode RecordingMode){
  defer cam.recordWG.Done()
  if starts.Add(1)==1 {close(started);<-ctx.Done();close(canceled);<-release;return}
  <-ctx.Done()
 }
 if err:=n.transitionRecording(cam.ID,"start","",worker);err!=nil {t.Fatal(err)}
 <-started
 done:=make(chan error,1)
 go func(){done<-n.transitionRecording(cam.ID,"mode",ModeEventBased,worker)}()
 select {case <-canceled:case <-time.After(time.Second):t.Fatal("old worker not canceled")}
 if starts.Load()!=1 {t.Fatal("replacement started before old worker completed")}
 // State readers must not be blocked by the worker-completion wait.
 readDone:=make(chan struct{})
 go func(){n.mu.RLock();n.mu.RUnlock();close(readDone)}()
 select {case <-readDone:case <-time.After(time.Second):t.Fatal("state lock held while waiting")}
 close(release)
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(time.Second):t.Fatal("mode change did not complete")}
 if err:=n.transitionRecording(cam.ID,"stop","",worker);err!=nil {t.Fatal(err)}
}

func TestControlRequestValidation(t *testing.T) {
 cases:=[]struct{method,body string;status int}{
  {"GET",`{"id":"test"}`,405},
  {"POST",`{`,400},
  {"POST",`{}`,400},
  {"POST",`{"id":"test"} {}`,400},
  {"POST",`{"id":"test","unexpected":true}`,400},
 }
 for _,c:=range cases {
  w:=httptest.NewRecorder();r:=httptest.NewRequest(c.method,"/api/record/start",strings.NewReader(c.body))
  if _,ok:=decodeRecordingRequest(w,r);ok || w.Code!=c.status {t.Errorf("%s %s: status=%d",c.method,c.body,w.Code)}
 }
}
