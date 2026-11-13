package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func publicationCheckpointJob(t *testing.T) (*BufferManager,*bufferEvent) {
 t.Helper();bm:=bufferFixture(t)
 if err:=bm.TriggerEvent(bufferTestEvent("finalization"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan
 t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
 return bm,job
}
func TestEventPublicationCheckpointBorrowsSameLease(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 var publicationLease *mediaMutationLease
 publish:=func(ctx context.Context,lease *mediaMutationLease,from,to string)error{
  publicationLease=lease
  if !lease.held() {return errPublicationOwnership};return nil
 }
 original:=job.recoveryWriter
 job.recoveryWriter=func(dir string,data []byte)error{
  if publicationLease==nil || !publicationLease.held() || job.recoveryBorrow==nil || job.recoveryBorrow.lease!=publicationLease {return errors.New("lease ended or changed before checkpoint")}
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  waitCtx:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
  result:=make(chan error,1)
  go func(){lease,err:=recordingMediaOwnership.acquire(waitCtx);if err==nil {lease.release()};result<-err}()
  awaitShutdownStep(t,waitCtx.waiting,"contender did not enter ownership wait");cancel()
  select {case err:=<-result:if !errors.Is(err,context.Canceled) {return errors.New("contender entered during published checkpoint")};case <-time.After(3*time.Second):return errors.New("contender did not join")}
  return original(dir,data) // must borrow, never recursively acquire
 }
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 if err:=bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",false,publish);err!=nil {t.Fatal(err)}
 if job.recoveryFailed || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal || !publicationLease.held() {t.Fatal("checkpoint failed or lease handoff missing")}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal(m)}
 // Restore the writer before terminal retention; it must borrow the handed-off
 // lease, then close inventory and release ownership exactly once.
 job.recoveryWriter=original
 bm.releaseJob(job)
 if job.recoveryBorrow!=nil || publicationLease.held() || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("terminal handoff leaked")}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" || m.Revision!=4 {t.Fatal(m)}
}

func TestEventPublicationCheckpointProductionDurablePath(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 temporary:=filepath.Join(bm.camera.RecordDir,"published.mp4.tmp");final:=filepath.Join(bm.camera.RecordDir,"published.mp4")
 if err:=os.WriteFile(temporary,[]byte("synthetic published bytes"),0600);err!=nil {t.Fatal(err)}
 if err:=bm.publishEventRecording(context.Background(),bm.ctx,job,temporary,final,false);err!=nil {t.Fatal(err)}
 assertPublicationBytes(t,final,"synthetic published bytes")
 if _,err:=os.Stat(temporary);!os.IsNotExist(err) {t.Fatal("no-replace publication did not move candidate",err)}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal(m)}
 if job.recoveryFailed || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal || !job.recoveryBorrow.lease.held() {t.Fatal("published checkpoint/handoff failed")}
}

func TestEventPublicationCheckpointFailureRetainsPublishedOutcome(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 before,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
 injected:=errors.New("checkpoint failed")
 job.recoveryWriter=func(string,[]byte)error{return injected}
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 if err:=bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{return nil});err!=nil {t.Fatal("successful publication was erased",err)}
 if !job.keepStaging || !job.recoveryFailed || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal {t.Fatal("checkpoint failure did not retain/transfer ownership")}
 assertPublicationBytes(t,filepath.Join(job.dir,"recovery.json"),string(before))
}

func TestEventPublicationFailureClearsBorrowAndSkipsPublishedCheckpoint(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 before:=readRecoveryTest(t,job.dir);injected:=errors.New("post-rename publication ambiguity")
 if err:=bm.publishEventRecordingWith(context.Background(),bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{return injected});!errors.Is(err,injected) {t.Fatal(err)}
 if job.recoveryBorrow!=nil || !job.keepStaging {t.Fatal("failed finalization leaked borrow or retention")}
 after:=readRecoveryTest(t,job.dir)
 if before.Revision!=after.Revision || before.State!=after.State {t.Fatal("failed publication claimed published checkpoint")}
 if err:=job.recoveryCheckpoint("publication_failed","validation_or_publication_failed");err!=nil {t.Fatal("failure checkpoint reacquisition failed",err)}
}

func TestEventPublicationPartialAndLateCancellationRetain(t *testing.T) {
 for _,partial:=range []bool{false,true} {t.Run(map[bool]string{false:"canceled capture",true:"partial"}[partial],func(t *testing.T){
  bm,job:=publicationCheckpointJob(t)
  original:=job.recoveryWriter
  job.recoveryWriter=func(dir string,data []byte)error{if !partial {bm.cancel()};return original(dir,data)}
  ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
  if err:=bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",partial,func(context.Context,*mediaMutationLease,string,string)error{return nil});err!=nil {t.Fatal(err)}
  if !job.keepStaging || job.recoveryFailed || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal {t.Fatal("cancellation/partial checkpoint semantics changed")}
  if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal("independent metadata budget lost",m)}
  job.recoveryWriter=original
 })}
}

func TestEventPublicationCanceledAdmissionDoesNotPublish(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 before,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
 owner:=publishedEngineLease(t);defer owner.release()
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 ctx:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
 result:=make(chan error,1);called:=false
 go func(){result<-bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{called=true;return nil})}()
 awaitShutdownStep(t,ctx.waiting,"finalizer did not wait");cancel()
 select {case err:=<-result:if !errors.Is(err,context.Canceled) {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("finalizer wait did not join")}
 if called || job.recoveryBorrow!=nil || !owner.held() {t.Fatal("canceled admission entered publication or disturbed owner")}
 assertPublicationBytes(t,filepath.Join(job.dir,"recovery.json"),string(before))
}

func TestEventPublicationExpiredBorrowRefusesCheckpoint(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 if err:=bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{cancel();return nil});err!=nil {t.Fatal(err)}
 if !job.recoveryFailed || !job.keepStaging || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal {t.Fatal("expired metadata budget ignored")}
 if m:=readRecoveryTest(t,job.dir);m.State!="pinned" {t.Fatal("expired budget wrote checkpoint",m)}
}
