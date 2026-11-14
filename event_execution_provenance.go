package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "path/filepath"
)

type eventExecutionStage uint8
const (
 eventExecutionReady eventExecutionStage = iota
 eventExecutionCapturing
 eventExecutionCaptured
 eventExecutionAssembling
 eventExecutionJoined
 eventExecutionAssembled
 eventExecutionRefused
)

// NEW IMPLEMENTATION. A bounded, owner-serialized observation of execution, not
// media coverage or a cleanup permit. Only production job setup creates it.
// No local capture/assembly context, descriptor, goroutine or lease is retained.
type eventExecutionProvenance struct {
 bm *BufferManager
 job *bufferEvent
 inventory *ownedStagingInventory
 camera *Camera
 manifest *eventRecoveryManifest
 dir, eventID string
 duration int
 stage eventExecutionStage
 checkpoint [sha256.Size]byte
 checkpointSeen bool
 publication bool
 selection *eventSelectionTiming
 concat *eventConcatPlanEvidence
}

func newEventExecutionProvenance(bm *BufferManager,job *bufferEvent) *eventExecutionProvenance {
 return &eventExecutionProvenance{bm:bm,job:job,inventory:job.inventory,camera:bm.camera,manifest:job.recovery,dir:job.dir,eventID:job.event.ID,duration:job.event.Duration}
}
func (p *eventExecutionProvenance) invalidate() {if p!=nil {p.stage=eventExecutionRefused}}
func (p *eventExecutionProvenance) bound(bm *BufferManager,job *bufferEvent) bool {
 return p!=nil && bm!=nil && job!=nil && job.execution==p && p.bm==bm && p.job==job && p.inventory!=nil && job.inventory==p.inventory && p.inventory.path==p.dir && p.camera==bm.camera && p.manifest!=nil && job.recovery==p.manifest && job.dir==p.dir && job.event.ID==p.eventID && job.event.Duration==p.duration && !job.recoveryFailed && bm.ctx!=nil && bm.ctx.Err()==nil && p.selection.bound(p,bm,job)
}
func (p *eventExecutionProvenance) advance(bm *BufferManager,job *bufferEvent,from,to eventExecutionStage) bool {
 if !p.bound(bm,job) || p.stage!=from {p.invalidate();return false}
 p.stage=to;return true
}
func (p *eventExecutionProvenance) beginCapture(bm *BufferManager,job *bufferEvent) {
 p.advance(bm,job,eventExecutionReady,eventExecutionCapturing)
}
func (p *eventExecutionProvenance) endCaptureAttempt() {
 if p!=nil && p.stage!=eventExecutionCaptured {p.invalidate()}
}

// Called only after the actual owned writer returns, including its close result.
// A callback returning nil without writing cannot create this bounded receipt.
func (p *eventExecutionProvenance) ownedCheckpoint(data []byte,err error) {
 if p==nil {return}
 if err!=nil || len(data)>maxRecoveryManifestBytes {p.invalidate();return}
 p.checkpoint=sha256.Sum256(data);p.checkpointSeen=true
}
func (p *eventExecutionProvenance) checkpointCurrent(job *bufferEvent,state string) bool {
 if p==nil || !p.checkpointSeen || job.recovery==nil || job.recovery.State!=state {return false}
 data,err:=json.Marshal(job.recovery)
 return err==nil && len(data)<=maxRecoveryManifestBytes && p.checkpoint==sha256.Sum256(data)
}
func (p *eventExecutionProvenance) finishCapture(bm *BufferManager,job *bufferEvent,ctx context.Context) {
 if p==nil {return}
 if ctx.Err()!=nil || !p.selection.captureComplete() || !p.checkpointCurrent(job,"captured") {p.invalidate();return}
 if !p.advance(bm,job,eventExecutionCapturing,eventExecutionCaptured) {return}
 if _,ok:=p.inventory.entries["event.mp4"];!ok {p.invalidate()}
}
func (p *eventExecutionProvenance) beginAssembly(bm *BufferManager,job *bufferEvent,eventFile string) {
 if p==nil {return}
 if eventFile!=filepath.Join(p.dir,"event.mp4") {p.invalidate();return}
 p.advance(bm,job,eventExecutionCaptured,eventExecutionAssembling)
}
func (p *eventExecutionProvenance) joinAssembly(bm *BufferManager,job *bufferEvent,ctx context.Context,err error) {
 if p==nil {return}
 if err!=nil || ctx.Err()!=nil || !p.concat.ready(p,bm,job) {p.invalidate();return}
 if !p.advance(bm,job,eventExecutionAssembling,eventExecutionJoined) {return}
 if _,ok:=p.inventory.entries["concat.txt"];!ok {p.invalidate()}
}
func (p *eventExecutionProvenance) published(bm *BufferManager,job *bufferEvent,ctx context.Context) {
 if p==nil {return}
 if !p.bound(bm,job) || p.stage!=eventExecutionJoined || p.publication || ctx.Err()!=nil || job.recoveryBorrow==nil || job.recoveryBorrow.output==nil {p.invalidate();return}
 p.publication=true
}
func (p *eventExecutionProvenance) finishAssembly(bm *BufferManager,job *bufferEvent,ctx,assemblyCtx context.Context,incomplete bool,checkpointErr error) {
 if p==nil {return}
 if incomplete || checkpointErr!=nil || ctx.Err()!=nil || assemblyCtx.Err()!=nil || !p.publication || !p.checkpointCurrent(job,"published") {p.invalidate();return}
 p.advance(bm,job,eventExecutionJoined,eventExecutionAssembled)
}
func (p *eventExecutionProvenance) endAssemblyAttempt() {
 if p!=nil && p.stage!=eventExecutionAssembled {p.invalidate()}
}
func (p *eventExecutionProvenance) qualifies(bm *BufferManager,job *bufferEvent) bool {
 return p.bound(bm,job) && p.stage==eventExecutionAssembled && p.publication && p.selection.captureComplete() && p.concat.ready(p,bm,job)
}

func (p *eventExecutionProvenance) requireCheckpoint(job *bufferEvent,state string) {
 if p!=nil && !p.checkpointCurrent(job,state) {p.invalidate()}
}
