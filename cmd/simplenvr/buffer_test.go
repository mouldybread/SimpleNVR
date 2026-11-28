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
 if !job.keepStaging || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("unprocessed release did not retain/close/return capacity")}
 assertPublicationBytes(t,job.preFiles[0],content)
 if m:=readRecoveryTest(t,job.dir);m.State!="pinned" || m.PinnedCount!=1 {t.Fatal(m)}
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

func TestEventAdmissionBoundedAndCancellationRetainsCheckpoints(t *testing.T) {
 bm:=bufferFixture(t)
 for _,id:=range []string{"one","two"} {if err:=bm.TriggerEvent(bufferTestEvent(id));err!=nil {t.Fatal(err)}}
 if err:=bm.TriggerEvent(bufferTestEvent("three"));!errors.Is(err,errEventBusy) {t.Fatalf("expected capacity error: %v",err)}
 bm.cancel()
 bm.runWorkers(nil,func(job *bufferEvent){t.Error("canceled job executed")})
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("canceled jobs leaked slots")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=2 {t.Fatal("canceled checkpoints lost",err)}
 seen:=make(map[string]bool)
 for _,entry:=range entries {
  m:=readRecoveryTest(t,filepath.Join(bm.camera.RecordDir,"temp",entry.Name()))
  if m.State!="pinned" || m.PinnedCount!=0 || len(m.Inputs)!=0 || m.Revision!=3 || m.OutputPublished!="" {t.Fatalf("bad retained checkpoint: %+v",m)}
  seen[m.EventID]=true
 }
 if !seen["one"] || !seen["two"] {t.Fatal("retained event identities changed")}
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

// No consumer makes queue refusal deterministic. Cancellation is synchronized
// after slot acquisition but before pinning finishes, not with a timing guess.
func TestEventQueueRefusalAndCancellationRetainCheckpoints(t *testing.T) {
 for _,pins:=range []bool{false,true} {
  for _,cancelDuringPin:=range []bool{false,true} {
   name:="no-pins";if pins {name="pins"};if cancelDuringPin {name+="/canceled"} else {name+="/refused"}
   t.Run(name,func(t *testing.T){
    bm:=bufferFixture(t)
    bm.eventChan=make(chan *bufferEvent)
    source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
    content:=strings.Repeat("p",2048)
    if pins {
     if err:=os.WriteFile(source,[]byte(content),0600);err!=nil {t.Fatal(err)}
     now:=time.Now().UTC();bm.registerCompleted(source,now.Add(-time.Second),now)
    }
    want:=errEventBusy
    var got error
    if cancelDuringPin {
     want=errEventStopped
     bm.bufferMutex.Lock()
     result:=make(chan error,1)
     go func(){result<-bm.TriggerEvent(bufferTestEvent("unprocessed"))}()
     deadline:=time.Now().Add(5*time.Second)
     for len(bm.slots)==0 && time.Now().Before(deadline) {time.Sleep(time.Millisecond)}
     admitted:=len(bm.slots)==1
     bm.cancel()
     bm.bufferMutex.Unlock()
     select {case got=<-result:case <-time.After(5*time.Second):t.Fatal("admission did not join")}
     if !admitted {t.Fatal("admission never acquired slot")}
    } else {got=bm.TriggerEvent(bufferTestEvent("unprocessed"))}
    if !errors.Is(got,want) {t.Fatalf("admission error=%v, want %v",got,want)}
    if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("unprocessed job leaked capacity")}
    entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatal("checkpoint staging lost",err)}
    dir:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name())
    m:=readRecoveryTest(t,dir)
    count:=0;if pins {count=1}
    if m.EventID!="unprocessed" || m.CameraID!=bm.camera.ID || m.State!="pinned" || m.Revision!=3 || m.PinnedCount!=count || len(m.Inputs)!=count || m.OutputPublished!="" {t.Fatalf("bad retained checkpoint: %+v",m)}
    if _,err:=os.Stat(filepath.Join(dir,"event.mp4"));!os.IsNotExist(err) {t.Fatal("unprocessed event acquired output",err)}
    if pins {
     got,err:=os.ReadFile(source);if err!=nil || string(got)!=content {t.Fatal("source changed",err)}
     bm.pruneBefore(time.Now().UTC().Add(time.Second))
     got,err=os.ReadFile(filepath.Join(dir,"pre_000.mp4"));if err!=nil || string(got)!=content {t.Fatal("retained pin lost after pruning",err)}
    }
   })
  }
 }
}

func TestTerminalReleaseRetainsRegardlessOfLegacyFlag(t *testing.T) {
 for _,kind:=range []string{"fresh","manual","checkpoint failure"} {t.Run(kind,func(t *testing.T){
  bm:=bufferFixture(t)
  var job *bufferEvent
  if kind=="manual" {job=&bufferEvent{dir:t.TempDir(),event:bufferTestEvent("manual")};bm.slots<-struct{}{}} else {
   if err:=bm.TriggerEvent(bufferTestEvent(kind));err!=nil {t.Fatal(err)}
   job=<-bm.eventChan
   if !job.keepStaging {t.Fatal("new job did not default to retention")}
  }
  inventory:=job.inventory
  input:=filepath.Join(job.dir,"foreign");if err:=os.WriteFile(input,[]byte("retain bytes"),0600);err!=nil {t.Fatal(err)}
  nested:=filepath.Join(job.dir,"nested");if err:=os.Mkdir(nested,0700);err!=nil {t.Fatal(err)}
  child:=filepath.Join(nested,"evidence");if err:=os.WriteFile(child,[]byte("nested bytes"),0600);err!=nil {t.Fatal(err)}
  var before []byte
  if kind=="checkpoint failure" {
   var err error;before,err=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
   job.recoveryWriter=func(string,[]byte)error{return os.ErrPermission}
  }
  job.keepStaging=false
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("terminal ownership/capacity leaked")}
  if inventory!=nil {assertJobInventoryClosed(t,inventory)}
  assertPublicationBytes(t,input,"retain bytes");assertPublicationBytes(t,child,"nested bytes")
  if kind=="checkpoint failure" {assertPublicationBytes(t,filepath.Join(job.dir,"recovery.json"),string(before));if !job.recoveryFailed {t.Fatal("failure not latched")}}
 })}
}
