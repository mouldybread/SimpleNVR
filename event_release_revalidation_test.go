package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "testing"
 "time"
)

// Synthetic registry/process witnesses establish necessary checks, NOT full
// camera capture. No release-engine invocation or deletion permission is tested.
func releaseRevalidationFixture(t *testing.T) (*NVR,*BufferManager,*bufferEvent,*eventReleaseRevalidation,*mediaMutationLease) {
 t.Helper();installEventFailureTools(t,0,false);bm,job:=publicationCheckpointJob(t)
 input:=filepath.Join(job.dir,"event.mp4")
 if err:=job.withOwnedStagingCreator(context.Background(),"event.mp4",func(f *os.File)error{_,err:=f.WriteString("capture bytes");return err});err!=nil {t.Fatal(err)}
 if err:=job.writeEventConcat(context.Background(),input);err!=nil {t.Fatal(err)}
 final:=filepath.Join(bm.camera.RecordDir,"output.mp4");temporary:=final+".tmp"
 if err:=os.WriteFile(temporary,[]byte("published bytes"),0600);err!=nil {t.Fatal(err)}
 job.recovery.OutputTemporary=filepath.Base(temporary);job.recovery.OutputPublished=filepath.Base(final)
 if err:=bm.publishEventRecording(context.Background(),bm.ctx,job,temporary,final,false);err!=nil {t.Fatal(err)}
 if job.recoveryBorrow.revalidation==nil || job.recoveryBorrow.revalidationErr==nil {t.Fatal("unregistered fixture bypassed production revalidation")}
 n:=&NVR{cameras:map[string]*Camera{bm.camera.ID:bm.camera}}
 bm.camera.IsRecording=true;bm.camera.RecordingMode=ModeEventBased;bm.camera.bufferManager=bm;bm.camera.recordCancel=func(){}
 check:=newEventReleaseRevalidation(n,bm,job,false)
 job.recoveryBorrow.revalidation=check
 job.recoveryBorrow.revalidationErr=check.check(context.Background(),job.recoveryBorrow.lease)
 if job.recoveryBorrow.revalidationErr!=nil {t.Fatal(job.recoveryBorrow.revalidationErr)}
 return n,bm,job,check,job.recoveryBorrow.lease
}
func TestReleaseRevalidationAllowsOwnedCheckpointReplacementButNotCleanup(t *testing.T) {
 _,bm,job,check,lease:=releaseRevalidationFixture(t)
 if err:=job.recoveryCheckpoint("retained","");err!=nil {t.Fatal(err)}
 if err:=check.check(context.Background(),lease);err!=nil {t.Fatal(err)}
 bm.releaseJob(job)
 if !job.keepStaging || job.recoveryBorrow!=nil || lease.held() || len(bm.slots)!=0 {t.Fatal("revalidation changed retention/teardown")}
 if _,err:=os.Stat(job.dir);err!=nil {t.Fatal("necessary checks enabled cleanup",err)}
 if err:=check.check(context.Background(),lease);err==nil {t.Fatal("closed job accepted")}
}
func TestReleaseRevalidationRefusesChangedRegistryBeforeIO(t *testing.T) {
 for _,change:=range []string{"missing","replacement","path","mode","inactive","manager","cancel handle","shutdown"} {t.Run(change,func(t *testing.T){
  n,bm,job,check,lease:=releaseRevalidationFixture(t)
  n.mu.Lock()
  switch change {case "missing":delete(n.cameras,bm.camera.ID);case "replacement":n.cameras[bm.camera.ID]=&Camera{ID:bm.camera.ID};case "path":bm.camera.RecordDir+="-changed";case "mode":bm.camera.RecordingMode=ModeContinuous;case "inactive":bm.camera.IsRecording=false;case "manager":bm.camera.bufferManager=nil;case "cancel handle":bm.camera.recordCancel=nil;case "shutdown":n.shuttingDown.Store(true)}
  n.mu.Unlock()
  calls:=0
  if err:=check.checkWithOutput(context.Background(),lease,func(context.Context,*mediaMutationLease)error{calls++;return nil});err==nil || calls!=0 {t.Fatal("stale registry reached I/O",err,calls)}
  assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"capture bytes")
 })}
}
func TestReleaseRevalidationRefusesChangedEvidenceWithoutMutation(t *testing.T) {
 for _,change:=range []string{"unpersisted checkpoint","partial","unknown","missing input","replaced output"} {t.Run(change,func(t *testing.T){
  _,_,job,check,lease:=releaseRevalidationFixture(t)
  switch change {
  case "unpersisted checkpoint":job.recovery.Revision++
  case "partial":job.recovery.Partial=true
  case "unknown":putStagingAdmissionFile(t,filepath.Join(job.dir,"foreign"),"foreign bytes")
  case "missing input":if err:=os.Rename(filepath.Join(job.dir,"event.mp4"),filepath.Join(job.dir,"saved"));err!=nil {t.Fatal(err)}
  case "replaced output":path:=check.output.path;if err:=os.Rename(path,path+"-saved");err!=nil {t.Fatal(err)};putStagingAdmissionFile(t,path,"replacement bytes")
  }
  job.keepStaging=false
  before:=eventStorageTree(t,job.dir)
  if err:=check.check(context.Background(),lease);err==nil {t.Fatal("changed evidence accepted",change)}
  if !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal("read-only check changed evidence")}
 })}
}
func TestReleaseRevalidationRequiresContextLeaseAndHandoffObservation(t *testing.T) {
 n,bm,job,check,lease:=releaseRevalidationFixture(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=check.check(ctx,lease);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if err:=check.check(context.Background(),nil);err==nil {t.Fatal("nil lease accepted")}
 job.keepStaging=false
 blocked:=newEventReleaseRevalidation(n,bm,job,true)
 if err:=blocked.check(context.Background(),lease);err==nil {t.Fatal("failed handoff observation overridden by false flag")}
 bm.cancel()
 if err:=check.check(context.Background(),lease);!errors.Is(err,context.Canceled) {t.Fatal(err)}
}
func TestReleaseRevalidationRechecksRegistryAfterIOWithoutHoldingLock(t *testing.T) {
 n,_,_,check,lease:=releaseRevalidationFixture(t)
 done:=make(chan struct{})
 err:=check.checkWithOutput(context.Background(),lease,func(ctx context.Context,lease *mediaMutationLease)error{
  go func(){n.mu.RLock();_=len(n.cameras);n.mu.RUnlock();n.mu.Lock();delete(n.cameras,check.id);n.mu.Unlock();close(done)}()
  select {case <-done:case <-time.After(3*time.Second):return context.DeadlineExceeded}
  return check.output.check(ctx,lease)
 })
 select {case <-done:case <-time.After(3*time.Second):t.Fatal("registry reader/writer did not join")}
 if !errors.Is(err,errEventReleaseRevalidation) {t.Fatal("stale registry accepted or registry lock held across I/O",err)}
}
func TestReleaseRevalidationRunsAgainAfterTerminalCheckpoint(t *testing.T) {
 n,bm,job,check,lease:=releaseRevalidationFixture(t)
 borrowed:=job.recoveryBorrow;original:=job.recoveryWriter
 job.recoveryWriter=func(dir string,data []byte)error{
  if borrowed.revalidationErr!=nil {return borrowed.revalidationErr}
  n.shuttingDown.Store(true)
  return original(dir,data)
 }
 bm.releaseJob(job)
 if !errors.Is(borrowed.revalidationErr,errEventReleaseRevalidation) || job.recoveryFailed || !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("terminal revalidation/metadata/teardown changed")}
 if err:=check.check(context.Background(),lease);err==nil {t.Fatal("terminal job reusable")}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal(m)}
}
