package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

type jobInventoryWaitContext struct {context.Context;waiting chan struct{};once sync.Once}
func (ctx *jobInventoryWaitContext) Done() <-chan struct{} {ctx.once.Do(func(){close(ctx.waiting)});return ctx.Context.Done()}
func assertJobInventoryClosed(t *testing.T,inventory *ownedStagingInventory) {
 t.Helper();if inventory==nil || inventory.root!=nil || inventory.parent!=nil || len(inventory.entries)!=0 {t.Fatal("inventory not closed")}
}
func TestProductionJobAdoptsPinsAndOwnedCheckpoints(t *testing.T) {
 bm:=bufferFixture(t);source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
 if err:=os.WriteFile(source,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(source,now.Add(-time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("owned"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan;inventory:=job.inventory
 if inventory==nil || len(inventory.entries)!=2 {t.Fatal("production pin/checkpoint ownership missing")}
 oldCheckpoint:=inventory.entries[ownedCheckpointName].file
 sourcePin:=inventory.entries["pre_000.mp4"].file
 lease:=publishedEngineLease(t)
 if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
 lease.release()
 if err:=job.recoveryCheckpoint("captured","");err!=nil {t.Fatal(err)}
 if _,err:=oldCheckpoint.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("checkpoint descriptor not superseded",err)}
 m:=readRecoveryTest(t,job.dir);if m.State!="captured" || m.Revision!=3 || m.PinnedCount!=1 {t.Fatal(m)}
 job.keepStaging=true;bm.releaseJob(job)
 assertJobInventoryClosed(t,inventory)
 if _,err:=sourcePin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("input pin leaked",err)}
 if len(bm.slots)!=0 || job.inventory!=nil {t.Fatal("terminal ownership leaked")}
 if err:=job.writeOwnedJobRecovery(job.dir,[]byte("must not write"));err==nil {t.Fatal("closed job fell back to legacy writer")}
 m=readRecoveryTest(t,job.dir);if m.Revision!=4 || m.State!="captured" {t.Fatal(m)}
}

func TestPinFailureClosesInventoryAndPreservesProgress(t *testing.T) {
 bm:=bufferFixture(t);source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
 if err:=os.WriteFile(source,[]byte("pin evidence"),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bm.buffer=[]BufferSegment{{now.Add(-time.Second),now,source},{now.Add(-time.Second),now,source+"-missing"}}
 var captured *ownedStagingInventory
 _,err:=bm.pinOwnedBufferWithOpen(bufferTestEvent("pin-error"),func(ctx context.Context,lease *mediaMutationLease,dir string)(*ownedStagingInventory,error){var err error;captured,err=openOwnedStagingInventory(ctx,lease,dir);return captured,err})
 if err==nil {t.Fatal("missing pin admitted")}
 assertJobInventoryClosed(t,captured)
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatal(entries,err)}
 dir:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name())
 m:=readRecoveryTest(t,dir);if m.State!="pin_failed" || m.FailureCode!="link_failed" || m.PinnedCount!=1 || len(m.Inputs)!=2 {t.Fatal(m)}
 assertPublicationBytes(t,filepath.Join(dir,"pre_000.mp4"),"pin evidence")
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("setup error leaked lease")}
}

func TestCanceledQueuedAndActiveJobsCloseInventory(t *testing.T) {
 for _,active:=range []bool{false,true} {t.Run(map[bool]string{false:"queued",true:"active"}[active],func(t *testing.T){
  bm:=bufferFixture(t)
  if err:=bm.TriggerEvent(bufferTestEvent("terminal"));err!=nil {t.Fatal(err)}
  job:=<-bm.eventChan;inventory:=job.inventory;bm.eventChan<-job
  entered,release,done:=make(chan struct{}),make(chan struct{}),make(chan struct{})
  var once sync.Once;defer once.Do(func(){close(release)})
  if !active {bm.cancel()}
  go func(){defer close(done);bm.runWorkers(nil,func(job *bufferEvent){if !active {t.Error("queued cancellation executed capture")};job.keepStaging=true;close(entered);<-release})}()
  if active {awaitShutdownStep(t,entered,"worker did not enter");bm.cancel()}
  once.Do(func(){close(release)})
  awaitShutdownStep(t,done,"workers did not join")
  assertJobInventoryClosed(t,inventory)
  if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("terminal capacity leaked")}
  m:=readRecoveryTest(t,job.dir);if m.State!="pinned" || m.Revision!=3 {t.Fatal(m)}
 })}
}

func TestRecoveryWriteFailureStillClosesInventoryAndReturnsSlot(t *testing.T) {
 bm:=bufferFixture(t)
 if err:=bm.TriggerEvent(bufferTestEvent("write-error"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan;inventory:=job.inventory
 before,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil {t.Fatal(err)}
 job.recoveryWriter=func(string,[]byte)error{return os.ErrPermission}
 job.keepStaging=true;bm.releaseJob(job)
 assertJobInventoryClosed(t,inventory)
 after,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil || string(after)!=string(before) {t.Fatal("failed retention checkpoint changed evidence",err)}
 if !job.recoveryFailed || len(bm.slots)!=0 {t.Fatal("failure did not retain/return capacity")}
}

func TestOwnedJobCheckpointCanceledWaitPreservesInventory(t *testing.T) {
 bm:=bufferFixture(t)
 if err:=bm.TriggerEvent(bufferTestEvent("wait"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan
 defer func(){job.keepStaging=true;bm.releaseJob(job)}()
 before,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil {t.Fatal(err)}
 owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 ctx:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
 result:=make(chan error,1);go func(){result<-job.writeOwnedJobRecoveryContext(ctx,job.dir,before)}()
 awaitShutdownStep(t,ctx.waiting,"checkpoint did not enter media wait");cancel()
 select {case err:=<-result:if !errors.Is(err,context.Canceled) {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("checkpoint waiter did not join")}
 if !owner.held() || job.inventory.root==nil {t.Fatal("wait cancellation disturbed owner or inventory")}
 assertPublicationBytes(t,filepath.Join(job.dir,ownedCheckpointName),string(before))
}

func TestProductionPinErrorReturnsReservedCapacity(t *testing.T) {
 bm:=bufferFixture(t);now:=time.Now().UTC()
 bm.buffer=[]BufferSegment{{now.Add(-time.Second),now,filepath.Join(bm.camera.RecordDir,"missing.mp4")}}
 if err:=bm.TriggerEvent(bufferTestEvent("missing"));err==nil {t.Fatal("pin error admitted")}
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 || len(recordingMediaOwnership.token)!=0 {t.Fatal("pin error leaked admission")}
}

func TestOwnedJobPinRejectsNonBasenameBeforeLink(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 source:=filepath.Join(t.TempDir(),"source");if err:=os.WriteFile(source,[]byte("outside evidence"),0600);err!=nil {t.Fatal(err)}
 for _,name:=range []string{"",".","..","../outside","nested/input","back\\slash","nul\x00name"} {
  if err:=pinOwnedJobInput(context.Background(),lease,inventory,source,name);err==nil {t.Fatal("invalid pin basename accepted",name)}
 }
 entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=0 {t.Fatal("invalid pin changed staging",entries,err)}
 parent,err:=os.ReadDir(filepath.Dir(dir));if err!=nil || len(parent)!=1 {t.Fatal("pin escaped staging",parent,err)}
 assertPublicationBytes(t,source,"outside evidence")
}
