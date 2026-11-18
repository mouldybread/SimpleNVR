package main

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
)

// All media/process witnesses here are synthetic. Successful execution of the
// production control flow is NOT proof of requested duration or camera footage.
func TestExecutionProvenanceOrderedProductionAndTerminalLifetime(t *testing.T) {
 _,bm,job,check,lease:=releaseRevalidationFixture(t)
 p:=job.execution
 if !p.qualifies(bm,job) || p.stage!=eventExecutionAssembled {t.Fatal("ordered execution not observed")}
 // Both function-local contexts have already been canceled by normal returns.
 if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("normal deferred cancel invalidated execution",err)}
 job.keepStaging=false
 bm.releaseJob(job)
 if p.qualifies(bm,job) || p.stage!=eventExecutionRefused || !job.keepStaging || lease.held() || len(bm.slots)!=0 || job.inventory!=nil || job.recoveryBorrow!=nil {t.Fatal("terminal lifetime/slot/lease changed")}
 assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"synthetic output")
 if _,err:=os.Stat(check.output.path);err!=nil {t.Fatal("publication lost",err)}
}

func TestExecutionProvenanceCaptureRefusalMatrix(t *testing.T) {
 for _,kind:=range []string{"process failure","canceled manager","replacement","capturing callback","captured callback","captured persistence failure","cancel after join"} {t.Run(kind,func(t *testing.T){
  exit:=0;if kind=="process failure" {exit=1}
  installEventFailureTools(t,exit,false)
  bm,job:=publicationCheckpointJob(t)
  p:=job.execution
  if p==nil || p.qualifies(bm,job) {t.Fatal("new job not default refusing")}
  original:=job.recoveryWriter
  if kind=="replacement" {
   bin:=t.TempDir()
   script:="#!/bin/sh\nfor last do :; done\nmv -- \"$EXEC_CAPTURE\" \"$EXEC_CAPTURE.saved\" || exit 9\nprintf foreign > \"$EXEC_CAPTURE\"\nprintf synthetic > \"$last\"\n"
   if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
   t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"));t.Setenv("EXEC_CAPTURE",filepath.Join(job.dir,"event.mp4"))
  }
  job.recoveryWriter=func(dir string,data []byte)error{
   var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {return err}
   if kind=="capturing callback" && m.State=="capturing" {return nil}
   if m.State=="captured" {
    if p.stage==eventExecutionCaptured || p.qualifies(bm,job) {t.Error("capture qualified before checkpoint outcome")}
    switch kind {case "captured callback":return nil;case "captured persistence failure":return os.ErrPermission;case "cancel after join":bm.cancel()}
   }
   return original(dir,data)
  }
  if kind=="canceled manager" {bm.cancel()}
  file:=bm.recordEventDuration(job)
  if p.stage!=eventExecutionRefused || p.qualifies(bm,job) {t.Fatal("failed/canceled/unowned capture qualified",kind)}
  if kind=="capturing callback" || kind=="captured callback" || kind=="cancel after join" {
   // A callback must not create provenance, even when legacy capture returns a
   // path. A subsequent ordered-looking assembly cannot repair that refusal.
   if file!="" && bm.ctx.Err()==nil {bm.concatenateEventRecording(job,file)}
   if p.qualifies(bm,job) {t.Fatal("assembly repaired refused capture")}
  } else if file!="" {t.Fatal("capture failure published success",kind)}
  job.recoveryWriter=original
  bm.releaseJob(job)
  if !job.keepStaging || len(bm.slots)!=0 || job.inventory!=nil {t.Fatal("refused capture leaked capacity or removed evidence")}
  if _,err:=os.Stat(job.dir);err!=nil {t.Fatal(err)}
 })}
}

func TestExecutionProvenanceDirectAssemblyStillPublishesButRefuses(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 input:=filepath.Join(job.dir,"event.mp4")
 if err:=job.withOwnedStagingCreator(context.Background(),"event.mp4",func(f *os.File)error{_,err:=f.WriteString("creator-only bytes");return err});err!=nil {t.Fatal(err)}
 // Even a genuinely persisted captured string does not observe production capture.
 if err:=job.recoveryCheckpoint("captured","");err!=nil {t.Fatal(err)}
 job.keepStaging=false
 final:=bm.concatenateEventRecording(job,input)
 if final=="" {t.Fatal("direct assembly publication compatibility lost")}
 if job.execution.qualifies(bm,job) || job.recoveryBorrow.revalidationErr==nil {t.Fatal("creator/checkpoint/false flag substituted for capture")}
 n:=&NVR{cameras:map[string]*Camera{bm.camera.ID:bm.camera}}
 bm.camera.IsRecording=true;bm.camera.RecordingMode=ModeEventBased;bm.camera.bufferManager=bm;bm.camera.recordCancel=func(){}
 check:=newEventReleaseRevalidation(n,bm,job,false)
 calls:=0
 if err:=check.checkWithOutput(context.Background(),job.recoveryBorrow.lease,func(context.Context,*mediaMutationLease)error{calls++;return nil});err==nil || calls!=0 {t.Fatal("direct assembly reached revalidation I/O")}
 bm.releaseJob(job)
 assertPublicationBytes(t,input,"creator-only bytes")
 assertPublicationBytes(t,final,"synthetic output")
 if !job.keepStaging || len(bm.slots)!=0 {t.Fatal("direct assembly release changed")}
}

func TestExecutionProvenanceAssemblyRefusalMatrix(t *testing.T) {
 for _,kind:=range []string{"process failure","publication failure","published callback","publishing callback","published persistence failure","cancel after join"} {t.Run(kind,func(t *testing.T){
  installEventFailureTools(t,0,false)
  bm,job:=publicationCheckpointJob(t)
  input:=bm.recordEventDuration(job)
  if input=="" || job.execution.stage!=eventExecutionCaptured {t.Fatal("capture prerequisite failed")}
  if kind=="process failure" {installEventFailureTools(t,1,false)}
  if kind=="publication failure" {installEventFailureTools(t,0,true)}
  original:=job.recoveryWriter
  job.recoveryWriter=func(dir string,data []byte)error{
   var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {return err}
   if kind=="publishing callback" && m.State=="publishing" {return nil}
   if m.State=="published" {
    if job.execution.qualifies(bm,job) {t.Error("assembly qualified before published checkpoint result")}
    switch kind {case "published callback":return nil;case "published persistence failure":return os.ErrPermission;case "cancel after join":bm.cancel()}
   }
   return original(dir,data)
  }
  final:=bm.concatenateEventRecording(job,input)
  if kind!="publication failure" && final=="" {t.Fatal("legacy publication result lost",kind)}
  if kind=="publication failure" && final!="" {t.Fatal("invalid output published")}
  if job.execution.stage!=eventExecutionRefused || job.execution.qualifies(bm,job) {t.Fatal("failed assembly qualified",kind)}
  job.recoveryWriter=original
  bm.releaseJob(job)
  assertPublicationBytes(t,input,"synthetic output")
  if !job.keepStaging || len(bm.slots)!=0 || job.recoveryBorrow!=nil {t.Fatal("refusal broke terminal teardown")}
 })}
}

func TestExecutionProvenanceInjectedPublicationCannotQualify(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
 p:=job.execution
 // Explicit state-machine seam, NOT an actual assembly-process witness. Even
 // synthetic ordered outcomes plus a successful callback cannot bypass the real
 // publication observer, owned checkpoint and output requirements.
 p.beginAssembly(bm,job,input)
 if err:=job.writeEventConcat(bm.ctx,input);err!=nil {t.Fatal(err)}
 p.joinAssembly(bm,job,bm.ctx,nil)
 if err:=bm.publishEventRecordingWith(context.Background(),bm.ctx,job,"temporary","final",false,func(context.Context,*mediaMutationLease,string,string)error{return nil});err!=nil {t.Fatal(err)}
 if p.qualifies(bm,job) || p.stage!=eventExecutionRefused {t.Fatal("success callback forged execution")}
 bm.releaseJob(job)
 if len(bm.slots)!=0 || !job.keepStaging {t.Fatal("injected publication leaked")}
}

func TestExecutionProvenanceBindingsReplayAndOrdering(t *testing.T) {
 for _,kind:=range []string{"wrong job","wrong inventory","wrong manager","wrong camera","wrong manifest","wrong duration","replaced witness","capture replay","assembly replay","publication replay"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  p:=job.execution
  switch kind {
  case "wrong job":copy:=*job;if p.qualifies(bm,&copy) {t.Fatal("copied job accepted")};return
  case "wrong inventory":old:=job.inventory;job.inventory=&ownedStagingInventory{};defer func(){job.inventory=old}()
  case "wrong manager":other:=newBufferManager(context.Background(),bm.camera);defer other.cancel()
   if p.qualifies(other,job) {t.Fatal("different manager accepted")};return
  case "wrong camera":old:=bm.camera;bm.camera=&Camera{};defer func(){bm.camera=old}()
  case "wrong manifest":old:=job.recovery;m:=*old;job.recovery=&m;defer func(){job.recovery=old}()
  case "wrong duration":job.event.Duration++
  case "replaced witness":job.execution=newEventExecutionProvenance(bm,job)
  case "capture replay":p.beginCapture(bm,job)
  case "assembly replay":p.beginAssembly(bm,job,filepath.Join(job.dir,"event.mp4"))
  case "publication replay":p.published(bm,job,context.Background())
  }
  job.keepStaging=false
  if p.qualifies(bm,job) {t.Fatal("invalid identity/replay accepted",kind)}
  if err:=check.check(context.Background(),lease);err==nil {t.Fatal("bound revalidation accepted changed execution",kind)}
 })}
 for _,kind:=range []string{"assembly before capture","finish before entry","join before assembly","manual"} {t.Run(kind,func(t *testing.T){
  bm,job:=publicationCheckpointJob(t);p:=job.execution
  switch kind {
  case "assembly before capture":p.beginAssembly(bm,job,filepath.Join(job.dir,"event.mp4"))
  case "finish before entry":p.finishCapture(bm,job,context.Background())
  case "join before assembly":p.joinAssembly(bm,job,context.Background(),nil)
  case "manual":job.execution=nil
  }
  if p.qualifies(bm,job) {t.Fatal("out of order/manual job qualified")}
  p.beginCapture(bm,job)
  if p.stage!=eventExecutionRefused {t.Fatal("refusal reset by later entry")}
 })}
 var absent *eventExecutionProvenance
 if absent.qualifies(nil,&bufferEvent{keepStaging:false}) {t.Fatal("manual zero witness accepted")}
}

func TestExecutionProvenanceActualWriterFailureIsSticky(t *testing.T) {
 bm,job:=publicationCheckpointJob(t)
 p:=job.execution
 if err:=job.writeOwnedJobRecoveryContext(context.Background(),"wrong directory",[]byte("{}"));err==nil {t.Fatal("wrong directory accepted")}
 p.beginCapture(bm,job)
 if p.stage!=eventExecutionRefused {t.Fatal("failed writer observation reset")}
}
