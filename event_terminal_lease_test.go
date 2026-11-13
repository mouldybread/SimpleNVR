package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestEventTerminalRetentionBorrowsTransferredLeaseAfterCallerCancel(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 var lease *mediaMutationLease
 if err:=bm.publishEventRecordingWith(ctx,bm.ctx,job,"temporary","final",false,func(_ context.Context,owned *mediaMutationLease,_,_ string)error{lease=owned;return nil});err!=nil {t.Fatal(err)}
 inventory:=job.inventory
 pin:=inventory.entries[ownedCheckpointName].file
 if !lease.held() || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal {t.Fatal("handoff absent")}
 cancel() // emulate concatenateEventRecording's deferred finalization cancel
 original:=job.recoveryWriter
 called:=false
 job.recoveryWriter=func(dir string,data []byte)error{
  called=true
  borrowed:=job.recoveryBorrow
  if borrowed==nil || borrowed.lease!=lease || !borrowed.terminal || !lease.held() || borrowed.ctx.Err()!=nil || len(bm.slots)!=1 {return errors.New("terminal checkpoint did not keep same lease/fresh budget/capacity")}
  parent,stop:=context.WithCancel(context.Background());defer stop()
  waiting:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
  result:=make(chan error,1)
  go func(){other,err:=recordingMediaOwnership.acquire(waiting);if err==nil {other.release()};result<-err}()
  awaitShutdownStep(t,waiting.waiting,"contender did not wait during terminal checkpoint");stop()
  select {case err:=<-result:if !errors.Is(err,context.Canceled) {return errors.New("contender entered before terminal retention")};case <-time.After(3*time.Second):return errors.New("terminal contender did not join")}
  return original(dir,data)
 }
 bm.releaseJob(job)
 if !called || job.recoveryFailed || !job.keepStaging || job.recoveryBorrow!=nil || job.inventory!=nil || lease.held() || len(bm.slots)!=0 {t.Fatal("terminal ownership/capacity leak")}
 assertJobInventoryClosed(t,inventory)
 if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("checkpoint pin leaked",err)}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" || m.Revision!=4 {t.Fatal(m)}
 freshCtx,stop:=context.WithTimeout(context.Background(),time.Second);defer stop()
 next,err:=recordingMediaOwnership.acquire(freshCtx);if err!=nil {t.Fatal("terminal lease not returned",err)};next.release()
}

func TestEventTerminalCheckpointFailureStillClosesAndReturnsLease(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 if err:=bm.publishEventRecordingWith(context.Background(),bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{return nil});err!=nil {t.Fatal(err)}
 inventory:=job.inventory;lease:=job.recoveryBorrow.lease
 before,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil {t.Fatal(err)}
 job.recoveryWriter=func(string,[]byte)error{return os.ErrPermission}
 bm.releaseJob(job)
 if !job.recoveryFailed || !job.keepStaging || job.recoveryBorrow!=nil || job.inventory!=nil || lease.held() || len(bm.slots)!=0 {t.Fatal("failed terminal checkpoint leaked ownership")}
 assertJobInventoryClosed(t,inventory)
 assertPublicationBytes(t,filepath.Join(job.dir,ownedCheckpointName),string(before))
}

func TestEventConcatHandsLeaseToReleaseAfterFinalizationContextEnds(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 input:=filepath.Join(job.dir,"event.mp4")
 if err:=os.WriteFile(input,[]byte("original capture bytes"),0600);err!=nil {t.Fatal(err)}
 final:=bm.concatenateEventRecording(job,input)
 if final=="" || job.recoveryBorrow==nil || !job.recoveryBorrow.terminal {t.Fatal("production concat did not transfer lease")}
 borrowed:=job.recoveryBorrow
 if !errors.Is(borrowed.ctx.Err(),context.Canceled) || !borrowed.lease.held() {t.Fatal("caller cancellation incorrectly ended ownership")}
 before:=readRecoveryTest(t,job.dir)
 bm.cancel()
 bm.releaseJob(job)
 if job.recoveryFailed || !job.keepStaging || borrowed.terminal || borrowed.lease.held() || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("production terminal handoff failed")}
 after:=readRecoveryTest(t,job.dir)
 if before.State!="published" || after.State!=before.State || after.Revision!=before.Revision+1 {t.Fatal("terminal checkpoint state changed",after)}
 assertPublicationBytes(t,input,"original capture bytes")
 assertPublicationBytes(t,final,"synthetic output")
 if _,err:=os.Stat(filepath.Join(job.dir,"concat.txt"));err!=nil {t.Fatal("concat evidence lost",err)}
}

func TestEventTerminalLeaseRefusesMediaReentryBeforeAdmission(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 if err:=bm.publishEventRecordingWith(context.Background(),bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{return nil});err!=nil {t.Fatal(err)}
 before,err:=os.ReadFile(filepath.Join(job.dir,ownedCheckpointName));if err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Second);defer cancel()
 if err:=bm.captureEventFile(ctx,job,filepath.Join(job.dir,"event.mp4"));!errors.Is(err,errPublicationOwnership) {t.Fatal("capture reentry not refused before gate wait",err)}
 if err:=job.writeEventConcat(ctx,filepath.Join(job.dir,"event.mp4"));!errors.Is(err,errPublicationOwnership) {t.Fatal("concat reentry not refused before gate wait",err)}
 called:=false
 if err:=job.withOwnedStagingCreator(ctx,"event.mp4",func(*os.File)error{called=true;return nil});!errors.Is(err,errPublicationOwnership) || called {t.Fatal("creator reentry executed",err)}
 for _,name:=range []string{"event.mp4","concat.txt"} {if _,err:=os.Lstat(filepath.Join(job.dir,name));!os.IsNotExist(err) {t.Fatal("media reentry created evidence",err)}}
 assertPublicationBytes(t,filepath.Join(job.dir,ownedCheckpointName),string(before))
 if !job.recoveryBorrow.lease.held() {t.Fatal("reentry released the terminal owner's lease")}
}
