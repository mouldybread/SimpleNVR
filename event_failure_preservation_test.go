package main

import (
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestPartialPinFailurePreservesEarlierLinks(t *testing.T) {
 bm:=bufferFixture(t)
 source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
 if err:=os.WriteFile(source,[]byte("recoverable prebuffer"),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bm.buffer=[]BufferSegment{
  {StartTime:now.Add(-10*time.Second),EndTime:now.Add(-5*time.Second),FilePath:source},
  {StartTime:now.Add(-5*time.Second),EndTime:now,FilePath:filepath.Join(bm.camera.RecordDir,"missing.mp4")},
 }
 if err:=bm.TriggerEvent(bufferTestEvent("pin-failed"));err==nil {t.Fatal("missing source accepted")}
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("failed admission leaked slot or queued work")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatalf("expected retained staging: %v %v",entries,err)}
 if err:=os.Remove(source);err!=nil {t.Fatal(err)}
 pinned:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name(),"pre_000.mp4")
 got,err:=os.ReadFile(pinned);if err!=nil || string(got)!="recoverable prebuffer" {t.Fatalf("earlier pin lost: %q %v",got,err)}
}

func TestFirstPinFailureRemovesOnlyEmptyStaging(t *testing.T) {
 bm:=bufferFixture(t)
 now:=time.Now().UTC()
 bm.buffer=[]BufferSegment{{StartTime:now.Add(-time.Second),EndTime:now,FilePath:filepath.Join(bm.camera.RecordDir,"missing.mp4")}}
 if err:=bm.TriggerEvent(bufferTestEvent("first-pin-failed"));err==nil {t.Fatal("missing source accepted")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=0 {t.Fatalf("empty staging not removed: %v %v",entries,err)}
 if len(bm.slots)!=0 {t.Fatal("slot leaked")}
}

func TestQueueRefusalPreservesPinnedInputs(t *testing.T) {
 bm:=bufferFixture(t)
 // Force the otherwise defensive queue-refusal path without concurrent workers.
 bm.eventChan=make(chan *bufferEvent)
 source:=filepath.Join(bm.camera.RecordDir,"source.mp4")
 if err:=os.WriteFile(source,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(source,now.Add(-time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("queue-refused"));err!=errEventBusy {t.Fatalf("expected queue refusal: %v",err)}
 if len(bm.slots)!=0 {t.Fatal("slot leaked")}
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatalf("staging lost: %v %v",entries,err)}
 bm.pruneBefore(now.Add(time.Second))
 pinned:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name(),"pre_000.mp4")
 if info,err:=os.Stat(pinned);err!=nil || info.Size()!=2048 {t.Fatalf("pin lost after pruning: %v",err)}
}

// These executables deliberately do not produce or validate media. They test
// real process/publication/cleanup control flow, not FFmpeg correctness.
func installEventFailureTools(t *testing.T,exitCode int,probeFails bool) {
 t.Helper()
 bin:=t.TempDir()
 ffmpeg:=fmt.Sprintf("#!/bin/sh\nfor last do :; done\nprintf 'synthetic output' > \"$last\"\nexit %d\n",exitCode)
 probe:="#!/bin/sh\nprintf 'synthetic-probe-result\\n'\n"
 if probeFails {probe="#!/bin/sh\nexit 1\n"}
 for name,body:=range map[string]string{"ffmpeg":ffmpeg,"ffprobe":probe} {
  if err:=os.WriteFile(filepath.Join(bin,name),[]byte(body),0700);err!=nil {t.Fatal(err)}
 }
 t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAssemblyFailurePublicationPreservesStaging(t *testing.T) {
 for _,tc:=range []struct{name string;exitCode int;probeFails bool;collision bool;wantPublished bool;wantKeep bool}{
  {name:"successful assembly",wantPublished:true},
  {name:"failed assembly readable partial",exitCode:1,wantPublished:true,wantKeep:true},
  {name:"failed output validation",probeFails:true,wantKeep:true},
  {name:"publication collision",collision:true,wantKeep:true},
 } {
  t.Run(tc.name,func(t *testing.T){
   installEventFailureTools(t,tc.exitCode,tc.probeFails)
   bm:=bufferFixture(t)
   event:=bufferTestEvent("assembly")
   job,err:=bm.pinBuffer(event);if err!=nil {t.Fatal(err)}
   bm.slots<-struct{}{}
   input:=filepath.Join(job.dir,"event.mp4")
   if err:=os.WriteFile(input,[]byte("original capture evidence"),0600);err!=nil {t.Fatal(err)}
   name:=fmt.Sprintf("%s_%s_%s.mp4",job.preStart.UTC().Format("20060102_150405"),sanitizeFilename(event.Label),sanitizeFilename(event.ID))
   expected:=filepath.Join(bm.camera.RecordDir,name)
   if tc.exitCode!=0 {expected=partialRecordingName(expected)}
   if tc.collision {if err:=os.WriteFile(expected,[]byte("existing evidence"),0600);err!=nil {t.Fatal(err)}}
   final:=bm.concatenateEventRecording(job,input)
   if (final!="")!=tc.wantPublished {t.Fatalf("publication=%q",final)}
   if final!="" && final!=expected {t.Fatalf("wrong output path: %q",final)}
   if job.keepStaging!=tc.wantKeep {t.Fatalf("keepStaging=%v",job.keepStaging)}
   bm.releaseJob(job)
   if len(bm.slots)!=0 {t.Fatal("slot leaked")}
   if tc.wantKeep {
    got,err:=os.ReadFile(input);if err!=nil || string(got)!="original capture evidence" {t.Fatalf("input lost: %q %v",got,err)}
    if _,err:=os.Stat(filepath.Join(job.dir,"concat.txt"));err!=nil {t.Fatal("concat list lost",err)}
   } else if _,err:=os.Stat(job.dir);!os.IsNotExist(err) {t.Fatalf("successful staging not cleaned: %v",err)}
   if tc.wantPublished {if got,err:=os.ReadFile(expected);err!=nil || string(got)!="synthetic output" {t.Fatalf("output lost: %q %v",got,err)}}
   if tc.collision {if got,err:=os.ReadFile(expected);err!=nil || string(got)!="existing evidence" {t.Fatalf("existing output changed: %q %v",got,err)}}
   if tc.probeFails || tc.collision {if got,err:=os.ReadFile(filepath.Join(bm.camera.RecordDir,name)+".tmp");err!=nil || string(got)!="synthetic output" {t.Fatalf("unpublished output lost: %q %v",got,err)}}
  })
 }
}

func TestCanceledAssemblyRetainsStaging(t *testing.T) {
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("canceled"));if err!=nil {t.Fatal(err)}
 bm.slots<-struct{}{}
 input:=filepath.Join(job.dir,"event.mp4")
 if err:=os.WriteFile(input,[]byte("capture evidence"),0600);err!=nil {t.Fatal(err)}
 bm.cancel()
 if final:=bm.concatenateEventRecording(job,input);final!="" {t.Fatalf("unexpected publication: %q",final)}
 if !job.keepStaging {t.Fatal("canceled assembly authorized cleanup")}
 bm.releaseJob(job)
 if got,err:=os.ReadFile(input);err!=nil || string(got)!="capture evidence" {t.Fatalf("canceled input lost: %q %v",got,err)}
 if len(bm.slots)!=0 {t.Fatal("slot leaked")}
}

func TestFailedCaptureRetainsEvenEmptyStaging(t *testing.T) {
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("capture-failed"));if err!=nil {t.Fatal(err)}
 bm.slots<-struct{}{}
 // A canceled command cannot produce output; failure still must not authorize
 // recursive cleanup based on an output-size/stat guess.
 bm.cancel()
 if file:=bm.recordEventDuration(job);file!="" {t.Fatalf("unexpected capture: %q",file)}
 if !job.keepStaging {t.Fatal("failed capture authorized cleanup")}
 bm.releaseJob(job)
 if _,err:=os.Stat(job.dir);err!=nil {t.Fatal("failed capture staging lost",err)}
 if len(bm.slots)!=0 {t.Fatal("slot leaked")}
}
