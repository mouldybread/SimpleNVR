package main

import (
 "fmt"
 "os"
 "path/filepath"
 "time"

 "golang.org/x/sys/unix"
)

type eventCaptureRunStage uint8
const (
 eventCaptureRunNotObserved eventCaptureRunStage=iota
 eventCaptureRunRunning
 eventCaptureRunSucceeded
 eventCaptureRunRefused
)

// These declared segment times originate from filename/CSV indexing. They are
// preserved AS DECLARED, not promoted into trusted media-to-wall-clock anchors.
type eventSelectedInputTiming struct {
 source,staged string
 declaredStart,declaredEnd time.Time
 file *os.File
 identity unix.Stat_t
 segmentClock segmentClockObservation // immutable completion-notification snapshot, not arrival timing
}

// Owner-serialized, bounded in-memory provenance. No new descriptor, lease,
// process, goroutine, persisted schema, global map or cleanup capability.
// Host Run entry/return times include startup/connection/drain and do NOT map
// first/last frame PTS to wall clock. No positive interval-coverage API is exposed.
type eventSelectionTiming struct {
 self *eventSelectionTiming
 execution *eventExecutionProvenance
 bm *BufferManager
 job *bufferEvent
 inventory *ownedStagingInventory
 dir,eventID string
 duration int
 requestedStart,createdAt,preStart time.Time
 selectedAt,pinnedAt,cutoff time.Time
 inputs []eventSelectedInputTiming
 runStage eventCaptureRunStage
 runEntered,runReturned time.Time
}

// Called once after real pinning/checkpoint success while the existing buffer
// read lock and setup lease remain held. Copy values; never retain selected's slice.
func newEventSelectionTiming(bm *BufferManager,job *bufferEvent,selected []BufferSegment,selectedAt,pinnedAt time.Time) *eventSelectionTiming {
 if bm==nil || job==nil || job.execution==nil || job.inventory==nil || len(selected)!=len(job.preFiles) || len(selected)>maxRecoveryInputs || selectedAt.IsZero() || pinnedAt.IsZero() || pinnedAt.Before(selectedAt) {return nil}
 t:=&eventSelectionTiming{execution:job.execution,bm:bm,job:job,inventory:job.inventory,dir:job.dir,eventID:job.event.ID,duration:job.event.Duration,requestedStart:job.event.StartTime,createdAt:job.event.CreatedAt,preStart:job.preStart,selectedAt:selectedAt,pinnedAt:pinnedAt,cutoff:selectedAt.UTC().Add(-bufferDuration)}
 t.self=t
 t.inputs=make([]eventSelectedInputTiming,len(selected))
 for i,segment:=range selected {
  name:=fmt.Sprintf("pre_%03d.mp4",i)
  entry,ok:=job.inventory.entries[name]
  if !ok || entry.file==nil || job.preFiles[i]!=filepath.Join(job.dir,name) {return nil}
  t.inputs[i]=eventSelectedInputTiming{source:segment.FilePath,staged:job.preFiles[i],declaredStart:segment.StartTime,declaredEnd:segment.EndTime,file:entry.file,identity:entry.identity,segmentClock:bm.segmentClocks[segment.FilePath].freeze(segment,entry.identity)}
 }
 return t
}

// Pure binding checks. Live manager/persistence/registry and filesystem checks
// remain in their existing owners. Never open a source path or inspect the ring.
func (t *eventSelectionTiming) bound(p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) bool {
 if t==nil || t.self!=t || p==nil || bm==nil || job==nil || t.execution!=p || job.execution!=p || p.selection!=t || t.bm!=bm || t.job!=job || t.inventory==nil || job.inventory!=t.inventory || job.dir!=t.dir || job.event.ID!=t.eventID || job.event.Duration!=t.duration || !job.event.StartTime.Equal(t.requestedStart) || !job.event.CreatedAt.Equal(t.createdAt) || !job.preStart.Equal(t.preStart) || len(job.preFiles)!=len(t.inputs) || len(t.inputs)>maxRecoveryInputs || t.runStage>eventCaptureRunSucceeded {return false}
 if t.selectedAt.IsZero() || t.pinnedAt.IsZero() || t.pinnedAt.Before(t.selectedAt) || !t.cutoff.Equal(t.selectedAt.UTC().Add(-bufferDuration)) {return false}
 if t.runStage==eventCaptureRunRunning && (t.runEntered.IsZero() || t.runEntered.Before(t.pinnedAt)) {return false}
 if t.runStage==eventCaptureRunSucceeded && !t.captureComplete() {return false}
 for i,input:=range t.inputs {
  name:=fmt.Sprintf("pre_%03d.mp4",i)
  entry,ok:=t.inventory.entries[name]
  if !ok || input.file==nil || entry.file!=input.file || !stagingInventoryIdentity(input.identity,entry.identity) || input.staged!=filepath.Join(t.dir,name) || job.preFiles[i]!=input.staged {return false}
 }
 return true
}
func (t *eventSelectionTiming) startRun(at time.Time) bool {
 if t==nil {return false}
 if t.runStage!=eventCaptureRunNotObserved || at.IsZero() || at.Before(t.pinnedAt) {t.runStage=eventCaptureRunRefused;return false}
 t.runEntered=at;t.runStage=eventCaptureRunRunning
 return true
}
func (t *eventSelectionTiming) finishRun(at time.Time,runErr,contextErr error) bool {
 if t==nil {return false}
 if t.runStage!=eventCaptureRunRunning {t.runStage=eventCaptureRunRefused;return false}
 t.runReturned=at
 if at.IsZero() || at.Before(t.runEntered) || runErr!=nil || contextErr!=nil {t.runStage=eventCaptureRunRefused;return false}
 t.runStage=eventCaptureRunSucceeded
 return true
}
func (t *eventSelectionTiming) captureComplete() bool {
 return t!=nil && t.runStage==eventCaptureRunSucceeded && !t.runEntered.IsZero() && !t.runReturned.IsZero() && !t.runEntered.Before(t.pinnedAt) && !t.runReturned.Before(t.runEntered)
}
func (p *eventExecutionProvenance) observeCaptureRunStarted(bm *BufferManager,job *bufferEvent,at time.Time) {
 if p==nil {return}
 if p.stage!=eventExecutionCapturing || !p.bound(bm,job) || !p.selection.startRun(at) {p.invalidate()}
}
func (p *eventExecutionProvenance) observeCaptureRunReturned(bm *BufferManager,job *bufferEvent,at time.Time,runErr,contextErr error) {
 if p==nil {return}
 // Snapshot return/failure even after manager cancellation; do not revive it.
 if p.stage!=eventExecutionCapturing || !p.selection.bound(p,bm,job) {p.invalidate();return}
 if !p.selection.finishRun(at,runErr,contextErr) {p.invalidate()}
}
