package main

import (
 "context"
 "net/http"
 "os"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func TestBackgroundCleanupIsCanceledAndJoinedBeforePersistence(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{}}
 gate:=newShutdownGate(http.HandlerFunc(func(http.ResponseWriter,*http.Request){}))
 started,canceled,release:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once
 defer once.Do(func(){close(release)})
 if !gate.startBackgroundJob(func(ctx context.Context){close(started);<-ctx.Done();close(canceled);<-release}) {t.Fatal("background worker rejected before shutdown")}
 awaitShutdownStep(t,started,"background worker not started")
 var persisted atomic.Bool
 done:=make(chan error,1)
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 go func(){done<-shutdownApplication(ctx,n,gate,nil,func() error {persisted.Store(true);return nil})}()
 awaitShutdownStep(t,canceled,"shutdown did not cancel background cleanup")
 if persisted.Load() {t.Fatal("persisted before cleanup exited")}
 if gate.startBackgroundJob(func(context.Context){t.Error("late worker ran")}) {t.Fatal("background worker admitted after shutdown")}
 once.Do(func(){close(release)})
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("cleanup join did not complete")}
 if !persisted.Load() {t.Fatal("final persistence missing")}
 gate.mu.Lock();remaining:=len(gate.jobs);gate.mu.Unlock()
 if remaining!=0 {t.Fatalf("background job registration leaked: %d",remaining)}
}

func TestCanceledCleanupTickDoesNotMutateEventsOrRemoveMedia(t *testing.T) {
 dir:=t.TempDir()
 file:=filepath.Join(dir,"20000101_000000_expired.mp4")
 putCleanupFixture(t,file,"published")
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",Name:"A",RecordDir:dir,RetentionDays:1}},events:[]Event{{CameraName:"A",StartTime:time.Now().Add(-48*time.Hour)}}}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 ticks:=make(chan time.Time,1);ticks<-time.Now()
 done:=make(chan struct{})
 go func(){defer close(done);runCleanupTicks(ctx,n,ticks)}()
 awaitShutdownStep(t,done,"canceled cleanup worker did not exit")
 if len(n.events)!=1 {t.Fatal("canceled cleanup changed event metadata")}
 if _,err:=os.Stat(file);err!=nil {t.Fatalf("canceled cleanup removed media: %v",err)}
 cleanupOldRecordingsContext(ctx,dir,time.Hour)
 if _,err:=os.Stat(file);err!=nil {t.Fatalf("canceled retention removed media: %v",err)}
}

func TestManagedCleanupTickPreservesStagingAndRetainsPublishedPolicy(t *testing.T) {
 dir:=t.TempDir()
 expired:=filepath.Join(dir,"20000101_000000_expired.mp4")
 staging:=filepath.Join(dir,"old.mp4.tmp")
 nested:=filepath.Join(dir,"temp","event-1","pre_000.mp4")
 putCleanupFixture(t,expired,"published");putCleanupFixture(t,staging,"retained");putCleanupFixture(t,nested,"pinned")
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",Name:"A",RecordDir:dir,RetentionDays:1}}}
 ticks:=make(chan time.Time,1);ticks<-time.Now();close(ticks)
 runCleanupTicks(context.Background(),n,ticks)
 if _,err:=os.Stat(expired);!os.IsNotExist(err) {t.Fatalf("expired published media retained: %v",err)}
 for _,file:=range []string{staging,nested} {if _,err:=os.Stat(file);err!=nil {t.Fatalf("recovery artifact changed: %v",err)}}
}
