package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "runtime"
 "testing"
 "time"
)

// Full job-bound native track controls. No production launch or media flags.
func TestCorrResourceNativeTrackIsolation(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate native resource job only")}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT")
 if destination=="" {t.Fatal("explicit artifact destination required")}
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 binary,err=filepath.Abs(binary);if err!=nil {t.Fatal(err)}
 type row struct {
  Name,Stage,Error string
  Repeat int
  Seconds float64
  Collection corrStreamReport
 }
 report:=struct{
  Base,Head,GoVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Rows []row
  FailedCases []string
 }{Base:"b4daa50a9d92f4aa6ade4dd388c2c9bf788ff441",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"native 360p H264/AAC track isolation; no prefix; 6.4s generated/6s requested; file-copy capture; prebuilt wrong native output substitution; no recorder-host or ingestion-time qualification"}
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  if err:=os.WriteFile(filepath.Join(filepath.Dir(destination),"native-track-isolation.json"),append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 dir:=t.TempDir()
 media:=func(args ...string){
  t.Helper();ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
  cmd:=exec.CommandContext(ctx,binary,append([]string{"-hide_banner","-loglevel","error","-nostdin"},args...)...)
  cmd.WaitDelay=2*time.Second
  diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
  if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatal("native track generation",err,diagnostic.String())}
 }
 paths:=make([]string,5)
 for i:=0;i<4;i++ {
  hue,frequency:=0,600
  if i==1 || i==3 {hue=90};if i==2 || i==3 {frequency=900}
  paths[i]=filepath.Join(dir,fmt.Sprintf("track-%d.mp4",i))
  media("-f","lavfi","-i","testsrc2=size=640x360:rate=10:duration=6.4","-f","lavfi","-i",fmt.Sprintf("sine=frequency=%d:sample_rate=48000",frequency),"-t","6.4","-vf",fmt.Sprintf("hue=h=%d",hue),"-c:v","libx264","-preset","ultrafast","-threads","1","-g","16","-bf","0","-pix_fmt","yuv420p","-af","asetpts=PTS+1024","-c:a","aac","-b:a","64k","-shortest",paths[i])
 }
 paths[4]=filepath.Join(dir,"missing-audio.mp4")
 media("-i",paths[0],"-map","0:v:0","-c","copy",paths[4])
 // Establish independent per-track positive and negative controls OUTSIDE
 // job setup and measurement. Do not infer unaffected-track equality from an
 // early-cancelled two-stream report, and do not make an owner receipt here.
 normalized:=make([][2]*os.File,4)
 for i:=0;i<4;i++ {
  donor,err:=os.Open(paths[i]);if err!=nil {t.Fatal(err)}
  for kind:=0;kind<2;kind++ {
   dst,err:=os.CreateTemp(dir,"normalized-");if err!=nil {donor.Close();t.Fatal(err)}
   t.Cleanup(func(){_=dst.Close()});normalized[i][kind]=dst
   ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
   counters:=corrPrototypeReport{}
   normalizeErr:=corrPrototypeNormalize(ctx,[]*os.File{donor},false,kind==1,dst,&counters)
   cancel()
   if normalizeErr!=nil {donor.Close();t.Fatal("track preflight normalization",i,kind,normalizeErr)}
  }
  if err:=donor.Close();err!=nil {t.Fatal(err)}
 }
 for i:=0;i<4;i++ {for kind:=0;kind<2;kind++ {
  ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
  equal,_,err:=corrPrototypeEqual(ctx,normalized[0][kind],normalized[i][kind]);cancel()
  want:=i==0 || (i==1 && kind==1) || (i==2 && kind==0)
  if err!=nil || equal!=want {t.Fatal("track isolation control invalid",i,kind,equal,want,err)}
 }}
 for i,name:=range []string{"normal","video-changed","audio-changed","both-changed","missing-audio"} {
  ok:=t.Run(name,func(t *testing.T){
   bin:=t.TempDir()
   script:="#!/bin/sh\nconcat=0\nfor arg do\n if [ \"$arg\" = -rtsp_transport ]; then exec /bin/cat \"$CORR_TRACK_CAPTURE\" > /proc/self/fd/3; fi\n [ \"$arg\" != concat.txt ] || concat=1\n target=\"$arg\"\ndone\nif [ \"$concat\" = 1 ] && [ \"$CORR_TRACK_MUTATE\" = 1 ]; then\n exec /bin/cat \"$CORR_TRACK_OUTPUT\" > \"$target\"\nfi\nexec \"$CORR_TRACK_FFMPEG\" \"$@\"\n"
   if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
   t.Setenv("CORR_TRACK_CAPTURE",paths[0]);t.Setenv("CORR_TRACK_OUTPUT",paths[i]);t.Setenv("CORR_TRACK_FFMPEG",binary)
   mutate:="0";if i!=0 {mutate="1"};t.Setenv("CORR_TRACK_MUTATE",mutate)
   t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
   bm:=bufferFixture(t)
   event:=bufferTestEvent("native-track");event.Duration=6
   if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)}
   job:=<-bm.eventChan;t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
   capture:=bm.recordEventDuration(job);if capture=="" {t.Fatal("native track capture")}
   final:=bm.concatenateEventRecording(job,capture);if final=="" {t.Fatal("native track publication")}
   if job.recoveryBorrow==nil {t.Fatal("native track ownership absent")}
   borrowed:=job.recoveryBorrow;e:=borrowed.duration;lease:=borrowed.lease
   if e==nil || borrowed.durationErr!=nil {
    report.Rows=append(report.Rows,row{Name:name,Stage:"unexpected evidence refusal",Error:fmt.Sprint(borrowed.durationErr)})
    corrBoundLogPresentationFailure(t,lease,capture,final,event.Duration)
    t.Fatal("track control did not reach comparator",borrowed.durationErr)
   }
   if e.presentation==nil || !job.execution.qualifies(bm,job) || len(job.execution.selection.inputs)!=0 {t.Fatal("native track prerequisites")}
   tree:=eventStorageTree(t,job.dir)
   outputHash,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
   captureHash,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   for repeat:=1;repeat<=2;repeat++ {
    started:=time.Now();collection,inspectErr:=gate.inspect(context.Background(),lease,e,nil,nil)
    r:=row{Name:name,Repeat:repeat,Stage:"matched",Seconds:time.Since(started).Seconds(),Collection:collection}
    if inspectErr!=nil {r.Stage="comparison refusal";r.Error=inspectErr.Error()}
    report.Rows=append(report.Rows,r);corrStreamAssertJoined(t,collection)
    if collection.ChildrenStarted!=2 || collection.PipeDescriptorsCreated!=8 {t.Fatal("control bypassed native collector",collection,inspectErr)}
    if i==0 {
     if inspectErr!=nil || !collection.Video.Equal || !collection.Audio.Equal {t.Fatal("native track positive control",collection,inspectErr)}
    } else if i<4 {
     if !errors.Is(inspectErr,errCorrStreamMismatch) {t.Fatal("changed track escaped content comparison",collection,inspectErr)}
    } else if inspectErr==nil || errors.Is(inspectErr,context.DeadlineExceeded) || errors.Is(inspectErr,context.Canceled) {
     t.Fatal("missing audio accepted or timed out instead of refusing",collection,inspectErr)
    }
    if len(gate.slot)!=0 || !lease.held() || borrowed.duration!=e || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("native track owner/staging changed")}
    afterOutput,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
    afterCapture,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
    if afterOutput!=outputHash || afterCapture!=captureHash {t.Fatal("native track media changed")}
   }
   inventory:=job.inventory;bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("native track terminal invariant")}
   for _,path:=range []string{capture,final} {if _,err:=os.Stat(path);err!=nil {t.Fatal("native track evidence lost",err)}}
  })
  if !ok {report.FailedCases=append(report.FailedCases,name)}
 }
 if len(report.Rows)!=10 {t.Errorf("incomplete native track matrix: %d of 10 rows",len(report.Rows))}
}
