package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "reflect"
 "testing"
 "time"
)

// Exercise faults through the outer admission owner, not just corrStreamRun.
// Every refusal must finish teardown before the same gate accepts another call.
func TestCorrAdmissionBatchChildFailureRecovery(t *testing.T) {
 cases:=[]struct{name,left,right string;want error;partial bool}{
  {"video mismatch","normal","video mismatch",errCorrStreamMismatch,false},
  {"audio mismatch","normal","audio mismatch",errCorrStreamMismatch,false},
  {"late mismatch","late reference","late mismatch",errCorrStreamMismatch,false},
  {"trailing bytes","normal","extra",errCorrStreamMismatch,false},
  {"empty outputs","empty","empty",errCorrStreamEmpty,false},
  {"stderr","normal","diagnostic",errCorrPrototype,false},
  {"stderr ceiling","normal","diagnostic ceiling",errCorrPrototype,false},
  {"nonzero after output","normal","nonzero",errCorrPrototype,false},
  {"stream ceiling","oversize","oversize",errCorrStreamLimit,false},
  {"partial launch","normal","normal",nil,true},
  {"stall before output","stall","stall",context.DeadlineExceeded,false},
  {"stall after EOF","late stall","late stall",context.DeadlineExceeded,false},
  {"opposed write order","cross video","cross audio",context.DeadlineExceeded,false},
 }
 for _,prefix:=range []bool{false,true} {for _,tc:=range cases {
  t.Run(fmt.Sprintf("prefix=%t/%s",prefix,tc.name),func(t *testing.T){
   bm,job,lease:=corrSafetyFixture(t,prefix);defer bm.releaseJob(job)
   evidence:=job.recoveryBorrow.duration
   tree:=eventStorageTree(t,job.dir)
   published,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   budget:=corrAdmissionBudget
   if errors.Is(tc.want,context.DeadlineExceeded) {budget=400*time.Millisecond}
   ctx,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
   report,err:=gate.inspect(ctx,lease,evidence,corrStreamChildFactory(t,[]string{tc.left,tc.right},tc.partial),nil)
   corrStreamAssertJoined(t,report)
   children:=2;if tc.partial {children=1}
   if err==nil || (tc.want!=nil && !errors.Is(err,tc.want)) || report.ChildrenStarted!=children || len(gate.slot)!=0 {t.Fatal("admitted fault or leaked slot",report,err)}
   for retry:=0;retry<3;retry++ {
    report,err=gate.inspect(context.Background(),lease,evidence,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil)
    corrStreamAssertJoined(t,report)
    if err!=nil || report.ChildrenStarted!=2 || !report.Video.Equal || !report.Audio.Equal || len(gate.slot)!=0 {t.Fatal("gate poisoned after refusal",retry,report,err)}
   }
   if !lease.held() || job.recoveryBorrow.duration!=evidence || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("failure/retry changed owner evidence")}
   assertPublicationBytes(t,evidence.output.path,string(published))
  })
 }}
}

func TestCorrAdmissionBatchCancellationBoundaries(t *testing.T) {
 for _,prefix:=range []bool{false,true} {for phase:=0;phase<6;phase++ {
  t.Run(fmt.Sprintf("prefix=%t/phase=%d",prefix,phase),func(t *testing.T){
   bm,job,lease:=corrSafetyFixture(t,prefix);defer bm.releaseJob(job)
   evidence:=job.recoveryBorrow.duration;tree:=eventStorageTree(t,job.dir)
   published,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   ctx,cancel:=context.WithCancel(context.Background());defer cancel()
   reached:=false
   hook:=func(boundary int){
    if boundary!=phase {return};reached=true
    // Busy refusal must precede binding and allocation at EVERY boundary,
    // including after collection joins and before final revalidation returns.
    denied,denyErr:=gate.inspect(context.Background(),nil,nil,nil,nil)
    if !errors.Is(denyErr,errCorrAdmissionBusy) || denied.ChildrenStarted!=0 || denied.PipeDescriptorsCreated!=0 {t.Fatal("overlap admitted",boundary,denied,denyErr)}
    cancel()
   }
   report,err:=gate.inspect(ctx,lease,evidence,corrStreamChildFactory(t,[]string{"normal","normal"},false),hook)
   corrStreamAssertJoined(t,report)
   children:=2;if phase<2 {children=phase}
   if !reached || !errors.Is(err,context.Canceled) || report.ChildrenStarted!=children || len(gate.slot)!=0 {t.Fatal("cancellation escaped outer owner",report,err,reached)}
   retry,retryErr:=gate.inspect(context.Background(),lease,evidence,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil)
   corrStreamAssertJoined(t,retry)
   if retryErr!=nil || !retry.Video.Equal || !retry.Audio.Equal || len(gate.slot)!=0 {t.Fatal("cancelled invocation poisoned gate",retry,retryErr)}
   if !lease.held() || job.recoveryBorrow.duration!=evidence || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("cancellation/retry changed owner evidence")}
   assertPublicationBytes(t,evidence.output.path,string(published))
  })
 }}
}
