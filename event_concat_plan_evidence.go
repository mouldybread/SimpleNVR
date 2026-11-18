package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "errors"
 "fmt"
 "io"
 "os"
 "strings"

 "golang.org/x/sys/unix"
)

var errEventConcatPlan=errors.New("owned event concat plan unavailable or changed")
const maxEventConcatPlanBytes=maxRecoveryManifestBytes

type eventConcatPlanStage uint8
const (
 eventConcatPlanUnobserved eventConcatPlanStage=iota
 eventConcatPlanRegistered
 eventConcatPlanWriterClosed
)

// This is an observed INPUT PLAN, not proof of muxed frame correspondence,
// interval coverage or permission to delete. No new pin, lease or process.
type eventConcatPlanEvidence struct {
 self *eventConcatPlanEvidence
 execution *eventExecutionProvenance
 selection *eventSelectionTiming
 inventory *ownedStagingInventory
 file,capture *os.File
 identity,captureID,snapshot unix.Stat_t
 digest [sha256.Size]byte
 stage eventConcatPlanStage
}
func expectedEventConcatPlan(p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) ([]byte,error) {
 if p==nil || !p.bound(bm,job) || !p.selection.captureComplete() {return nil,errEventConcatPlan}
 var list strings.Builder
 for i:=range p.selection.inputs {
  fmt.Fprintf(&list,"file 'pre_%03d.mp4'\n",i)
  if list.Len()>maxEventConcatPlanBytes {return nil,errEventConcatPlan}
 }
 list.WriteString("file 'event.mp4'\n")
 if list.Len()>maxEventConcatPlanBytes {return nil,errEventConcatPlan}
 return []byte(list.String()),nil
}
func (c *eventConcatPlanEvidence) bound(p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) bool {
 if c==nil || c.self!=c || p==nil || c.execution!=p || p.concat!=c || !p.bound(bm,job) || c.selection!=p.selection || c.inventory!=p.inventory || c.file==nil || c.capture==nil || p.stage<eventExecutionAssembling || p.stage>eventExecutionAssembled {return false}
 list,listOK:=c.inventory.entries["concat.txt"];capture,captureOK:=c.inventory.entries["event.mp4"]
 return listOK && captureOK && list.file==c.file && capture.file==c.capture && stagingInventoryIdentity(list.identity,c.identity) && stagingInventoryIdentity(capture.identity,c.captureID)
}
func (c *eventConcatPlanEvidence) ready(p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) bool {
 return c!=nil && c.stage==eventConcatPlanWriterClosed && c.bound(p,bm,job)
}
func (c *eventConcatPlanEvidence) readAndMatch(ctx context.Context,lease *mediaMutationLease,p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) error {
 if err:=ctx.Err();err!=nil {return err}
 if !c.bound(p,bm,job) {return errEventConcatPlan}
 if err:=c.inventory.Preflight(ctx,lease);err!=nil {return err}
 expected,err:=expectedEventConcatPlan(p,bm,job);if err!=nil {return err}
 if c.digest!=sha256.Sum256(expected) {return errEventConcatPlan}
 before,err:=eventPresentationDescriptorStat(ctx,lease,c.file);if err!=nil {return err}
 if !stagingInventoryIdentity(before,c.identity) || !eventPresentationDescriptorUnchanged(before,c.snapshot) || before.Size!=int64(len(expected)) {return errEventConcatPlan}
 // Read one byte beyond the expected plan: a matching prefix is insufficient.
 // ReadAt preserves the shared descriptor's offset. No pathname open/fallback.
 data:=make([]byte,len(expected)+1)
 n,readErr:=c.file.ReadAt(data,0)
 if n!=len(expected) || readErr!=io.EOF || !bytes.Equal(data[:n],expected) {return errEventConcatPlan}
 if err:=ctx.Err();err!=nil {return err}
 after,err:=eventPresentationDescriptorStat(ctx,lease,c.file);if err!=nil {return err}
 if !eventPresentationDescriptorUnchanged(before,after) {return errEventConcatPlan}
 if err:=c.inventory.Preflight(ctx,lease);err!=nil {return err}
 if err:=ctx.Err();err!=nil {return err}
 if !c.bound(p,bm,job) {return errEventConcatPlan}
 return nil
}
func (c *eventConcatPlanEvidence) check(ctx context.Context,lease *mediaMutationLease,p *eventExecutionProvenance,bm *BufferManager,job *bufferEvent) error {
 if err:=ctx.Err();err!=nil {return err}
 if !c.ready(p,bm,job) {return errEventConcatPlan}
 return c.readAndMatch(ctx,lease,p,bm,job)
}

// Only called after the actual creator write and Remember return successfully,
// borrowing the EXISTING registration lease. Its caller still owns writer close.
func (p *eventExecutionProvenance) observeConcatRegistered(ctx context.Context,lease *mediaMutationLease,job *bufferEvent,inventory *ownedStagingInventory) {
 if p==nil {return}
 if p.stage!=eventExecutionAssembling || p.concat!=nil || !p.bound(p.bm,job) || inventory!=p.inventory {p.invalidate();return}
 expected,err:=expectedEventConcatPlan(p,p.bm,job);if err!=nil {p.invalidate();return}
 list,listOK:=inventory.entries["concat.txt"];capture,captureOK:=inventory.entries["event.mp4"]
 if !listOK || !captureOK {p.invalidate();return}
 snapshot,err:=eventPresentationDescriptorStat(ctx,lease,list.file);if err!=nil {p.invalidate();return}
 c:=&eventConcatPlanEvidence{execution:p,selection:p.selection,inventory:inventory,file:list.file,capture:capture.file,identity:list.identity,captureID:capture.identity,snapshot:snapshot,digest:sha256.Sum256(expected)}
 c.self=c;p.concat=c
 if err:=c.readAndMatch(ctx,lease,p,p.bm,job);err!=nil {p.invalidate();return}
 c.stage=eventConcatPlanRegistered
}
func (p *eventExecutionProvenance) finishConcatWrite(job *bufferEvent,writeErr error) {
 if p==nil {return}
 c:=p.concat
 if writeErr!=nil || p.stage!=eventExecutionAssembling || c==nil || c.stage!=eventConcatPlanRegistered || !c.bound(p,p.bm,job) {p.invalidate();return}
 // The wrapper has now returned, including its writer Close result. A callback
 // alone or a registered descriptor before successful close cannot seal this.
 c.stage=eventConcatPlanWriterClosed
}
