package main

import (
 "context"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func planClip(base time.Time,path string,start,end time.Duration) recordingInterval {
 return recordingInterval{CameraID:"front",Path:path,Start:base.Add(start),End:base.Add(end),Size:1}
}
func TestExportPlanIntersectionsAndOrdering(t *testing.T) {
 base:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 window:=timelineWindow{Start:base,End:base.Add(10*time.Second)}
 cases:=[]struct{name string;clips []recordingInterval;parts int;trim bool;in,out time.Duration}{
  {"whole",[]recordingInterval{planClip(base,"a",0,10*time.Second)},1,false,0,10*time.Second},
  {"inside-one",[]recordingInterval{planClip(base,"a",-5*time.Second,20*time.Second)},1,true,5*time.Second,15*time.Second},
  {"adjacent",[]recordingInterval{planClip(base,"b",5*time.Second,12*time.Second),planClip(base,"a",-2*time.Second,5*time.Second)},2,true,2*time.Second,7*time.Second},
  {"touching-outside",[]recordingInterval{planClip(base,"before",-2*time.Second,0),planClip(base,"a",0,10*time.Second),planClip(base,"after",10*time.Second,12*time.Second)},1,false,0,10*time.Second},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  first:=tc.clips[0].Path
  plan,err:=buildExportPlan(tc.clips,window);if err!=nil {t.Fatal(err)}
  if tc.clips[0].Path!=first || len(plan.Parts)!=tc.parts || plan.NeedsTrim!=tc.trim || plan.Parts[0].In!=tc.in || plan.Parts[0].Out!=tc.out {t.Fatalf("bad plan: %+v",plan)}
  var total time.Duration
  for _,part:=range plan.Parts {total+=part.End.Sub(part.Start);if part.In<0 || part.Out<=part.In {t.Fatal("invalid offsets")}}
  if total!=10*time.Second || plan.clips()[0].Path!="a" {t.Fatal("plan is not ordered exact requested coverage")}
 })}
 // Retain fractional boundaries rather than round to whole seconds.
 window=timelineWindow{Start:base.Add(500*time.Millisecond),End:base.Add(1500*time.Millisecond)}
 plan,err:=buildExportPlan([]recordingInterval{planClip(base,"a",0,2*time.Second)},window)
 if err!=nil || plan.Parts[0].In!=500*time.Millisecond || plan.Parts[0].Out!=1500*time.Millisecond {t.Fatal(plan,err)}
}
func TestExportPlanRejectsMissingAndAmbiguousCoverage(t *testing.T) {
 base:=time.Date(2026,11,10,0,0,0,0,time.UTC);window:=timelineWindow{Start:base,End:base.Add(10*time.Second)}
 for _,tc:=range []struct{name string;clips []recordingInterval;want error}{
  {"none",nil,errExportPlanGap},
  {"leading",[]recordingInterval{planClip(base,"a",time.Second,10*time.Second)},errExportPlanGap},
  {"trailing-partial",[]recordingInterval{planClip(base,"partial",0,9*time.Second)},errExportPlanGap},
  {"interior",[]recordingInterval{planClip(base,"a",0,4*time.Second),planClip(base,"b",5*time.Second,10*time.Second)},errExportPlanGap},
  {"overlap",[]recordingInterval{planClip(base,"a",0,6*time.Second),planClip(base,"b",5*time.Second,10*time.Second)},errExportPlanOverlap},
  {"duplicate",[]recordingInterval{planClip(base,"a",0,10*time.Second),planClip(base,"b",0,10*time.Second)},errExportPlanOverlap},
  {"zero-clip",[]recordingInterval{planClip(base,"a",0,0)},errExportPlanInvalid},
 } {t.Run(tc.name,func(t *testing.T){if _,err:=buildExportPlan(tc.clips,window);!errors.Is(err,tc.want) {t.Fatal(err)}})}
 a:=planClip(base,"a",0,5*time.Second);b:=planClip(base,"b",5*time.Second,10*time.Second);b.CameraID="other"
 if _,err:=buildExportPlan([]recordingInterval{a,b},window);!errors.Is(err,errExportPlanInvalid) {t.Fatal("mixed cameras accepted")}
 if _,err:=buildExportPlan([]recordingInterval{a},timelineWindow{Start:base,End:base});!errors.Is(err,errExportPlanInvalid) {t.Fatal("empty range accepted")}
 many:=make([]recordingInterval,maxPinnedExportInputs+1)
 if _,err:=buildExportPlan(many,window);!errors.Is(err,errExportInputLimit) {t.Fatal("input bound missing")}
}
func TestExportPlanRejectsBeforePinningOrVideoHeaders(t *testing.T) {
 for _,kind:=range []string{"gap","overlap"} {t.Run(kind,func(t *testing.T){
  dir:=t.TempDir();exportClipFixture(t,dir,"20261110_000000_a.mp4","media")
  if kind=="overlap" {exportClipFixture(t,dir,"20261110_000001_b.mp4","media")}
  n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
  cache:=newDurationCache(2,1,func(context.Context,*os.File)(time.Duration,error){return 2*time.Second,nil})
  pool:=newExportInputPool(1,2);opened:=false
  pool.open=func(string)(*os.File,error){opened=true;return nil,os.ErrNotExist}
  end:="2026-11-10T00:00:03Z";if kind=="gap" {end="2026-11-10T00:00:04Z"}
  response:=httptest.NewRecorder();serveDownload(response,httptest.NewRequest("GET","/api/download?camera_id=front&start_time=2026-11-10T00:00:00Z&end_time="+end,nil),n,cache,pool)
  if response.Code!=409 || response.Header().Get("X-Export-Plan-Status")!=kind || opened || strings.HasPrefix(response.Header().Get("Content-Type"),"video/") || response.Header().Get("Content-Disposition")!="" {t.Fatal("invalid plan reached streaming",response.Code,response.Header())}
 })}
}
func TestWholeClipExportDeclaresPendingTrimExecution(t *testing.T) {
 dir:=t.TempDir();clips:=syntheticExportClips(t,dir)
 for index,path:=range []string{"20261110_000000_a.mp4","20261110_000002_b.mp4"} {if err:=os.Rename(clips[index].Path,filepath.Join(dir,path));err!=nil {t.Fatal(err)}}
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir();cache:=newDurationCache(2,2,probeVideoDuration)
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 request:=httptest.NewRequest("GET","/api/download?camera_id=front&start_time=2026-11-10T00:00:00.5Z&end_time=2026-11-10T00:00:03.5Z",nil).WithContext(ctx)
 response:=httptest.NewRecorder();serveDownload(response,request,n,cache,pool)
 if response.Code!=200 || response.Header().Get("X-Export-Range-Policy")!="whole-clips-untrimmed" || response.Header().Get("X-Export-Trim-Required")!="true" || response.Header().Get("X-Export-Requested-Start")!="2026-11-10T00:00:00.5Z" || response.Header().Get("X-Export-Media-End")!="2026-11-10T00:00:04Z" {t.Fatal("incorrect range policy",response.Code,response.Header())}
 output:=filepath.Join(dir,"output.mp4");if err:=os.WriteFile(output,response.Body.Bytes(),0600);err!=nil {t.Fatal(err)}
 file,err:=os.Open(output);if err!=nil {t.Fatal(err)};defer file.Close()
 duration,err:=probeVideoDuration(ctx,file)
 if err!=nil || duration<3900*time.Millisecond || duration>4100*time.Millisecond {t.Fatal("whole-clip output was misrepresented as trimmed",duration,err)}
 if len(pool.slots)!=0 {t.Fatal("export slot leaked")}
}
