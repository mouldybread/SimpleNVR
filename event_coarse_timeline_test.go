package main

import (
 "context"
 "errors"
 "math"
 "path/filepath"
 "reflect"
 "testing"
 "time"
)

// Synthetic observations only. This does NOT establish input arrival or media
// equivalence; production observations are frozen during real descriptor pinning.
func installSyntheticCoarseTimeline(t *eventSelectionTiming) {
 start:=t.selectedAt.Add(-20*time.Second)
 first:=start.Add(5*time.Second);second:=start.Add(10*time.Second)
 t.inputs[0].segmentClock=segmentClockObservation{session:"synthetic-session",sequence:1,startReturned:start,received:first,mediaBegin:0,mediaEnd:5,observed:true}
 t.inputs[1].segmentClock=segmentClockObservation{session:"synthetic-session",sequence:2,startReturned:start,previousReceived:first,received:second,mediaBegin:5,mediaEnd:10,observed:true}
}
func syntheticCoarseTimeline() *eventSelectionTiming {
 selected:=time.Unix(1000,0).UTC()
 out:=&eventSelectionTiming{selectedAt:selected,pinnedAt:selected.Add(time.Second),runEntered:selected.Add(2*time.Second),runReturned:selected.Add(32*time.Second),runStage:eventCaptureRunSucceeded,inputs:make([]eventSelectedInputTiming,2)}
 installSyntheticCoarseTimeline(out)
 return out
}

func TestCoarseTimelineCoherenceIsNotCoverageAndDoesNotMutate(t *testing.T) {
 s:=syntheticCoarseTimeline()
 before:=*s;inputs:=append([]eventSelectedInputTiming(nil),s.inputs...)
 if !s.coarseTimelineCoherent() || !reflect.DeepEqual(before,*s) || !reflect.DeepEqual(inputs,s.inputs) {t.Fatal("coherent observations refused or mutated")}
 // Labels in a different textual zone represent the same receipt instants.
 s.inputs[1].segmentClock.previousReceived=s.inputs[0].segmentClock.received.In(time.FixedZone("same instant",3600))
 if !s.coarseTimelineCoherent() {t.Fatal("timezone formatting changed clock identity")}
 // A selection can start later in a session; no invented preceding observation.
 s=syntheticCoarseTimeline();s.inputs=s.inputs[1:]
 if !s.coarseTimelineCoherent() {t.Fatal("valid interior selection refused")}
 s=syntheticCoarseTimeline();s.inputs=nil
 if !s.coarseTimelineCoherent() {t.Fatal("no-prefix case requires nonexistent segment clocks")}
 var absent *eventSelectionTiming
 if absent.coarseTimelineCoherent() {t.Fatal("nil provenance accepted")}
}

func TestCoarseTimelineRefusesMissingAndBrokenChains(t *testing.T) {
 for _,kind:=range []string{"missing","empty session","zero sequence","uncertain","backwards","session change","session start change","sequence break","predecessor mismatch","CSV gap","CSV overlap","NaN","infinity","empty media span","missing first receipt","missing start","missing predecessor","receipt reversal","receipt after selection","first predecessor","capture reversal","missing selection","not completed","too many inputs"} {t.Run(kind,func(t *testing.T){
  s:=syntheticCoarseTimeline();a:=&s.inputs[0].segmentClock;b:=&s.inputs[1].segmentClock
  switch kind {
  case "missing":a.observed=false
  case "empty session":a.session=""
  case "zero sequence":a.sequence=0
  case "uncertain":a.priorUncertain=true
  case "backwards":a.clockBackwards=true
  case "session change":b.session="other"
  case "session start change":b.startReturned=b.startReturned.Add(time.Nanosecond)
  case "sequence break":b.sequence++
  case "predecessor mismatch":b.previousReceived=b.previousReceived.Add(-time.Nanosecond)
  case "CSV gap":b.mediaBegin++
  case "CSV overlap":b.mediaBegin--
  case "NaN":a.mediaBegin=math.NaN()
  case "infinity":a.mediaEnd=math.Inf(1)
  case "empty media span":a.mediaEnd=a.mediaBegin
  case "missing first receipt":a.received=time.Time{}
  case "missing start":a.startReturned=time.Time{}
  case "missing predecessor":b.previousReceived=time.Time{}
  case "receipt reversal":b.received=b.previousReceived.Add(-time.Nanosecond)
  case "receipt after selection":b.received=s.selectedAt.Add(time.Nanosecond)
  case "first predecessor":a.previousReceived=a.startReturned
  case "capture reversal":s.runReturned=s.runEntered.Add(-time.Nanosecond)
  case "missing selection":s.selectedAt=time.Time{}
  case "not completed":s.runStage=eventCaptureRunRunning
  case "too many inputs":s.inputs=make([]eventSelectedInputTiming,maxRecoveryInputs+1)
  }
  if s.coarseTimelineCoherent() {t.Fatal(kind)}
 })}
}

func TestCoarseTimelineProductionRefusalPreservesPublicationAndProbeBudget(t *testing.T) {
 for _,kind:=range []string{"coherent","missing","CSV gap"} {t.Run(kind,func(t *testing.T){
  installEventFailureTools(t,0,false)
  bm,job:=selectionTimingJob(t)
  if kind!="missing" {installSyntheticCoarseTimeline(job.execution.selection)}
  if kind=="CSV gap" {job.execution.selection.inputs[1].segmentClock.mediaBegin++}
  counter:=filepath.Join(t.TempDir(),"coarse-probe-count")
  t.Setenv("COARSE_COUNTER",counter)
  installEventSpanProbe(t,`case "$*" in
 *duration_ts,time_base*) ;;
 *) printf 'synthetic-codec\n'; exit 0;;
esac
count=0
[ ! -f "$COARSE_COUNTER" ] || count=$(cat "$COARSE_COUNTER")
count=$((count+1))
printf '%s' "$count" > "$COARSE_COUNTER" || exit 9
printf '%s' '{"streams":[{"index":0,"codec_type":"video","duration_ts":30000,"time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":30000}]}'
`)
  input:=bm.recordEventDuration(job);if input=="" {t.Fatal("clock qualification changed capture")}
  // Qualification must consume frozen pins/observations, not mutable ring state.
  bm.bufferMutex.Lock();bm.buffer=nil;bm.segmentClocks=nil;bm.bufferMutex.Unlock()
  final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("clock refusal rolled back publication")}
  borrowed:=job.recoveryBorrow
  if borrowed==nil || !borrowed.terminal || !borrowed.lease.held() || job.recoveryFailed {t.Fatal("publication/ownership changed")}
  if kind=="coherent" {
   if borrowed.duration==nil || borrowed.durationErr!=nil {t.Fatal("coherent frozen chain refused",borrowed.durationErr)}
   check:=durationIntegrationCheck(bm,job)
   if err:=check.check(context.Background(),borrowed.lease);err!=nil {t.Fatal(err)}
   s:=job.execution.selection;saved:=s.inputs[1].segmentClock
   for _,afterIO:=range []bool{false,true} {
    if !afterIO {s.inputs[1].segmentClock.mediaBegin++}
    calls:=0
    err:=check.checkWithOutput(context.Background(),borrowed.lease,func(ctx context.Context,lease *mediaMutationLease)error{
     calls++;if afterIO {s.inputs[1].segmentClock.mediaBegin++};return check.output.check(ctx,lease)
    })
    want:=0;if afterIO {want=1}
    if !errors.Is(err,errEventCoarseTimeline) || calls!=want {t.Fatal("broken chain survived revalidation",err,calls)}
    s.inputs[1].segmentClock=saved
   }
   if err:=check.check(context.Background(),borrowed.lease);err!=nil {t.Fatal("restored chain refused",err)}
  } else if borrowed.duration!=nil || !errors.Is(borrowed.durationErr,errEventCoarseTimeline) {t.Fatal("missing/broken chain qualified",borrowed.durationErr)}
  before:=readRecoveryTest(t,job.dir)
  if before.State!="published" || before.Coverage==nil || before.Coverage.Complete || before.Coverage.Status!="unknown" || durationIntegrationCount(t,counter)!=2 {t.Fatal("publication, coverage uncertainty or two-probe budget changed")}
  lease:=borrowed.lease;inventory:=job.inventory
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || lease.held() || len(bm.slots)!=0 || job.recoveryFailed || durationIntegrationCount(t,counter)!=2 {t.Fatal("clock refusal changed teardown or re-probed")}
  assertJobInventoryClosed(t,inventory)
  after:=readRecoveryTest(t,job.dir)
  if after.State!="published" || !reflect.DeepEqual(before.Coverage,after.Coverage) {t.Fatal("retention-only checkpoint changed coverage evidence")}
  assertPublicationBytes(t,input,"synthetic output");assertPublicationBytes(t,final,"synthetic output")
 })}
}
