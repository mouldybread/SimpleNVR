package main

import (
 "context"
 "errors"
 "os/exec"
 "reflect"
 "testing"
 "time"
)

// Experimental composition only: one slot and deadline cover fresh real evidence
// collection plus comparison. No production caller or stored receipt is added.
type corrCompleteValidationReport struct {
 EvidenceSeconds,ComparisonSeconds,TotalSeconds float64
 EvidenceCompleted bool
 Collection corrStreamReport
}
type corrEvidenceCollector func(context.Context,*mediaMutationLease,*BufferManager,*bufferEvent)(*eventDurationEvidence,error)
func (g *corrAdmissionGate) inspectComplete(parent context.Context,lease *mediaMutationLease,bm *BufferManager,job *bufferEvent,collect corrEvidenceCollector,factory corrStreamFactory)(report corrCompleteValidationReport,result error) {
 started:=time.Now();defer func(){report.TotalSeconds=time.Since(started).Seconds()}()
 ctx,cancel:=context.WithTimeout(parent,corrAdmissionBudget);defer cancel()
 if err:=ctx.Err();err!=nil {return report,err}
 if g==nil || g.slot==nil || cap(g.slot)!=1 {return report,errCorrPrototype}
 select {case g.slot<-struct{}{}:defer func(){<-g.slot}();default:return report,errCorrAdmissionBusy}
 if err:=ctx.Err();err!=nil {return report,err}
 if collect==nil {collect=inspectEventDurationPresentationEvidence}
 evidenceStarted:=time.Now()
 evidence,err:=collect(ctx,lease,bm,job)
 report.EvidenceSeconds=time.Since(evidenceStarted).Seconds()
 if err!=nil {return report,errors.Join(err,ctx.Err())}
 if err:=ctx.Err();err!=nil {return report,err}
 if evidence==nil || evidence.presentation==nil {return report,errEventVideoPresentation}
 report.EvidenceCompleted=true
 comparisonStarted:=time.Now()
 report.Collection,result=corrStreamInspect(ctx,lease,evidence,factory,nil)
 report.ComparisonSeconds=time.Since(comparisonStarted).Seconds()
 return report,errors.Join(result,ctx.Err())
}

func TestCorrCompleteValidationAdmissionAndSharedDeadline(t *testing.T) {
 for _,short:=range []bool{false,true} {t.Run(map[bool]string{false:"aggregate",true:"short-parent"}[short],func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,false);defer bm.releaseJob(job)
  original:=job.recoveryBorrow.duration;tree:=eventStorageTree(t,job.dir)
  budget:=10*time.Second;if short {budget=2*time.Second}
  ctx,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
  parentDeadline,_:=ctx.Deadline()
  gate:=newCorrAdmissionGate();calls:=0
  var shared context.Context
  collect:=func(child context.Context,l *mediaMutationLease,b *BufferManager,j *bufferEvent)(*eventDurationEvidence,error){
   calls++;shared=child
   if l!=lease || b!=bm || j!=job {t.Fatal("collector binding changed")}
   deadline,ok:=child.Deadline();if !ok {t.Fatal("missing evidence deadline")}
   if short && !deadline.Equal(parentDeadline) {t.Fatal("parent deadline extended")}
   denied,err:=gate.inspectComplete(context.Background(),nil,nil,nil,func(context.Context,*mediaMutationLease,*BufferManager,*bufferEvent)(*eventDurationEvidence,error){t.Fatal("busy call collected");return nil,nil},nil)
   if !errors.Is(err,errCorrAdmissionBusy) || denied.EvidenceCompleted || denied.Collection.ChildrenStarted!=0 {t.Fatal("slot did not cover evidence collection",denied,err)}
   // Genuine existing fixture receipt; this controlled seam tests context/slot
   // composition, not native probe cost. Native measurements use nil collector.
   return original,nil
  }
  base:=corrStreamChildFactory(t,[]string{"normal","normal"},false)
  factory:=func(child context.Context,args []string)*exec.Cmd{
   a,ok:=shared.Deadline();b,ok2:=child.Deadline()
   if !ok || !ok2 || !a.Equal(b) {t.Fatal("comparison renewed evidence deadline")}
   return base(child,args)
  }
  report,err:=gate.inspectComplete(ctx,lease,bm,job,collect,factory)
  corrStreamAssertJoined(t,report.Collection)
  if err!=nil || calls!=1 || !report.EvidenceCompleted || !report.Collection.Video.Equal || !report.Collection.Audio.Equal || len(gate.slot)!=0 {t.Fatal(report,err)}
  if !lease.held() || job.recoveryBorrow.duration!=original || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("composition changed stored evidence")}
 })}
}

func TestCorrCompleteValidationEvidenceFailureAndRetry(t *testing.T) {
 for _,kind:=range []string{"error","cancel","nil receipt","no presentation","pre-cancelled"} {t.Run(kind,func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,false);defer bm.releaseJob(job)
  original:=job.recoveryBorrow.duration;gate:=newCorrAdmissionGate()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  injected:=errors.New("injected collection refusal");calls:=0
  if kind=="pre-cancelled" {cancel()}
  collect:=func(context.Context,*mediaMutationLease,*BufferManager,*bufferEvent)(*eventDurationEvidence,error){
   calls++
   switch kind {
   case "cancel":cancel();return original,nil
   case "nil receipt":return nil,nil
   case "no presentation":copy:=*original;copy.presentation=nil;return &copy,nil
   default:return nil,injected
   }
  }
  forbidden:=func(context.Context,[]string)*exec.Cmd{t.Fatal("refused evidence launched comparator");return nil}
  report,err:=gate.inspectComplete(ctx,lease,bm,job,collect,forbidden)
  if err==nil || report.Collection.ChildrenStarted!=0 || report.Collection.PipeDescriptorsCreated!=0 || len(gate.slot)!=0 {t.Fatal(report,err)}
  if kind=="pre-cancelled" && calls!=0 {t.Fatal("cancelled call collected evidence")}
  if kind=="error" && !errors.Is(err,injected) {t.Fatal(err)}
  good:=func(context.Context,*mediaMutationLease,*BufferManager,*bufferEvent)(*eventDurationEvidence,error){return original,nil}
  retried,retryErr:=gate.inspectComplete(context.Background(),lease,bm,job,good,corrStreamChildFactory(t,[]string{"normal","normal"},false))
  corrStreamAssertJoined(t,retried.Collection)
  if retryErr!=nil || !retried.EvidenceCompleted || len(gate.slot)!=0 || job.recoveryBorrow.duration!=original || !lease.held() {t.Fatal("composition poisoned retry",retried,retryErr)}
 })}
}
