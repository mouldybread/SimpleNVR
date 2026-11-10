package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func bufferFixture(t *testing.T) *BufferManager {
 t.Helper()
 cam:=&Camera{ID:"test",Name:"Test",RecordDir:t.TempDir()}
 bm:=newBufferManager(context.Background(),cam)
 t.Cleanup(bm.cancel)
 return bm
}

func bufferTestEvent(id string) Event {return Event{ID:id,Label:"Motion",Duration:30,CreatedAt:time.Now().UTC(),StartTime:time.Now().UTC()}}

func TestPinnedPrebufferSurvivesPruning(t *testing.T) {
 bm:=bufferFixture(t)
 dir:=filepath.Join(bm.camera.RecordDir,"buffer");if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 path:=filepath.Join(dir,"closed.mp4")
 content:=strings.Repeat("x",2048)
 if err:=os.WriteFile(path,[]byte(content),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bm.registerCompleted(path,now.Add(-5*time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("first"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan
 if len(job.preFiles)!=1 {t.Fatalf("pinned %d files",len(job.preFiles))}
 bm.pruneBefore(now.Add(time.Second))
 if _,err:=os.Stat(path);!os.IsNotExist(err) {t.Fatalf("source not pruned: %v",err)}
 got,err:=os.ReadFile(job.preFiles[0]);if err!=nil || string(got)!=content {t.Fatal("pinned prebuffer was lost",err)}
 bm.releaseJob(job)
 if _,err:=os.Stat(job.dir);!os.IsNotExist(err) {t.Fatal("staging leaked")}
}

func TestUnreportedActiveSegmentIsNotPinned(t *testing.T) {
 bm:=bufferFixture(t)
 dir:=filepath.Join(bm.camera.RecordDir,"buffer");if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 closed:=filepath.Join(dir,"closed.mp4");active:=filepath.Join(dir,"active.mp4")
 for _,path:=range []string{closed,active} {if err:=os.WriteFile(path,[]byte(strings.Repeat("x",2048)),0600);err!=nil {t.Fatal(err)}}
 now:=time.Now().UTC();bm.registerCompleted(closed,now.Add(-5*time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("first"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan;defer bm.releaseJob(job)
 if len(job.preFiles)!=1 {t.Fatal("active segment was admitted")}
}

func TestEventAdmissionBoundedAndCancellationDrainsStaging(t *testing.T) {
 bm:=bufferFixture(t)
 for _,id:=range []string{"one","two"} {if err:=bm.TriggerEvent(bufferTestEvent(id));err!=nil {t.Fatal(err)}}
 if err:=bm.TriggerEvent(bufferTestEvent("three"));!errors.Is(err,errEventBusy) {t.Fatalf("expected capacity error: %v",err)}
 bm.cancel()
 bm.runWorkers(nil,func(job *bufferEvent){t.Error("canceled job executed")})
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("canceled jobs leaked slots")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=0 {t.Fatal("canceled staging leaked",err)}
 if err:=bm.TriggerEvent(bufferTestEvent("late"));!errors.Is(err,errEventStopped) {t.Fatalf("stopped recorder accepted event: %v",err)}
}

func TestEventManagerJoinsMaintenanceAndActiveJobs(t *testing.T) {
 bm:=bufferFixture(t)
 maintenanceStarted:=make(chan struct{},2)
 maintenanceCanceled:=make(chan struct{},2)
 active:=make(chan struct{})
 jobCanceled:=make(chan struct{})
 release:=make(chan struct{})
 stopped:=make(chan struct{})
 task:=func(){maintenanceStarted<-struct{}{};<-bm.ctx.Done();maintenanceCanceled<-struct{}{}}
 if err:=bm.TriggerEvent(bufferTestEvent("one"));err!=nil {t.Fatal(err)}
 bm.camera.recordWG.Add(1)
 go func(){defer bm.camera.recordWG.Done();bm.runWorkers([]func(){task,task},func(job *bufferEvent){close(active);<-bm.ctx.Done();close(jobCanceled);<-release})}()
 go func(){bm.camera.recordWG.Wait();close(stopped)}()
 for i:=0;i<2;i++ {select {case <-maintenanceStarted:case <-time.After(time.Second):t.Fatal("maintenance not started")}}
 select {case <-active:case <-time.After(time.Second):t.Fatal("event worker not started")}
 bm.cancel()
 for i:=0;i<2;i++ {select {case <-maintenanceCanceled:case <-time.After(time.Second):t.Fatal("maintenance not canceled")}}
 select {case <-jobCanceled:case <-time.After(time.Second):t.Fatal("event not canceled")}
 select {case <-stopped:t.Fatal("parent completed before child");default:}
 close(release)
 select {case <-stopped:case <-time.After(time.Second):t.Fatal("children not joined")}
 if len(bm.slots)!=0 {t.Fatal("active slot leaked")}
}

func TestInvalidEventDurationDoesNotReserveSlot(t *testing.T) {
 bm:=bufferFixture(t)
 for _,duration:=range []int{0,-1,maxEventSeconds+1} {
  event:=bufferTestEvent("bad");event.Duration=duration
  if err:=bm.TriggerEvent(event);!errors.Is(err,errEventInvalid) {t.Fatal(err)}
 }
 if len(bm.slots)!=0 {t.Fatal("invalid event reserved slot")}
}
