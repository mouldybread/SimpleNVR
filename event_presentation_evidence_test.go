package main

import (
 "context"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)

func TestCombinedDurationPresentationRequiresStrictSameStream(t *testing.T) {
 raw:=strings.Replace(eventPresentationFixture,`"index":0`,`"index":0,"duration_ts":2000`,1)
 observation,err:=decodeEventDurationPresentation(context.Background(),[]byte(raw))
 if err!=nil || !observation.duration.atLeast(2) || !observation.presentation.continuity.atLeastSeconds(2) {t.Fatal(observation,err)}
 for _,bad:=range []string{
  eventPresentationFixture,
  strings.Replace(raw,`"duration_ts":2000`,`"duration_ts":2000,"duration_ts":2000`,1),
  strings.Replace(raw,`"duration_ts":2000`,`"duration_ts":"2000"`,1),
  strings.Replace(raw,`"duration_ts":2000`,`"duration_ts":0`,1),
  strings.Replace(raw,`"duration_ts":2000`,`"duration_ts":2e3`,1),
  strings.Replace(raw,`"stream_index":0`,`"stream_index":1`,1),
  strings.Replace(raw,`"pts":1000`,`"pts":1001`,1),
  strings.Replace(raw,`"index":0`,`"index":0,"unknown":1`,1),
 } {
  observation,err:=decodeEventDurationPresentation(context.Background(),[]byte(bad))
  if err==nil || observation!=(eventVideoProbeObservation{}) {t.Fatal(bad,observation,err)}
 }
 for _,path:=range []string{"tests/fixtures/event-combined-presentation-ffprobe-8.json","tests/fixtures/event-combined-libx264-ffprobe-8.json","tests/fixtures/event-combined-libx265-ffprobe-8.json"} {
  fixture,err:=os.ReadFile(path);if err!=nil {t.Fatal(err)}
  observation,err=decodeEventDurationPresentation(context.Background(),fixture)
  if err!=nil || observation.presentation.continuity.frames!=5 || !observation.duration.atLeast(1) || !observation.presentation.continuity.atLeastSeconds(1) || observation.duration.atLeast(2) {t.Fatal(path,observation,err)}
 }
}

func TestProductionPresentationReceiptReusedWithoutAdditionalProcesses(t *testing.T) {
 bm,job,counter:=durationIntegrationFixture(t,"success")
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
 final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("publication failed")}
 borrowed:=job.recoveryBorrow;lease:=borrowed.lease;e:=borrowed.duration
 if e==nil || e.presentation==nil || e.presentation.duration!=e || durationIntegrationCount(t,counter)!=2 {t.Fatal("combined production receipt missing")}
 p:=e.presentation
 check:=durationIntegrationCheck(bm,job)
 for i:=0;i<3;i++ {if err:=check.check(context.Background(),lease);err!=nil {t.Fatal(err)}}
 if check.presentation!=p || durationIntegrationCount(t,counter)!=2 {t.Fatal("revalidation substituted or re-probed")}
 bm.releaseJob(job)
 if durationIntegrationCount(t,counter)!=2 || borrowed.revalidationErr!=nil || job.recoveryFailed || !job.keepStaging || job.recoveryBorrow!=nil || job.inventory!=nil || len(bm.slots)!=0 || lease.held() {t.Fatal("terminal evidence observation changed teardown")}
 if err:=p.check(context.Background(),lease,e);err==nil {t.Fatal("terminal receipt reusable")}
 assertPublicationBytes(t,input,"synthetic output");assertPublicationBytes(t,final,"synthetic output")
}

func TestPresentationRevalidationBindsExactReceiptBeforeAndAfterIO(t *testing.T) {
 for _,kind:=range []string{"missing","substituted","wrong duration","short capture","short output","changed time base","removed during IO"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  e:=job.recoveryBorrow.duration;p:=e.presentation;saved:=*p;copy:=*p
  before:=eventStorageTree(t,job.dir)
  switch kind {
  case "missing":e.presentation=nil
  case "substituted":e.presentation=&copy
  case "wrong duration":other:=*e;p.duration=&other
  case "short capture":p.capture.continuity.end=p.capture.continuity.first+1
  case "short output":p.published.continuity.end=p.published.continuity.first+1
  case "changed time base":p.capture.continuity.denominator++
  }
  calls:=0
  err:=check.checkWithOutput(context.Background(),lease,func(ctx context.Context,lease *mediaMutationLease)error{
   calls++;if kind=="removed during IO" {e.presentation=nil};return check.output.check(ctx,lease)
  })
  wantCalls:=0;if kind=="removed during IO" {wantCalls=1}
  if err==nil || calls!=wantCalls || !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal(kind,err,calls)}
  *p=saved;e.presentation=p
  if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("genuine receipt refused",err)}
  bm.releaseJob(job)
  if !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("receipt refusal changed terminal retention")}
 })}
}

func TestPresentationReceiptRefusesDescriptorMetadataDrift(t *testing.T) {
 for _,which:=range []string{"capture","published"} {t.Run(which,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  path:=filepath.Join(job.dir,"event.mp4");if which=="published" {path=check.output.path}
  info,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
  if err:=os.Chtimes(path,info.ModTime(),info.ModTime().Add(time.Second));err!=nil {t.Fatal(err)}
  before:=eventStorageTree(t,job.dir)
  if err:=check.check(context.Background(),lease);err==nil {t.Fatal("descriptor drift accepted",which)}
  if !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal("read-only refusal changed bytes")}
  bm.releaseJob(job)
  if job.recoveryFailed || !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("descriptor drift changed persistence/teardown")}
  assertPublicationBytes(t,path,"synthetic output")
 })}
}

func TestDurationOnlyObservationCannotReplaceProductionPresentationReceipt(t *testing.T) {
 _,bm,job,_,lease:=releaseRevalidationFixture(t)
 borrowed:=job.recoveryBorrow;original:=borrowed.duration
 legacy,err:=inspectEventDurationEvidenceWithProbe(context.Background(),lease,bm,job,func(context.Context,*os.File)(eventVideoSpan,error){return eventVideoSpan{30,1,1},nil})
 if err!=nil || legacy.presentation!=nil {t.Fatal(legacy,err)}
 borrowed.duration=legacy
 check:=newEventReleaseRevalidation(nil,bm,job,false)
 if err:=check.durationCurrent(context.Background(),lease);err==nil {t.Fatal("duration-only evidence qualified as presentation")}
 borrowed.duration=original
 bm.releaseJob(job)
 if !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("legacy refusal disturbed teardown")}
}

func TestCombinedProbeSideDataIsOnlyAnEmptySelectedEnvelope(t *testing.T) {
 raw:=strings.Replace(eventPresentationFixture,`"index":0`,`"index":0,"duration_ts":2000`,1)
 for _,side:=range []string{`[]`,`[{}]`,`[{},{}]`} {
  value:=strings.Replace(raw,`"pts":0`,`"side_data_list":`+side+`,"pts":0`,1)
  if _,err:=decodeEventDurationPresentation(context.Background(),[]byte(value));err!=nil {t.Fatal(side,err)}
 }
 for _,side:=range []string{`null`,`{}`,`[null]`,`[[]]`,`[{"side_data_type":"anything"}]`,`[{}, {"x":0}]`} {
  value:=strings.Replace(raw,`"pts":0`,`"side_data_list":`+side+`,"pts":0`,1)
  if _,err:=decodeEventDurationPresentation(context.Background(),[]byte(value));err==nil {t.Fatal("nonempty/invalid side data accepted",side)}
 }
 for _,fields:=range []string{`"side_data_list":[],"side_data_list":[],`,`"side_data_list":[],"side_data_l\u0069st":[],`,`"unknown":[],`} {
  value:=strings.Replace(raw,`"pts":0`,fields+`"pts":0`,1)
  if _,err:=decodeEventDurationPresentation(context.Background(),[]byte(value));err==nil {t.Fatal("duplicate/unknown frame field accepted",fields)}
 }
}
