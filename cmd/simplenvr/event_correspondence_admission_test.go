package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "reflect"
 "testing"
 "time"
)

// Candidate policy only. No production caller, global semaphore or receipt.
const corrAdmissionBudget=5*time.Second
var errCorrAdmissionBusy=errors.New("experimental correspondence admission busy")

type corrAdmissionGate struct {slot chan struct{}}
func newCorrAdmissionGate()*corrAdmissionGate {
 return &corrAdmissionGate{slot:make(chan struct{},1)}
}

// One shared gate admits one entire adapter invocation, not one child at a
// time. It never queues while holding a media lease. The slot remains held
// until corrStreamInspect has joined all managed children and workers.
func (g *corrAdmissionGate) inspect(parent context.Context,lease *mediaMutationLease,e *eventDurationEvidence,factory corrStreamFactory,hook func(int))(corrStreamReport,error) {
 ctx,cancel:=context.WithTimeout(parent,corrAdmissionBudget);defer cancel()
 if err:=ctx.Err();err!=nil {return corrStreamReport{},err}
 if g==nil || g.slot==nil || cap(g.slot)!=1 {return corrStreamReport{},errCorrPrototype}
 select {
 case g.slot<-struct{}{}:
  defer func(){<-g.slot}()
 default:
  return corrStreamReport{},errCorrAdmissionBusy
 }
 if err:=ctx.Err();err!=nil {return corrStreamReport{},err}
 // Unlike the raw experimental collector, this outer deadline includes the
 // adapter's initial binding check as well as collection and revalidation.
 return corrStreamInspect(ctx,lease,e,factory,hook)
}

func TestCorrAdmissionBoundAdapterDeadlines(t *testing.T) {
 for _,prefix:=range []bool{false,true} {for _,shortParent:=range []bool{false,true} {t.Run(fmt.Sprintf("prefix=%t/short-parent=%t",prefix,shortParent),func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,prefix);defer bm.releaseJob(job)
  e:=job.recoveryBorrow.duration;tree:=eventStorageTree(t,job.dir)
  output,err:=os.ReadFile(e.output.path);if err!=nil {t.Fatal(err)}
  budget:=10*time.Second;if shortParent {budget=2*time.Second}
  parent,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
  parentDeadline,_:=parent.Deadline();before:=time.Now()
  gate:=newCorrAdmissionGate()
  base:=corrStreamChildFactory(t,[]string{"normal","normal"},false)
  calls:=0;var first context.Context
  factory:=func(child context.Context,args []string)*exec.Cmd{
   calls++;deadline,ok:=child.Deadline()
   if !ok {t.Fatal("missing adapter deadline")}
   if shortParent {
    if !deadline.Equal(parentDeadline) {t.Fatal("shorter parent deadline changed",deadline,parentDeadline)}
   } else if deadline.Before(before.Add(corrAdmissionBudget)) || deadline.After(time.Now().Add(corrAdmissionBudget)) {
    t.Fatal("adapter did not receive candidate aggregate deadline",deadline)
   }
   if first==nil {first=child} else if first!=child {t.Fatal("per-child context renewed")}
   return base(child,args)
  }
  boundaries:=0
  forbidden:=func(context.Context,[]string)*exec.Cmd{t.Fatal("busy admission launched a child");return nil}
  hook:=func(phase int){
   boundaries++
   if phase!=2 && phase!=5 {return}
   // Check while children exist, and again after both have joined but
   // before the outer invocation returns. No second binding I/O is allowed.
   refused,err:=gate.inspect(context.Background(),nil,nil,forbidden,nil)
   if !errors.Is(err,errCorrAdmissionBusy) || refused.ChildrenStarted!=0 || refused.PipeDescriptorsCreated!=0 {t.Fatal("overlap admitted",phase,refused,err)}
  }
  report,err:=gate.inspect(parent,lease,e,factory,hook)
  corrStreamAssertJoined(t,report)
  if err!=nil || calls!=2 || boundaries!=6 || !report.Video.Equal || !report.Audio.Equal || len(gate.slot)!=0 {t.Fatal(report,err,calls,boundaries)}
  if !lease.held() || job.recoveryBorrow.duration!=e || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("admission altered ownership or evidence")}
  assertPublicationBytes(t,e.output.path,string(output))
 })}}
}

func TestCorrAdmissionRefusalReleasesSlot(t *testing.T) {
 for _,kind:=range []string{"pre-cancelled","absent binding","partial launch","mismatch","cancel after launch","aggregate deadline"} {t.Run(kind,func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,false);defer bm.releaseJob(job)
  e:=job.recoveryBorrow.duration;tree:=eventStorageTree(t,job.dir)
  output,err:=os.ReadFile(e.output.path);if err!=nil {t.Fatal(err)}
  gate:=newCorrAdmissionGate()
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  evidence:=e;modes:=[]string{"normal","normal"};failSecond:=false
  var want error;wantChildren:=2
  switch kind {
  case "pre-cancelled":cancel();want=context.Canceled;wantChildren=0
  case "absent binding":evidence=nil;wantChildren=0
  case "partial launch":failSecond=true;wantChildren=1
  case "mismatch":modes[1]="video mismatch";want=errCorrStreamMismatch
  case "cancel after launch":want=context.Canceled
  case "aggregate deadline":modes=[]string{"stall","stall"};want=context.DeadlineExceeded
  }
  factory:=corrStreamChildFactory(t,modes,failSecond)
  hook:=func(phase int){if kind=="cancel after launch" && phase==2 {cancel()}}
  report,err:=gate.inspect(parent,lease,evidence,factory,hook)
  corrStreamAssertJoined(t,report)
  if err==nil || (want!=nil && !errors.Is(err,want)) || report.ChildrenStarted!=wantChildren || len(gate.slot)!=0 {t.Fatal(kind,report,err)}
  if wantChildren==0 && report.PipeDescriptorsCreated!=0 {t.Fatal("refused admission allocated pipes",report)}
  if !lease.held() || job.recoveryBorrow.duration!=e || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("refusal altered ownership or evidence")}
  assertPublicationBytes(t,e.output.path,string(output))
  // A failed invocation must not poison admission or the genuine witnesses.
  retried,retryErr:=gate.inspect(context.Background(),lease,e,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil)
  corrStreamAssertJoined(t,retried)
  if retryErr!=nil || !retried.Video.Equal || !retried.Audio.Equal || len(gate.slot)!=0 || !lease.held() {t.Fatal("slot or evidence not reusable",retried,retryErr)}
 })}
}

func TestCorrAdmissionAbsentGateDoesNotLaunch(t *testing.T) {
 forbidden:=func(context.Context,[]string)*exec.Cmd{t.Fatal("invalid gate launched");return nil}
 for _,gate:=range []*corrAdmissionGate{nil,{}, {slot:make(chan struct{},2)}} {
  report,err:=gate.inspect(context.Background(),nil,nil,forbidden,nil)
  if err==nil || report.ChildrenStarted!=0 || report.PipeDescriptorsCreated!=0 {t.Fatal(report,err)}
 }
}
