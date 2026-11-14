package main

import (
 "context"
 "errors"
 "math"
 "reflect"
 "testing"
)

func TestAssemblyPresentationComparesCaptureNotOnlyRequestedDuration(t *testing.T) {
 capture:=eventVideoContinuity{first:0,end:40000,numerator:1,denominator:1000,frames:400}
 output:=eventVideoContinuity{first:0,end:30000,numerator:1,denominator:1000,frames:300}
 if !capture.atLeastSeconds(30) || !output.atLeastSeconds(30) {t.Fatal("fixture must pass old independent bounds")}
 for _,prefix:=range []bool{false,true} {
  if eventAssemblyPresentationCompatible(capture,output,prefix) {t.Fatal("shorter captured component accepted",prefix)}
  sameDuration:=capture;sameDuration.frames--
  if eventAssemblyPresentationCompatible(capture,sameDuration,prefix) {t.Fatal("fewer frames accepted",prefix)}
 }
 longer:=capture;longer.end+=1000;longer.frames++
 if eventAssemblyPresentationCompatible(capture,longer,false) {t.Fatal("unexplained no-prefix extension accepted")}
 if !eventAssemblyPresentationCompatible(capture,longer,true) {t.Fatal("prefix extension refused")}
 // Equality with a prefix is only a lower-bound result, never prefix proof.
 if !eventAssemblyPresentationCompatible(capture,capture,true) {t.Fatal("necessary lower bound changed meaning")}
}

func TestAssemblyPresentationAllowsExactRebaseAndDifferentTimebases(t *testing.T) {
 capture:=eventVideoContinuity{first:-1024,end:2048,numerator:1,denominator:1024,frames:30}
 output:=eventVideoContinuity{first:900000,end:990000,numerator:1,denominator:30000,frames:30}
 if !eventAssemblyPresentationCompatible(capture,output,false) {t.Fatal("equivalent rebased intervals refused")}
 output.end--
 if eventAssemblyPresentationCompatible(capture,output,true) {t.Fatal("one-tick short output accepted")}
}

func TestAssemblyPresentationUsesOverflowSafeExactArithmetic(t *testing.T) {
 capture:=eventVideoContinuity{first:math.MinInt64,end:math.MaxInt64,numerator:math.MaxInt64,denominator:math.MaxInt64,frames:3}
 if !eventAssemblyPresentationCompatible(capture,capture,false) {t.Fatal("wide subtraction/product overflow")}
 shorter:=capture;shorter.end--
 if eventAssemblyPresentationCompatible(capture,shorter,true) {t.Fatal("wide short interval accepted")}
 capture=eventVideoContinuity{first:0,end:1,numerator:1,denominator:math.MaxInt64-1,frames:1}
 shorter=eventVideoContinuity{first:0,end:1,numerator:1,denominator:math.MaxInt64,frames:1}
 if eventAssemblyPresentationCompatible(capture,shorter,true) {t.Fatal("fractional shortfall rounded away")}
}

func TestAssemblyPresentationRejectsMissingOrInvalidSummaries(t *testing.T) {
 valid:=eventVideoContinuity{first:0,end:30,numerator:1,denominator:1,frames:30}
 for _,kind:=range []string{"zero frames","over frame limit","empty interval","reversed interval","zero numerator","negative denominator"} {
  bad:=valid
  switch kind {
  case "zero frames":bad.frames=0
  case "over frame limit":bad.frames=maxEventVideoContinuityFrames+1
  case "empty interval":bad.end=bad.first
  case "reversed interval":bad.end=bad.first-1
  case "zero numerator":bad.numerator=0
  case "negative denominator":bad.denominator=-1
  }
  if eventAssemblyPresentationCompatible(bad,valid,true) || eventAssemblyPresentationCompatible(valid,bad,true) {t.Fatal(kind)}
 }
}

// Synthetic bound metadata exercises before/after revalidation. No media
// equivalence or real-camera acceptance is claimed by these mutation seams.
func TestAssemblyPresentationRevalidationReusesBoundObservations(t *testing.T) {
 for _,kind:=range []string{"capture longer","output fewer frames","no-prefix extension","changed after IO"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  evidence:=job.recoveryBorrow.duration.presentation;saved:=*evidence
  if len(job.execution.selection.inputs)!=0 {t.Fatal("no-prefix fixture changed")}
  before:=eventStorageTree(t,job.dir)
  switch kind {
  case "capture longer":evidence.capture.continuity.end++
  case "output fewer frames":evidence.capture.continuity.frames++
  case "no-prefix extension":evidence.published.continuity.end++
  }
  calls:=0
  err:=check.checkWithOutput(context.Background(),lease,func(ctx context.Context,lease *mediaMutationLease)error{
   calls++
   if kind=="changed after IO" {evidence.capture.continuity.frames++}
   return check.output.check(ctx,lease)
  })
  want:=0;if kind=="changed after IO" {want=1}
  if !errors.Is(err,errEventAssemblyPresentation) || calls!=want || !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal(kind,err,calls)}
  *evidence=saved
  if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("genuine observations refused",err)}
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || lease.held() || len(bm.slots)!=0 {t.Fatal("refusal changed teardown")}
 })}
}

// Synthetic child output exercises actual production finalization: capture and
// publication both exceed the request, but publication omits part of capture.
func TestAssemblyPresentationRefusalPreservesPublishedOutcomeAndTwoProbeBudget(t *testing.T) {
 bm,job,counter:=durationIntegrationFixture(t,"success")
 installEventSpanProbe(t,`case "$*" in
 *duration_ts,time_base*) ;;
 *) printf 'synthetic-codec\n'; exit 0;;
esac
count=0
[ ! -f "$DURATION_COUNTER" ] || count=$(cat "$DURATION_COUNTER")
count=$((count+1))
printf '%s' "$count" > "$DURATION_COUNTER" || exit 9
ticks=30000
[ "$count" != 1 ] || ticks=40000
printf '{"streams":[{"index":0,"codec_type":"video","duration_ts":%s,"time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":%s}]}' "$ticks" "$ticks"
`)
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
 final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("observation refusal rolled back publication")}
 borrowed:=job.recoveryBorrow
 if borrowed==nil || !borrowed.terminal || !borrowed.lease.held() || borrowed.duration!=nil || !errors.Is(borrowed.durationErr,errEventAssemblyPresentation) || durationIntegrationCount(t,counter)!=2 || job.recoveryFailed {t.Fatal("assembly inconsistency not conservatively refused")}
 if m:=readRecoveryTest(t,job.dir);m.State!="published" {t.Fatal("publication checkpoint lost",m)}
 lease:=borrowed.lease
 bm.releaseJob(job)
 if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 || lease.held() || durationIntegrationCount(t,counter)!=2 {t.Fatal("terminal teardown retried probes or leaked ownership")}
 assertPublicationBytes(t,input,"synthetic output");assertPublicationBytes(t,final,"synthetic output")
}
