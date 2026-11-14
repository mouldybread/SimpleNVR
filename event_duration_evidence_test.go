package main

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)

func TestEventVideoSpanStrictStreamEvidenceAndExactBoundary(t *testing.T) {
 for _,tc:=range []struct{name,raw string;valid,enough bool}{
  {"exact",`{"streams":[{"codec_type":"video","duration_ts":30000,"time_base":"1/1000"}]}`,true,true},
  {"one tick short",`{"streams":[{"codec_type":"video","duration_ts":29999,"time_base":"1/1000"}]}`,true,false},
  {"fractional base",`{"streams":[{"codec_type":"video","duration_ts":15000,"time_base":"2/1000"}]}`,true,true},
  {"large exact products",`{"streams":[{"codec_type":"video","duration_ts":9223372036854775807,"time_base":"9223372036854775807/9223372036854775807"}]}`,true,true},
  {"container fallback forbidden",`{"streams":[{"codec_type":"video"}],"format":{"duration":"999"}}`,false,false},
  {"decimal duration forbidden",`{"streams":[{"codec_type":"video","duration":"30"}]}`,false,false},
  {"no stream",`{"streams":[]}`,false,false},
  {"audio",`{"streams":[{"codec_type":"audio","duration_ts":30000,"time_base":"1/1000"}]}`,false,false},
  {"two streams",`{"streams":[{"codec_type":"video"},{"codec_type":"video"}]}`,false,false},
  {"negative ticks",`{"streams":[{"codec_type":"video","duration_ts":-1,"time_base":"1/1000"}]}`,false,false},
  {"zero ticks",`{"streams":[{"codec_type":"video","duration_ts":0,"time_base":"1/1000"}]}`,false,false},
  {"exponent",`{"streams":[{"codec_type":"video","duration_ts":3e4,"time_base":"1/1000"}]}`,false,false},
  {"overflow ticks",`{"streams":[{"codec_type":"video","duration_ts":9223372036854775808,"time_base":"1/1000"}]}`,false,false},
  {"zero base",`{"streams":[{"codec_type":"video","duration_ts":30000,"time_base":"1/0"}]}`,false,false},
  {"negative base",`{"streams":[{"codec_type":"video","duration_ts":30000,"time_base":"-1/1000"}]}`,false,false},
  {"malformed base",`{"streams":[{"codec_type":"video","duration_ts":30000,"time_base":"1/1000/1"}]}`,false,false},
  {"trailing bytes",`{"streams":[]}garbage`,false,false},
  {"oversized",strings.Repeat(" ",maxRecoveryManifestBytes+1),false,false},
 } {t.Run(tc.name,func(t *testing.T){
  span,err:=decodeEventVideoSpan([]byte(tc.raw))
  if (err==nil)!=tc.valid {t.Fatal("validity",err)}
  if tc.valid && span.atLeast(30)!=tc.enough {t.Fatal("duration rounded or boundary changed",span)}
 })}
 for _,span:=range []eventVideoSpan{{},{-1,1,1},{30,0,1},{30,1,0}} {if span.atLeast(30) {t.Fatal("invalid span accepted",span)}}
 for _,seconds:=range []int{0,-1,maxEventSeconds+1} {if (eventVideoSpan{3600,1,1}).atLeast(seconds) {t.Fatal("invalid request accepted")}}
}
func installEventSpanProbe(t *testing.T,body string) {
 t.Helper();bin:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(bin,"ffprobe"),[]byte("#!/bin/sh\n"+body),0700);err!=nil {t.Fatal(err)}
 t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
func TestEventVideoSpanProbeUsesHeldDescriptorNotReplacedName(t *testing.T) {
 dir:=t.TempDir();path:=filepath.Join(dir,"capture.mp4")
 file,err:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_RDWR,0600);if err!=nil {t.Fatal(err)};defer file.Close()
 if _,err:=file.WriteString("descriptor evidence");err!=nil {t.Fatal(err)}
 if err:=os.Rename(path,path+".saved");err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("foreign replacement"),0600);err!=nil {t.Fatal(err)}
 installEventSpanProbe(t,"case \"$*\" in *duration_ts,time_base*) ;; *) exit 8;; esac\n[ \"$(cat /proc/self/fd/3)\" = 'descriptor evidence' ] || exit 9\nprintf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"duration_ts\":30000,\"time_base\":\"1/1000\"}]}'\n")
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
 span,err:=probeEventVideoSpan(ctx,file);if err!=nil || !span.atLeast(30) {t.Fatal(span,err)}
 assertPublicationBytes(t,path,"foreign replacement")
 assertPublicationBytes(t,path+".saved","descriptor evidence")
 if _,err:=file.Stat();err!=nil {t.Fatal("borrowed pin closed",err)}
}
func TestEventVideoSpanProbeRefusesProcessAndParserFailures(t *testing.T) {
 for _,body:=range []string{"exit 1\n","printf '%s' '{\"format\":{\"duration\":\"30\"}}'\n","printf garbage\n"} {t.Run(body,func(t *testing.T){
  installEventSpanProbe(t,body)
  file,err:=os.CreateTemp(t.TempDir(),"capture");if err!=nil {t.Fatal(err)};defer file.Close()
  if _,err:=file.WriteString("retained");err!=nil {t.Fatal(err)}
  ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
  if _,err:=probeEventVideoSpan(ctx,file);err==nil {t.Fatal("probe failure accepted")}
  canceled,stop:=context.WithCancel(context.Background());stop()
  if _,err:=probeEventVideoSpan(canceled,file);!errors.Is(err,context.Canceled) {t.Fatal(err)}
  assertPublicationBytes(t,file.Name(),"retained")
 })}
}

// The production paths in this fixture use synthetic process witnesses; these
// component observations are NOT proof of camera footage or full coverage.
func TestEventDurationEvidenceReadOnlySameLeaseAndTerminalLifetime(t *testing.T) {
 _,bm,job,_,lease:=releaseRevalidationFixture(t)
 borrowed:=job.recoveryBorrow;ended:=borrowed.ctx
 before:=eventStorageTree(t,job.dir)
 installEventSpanProbe(t,"printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"duration_ts\":30000,\"time_base\":\"1/1000\"}]}'\n")
 evidence,err:=inspectEventDurationEvidence(context.Background(),lease,bm,job);if err!=nil {t.Fatal(err)}
 if !errors.Is(ended.Err(),context.Canceled) || borrowed.ctx!=ended || borrowed.lease!=lease || !lease.held() || job.recoveryFailed {t.Fatal("inspection changed metadata budget/ownership")}
 if !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal("duration inspection changed staging")}
 // Inspection's own context has ended normally; the receipt does not retain it.
 if err:=evidence.check(context.Background(),lease);err!=nil {t.Fatal(err)}
 bm.releaseJob(job)
 if err:=evidence.check(context.Background(),lease);err==nil {t.Fatal("closed job evidence reusable")}
 if !job.keepStaging || len(bm.slots)!=0 || job.inventory!=nil || job.recoveryBorrow!=nil || lease.held() {t.Fatal("inspection disturbed terminal retention/teardown")}
 assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"synthetic output")
}
func TestEventDurationEvidenceRefusesShortCaptureAndOutputSeparately(t *testing.T) {
 for _,which:=range []string{"capture","published","probe failure","cancel during probe","modified input","replaced output"} {t.Run(which,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  if which=="modified input" {if err:=os.WriteFile(filepath.Join(job.dir,"event.mp4"),[]byte("changed recovery bytes"),0600);err!=nil {t.Fatal(err)}}
  if which=="replaced output" {if err:=os.Rename(check.output.path,check.output.path+".saved");err!=nil {t.Fatal(err)};putStagingAdmissionFile(t,check.output.path,"replacement")}
  before:=eventStorageTree(t,job.dir)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  calls:=0
  evidence,err:=inspectEventDurationEvidenceWithProbe(ctx,lease,bm,job,func(ctx context.Context,file *os.File)(eventVideoSpan,error){
   calls++
   if file!=job.inventory.entries["event.mp4"].file && file!=check.output.file {t.Error("unowned probe descriptor")}
   if which=="probe failure" {return eventVideoSpan{},os.ErrInvalid}
   if which=="cancel during probe" {cancel()}
   if (which=="capture" && calls==1) || (which=="published" && calls==2) {return eventVideoSpan{29999,1,1000},nil}
   return eventVideoSpan{30000,1,1000},nil
  })
  if err==nil || evidence!=nil {t.Fatal("insufficient/invalid evidence accepted",which)}
  if which=="capture" && calls!=1 {t.Fatal("short capture needlessly probed output",calls)}
  if (which=="modified input" || which=="replaced output") && calls!=0 {t.Fatal("changed ownership reached probe")}
  if which=="cancel during probe" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
  if !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) || !lease.held() || job.recoveryFailed {t.Fatal("read-only refusal changed evidence/ownership/persistence")}
 })}
}
func TestEventDurationEvidenceDefaultBindingsAndCancellation(t *testing.T) {
 _,bm,job,_,lease:=releaseRevalidationFixture(t)
 var probeContext context.Context
 calls:=0
 probe:=func(ctx context.Context,_ *os.File)(eventVideoSpan,error){
  calls++
  if probeContext==nil {probeContext=ctx} else if ctx!=probeContext {t.Error("per-file inspection budget renewed")}
  if _,ok:=ctx.Deadline();!ok {t.Error("unbounded probe context")}
  return eventVideoSpan{30,1,1},nil
 }
 e,err:=inspectEventDurationEvidenceWithProbe(context.Background(),lease,bm,job,probe);if err!=nil {t.Fatal(err)}
 if calls!=2 || !errors.Is(probeContext.Err(),context.Canceled) {t.Fatal("aggregate probe lifetime",calls)}
 canceled,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=inspectEventDurationEvidenceWithProbe(canceled,lease,bm,job,probe);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 for _,invalid:=range []*bufferEvent{nil,{}, {inventory:job.inventory}} {
  if evidence,err:=inspectEventDurationEvidenceWithProbe(context.Background(),lease,bm,invalid,probe);err==nil || evidence!=nil {t.Fatal("manual/unprocessed job accepted")}
 }
 if _,err:=inspectEventDurationEvidenceWithProbe(context.Background(),nil,bm,job,probe);err==nil {t.Fatal("nil lease accepted")}
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 if _,err:=inspectEventDurationEvidenceWithProbe(context.Background(),foreign,bm,job,probe);err==nil {t.Fatal("foreign lease accepted")}
 copy:=*job;e.job=&copy
 if err:=e.check(context.Background(),lease);err==nil {t.Fatal("copied job substituted")};e.job=job
 old:=e.capture;e.capture=nil
 if err:=e.check(context.Background(),lease);err==nil {t.Fatal("missing capture pin accepted")};e.capture=old
 job.event.Duration++
 if err:=e.check(context.Background(),lease);err==nil {t.Fatal("changed request accepted")}
 job.event.Duration--
 bm.cancel()
 if err:=e.check(context.Background(),lease);err==nil {t.Fatal("stopped manager accepted")}
 if !lease.held() || !foreign.held() {t.Fatal("inspection released a borrowed lease")}
}

func TestEventVideoSpanRealMP4StreamTicks(t *testing.T) {
 for _,tool:=range []string{"ffmpeg","ffprobe"} {if _,err:=exec.LookPath(tool);err!=nil {if os.Getenv("SIMPLENVR_REQUIRE_FFMPEG")=="1" {t.Fatal(err)};t.Skip("media tool unavailable")}}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 path:=filepath.Join(t.TempDir(),"one-second.mp4")
 cmd:=exec.CommandContext(ctx,"ffmpeg","-hide_banner","-loglevel","error","-f","lavfi","-i","color=c=black:s=64x64:r=10","-t","1","-c:v","mpeg4","-y",path)
 if raw,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("generate MP4: %v %s",err,raw)}
 file,err:=os.Open(path);if err!=nil {t.Fatal(err)};defer file.Close()
 span,err:=probeEventVideoSpan(ctx,file);if err!=nil {t.Fatal(err)}
 if !span.atLeast(1) || span.atLeast(2) {t.Fatal("real video-stream duration boundary",span)}
}
