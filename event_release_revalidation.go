package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "path/filepath"
 "strings"
)

var errEventReleaseRevalidation=errors.New("event release revalidation refused")

// Necessary live-state/evidence checks, NOT proof of complete capture or a cleanup
// capability. Bound to one job, manager, inventory, output and acquisition lease.
// Access is serialized by the job owner. No method acquires/releases ownership.
type eventReleaseRevalidation struct {
 n *NVR
 bm *BufferManager
 job *bufferEvent
 camera *Camera
 inventory *ownedStagingInventory
 output *eventPublishedOutput
 borrowed *eventRecoveryBorrow
 execution *eventExecutionProvenance
 duration *eventDurationEvidence
 id,dir,jobDir string
 blocked bool
}
func newEventReleaseRevalidation(n *NVR,bm *BufferManager,job *bufferEvent,blocked bool) *eventReleaseRevalidation {
 check:=&eventReleaseRevalidation{n:n,bm:bm,job:job,blocked:blocked}
 if bm!=nil {check.camera=bm.camera}
 if job!=nil {
  check.execution=job.execution
  check.inventory=job.inventory;check.borrowed=job.recoveryBorrow;check.jobDir=job.dir
  check.dir=filepath.Dir(filepath.Dir(job.dir))
  if job.recovery!=nil {check.id=job.recovery.CameraID}
  if check.borrowed!=nil {check.output=check.borrowed.output;check.duration=check.borrowed.duration}
 }
 return check
}
// Pure receipt/binding checks. Existing output/inventory/checkpoint inspection
// below still performs the filesystem checks; terminal rechecks never re-probe.
func (check *eventReleaseRevalidation) durationCurrent(ctx context.Context,lease *mediaMutationLease) error {
 if check==nil || check.borrowed==nil {return errEventReleaseRevalidation}
 e:=check.duration
 if e==nil || check.borrowed.duration!=e || check.borrowed.durationErr!=nil || e.bm!=check.bm || e.job!=check.job || e.inventory!=check.inventory || e.borrowed!=check.borrowed || e.output!=check.output || e.execution!=check.execution {return errEventReleaseRevalidation}
 if err:=e.bound(ctx,lease);err!=nil {return err}
 if !e.captureSpan.atLeast(e.requestedSeconds) || !e.publishedSpan.atLeast(e.requestedSeconds) {return errEventDurationEvidence}
 return nil
}
func (check *eventReleaseRevalidation) registryCurrent() error {
 if check.n==nil || check.camera==nil {return errEventReleaseRevalidation}
 check.n.mu.RLock();defer check.n.mu.RUnlock()
 cam:=check.camera
 if check.n.shuttingDown.Load() || check.n.cameras[check.id]!=cam || cam.ID!=check.id || cam.RecordDir!=check.dir || !cam.IsRecording || cam.RecordingMode!=ModeEventBased || cam.bufferManager!=check.bm || cam.recordCancel==nil {return errEventReleaseRevalidation}
 return nil
}
func (check *eventReleaseRevalidation) check(ctx context.Context,lease *mediaMutationLease) error {
 return check.checkWithOutput(ctx,lease,nil)
}
func (check *eventReleaseRevalidation) checkWithOutput(ctx context.Context,lease *mediaMutationLease,inspect func(context.Context,*mediaMutationLease)error) error {
 if err:=ctx.Err();err!=nil {return err}
 if check==nil || check.blocked || check.bm==nil || check.job==nil || check.borrowed==nil || check.inventory==nil || check.output==nil || !lease.held() || lease.gate!=recordingMediaOwnership || check.borrowed.lease!=lease {return errEventReleaseRevalidation}
 job:=check.job;m:=job.recovery
 if err:=check.bm.ctx.Err();err!=nil {return err}
 if !check.execution.qualifies(check.bm,job) {return errEventReleaseRevalidation}
 if err:=check.durationCurrent(ctx,lease);err!=nil {return err}
 if job.recoveryBorrow!=check.borrowed || job.inventory!=check.inventory || check.borrowed.output!=check.output || check.bm.camera!=check.camera || job.dir!=check.jobDir || check.inventory.path!=check.jobDir || job.recoveryFailed || m==nil {return errEventReleaseRevalidation}
 if !filepath.IsAbs(check.dir) || filepath.Clean(check.dir)!=check.dir || filepath.Dir(job.dir)!=filepath.Join(check.dir,"temp") || check.id=="" || check.id=="." || check.id==".." || len(check.id)>256 || len(job.event.ID)==0 || len(job.event.ID)>256 || job.event.Duration<1 || job.event.Duration>maxEventSeconds || strings.ContainsAny(check.id,"/\\\x00") {return errEventReleaseRevalidation}
 if m.Version!=1 || m.State!="published" || m.Partial || m.FailureCode!="" || m.CameraID!=check.id || m.EventID!=job.event.ID || m.DurationSeconds!=job.event.Duration || m.CaptureName!="event.mp4" || m.OutputPublished!=filepath.Base(check.output.path) || m.OutputTemporary!=m.OutputPublished+".tmp" || filepath.Dir(check.output.path)!=check.dir {return errEventReleaseRevalidation}
 if len(job.preFiles)>maxRecoveryInputs || m.PinnedCount!=len(job.preFiles) || len(m.Inputs)!=len(job.preFiles) {return errEventReleaseRevalidation}
 for i,file:=range job.preFiles {name:=fmt.Sprintf("pre_%03d.mp4",i);if file!=filepath.Join(job.dir,name) || m.Inputs[i].Name!=name {return errEventReleaseRevalidation}}
 for _,name:=range []string{"event.mp4","concat.txt"} {if _,ok:=check.inventory.entries[name];!ok {return errEventReleaseRevalidation}}
 if err:=check.registryCurrent();err!=nil {return err}
 // No registry lock spans descriptor inspection, enumeration or checkpoint I/O.
 if inspect==nil {inspect=check.output.check}
 if err:=inspect(ctx,lease);err!=nil {return err}
 if err:=check.inventory.Preflight(ctx,lease);err!=nil {return err}
 _,persisted,err:=readOwnedStagingReleaseCheckpoint(ctx,lease,check.inventory);if err!=nil {return err}
 expected,err:=json.Marshal(m);if err!=nil {return err}
 if len(expected)>maxRecoveryManifestBytes || !bytes.Equal(expected,persisted) {return errEventReleaseRevalidation}
 if err:=ctx.Err();err!=nil {return err};if err:=check.bm.ctx.Err();err!=nil {return err}
 if !check.execution.qualifies(check.bm,job) {return errEventReleaseRevalidation}
 if err:=check.durationCurrent(ctx,lease);err!=nil {return err}
 return check.registryCurrent()
}
