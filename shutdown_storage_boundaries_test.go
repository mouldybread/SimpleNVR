package main

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func TestShutdownDeadlineDoesNotSchedulePersistenceAfterLateDrain(t *testing.T) {
 for _,blocked:=range []string{"workers","mutations","http"} {t.Run(blocked,func(t *testing.T){
  workers,mutations:=make(chan struct{}),make(chan struct{})
  httpDone:=make(chan error,1)
  if blocked!="workers" {close(workers)}
  if blocked!="mutations" {close(mutations)}
  if blocked!="http" {httpDone<-nil}
  ctx,cancel:=context.WithTimeout(context.Background(),50*time.Millisecond);defer cancel()
  var saves atomic.Int32
  err:=completeApplicationShutdown(ctx,workers,mutations,httpDone,func()error{saves.Add(1);return nil})
  if !errors.Is(err,context.DeadlineExceeded) || saves.Load()!=0 {t.Fatal("blocked drain did not fail without saving",err,saves.Load())}
  switch blocked {case "workers":close(workers);case "mutations":close(mutations);case "http":httpDone<-nil}
  // The completion function has returned synchronously; no waiting save
  // coordinator remains to respond to the newly released channel.
  if saves.Load()!=0 {t.Fatal("late drain started persistence")}
 })}
}

func TestShutdownCompletionRejectsAlreadyCanceledContextAndPreservesErrors(t *testing.T) {
 joined:=make(chan struct{});close(joined)
 httpDone:=make(chan error,1);httpDone<-nil
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=completeApplicationShutdown(ctx,joined,joined,httpDone,func()error{t.Error("canceled shutdown started persistence");return nil});!errors.Is(err,context.Canceled) {t.Fatal(err)}
 httpErr,saveErr:=errors.New("http failure"),errors.New("save failure")
 httpDone=make(chan error,1);httpDone<-httpErr
 if err:=completeApplicationShutdown(context.Background(),joined,joined,httpDone,func()error{return saveErr});!errors.Is(err,httpErr) || !errors.Is(err,saveErr) {t.Fatal("completion lost errors",err)}
 httpDone=make(chan error,1);httpDone<-nil
 if err:=completeApplicationShutdown(context.Background(),joined,joined,httpDone,nil);err==nil {t.Fatal("missing persistence callback accepted")}
}

func TestBlockedAdmissionShutdownDeadlineJoinsAfterReleaseWithoutLateSave(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 entered,release,requestDone:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 var starts,saves atomic.Int32
 var admissionErr error
 worker:=func(_ context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();starts.Add(1)}
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  admissionErr=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string)error{close(entered);<-release;return nil})
  w.WriteHeader(503)
 }))
 go func(){defer close(requestDone);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("POST","/api/record/start",nil))}()
 awaitShutdownStep(t,entered,"admission did not block")
 requireRegistryAvailable(t,n)
 ctx,cancel:=context.WithTimeout(context.Background(),50*time.Millisecond);defer cancel()
 err:=shutdownApplication(ctx,n,gate,nil,func()error{saves.Add(1);return nil})
 if !errors.Is(err,context.DeadlineExceeded) || saves.Load()!=0 {t.Fatal("blocked admission reported shutdown success",err,saves.Load())}
 once.Do(func(){close(release)})
 awaitShutdownStep(t,requestDone,"blocked mutation did not finish after release")
 awaitShutdownStep(t,gate.drained,"gate did not join late mutation")
 if !errors.Is(admissionErr,errRecorderShuttingDown) || starts.Load()!=0 || saves.Load()!=0 {t.Fatal("late admission launched work or final save",admissionErr,starts.Load(),saves.Load())}
 n.mu.RLock();active,cancelFn:=cam.IsRecording,cam.recordCancel;n.mu.RUnlock()
 if active || cancelFn!=nil {t.Fatal("late admission left active state")}
}
