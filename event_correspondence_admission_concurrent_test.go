package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "reflect"
 "sync"
 "testing"
 "time"
)

// Unlike nested hook calls, these contenders execute on separate goroutines.
// Only the admitted owner touches the fixture's mutable evidence or media lease.
func TestCorrAdmissionConcurrentContentionMatrix(t *testing.T) {
 const contenders=16
 type result struct {report corrStreamReport;err error}
 for _,prefix:=range []bool{false,true} {for phase:=0;phase<6;phase++ {for _,cancelOwner:=range []bool{false,true} {
  t.Run(fmt.Sprintf("prefix=%t/phase=%d/cancel=%t",prefix,phase,cancelOwner),func(t *testing.T){
   bm,job,lease:=corrSafetyFixture(t,prefix)
   inventory:=job.inventory
   defer func(){
    bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
    if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Error("terminal owner invariant")}
   }()
   evidence:=job.recoveryBorrow.duration
   tree:=eventStorageTree(t,job.dir)
   published,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
   defer cancel()
   reached:=make(chan struct{})
   release:=make(chan struct{})
   completed:=make(chan result,1)
   var unblock sync.Once
   open:=func(){unblock.Do(func(){close(release)})}
   factory:=corrStreamChildFactory(t,[]string{"normal","normal"},false)
   go func(){
    hook:=func(boundary int){
     if boundary!=phase {return}
     close(reached)
     select {case <-release:case <-ctx.Done():}
    }
    report,err:=gate.inspect(ctx,lease,evidence,factory,hook)
    completed<-result{report,err}
   }()
   joined:=false
   // Ensure failure in the test owner also unblocks and joins the admitted call
   // before the fixture's deferred terminal teardown releases any descriptors.
   defer func(){cancel();open();if !joined {r:=<-completed;corrStreamAssertJoined(t,r.report)}}()
   timer:=time.NewTimer(8*time.Second);defer timer.Stop()
   select {
   case <-reached:
   case r:=<-completed:joined=true;corrStreamAssertJoined(t,r.report);t.Fatal("owner ended before target boundary",r.err)
   case <-timer.C:t.Fatal("owner did not reach target boundary")
   }
   start:=make(chan struct{})
   denied:=make(chan result,contenders)
   var workers sync.WaitGroup
   for i:=0;i<contenders;i++ {
    workers.Add(1)
    go func(){
     defer workers.Done();<-start
     // Nil evidence deliberately proves busy refusal precedes binding I/O.
     report,err:=gate.inspect(context.Background(),nil,nil,nil,nil)
     denied<-result{report,err}
    }()
   }
   close(start);workers.Wait();close(denied)
   for r:=range denied {
    corrStreamAssertJoined(t,r.report)
    if !errors.Is(r.err,errCorrAdmissionBusy) || r.report.ChildrenStarted!=0 || r.report.PipeDescriptorsCreated!=0 {t.Fatal("contender entered owner path",r.report,r.err)}
   }
   if cancelOwner {cancel()}
   open()
   r:=<-completed;joined=true
   corrStreamAssertJoined(t,r.report)
   if cancelOwner {
    children:=2;if phase<2 {children=phase}
    // Diagnostic detail only: the failing assertion is unchanged.
    if !errors.Is(r.err,context.Canceled) || r.report.ChildrenStarted!=children {t.Fatalf("owner cancellation: phase=%d prefix=%t returnedErr=%v errIsCanceled=%t ctxErrAfterReturn=%v childrenStarted=%d wantChildren=%d childrenJoined=%d elapsed=%s report=%+v",phase,prefix,r.err,errors.Is(r.err,context.Canceled),ctx.Err(),r.report.ChildrenStarted,children,r.report.ChildrenJoined,r.report.Elapsed,r.report)}
   } else if r.err!=nil || !r.report.Video.Equal || !r.report.Audio.Equal {t.Fatal("owner success",r.report,r.err)}
   if len(gate.slot)!=0 {t.Fatal("owner left admission occupied")}
   retry,retryErr:=gate.inspect(context.Background(),lease,evidence,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil)
   corrStreamAssertJoined(t,retry)
   if retryErr!=nil || retry.ChildrenStarted!=2 || !retry.Video.Equal || !retry.Audio.Equal || len(gate.slot)!=0 {t.Fatal("contention poisoned reuse",retry,retryErr)}
   if !lease.held() || job.recoveryBorrow.duration!=evidence || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("contention changed caller evidence")}
   assertPublicationBytes(t,evidence.output.path,string(published))
  })
 }}}
}
