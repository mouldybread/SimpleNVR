package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func selectionTimingJob(t *testing.T) (*BufferManager,*bufferEvent) {
 t.Helper();bm:=bufferFixture(t);now:=time.Now().UTC()
 for i:=0;i<2;i++ {
  path:=filepath.Join(bm.camera.RecordDir,fmt.Sprintf("completed-%d.mp4",i))
  if err:=os.WriteFile(path,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
  start:=now.Add(time.Duration(-10+5*i)*time.Second)
  bm.registerCompleted(path,start,start.Add(5*time.Second))
 }
 if err:=bm.TriggerEvent(bufferTestEvent("selection-timing"));err!=nil {t.Fatal(err)}
 job:=<-bm.eventChan
 t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
 if job.execution==nil || job.execution.selection==nil {t.Fatal("production selection observation missing")}
 return bm,job
}
func TestSelectionTimingCopiesOrderedPinnedInputsNotMutableRing(t *testing.T) {
 bm,job:=selectionTimingJob(t);p:=job.execution;s:=p.selection
 if !p.bound(bm,job) || s==nil || len(s.inputs)!=2 || s.captureComplete() || !s.cutoff.Equal(s.selectedAt.UTC().Add(-bufferDuration)) || s.pinnedAt.Before(s.selectedAt) {t.Fatal("missing/invalid production selection observation")}
 source:=s.inputs[0].source;declared:=s.inputs[0].declaredStart
 for i,input:=range s.inputs {
  if input.staged!=job.preFiles[i] || input.file!=job.inventory.entries[fmt.Sprintf("pre_%03d.mp4",i)].file {t.Fatal("selection detached from actual pins")}
 }
 bm.bufferMutex.Lock();bm.buffer[0].FilePath="changed index";bm.buffer[0].StartTime=time.Time{};bm.buffer=nil;bm.bufferMutex.Unlock()
 if s.inputs[0].source!=source || !s.inputs[0].declaredStart.Equal(declared) || !p.bound(bm,job) {t.Fatal("mutable ring changed frozen provenance")}
 // Different textual timezone, same request instant: do not invent a change.
 job.event.StartTime=job.event.StartTime.In(time.FixedZone("same instant",3600))
 if !p.bound(bm,job) {t.Fatal("same instant refused")}
 bm.releaseJob(job)
 if s.bound(p,bm,job) || p.qualifies(bm,job) || job.inventory!=nil || len(bm.slots)!=0 || !job.keepStaging {t.Fatal("terminal lifetime/retention changed")}
}
func TestSelectionTimingRefusesChangedRequestSelectionAndBindings(t *testing.T) {
 for _,kind:=range []string{"requested start","created at","prebuffer origin","order","missing input","replaced pin","changed identity","copied observation","missing observation"} {t.Run(kind,func(t *testing.T){
  bm,job:=selectionTimingJob(t);p:=job.execution;s:=p.selection
  switch kind {
  case "requested start":job.event.StartTime=job.event.StartTime.Add(time.Nanosecond)
  case "created at":job.event.CreatedAt=job.event.CreatedAt.Add(time.Nanosecond)
  case "prebuffer origin":job.preStart=job.preStart.Add(time.Nanosecond)
  case "order":job.preFiles[0],job.preFiles[1]=job.preFiles[1],job.preFiles[0]
  case "missing input":job.preFiles=job.preFiles[:1]
  case "replaced pin":entry:=job.inventory.entries["pre_000.mp4"];entry.file=nil;job.inventory.entries["pre_000.mp4"]=entry
  case "changed identity":entry:=job.inventory.entries["pre_000.mp4"];entry.identity.Ino++;job.inventory.entries["pre_000.mp4"]=entry
  case "copied observation":copy:=*s;p.selection=&copy
  case "missing observation":p.selection=nil
  }
  if p.bound(bm,job) {t.Fatal("changed selection/request accepted",kind)}
  p.beginCapture(bm,job)
  if p.stage!=eventExecutionRefused {t.Fatal("changed plan entered qualified capture")}
  // Restore inventory pins changed solely by this synthetic binding seam.
  if kind=="replaced pin" || kind=="changed identity" {entry:=job.inventory.entries["pre_000.mp4"];entry.file=s.inputs[0].file;entry.identity=s.inputs[0].identity;job.inventory.entries["pre_000.mp4"]=entry}
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("refusal broke retention/teardown")}
 })}
}
func TestCaptureRunTimingOrderedBookendsAndRefusal(t *testing.T) {
 for _,kind:=range []string{"success","run error","canceled","return before entry","clock reversal","replay"} {t.Run(kind,func(t *testing.T){
  bm,job:=selectionTimingJob(t);p:=job.execution;s:=p.selection
  entered:=s.pinnedAt.Add(time.Second);returned:=entered.Add(time.Second)
  p.beginCapture(bm,job)
  if kind=="return before entry" {
   p.observeCaptureRunReturned(bm,job,returned,nil,nil)
  } else {
   p.observeCaptureRunStarted(bm,job,entered)
   if s.captureComplete() || s.runStage!=eventCaptureRunRunning {t.Fatal("entry certified successful return")}
   var runErr,contextErr error
   if kind=="run error" {runErr=os.ErrInvalid}
   if kind=="canceled" {bm.cancel();contextErr=context.Canceled}
   if kind=="clock reversal" {returned=entered.Add(-time.Nanosecond)}
   p.observeCaptureRunReturned(bm,job,returned,runErr,contextErr)
   if !s.runReturned.Equal(returned) {t.Fatal("actual return snapshot lost")}
   if kind=="replay" {p.observeCaptureRunStarted(bm,job,returned.Add(time.Second))}
  }
  if kind=="success" {
   if !s.captureComplete() || p.stage!=eventExecutionCapturing {t.Fatal("Run observation skipped owned captured checkpoint")}
   if p.qualifies(bm,job) {t.Fatal("Run bookends alone qualified publication")}
  } else if p.stage!=eventExecutionRefused || s.captureComplete() {t.Fatal("invalid Run sequence qualified",kind)}
 })}
}
func TestSelectionTimingProductionRevalidationAndNormalCancellation(t *testing.T) {
 _,bm,job,check,lease:=releaseRevalidationFixture(t);p:=job.execution;s:=p.selection
 if !p.qualifies(bm,job) || !s.captureComplete() || s.runEntered.Before(s.pinnedAt) || s.runReturned.Before(s.runEntered) {t.Fatal("production capture Run bookends missing")}
 if !errors.Is(job.recoveryBorrow.ctx.Err(),context.Canceled) {t.Fatal("assembly-local budget unexpectedly live")}
 if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("normal deferred context cancellation invalidated observations",err)}
 original:=job.event.StartTime;job.event.StartTime=original.Add(time.Nanosecond)
 calls:=0
 if err:=check.checkWithOutput(context.Background(),lease,func(context.Context,*mediaMutationLease)error{calls++;return nil});err==nil || calls!=0 {t.Fatal("changed queued request reached evidence inspection",err,calls)}
 job.event.StartTime=original
 if err:=check.check(context.Background(),lease);err!=nil {t.Fatal(err)}
 bm.releaseJob(job)
 if !job.keepStaging || lease.held() || len(bm.slots)!=0 || s.bound(p,bm,job) {t.Fatal("terminal observation lifetime leaked")}
}
func TestSelectionTimingRetainsDeclaredTimesWithoutClaimingMediaMapping(t *testing.T) {
 bm,job:=selectionTimingJob(t);p:=job.execution
 selected:=[]BufferSegment{{FilePath:"declared-a",StartTime:time.Time{},EndTime:time.Time{}},{FilePath:"declared-b",StartTime:time.Unix(50,0),EndTime:time.Unix(40,0)}}
 observed:=newEventSelectionTiming(bm,job,selected,p.selection.selectedAt,p.selection.pinnedAt)
 if observed==nil || !observed.inputs[0].declaredStart.IsZero() || !observed.inputs[1].declaredEnd.Before(observed.inputs[1].declaredStart) {t.Fatal("untrusted declarations normalized into invented timing")}
 selected[1].StartTime=time.Unix(999,0)
 if !observed.inputs[1].declaredStart.Equal(time.Unix(50,0)) {t.Fatal("selection slice retained")}
 if observed.captureComplete() {t.Fatal("source labels created a capture observation")}
 if newEventSelectionTiming(bm,job,selected[:1],observed.selectedAt,observed.pinnedAt)!=nil {t.Fatal("incomplete pin selection accepted")}
 if newEventSelectionTiming(bm,job,selected,time.Time{},observed.pinnedAt)!=nil {t.Fatal("missing selection bookend accepted")}
}
