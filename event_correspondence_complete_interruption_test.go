package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

// Owned re-exec child, no shell or descendants. Readiness is outside job storage.
func TestCorrCompleteBlockingChild(t *testing.T) {
 if os.Getenv("CORR_COMPLETE_BLOCKING_CHILD")!="1" {return}
 if err:=os.WriteFile(os.Getenv("CORR_COMPLETE_READY"),[]byte("ready"),0600);err!=nil {os.Exit(91)}
 time.Sleep(20*time.Second)
 os.Exit(92)
}
func corrCompleteBlockingCommand(t *testing.T,ctx context.Context,marker string)*exec.Cmd {
 t.Helper();binary,err:=os.Executable();if err!=nil {t.Fatal(err)}
 cmd:=exec.CommandContext(ctx,binary,"-test.run=^TestCorrCompleteBlockingChild$")
 cmd.Env=append(os.Environ(),"GORACE=atexit_sleep_ms=0","CORR_COMPLETE_BLOCKING_CHILD=1","CORR_COMPLETE_READY="+marker)
 cmd.WaitDelay=2*time.Second
 return cmd
}

// Covers owned blocking evidence work and both comparator children, for jobs
// with and without a selected prefix. The evidence subprocess is a controlled
// probe substitute: it does not claim native FFprobe cancellation qualification.
func TestCorrCompleteValidationInterruptedStages(t *testing.T) {
 for _,prefix:=range []bool{false,true} {for _,stage:=range []string{"evidence","comparison"} {for _,cause:=range []string{"cancel","deadline"} {
  t.Run(fmt.Sprintf("prefix=%t/%s/%s",prefix,stage,cause),func(t *testing.T){
   bm,job,lease:=corrSafetyFixture(t,prefix)
   inventory:=job.inventory;released:=false
   t.Cleanup(func(){if !released {bm.releaseJob(job)}})
   original:=job.recoveryBorrow.duration;borrowed:=job.recoveryBorrow
   tree:=eventStorageTree(t,job.dir)
   donors:=[]*os.File{original.capture,original.output.file}
   for _,input:=range job.execution.selection.inputs {donors=append(donors,input.file)}
   digests:=make([]string,len(donors))
   for i,file:=range donors {var err error;digests[i],err=corrBoundDigest(file);if err!=nil {t.Fatal(err)}}
   budget:=2*time.Second;if cause=="cancel" {budget=5*time.Second}
   ctx,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
   readyDir:=t.TempDir();markers:=[]string{filepath.Join(readyDir,"probe")}
   if stage=="comparison" {markers=[]string{filepath.Join(readyDir,"reference"),filepath.Join(readyDir,"actual")}}
   gate:=newCorrAdmissionGate();observed:=make(chan error,1)
   // This observer never calls testing methods, and is joined below.
   go func(){
    tick:=time.NewTicker(5*time.Millisecond);defer tick.Stop()
    for {
     ready:=true
     for _,marker:=range markers {if _,err:=os.Stat(marker);err!=nil {ready=false}}
     if ready {
      denied,err:=gate.inspectComplete(context.Background(),nil,nil,nil,nil,nil)
      if !errors.Is(err,errCorrAdmissionBusy) || denied.EvidenceCompleted || denied.Collection.ChildrenStarted!=0 || len(gate.slot)!=1 {
       observed<-fmt.Errorf("admission did not cover blocked %s: %+v %v",stage,denied,err);cancel();return
      }
      observed<-nil
      if cause=="cancel" {cancel()}
      return
     }
     select {case <-ctx.Done():observed<-fmt.Errorf("%s never reached ready blocked stage: %w",stage,ctx.Err());return;case <-tick.C:}
    }
   }()
   var probe *exec.Cmd
   collect:=func(child context.Context,l *mediaMutationLease,b *BufferManager,j *bufferEvent)(*eventDurationEvidence,error){
    if l!=lease || b!=bm || j!=job {t.Fatal("collection binding changed")}
    if stage=="comparison" {return original,nil}
    probe=corrCompleteBlockingCommand(t,child,markers[0])
    err:=probe.Run()
    return nil,errors.Join(err,child.Err())
   }
   calls:=0
   factory:=func(child context.Context,args []string)*exec.Cmd{
    if stage!="comparison" || calls>=len(markers) {t.Fatal("unexpected comparator launch")}
    marker:=markers[calls];calls++
    return corrCompleteBlockingCommand(t,child,marker)
   }
   report,err:=gate.inspectComplete(ctx,lease,bm,job,collect,factory)
   cancel()
   if observationErr:=<-observed;observationErr!=nil {t.Fatal(observationErr)}
   var want error=context.Canceled;if cause=="deadline" {want=context.DeadlineExceeded}
   if !errors.Is(err,want) {t.Fatal("wrong interruption cause",report,err,want)}
   corrStreamAssertJoined(t,report.Collection)
   if stage=="evidence" {
    if report.EvidenceCompleted || report.Collection.ChildrenStarted!=0 || calls!=0 || probe==nil || probe.ProcessState==nil {t.Fatal("evidence refusal launched comparison or left probe unjoined",report)}
    var status unix.WaitStatus
    if _,err:=unix.Wait4(probe.Process.Pid,&status,unix.WNOHANG,nil);!errors.Is(err,unix.ECHILD) {t.Fatal("controlled probe not reaped",err)}
   } else if !report.EvidenceCompleted || report.Collection.ChildrenStarted!=2 || calls!=2 {t.Fatal("comparison interrupted before both children were active",report)}
   if len(gate.slot)!=0 || !lease.held() || borrowed!=job.recoveryBorrow || borrowed.duration!=original || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("interruption changed evidence/owner or retained admission")}
   for i,file:=range donors {digest,err:=corrBoundDigest(file);if err!=nil || digest!=digests[i] {t.Fatal("interruption changed donor",i,err)}}
   good:=func(context.Context,*mediaMutationLease,*BufferManager,*bufferEvent)(*eventDurationEvidence,error){return original,nil}
   retried,retryErr:=gate.inspectComplete(context.Background(),lease,bm,job,good,corrStreamChildFactory(t,[]string{"normal","normal"},false))
   corrStreamAssertJoined(t,retried.Collection)
   if retryErr!=nil || !retried.EvidenceCompleted || !retried.Collection.Video.Equal || !retried.Collection.Audio.Equal || len(gate.slot)!=0 {t.Fatal("interruption poisoned reuse",retried,retryErr)}
   for i,file:=range donors {digest,err:=corrBoundDigest(file);if err!=nil || digest!=digests[i] {t.Fatal("reuse changed donor",i,err)}}
   if !lease.held() || job.recoveryBorrow!=borrowed || borrowed.duration!=original || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("reuse changed original evidence")}
   bm.releaseJob(job);released=true;assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("interrupted job terminal invariant")}
  })
 }}}
}
