package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"
 "io/fs"
 "os"
 "path/filepath"
 "reflect"
 "testing"
 "time"
)

// Synthetic process/probe results exercise real lifecycle/filesystem behavior,
// not media validity, capture completeness, or ingestion-time coverage.
func installRecoveryContractTools(t *testing.T,captureExit,assemblyExit int,probeFails bool) {
 t.Helper();bin:=t.TempDir()
 script:=fmt.Sprintf(`#!/bin/sh
for last do :; done
case "$last" in
 /proc/self/fd/3|*/event.mp4) printf 'synthetic capture evidence\n' > "$last"; exit %d;;
 *) printf 'synthetic assembly evidence\n' > "$last"; exit %d;;
esac
`,captureExit,assemblyExit)
 probe:="#!/bin/sh\nprintf 'synthetic-probe-result\\n'\n"
 if probeFails {probe="#!/bin/sh\nexit 1\n"}
 for name,body:=range map[string]string{"ffmpeg":script,"ffprobe":probe} {
  if err:=os.WriteFile(filepath.Join(bin,name),[]byte(body),0700);err!=nil {t.Fatal(err)}
 }
 t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func recoveryContractTree(t *testing.T,root string) map[string]string {
 t.Helper();out:=make(map[string]string)
 err:=filepath.WalkDir(root,func(path string,entry fs.DirEntry,err error)error{
  if err!=nil {return err}
  relative,err:=filepath.Rel(root,path);if err!=nil {return err}
  info,err:=entry.Info();if err!=nil {return err};value:=info.Mode().String()
  if info.Mode().IsRegular() {
   data,err:=os.ReadFile(path);if err!=nil {return err}
   value+=fmt.Sprintf(":%d:%x",len(data),sha256.Sum256(data))
  } else if info.Mode()&os.ModeSymlink!=0 {
   target,err:=os.Readlink(path);if err!=nil {return err};value+=":"+target
  }
  out[relative]=value;return nil
 })
 if err!=nil {t.Fatal(err)};return out
}

func recoveryContractReadback(t *testing.T,bm *BufferManager,dir string) *eventRecoveryManifest {
 t.Helper();data,err:=os.ReadFile(filepath.Join(dir,"recovery.json"));if err!=nil {t.Fatal(err)}
 manifest,err:=parseRecoveryManifest(data,bm.camera.ID);if err!=nil {t.Fatal("producer checkpoint rejected",err)}
 before:=recoveryContractTree(t,bm.camera.RecordDir)
 report:=inspectEventRecovery(context.Background(),bm.camera.RecordDir,bm.camera.ID,filepath.Base(dir))
 if !reflect.DeepEqual(before,recoveryContractTree(t,bm.camera.RecordDir)) {t.Fatal("inspection changed fixture entries, modes or bytes")}
 if !report.Complete || len(report.Errors)!=0 || report.EventID!=manifest.EventID || report.State!=manifest.State || report.Revision!=manifest.Revision || !reflect.DeepEqual(report.Coverage,manifest.Coverage) {t.Fatalf("reader disagreed: %+v",report)}
 want:="inflight_or_interrupted"
 switch manifest.State {
 case "pin_failed","capture_failed","publication_failed":want="observed_processing_failure"
 case "publishing":want="publication_outcome_ambiguous"
 case "published":want="published_checkpoint_unverified"
 }
 if report.Classification!=want {t.Fatal("classification changed",report.Classification,want)}
 if manifest.Coverage!=nil && (manifest.Coverage.Complete || manifest.Coverage.Status!="unknown" || !reflect.DeepEqual(manifest.Coverage.Unknowns,eventCoverageUnknowns())) {t.Fatal("diagnostic gained positive authority")}
 if report.Partial==nil || *report.Partial!=manifest.Partial {t.Fatal("partial flag lost")}
 return manifest
}

func TestRecoveryContractProductionLifecycleAndPersistenceFailures(t *testing.T) {
 cases:=[]struct{name string;captureExit,assemblyExit int;probeFails,collision bool;failState,wantState string;wantPublished bool}{
  {name:"published",wantState:"published",wantPublished:true},
  {name:"assembly_partial",assemblyExit:1,wantState:"published",wantPublished:true},
  {name:"capture_failed",captureExit:1,wantState:"capture_failed"},
  {name:"validation_failed",probeFails:true,wantState:"publication_failed"},
  {name:"publication_collision",collision:true,wantState:"publication_failed"},
  {name:"checkpoint_capturing",failState:"capturing",wantState:"pinned"},
  {name:"checkpoint_captured",failState:"captured",wantState:"capturing"},
  {name:"checkpoint_assembling",failState:"assembling",wantState:"captured"},
  {name:"checkpoint_publishing",failState:"publishing",wantState:"assembling"},
  {name:"checkpoint_published",failState:"published",wantState:"publishing",wantPublished:true},
 }
 for _,pins:=range []int{0,2} {for _,tc:=range cases {t.Run(fmt.Sprintf("pins=%d/%s",pins,tc.name),func(t *testing.T){
  installRecoveryContractTools(t,tc.captureExit,tc.assemblyExit,tc.probeFails)
  bm:=bufferFixture(t);now:=time.Now().UTC();sources:=make(map[string][]byte)
  for i:=0;i<pins;i++ {
   path:=filepath.Join(bm.camera.RecordDir,fmt.Sprintf("source-%d.mp4",i));data:=bytes.Repeat([]byte{byte('a'+i)},2048)
   if err:=os.WriteFile(path,data,0600);err!=nil {t.Fatal(err)};sources[path]=data
   bm.registerCompleted(path,now.Add(time.Duration(i-3)*time.Second),now.Add(time.Duration(i-2)*time.Second))
  }
  event:=bufferTestEvent("contract-"+tc.name)
  if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)};job:=<-bm.eventChan
  t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
  if initial:=recoveryContractReadback(t,bm,job.dir);initial.State!="pinned" || initial.Coverage!=nil {t.Fatal("setup invented processing coverage")}
  expected,err:=stampedFinalPath(bm.camera.RecordDir,job.preStart,event.StartTime.Add(time.Duration(event.Duration)*time.Second),sanitizeFilename(event.Label)+"_"+sanitizeFilename(event.ID),tc.assemblyExit!=0)
  if err!=nil {t.Fatal(err)}
  if tc.collision {
   if err:=os.MkdirAll(filepath.Dir(expected),0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(expected,[]byte("existing published evidence"),0600);err!=nil {t.Fatal(err)}
  }
  originalWriter:=job.recoveryWriter;injected:=errors.New("injected checkpoint failure")
  attempts,committed:=[]string{},[]string{};var failedDisk []byte
  job.recoveryWriter=func(dir string,data []byte)error{
   candidate,err:=parseRecoveryManifest(data,bm.camera.ID);if err!=nil {t.Fatal("producer serialized unreadable checkpoint",err)}
   attempts=append(attempts,candidate.State)
   if candidate.State==tc.failState {
    failedDisk,err=os.ReadFile(filepath.Join(dir,"recovery.json"));if err!=nil {t.Fatal(err)};return injected
   }
   // Preserve the real owned writer and its execution checkpoint receipt.
   if err:=originalWriter(dir,data);err!=nil {return err}
   stored:=recoveryContractReadback(t,bm,dir)
   if stored.Revision!=candidate.Revision || stored.State!=candidate.State {t.Fatal("persisted snapshot differs")}
   committed=append(committed,stored.State);return nil
  }
  bm.handleEvent(job);stored:=recoveryContractReadback(t,bm,job.dir)
  if stored.State!=tc.wantState {t.Fatal("unexpected durable state",stored.State,tc.wantState,attempts)}
  if tc.failState!="" {
   if !job.recoveryFailed || len(failedDisk)==0 || len(attempts)==0 || attempts[len(attempts)-1]!=tc.failState {t.Fatal("failure did not latch")}
   disk,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil || !bytes.Equal(disk,failedDisk) {t.Fatal("prior persisted bytes changed",err)}
  }
  if tc.wantPublished {assertPublicationBytes(t,expected,"synthetic assembly evidence\n")}
  if tc.collision {assertPublicationBytes(t,expected,"existing published evidence")}
  if tc.wantPublished || tc.collision {
   report:=inspectEventRecovery(context.Background(),bm.camera.RecordDir,bm.camera.ID,filepath.Base(job.dir))
   observed:=false;for _,artifact:=range report.Artifacts {if artifact.Role=="published_output" {observed=artifact.Present}}
   if !report.Complete || !observed {t.Fatalf("day-folder output missing from inspection: %+v",report)}
   if stored.OutputPublished!=filepath.Base(expected) {t.Fatal("publication intent disagrees with stamped destination")}
  }
  if tc.name=="assembly_partial" && !stored.Partial {t.Fatal("partial output promoted")}
  before:=recoveryContractTree(t,bm.camera.RecordDir);coverage,err:=json.Marshal(stored.Coverage);if err!=nil {t.Fatal(err)}
  revision,attemptCount:=stored.Revision,len(attempts);inventory:=job.inventory
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("terminal ownership/capacity failure")}
  if inventory!=nil {assertJobInventoryClosed(t,inventory)}
  final:=recoveryContractReadback(t,bm,job.dir);afterCoverage,err:=json.Marshal(final.Coverage);if err!=nil {t.Fatal(err)}
  if final.State!=stored.State || !bytes.Equal(coverage,afterCoverage) {t.Fatal("terminal processing evidence changed")}
  if tc.failState!="" {
   if final.Revision!=revision || len(attempts)!=attemptCount {t.Fatal("failed persistence retried on release")}
  } else if final.Revision!=revision+1 || len(attempts)!=attemptCount+1 {t.Fatal("retention checkpoint missing")}
  after:=recoveryContractTree(t,bm.camera.RecordDir);checkpoint:=filepath.Join("temp",filepath.Base(job.dir),"recovery.json")
  delete(before,checkpoint);delete(after,checkpoint)
  if !reflect.DeepEqual(before,after) {t.Fatal("non-manifest evidence changed during release")}
  for path,data:=range sources {got,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(got,data) {t.Fatal("source changed",err)}}
  if len(committed)==0 && tc.failState!="capturing" {t.Fatal("no processing checkpoint observed")}
 })}}
}

func TestRecoveryContractFailedPinInspectionReflectsActualPrefix(t *testing.T) {
 for _,firstMissing:=range []bool{false,true} {t.Run(fmt.Sprint(firstMissing),func(t *testing.T){
  bm:=bufferFixture(t);now:=time.Now().UTC();source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
  if err:=os.WriteFile(source,bytes.Repeat([]byte("p"),2048),0600);err!=nil {t.Fatal(err)}
  first,second:=source,filepath.Join(bm.camera.RecordDir,"missing.mp4");if firstMissing {first,second=second,first}
  bm.buffer=[]BufferSegment{{FilePath:first,StartTime:now.Add(-2*time.Second),EndTime:now.Add(-time.Second)},
   {FilePath:second,StartTime:now.Add(-time.Second),EndTime:now}}
  if err:=bm.TriggerEvent(bufferTestEvent("failed-pin-contract"));err==nil {t.Fatal("missing source accepted")}
  if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("pin failure leaked capacity")}
  entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatal("failed staging lost",err)}
  dir:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name());manifest:=recoveryContractReadback(t,bm,dir)
  want:=1;if firstMissing {want=0}
  if manifest.State!="pin_failed" || manifest.PinnedCount!=want || manifest.Coverage!=nil || len(manifest.Inputs)!=2 {t.Fatal("pin intent/progress lost")}
  report:=inspectEventRecovery(context.Background(),bm.camera.RecordDir,bm.camera.ID,entries[0].Name());observed:=0
  for _,artifact:=range report.Artifacts {if artifact.Role=="prebuffer" && artifact.Present {observed++}}
  if observed!=want {t.Fatal("reader invented pinned inputs",observed,want)}
 })}
}
