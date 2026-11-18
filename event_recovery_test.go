package main

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func readRecoveryTest(t *testing.T,dir string) eventRecoveryManifest {
 t.Helper()
 data,err:=os.ReadFile(filepath.Join(dir,"recovery.json"));if err!=nil {t.Fatal(err)}
 var m eventRecoveryManifest
 if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
 return m
}

func TestRecoveryIdentityAndAtomicReplacement(t *testing.T) {
 bm:=bufferFixture(t)
 event:=bufferTestEvent("identity")
 job,err:=bm.pinBuffer(event);if err!=nil {t.Fatal(err)}
 initial:=readRecoveryTest(t,job.dir)
 if initial.Version!=1 || initial.Revision!=2 || initial.CameraID!=bm.camera.ID || initial.EventID!=event.ID || initial.State!="pinned" {t.Fatalf("bad identity/checkpoint: %+v",initial)}
 if !initial.RequestedStart.Equal(event.StartTime) || !initial.RequestedEnd.Equal(event.StartTime.Add(time.Duration(event.Duration)*time.Second)) {t.Fatal("requested range changed")}
 if initial.CaptureName!="event.mp4" || initial.Inputs==nil || initial.OutputPublished!="" {t.Fatal("invented media state")}
 job.recovery.FailureCode="long-old-failure-code"
 if err:=job.recoveryCheckpoint("capture_failed","");err!=nil {t.Fatal(err)}
 job.recovery.FailureCode=""
 if err:=job.recoveryCheckpoint("captured","");err!=nil {t.Fatal(err)}
 current:=readRecoveryTest(t,job.dir)
 if current.FailureCode!="" || current.State!="captured" || current.Revision!=4 {t.Fatal("atomic replacement left old data")}
 info,err:=os.Stat(filepath.Join(job.dir,"recovery.json"));if err!=nil || info.Mode().Perm()!=0600 {t.Fatal("manifest must be owner-only",err)}
 matches,err:=filepath.Glob(filepath.Join(job.dir,"recovery-*.json.tmp"));if err!=nil || len(matches)!=0 {t.Fatal("successful snapshot leaked temporary files",err)}
}

func TestRecoveryPinFailureRecordsIntentAndProgress(t *testing.T) {
 bm:=bufferFixture(t)
 source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
 if err:=os.WriteFile(source,[]byte("prebuffer evidence"),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bm.buffer=[]BufferSegment{
  {StartTime:now.Add(-2*time.Second),EndTime:now.Add(-time.Second),FilePath:source},
  {StartTime:now.Add(-time.Second),EndTime:now,FilePath:filepath.Join(bm.camera.RecordDir,"missing.mp4")},
 }
 if err:=bm.TriggerEvent(bufferTestEvent("incomplete"));err==nil {t.Fatal("missing input accepted")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatal("missing retained directory",err)}
 m:=readRecoveryTest(t,filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name()))
 if m.State!="pin_failed" || m.FailureCode!="link_failed" || len(m.Inputs)!=2 || m.PinnedCount!=1 || m.Inputs[0].Name!="pre_000.mp4" || m.Inputs[1].Name!="pre_001.mp4" {t.Fatalf("bad pin recovery: %+v",m)}
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("failed admission leaked work")}
}

func TestRecoveryFailureLatchPreservesLastSnapshotAndSlot(t *testing.T) {
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("write-failed"));if err!=nil {t.Fatal(err)}
 original,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
 calls:=0
 job.recoveryWriter=func(string,[]byte) error {calls++;return errors.New("injected write/sync failure")}
 bm.slots<-struct{}{}
 if err:=job.recoveryCheckpoint("capturing","");err==nil {t.Fatal("failure ignored")}
 job.keepStaging=false
 bm.releaseJob(job)
 got,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil || string(got)!=string(original) {t.Fatal("last snapshot lost",err)}
 if !job.recoveryFailed || !job.keepStaging || len(bm.slots)!=0 || calls!=1 {t.Fatal("failure latch did not preserve/join safely")}
}

func TestRecoveryFailureBeforeCapturePreventsProcess(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("no-capture"));if err!=nil {t.Fatal(err)}
 job.recoveryWriter=func(string,[]byte) error {return errors.New("injected checkpoint failure")}
 if path:=bm.recordEventDuration(job);path!="" {t.Fatal("capture proceeded")}
 if _,err:=os.Stat(filepath.Join(job.dir,"event.mp4"));!os.IsNotExist(err) {t.Fatal("process produced output despite checkpoint failure",err)}
 if !job.keepStaging {t.Fatal("checkpoint failure authorized cleanup")}
}

func TestRecoveryPublishedCheckpointFailureRetainsInputs(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("published-but-unrecorded"));if err!=nil {t.Fatal(err)}
 input:=filepath.Join(job.dir,"event.mp4")
 if err:=os.WriteFile(input,[]byte("capture evidence"),0600);err!=nil {t.Fatal(err)}
 job.recoveryWriter=func(dir string,data []byte) error {
  var m eventRecoveryManifest
  if err:=json.Unmarshal(data,&m);err!=nil {return err}
  if m.State=="published" {return errors.New("injected final checkpoint failure")}
  return writeEventRecovery(dir,data)
 }
 bm.slots<-struct{}{}
 final:=bm.concatenateEventRecording(job,input)
 if final=="" || !job.recoveryFailed || !job.keepStaging {t.Fatal("published/checkpoint failure semantics lost")}
 bm.releaseJob(job)
 if _,err:=os.Stat(final);err!=nil {t.Fatal("published output lost",err)}
 if got,err:=os.ReadFile(input);err!=nil || string(got)!="capture evidence" {t.Fatal("original capture lost",err)}
 m:=readRecoveryTest(t,job.dir)
 if m.State!="publishing" || m.OutputPublished!=filepath.Base(final) || m.OutputTemporary=="" {t.Fatalf("publication intent lost: %+v",m)}
 if len(bm.slots)!=0 {t.Fatal("slot leaked")}
}

func TestRecoveryRejectsOversizeAndRetainsFailedTemp(t *testing.T) {
 dir:=t.TempDir()
 if err:=writeEventRecovery(dir,make([]byte,maxRecoveryManifestBytes+1));err==nil {t.Fatal("oversize accepted")}
 if err:=os.Mkdir(filepath.Join(dir,"recovery.json"),0700);err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(dir,[]byte("snapshot evidence"));err==nil {t.Fatal("directory destination replaced")}
 matches,err:=filepath.Glob(filepath.Join(dir,"recovery-*.json.tmp"));if err!=nil || len(matches)!=1 {t.Fatal("failed snapshot discarded",err)}
 if got,err:=os.ReadFile(matches[0]);err!=nil || string(got)!="snapshot evidence" {t.Fatal("failed snapshot bytes lost",err)}
}
