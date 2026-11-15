package main

import (
 "errors"
 "math"
 "time"
)

var errEventCoarseTimeline=errors.New("frozen prebuffer/capture clock observations incoherent or unavailable")

// Keep both monotonic (when present) and UTC wall ordering. time.Time.Before
// alone can use monotonic time and hide a backwards host-wall-clock transition.
// Equal receipts are allowed; no duration or media-arrival bound is inferred.
func eventCoarseClockOrdered(before,after time.Time) bool {
 return !before.IsZero() && !after.IsZero() && !after.Before(before) && !after.UTC().Before(before.UTC())
}
func eventCoarseClockSame(a,b time.Time) bool {
 return !a.IsZero() && !b.IsZero() && a.Equal(b) && a.UTC().Equal(b.UTC())
}

// Necessary coherence only, called AFTER live execution/selection binding by
// eventPresentationEvidence.check. This pure bounded check never grants coverage,
// mutates provenance, reopens source paths, looks up the ring, or certifies media.
// Receipt times are segment-completion notices and Run bookends, NOT packet times.
func (t *eventSelectionTiming) coarseTimelineCoherent() bool {
 if t==nil || len(t.inputs)>maxRecoveryInputs || !t.captureComplete() || !eventCoarseClockOrdered(t.selectedAt,t.pinnedAt) || !eventCoarseClockOrdered(t.pinnedAt,t.runEntered) || !eventCoarseClockOrdered(t.runEntered,t.runReturned) {return false}
 var previous segmentClockObservation
 for i,input:=range t.inputs {
  o:=input.segmentClock
  if !o.observed || o.session=="" || o.sequence==0 || o.priorUncertain || o.clockBackwards || math.IsNaN(o.mediaBegin) || math.IsNaN(o.mediaEnd) || math.IsInf(o.mediaBegin,0) || math.IsInf(o.mediaEnd,0) || o.mediaEnd<=o.mediaBegin {return false}
  prior:=o.previousReceived
  if o.sequence==1 {
   if !prior.IsZero() {return false}
   prior=o.startReturned
  }
  if !eventCoarseClockOrdered(o.startReturned,prior) || !eventCoarseClockOrdered(prior,o.received) || !eventCoarseClockOrdered(o.received,t.selectedAt) {return false}
  if i>0 {
   // CSV boundary equality is declaration coherence, never actual media proof.
   if o.session!=previous.session || !eventCoarseClockSame(o.startReturned,previous.startReturned) || previous.sequence==^uint64(0) || o.sequence!=previous.sequence+1 || !eventCoarseClockSame(o.previousReceived,previous.received) || o.mediaBegin!=previous.mediaEnd {return false}
  }
  previous=o
 }
 return true
}
