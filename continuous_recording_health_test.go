package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "html/template"
 "log"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
)

const upstreamFixture = "[rtsp @ fixture] method DESCRIBE failed: 404 (Not Found)\nrtsp://dummy:SECRET@example.invalid/private?token=HIDDEN\n"

func fakeContinuousTools(t *testing.T,body string) {
 t.Helper();dir:=t.TempDir()
 for name,source:=range map[string]string{"ffmpeg":"#!/bin/sh\n"+body,"ffprobe":"#!/bin/sh\nprintf 'h264\\n1\\n'\n"} {
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(source),0700);err!=nil {t.Fatal(err)}
 }
 t.Setenv("PATH",dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
func noticeFrom(t *testing.T,err error) continuousRecordingNotice {
 t.Helper();var failure *continuousRecordingError
 if !errors.As(err,&failure) {t.Fatalf("expected safe recording failure, got %v",err)}
 return failure.notice
}
func noDiagnosticSecrets(t *testing.T,text string) {
 t.Helper()
 for _,bad:=range []string{"SECRET","HIDDEN","example.invalid","rtsp://dummy","lstat",".mp4.tmp"} {if strings.Contains(text,bad) {t.Errorf("diagnostic leaked %q: %s",bad,text)}}
}

func TestContinuousStderrTailBounded(t *testing.T) {
 var tail recordingStderrTail
 tail.Write(bytes.Repeat([]byte("x"),recordingStderrLimit*3))
 tail.Write([]byte(upstreamFixture))
 if len(tail.data)!=recordingStderrLimit || !bytes.HasSuffix(tail.data,[]byte(upstreamFixture)) {t.Fatal("incorrect tail")}
}

func TestContinuousUpstreamFailureAndCameraResponse(t *testing.T) {
 fakeContinuousTools(t,"printf '%s\\n' '[rtsp @ fixture] method DESCRIBE failed: 404 (Not Found)' 'rtsp://dummy:SECRET@example.invalid/private?token=HIDDEN' >&2\nexit 8\n")
 cam:=&Camera{ID:"affected",Name:"HALL2",RecordDir:t.TempDir(),RecordingMode:ModeContinuous,IsRecording:true}
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 worker:=beginContinuousHealth(ctx,cam);defer endContinuousHealth(cam,worker)
 err:=recordSegment(ctx,cam,1)
 notice:=noticeFrom(t,err)
 if notice.Code!="upstream_rtsp_unavailable" || notice.Detail!="DESCRIBE 404" || notice.PublicationMessage!="" {t.Fatalf("%+v",notice)}
 noDiagnosticSecrets(t,err.Error())
 updateContinuousHealth(cam,worker,err);updateContinuousHealth(cam,worker,err)
 other:=&Camera{ID:"other",Name:"other",RecordDir:t.TempDir(),RecordingMode:ModeContinuous,IsRecording:true}
 if snapshotContinuousHealth(other).Code!="" {t.Fatal("unaffected camera has error")}
 oldNVR,oldTemplates:=nvr,templates
 defer func(){nvr=oldNVR;templates=oldTemplates}()
 nvr=&NVR{cameras:map[string]*Camera{cam.ID:cam,other.ID:other},cameraOrder:[]string{cam.ID,other.ID}}
 var parseErr error
 templates,parseErr=template.ParseFiles("templates/cameras.html")
 if parseErr!=nil {t.Fatal(parseErr)}
 w:=httptest.NewRecorder();camerasHandler(w,httptest.NewRequest("GET","https://nvr.test/api/cameras",nil))
 if w.Code!=200 || strings.Count(w.Body.String(),`class="camera-recording-error"`)!=1 || !strings.Contains(w.Body.String(),"DESCRIBE 404") {t.Fatalf("%d %s",w.Code,w.Body.String())}
 noDiagnosticSecrets(t,w.Body.String())
 raw,_:=json.Marshal(cam)
 if strings.Contains(string(raw),"upstream_rtsp") {t.Fatal("transient health persisted")}
 var logs bytes.Buffer;oldWriter:=log.Writer();log.SetOutput(&logs);defer log.SetOutput(oldWriter)
 runContinuousRecorder(ctx,cam,func(context.Context,*Camera,int) error{return err},func(context.Context) bool{return false})
 if !strings.Contains(logs.String(),"DESCRIBE 404") {t.Fatal("safe diagnosis not logged")}
 noDiagnosticSecrets(t,logs.String())
}

func TestContinuousPublicationOutcomes(t *testing.T) {
 for _,tc:=range []struct{name,body,code string;partial bool}{
  {"success missing output","exit 0\n","recording_output_missing",false},
  {"generic failure","echo 'rtsp://dummy:SECRET@example.invalid/private' >&2\nexit 8\n","recorder_failed",false},
  {"valid partial salvage","for output do :; done\nprintf 'partial bytes' > \"$output\"\nexit 8\n","recorder_failed",true},
  {"normal publication","for output do :; done\nprintf 'normal bytes' > \"$output\"\nexit 0\n","",false},
 } {t.Run(tc.name,func(t *testing.T){
  fakeContinuousTools(t,tc.body)
  cam:=&Camera{ID:"test",Name:"test",RecordDir:t.TempDir()}
  err:=recordSegment(context.Background(),cam,1)
  if tc.code=="" {if err!=nil {t.Fatal(err)}} else if got:=noticeFrom(t,err);got.Code!=tc.code {t.Fatalf("%+v",got)}
  if err!=nil {noDiagnosticSecrets(t,err.Error())}
  files,globErr:=filepath.Glob(filepath.Join(cam.RecordDir,"????????","*.mp4"));if globErr!=nil {t.Fatal(globErr)}
  want:=0;if tc.partial || tc.code=="" {want=1}
  if len(files)!=want {t.Fatalf("published outputs=%v, want %d",files,want)}
  for _,file:=range files {
   stamp,err:=parseStampedName(filepath.Base(file));if err!=nil {t.Fatal("unstamped output",err)}
   if filepath.Base(filepath.Dir(file))!=stampedDayName(stamp.Start) || strings.HasSuffix(file,"_partial.mp4")!=tc.partial {t.Fatal("wrong day or partial flag",file)}
  }
  loose,globErr:=filepath.Glob(filepath.Join(cam.RecordDir,"*.mp4"));if globErr!=nil || len(loose)!=0 {t.Fatal("flat output published",loose,globErr)}
 })}
}

func TestContinuousStorageFailuresRemainVisible(t *testing.T) {
 parent:=context.Background();dir:=t.TempDir();temp:=filepath.Join(dir,"output.mp4.tmp")
 for _,pubErr:=range []error{
  &os.PathError{Op:"lstat",Path:filepath.Join(dir,"destination.mp4"),Err:os.ErrNotExist},
  &os.LinkError{Op:"rename",Old:temp,New:filepath.Join(dir,"destination.mp4"),Err:os.ErrNotExist},
  &os.PathError{Op:"open",Path:temp,Err:os.ErrPermission},
 } {
  err:=continuousAttemptResult(parent,parent,errors.New("process failure"),pubErr,temp,[]byte(upstreamFixture))
  if noticeFrom(t,err).PublicationMessage=="" {t.Fatal("publication error hidden")}
  if !errors.Is(err,pubErr) {t.Fatal("lost underlying publication error")}
  noDiagnosticSecrets(t,err.Error())
 }
 missing:=filepath.Join(dir,"absent-parent","output.mp4.tmp")
 err:=continuousAttemptResult(parent,parent,errors.New("process failure"),&os.PathError{Op:"lstat",Path:missing,Err:os.ErrNotExist},missing,[]byte(upstreamFixture))
 if noticeFrom(t,err).PublicationMessage=="" {t.Fatal("missing storage directory hidden")}
 err=continuousAttemptResult(parent,parent,nil,os.ErrPermission,temp,nil)
 if noticeFrom(t,err).Code!="recording_publication_failed" {t.Fatal("publication-only failure misclassified")}
}

func TestContinuousWorkerRecoveryCancellationAndReplacement(t *testing.T) {
 cam:=&Camera{ID:"test"};ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 old:=beginContinuousHealth(ctx,cam);defer endContinuousHealth(cam,old)
 failure:=&continuousRecordingError{notice:continuousRecordingNotice{Code:"upstream_rtsp_unavailable",Message:"safe"}}
 updateContinuousHealth(cam,old,failure)
 if !snapshotContinuousHealth(cam).Retrying {t.Fatal("failure not retrying")}
 updateContinuousHealth(cam,old,nil)
 if snapshotContinuousHealth(cam).Code!="" {t.Fatal("normal publication did not clear")}
 newer:=beginContinuousHealth(ctx,cam);defer endContinuousHealth(cam,newer)
 updateContinuousHealth(cam,newer,failure)
 updateContinuousHealth(cam,old,nil);endContinuousHealth(cam,old)
 if snapshotContinuousHealth(cam).Code=="" {t.Fatal("old worker cleared replacement")}
 replacement:=&Camera{ID:cam.ID}
 if snapshotContinuousHealth(replacement).Code!="" {t.Fatal("replacement inherited old status")}
 var wg sync.WaitGroup
 for i:=0;i<4;i++ {wg.Add(1);go func(){defer wg.Done();for j:=0;j<100;j++ {updateContinuousHealth(cam,newer,failure);snapshotContinuousHealth(cam)}}()}
 wg.Wait();cancel()
 if snapshotContinuousHealth(cam).Code!="" {t.Fatal("canceled worker still retrying")}
 err:=continuousAttemptResult(ctx,ctx,failure,os.ErrNotExist,"unused",nil)
 if !errors.Is(err,context.Canceled) {t.Fatal("intentional cancellation became failure")}
}

func TestContinuousTimeoutOnlyReportsCaptureDeadline(t *testing.T) {
 temp:=filepath.Join(t.TempDir(),"output.mp4.tmp")
 if err:=continuousAttemptOutcome(nil,nil,nil,nil,temp,nil);err!=nil {t.Fatalf("completed capture and publication reported %v",err)}
 for _,runErr:=range []error{nil,errors.New("signal: killed")} {
  err:=continuousAttemptOutcome(nil,context.DeadlineExceeded,runErr,nil,temp,[]byte(upstreamFixture))
  if got:=noticeFrom(t,err);got.Code!="recording_timeout" {t.Fatalf("deadline during capture misclassified: %+v",got)}
  if !errors.Is(err,context.DeadlineExceeded) {t.Fatal("deadline cause lost")}
  noDiagnosticSecrets(t,err.Error())
 }
 err:=continuousAttemptOutcome(nil,nil,nil,os.ErrPermission,temp,nil)
 if got:=noticeFrom(t,err);got.Code!="recording_publication_failed" {t.Fatalf("publication failure after completed capture misclassified: %+v",got)}
 err=continuousAttemptOutcome(context.Canceled,context.DeadlineExceeded,errors.New("signal: killed"),nil,temp,nil)
 if !errors.Is(err,context.Canceled) {t.Fatal("intentional cancellation became failure")}
}

func TestRecordSegmentFreezesCaptureOutcomeBeforePublication(t *testing.T) {
 raw,err:=os.ReadFile("recorder.go");if err!=nil {t.Fatal(err)}
 source:=string(raw)
 start,end:=strings.Index(source,"func recordSegment("),strings.Index(source,"func sanitizeFilename(")
 if start<0 || end<=start {t.Fatal("recordSegment not found")}
 body:=source[start:end]
 if strings.Count(body,"segmentCtx.Err()")!=1 {t.Fatal("capture context must be observed exactly once, when FFmpeg returns")}
 captureAt,publishAt:=strings.Index(body,"captureErr:=segmentCtx.Err()"),strings.Index(body,"publishStampedRecordingLeased(")
 if captureAt<0 || publishAt<0 || captureAt>publishAt {t.Fatal("capture outcome must be fixed before stamped publication")}
 if !strings.Contains(body,"continuousAttemptOutcome(ctx.Err(),captureErr,") || strings.Contains(body,"continuousAttemptResult(") {t.Fatal("recordSegment must classify with the frozen capture outcome")}
}
