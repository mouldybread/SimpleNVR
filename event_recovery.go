package main

import (
 "encoding/json"
 "errors"
 "fmt"
 "log"
 "os"
 "path/filepath"
 "time"
)

const maxRecoveryInputs=128
const maxRecoveryManifestBytes=65536

type eventRecoveryInput struct {
 Name string `json:"name"`
 Start time.Time `json:"metadata_start"`
 End time.Time `json:"metadata_end"`
}

type eventRecoveryManifest struct {
 Version int `json:"version"`
 Revision int `json:"revision"`
 CameraID string `json:"camera_id"`
 EventID string `json:"event_id"`
 RequestedStart time.Time `json:"requested_start"`
 RequestedEnd time.Time `json:"requested_end"`
 DurationSeconds int `json:"duration_seconds"`
 CreatedAt time.Time `json:"created_at"`
 ObservedAt time.Time `json:"observed_at"`
 State string `json:"state"`
 FailureCode string `json:"failure_code,omitempty"`
 Inputs []eventRecoveryInput `json:"prebuffer_inputs"`
 PinnedCount int `json:"pinned_count"`
 CaptureName string `json:"capture_name"`
 OutputTemporary string `json:"output_temporary,omitempty"`
 OutputPublished string `json:"output_published,omitempty"`
 Partial bool `json:"partial"`
 Coverage *eventCoverageAssessment `json:"coverage_assessment,omitempty"`
}

func syncRecoveryDirectory(path string) error {
 dir,err:=os.Open(path);if err!=nil {return err}
 return errors.Join(dir.Sync(),dir.Close())
}

// The payload file is synced before atomic replacement. A failed operation may
// leave an owner-only temporary snapshot; do not remove it as failed evidence.
func writeEventRecovery(dir string,data []byte) error {
 if len(data)>maxRecoveryManifestBytes {return errors.New("recovery manifest too large")}
 f,err:=os.CreateTemp(dir,"recovery-*.json.tmp");if err!=nil {return err}
 n,writeErr:=f.Write(data)
 if writeErr==nil && n!=len(data) {writeErr=errors.New("short recovery manifest write")}
 if writeErr==nil {writeErr=f.Sync()}
 closeErr:=f.Close()
 if err:=errors.Join(writeErr,closeErr);err!=nil {return err}
 if err:=os.Rename(f.Name(),filepath.Join(dir,"recovery.json"));err!=nil {return err}
 return syncRecoveryDirectory(dir)
}

func (bm *BufferManager) beginEventRecovery(job *bufferEvent,inputs []BufferSegment) error {
 if len(inputs)>maxRecoveryInputs || len(bm.camera.ID)==0 || len(bm.camera.ID)>256 || len(job.event.ID)==0 || len(job.event.ID)>256 || job.event.Duration<1 || job.event.Duration>maxEventSeconds {
  job.recoveryFailed=true;job.keepStaging=true
  return errors.New("invalid recovery identity or input count")
 }
 job.recovery=&eventRecoveryManifest{
  Version:1,CameraID:bm.camera.ID,EventID:job.event.ID,
  RequestedStart:job.event.StartTime.UTC(),RequestedEnd:job.event.StartTime.UTC().Add(time.Duration(job.event.Duration)*time.Second),
  DurationSeconds:job.event.Duration,CreatedAt:job.event.CreatedAt.UTC(),
  CaptureName:"event.mp4",Inputs:make([]eventRecoveryInput,0,len(inputs)),
 }
 for i,seg:=range inputs {job.recovery.Inputs=append(job.recovery.Inputs,eventRecoveryInput{Name:fmt.Sprintf("pre_%03d.mp4",i),Start:seg.StartTime.UTC(),End:seg.EndTime.UTC()})}
 // Persist the new staging entry and, if newly created, its temp parent entry.
 if err:=errors.Join(syncRecoveryDirectory(bm.camera.RecordDir),syncRecoveryDirectory(filepath.Dir(job.dir)));err!=nil {
  job.recoveryFailed=true;job.keepStaging=true;return err
 }
 return job.recoveryCheckpoint("pinning","")
}

func (job *bufferEvent) recoveryCheckpoint(state,code string) error {
 if job.recoveryFailed {return errors.New("recovery persistence previously failed")}
 if job.recovery==nil {job.recoveryFailed=true;job.keepStaging=true;return errors.New("missing recovery identity")}
 job.recovery.Revision++
 job.recovery.ObservedAt=time.Now().UTC()
 // A retention checkpoint must not erase the last processing/failure state.
 if state!="retained" {job.recovery.State=state}
 if code!="" {job.recovery.FailureCode=code}
 job.recovery.PinnedCount=len(job.preFiles)
 job.recovery.Coverage=snapshotEventCoverage(job)
 data,err:=json.Marshal(job.recovery)
 if err==nil {
  writer:=job.recoveryWriter;if writer==nil {writer=writeEventRecovery}
  err=writer(job.dir,data)
 }
 if err!=nil {
  job.recoveryFailed=true;job.keepStaging=true
  log.Printf("Recovery manifest persistence failed for event %s; staging retained",job.event.ID)
 }
 return err
}
