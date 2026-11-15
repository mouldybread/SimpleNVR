package main

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "runtime"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func TestCorrResourceCompleteValidationCost(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate resource job only")}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT");if destination=="" {t.Fatal("explicit report destination required")}
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 binary,err=filepath.Abs(binary);if err!=nil {t.Fatal(err)}
 type row struct {
  Profile,Mode,Error string
  Repeat,GeneratedSeconds,RequestedSeconds int
  ChildUserCPUSeconds,ChildSystemCPUSeconds,GoUserCPUSeconds,GoSystemCPUSeconds float64
  BackgroundRunningBefore,BackgroundRunningAfter bool
  Validation corrCompleteValidationReport
 }
 report:=struct{
  Base,Head,GoVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Rows []row
  FailedProfiles []string
 }{Base:"4ebb4d6a6e842c901be771fe14a2a7324d44efde",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"fresh descriptor-bound duration/presentation collection plus admitted streaming comparison; one 5s aggregate deadline; no prefix; cache-warm native aligned H264/AAC; excludes capture, assembly, publication and real recorder-host acceptance"}
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  if err:=os.WriteFile(filepath.Join(filepath.Dir(destination),"complete-validation.json"),append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 for _,profile:=range corrBoundProfiles() {
  ok:=t.Run(profile.Name,func(t *testing.T){
   seed:=filepath.Join(t.TempDir(),"capture.mp4")
   ctx,cancel:=context.WithTimeout(context.Background(),120*time.Second);defer cancel()
   cmd:=exec.CommandContext(ctx,binary,"-hide_banner","-loglevel","error","-nostdin","-f","lavfi","-i",fmt.Sprintf("testsrc2=size=%s:rate=%d:duration=%d",profile.Size,profile.FPS,profile.Seconds),"-f","lavfi","-i","sine=frequency=600:sample_rate=48000","-t",fmt.Sprint(profile.Seconds),"-c:v","libx264","-preset","ultrafast","-threads","1","-g",fmt.Sprint(profile.FPS),"-bf","0","-pix_fmt","yuv420p","-b:v",profile.Bitrate,"-minrate",profile.Bitrate,"-maxrate",profile.Bitrate,"-bufsize",profile.Buffer,"-x264-params","nal-hrd=cbr","-af","asetpts=PTS+1024","-c:a","aac","-b:a","64k","-shortest","-fs",fmt.Sprint(corrPrototypeInputLimit),seed)
   cmd.WaitDelay=2*time.Second
   diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
   if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatal("complete validation fixture generation",err,diagnostic.String())}
   bin:=t.TempDir()
   script:="#!/bin/sh\nfor arg do\n if [ \"$arg\" = -rtsp_transport ]; then exec /bin/cat \"$CORR_COMPLETE_CAPTURE\" > /proc/self/fd/3; fi\ndone\nexec \"$CORR_COMPLETE_FFMPEG\" \"$@\"\n"
   if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
   t.Setenv("CORR_COMPLETE_CAPTURE",seed);t.Setenv("CORR_COMPLETE_FFMPEG",binary)
   t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
   bm:=bufferFixture(t);event:=bufferTestEvent("complete-validation");event.Duration=profile.Seconds-corrBoundRequestHeadroomSeconds
   if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)}
   job:=<-bm.eventChan;t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
   capture:=bm.recordEventDuration(job);if capture=="" {t.Fatal("native capture")}
   final:=bm.concatenateEventRecording(job,capture);if final=="" || job.recoveryBorrow==nil {t.Fatal("native publication")}
   borrowed:=job.recoveryBorrow;original:=borrowed.duration;lease:=borrowed.lease
   if original==nil || borrowed.durationErr!=nil {t.Fatal("initial evidence unavailable",borrowed.durationErr)}
   tree:=eventStorageTree(t,job.dir)
   captureHash,err:=corrBoundDigest(original.capture);if err!=nil {t.Fatal(err)}
   outputHash,err:=corrBoundDigest(original.output.file);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   for _,mode:=range []string{"idle","paced segment writer"} {
    var load *corrBoundLoad
    if mode!="idle" {load=corrBoundStartLoad(t,binary,seed,t.TempDir())}
    for repeat:=1;repeat<=2;repeat++ {
     r:=row{Profile:profile.Name,Mode:mode,Repeat:repeat,GeneratedSeconds:profile.Seconds,RequestedSeconds:event.Duration}
     if load!=nil {select {case <-load.done:t.Fatal("background ended before chain",load.err);default:r.BackgroundRunningBefore=true}}
     var cb,ca,pb,pa unix.Rusage
     if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&cb);err!=nil {t.Fatal(err)}
     if err:=unix.Getrusage(unix.RUSAGE_SELF,&pb);err!=nil {t.Fatal(err)}
     validation,inspectErr:=gate.inspectComplete(bm.ctx,lease,bm,job,nil,nil)
     if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&ca);err!=nil {t.Fatal(err)}
     if err:=unix.Getrusage(unix.RUSAGE_SELF,&pa);err!=nil {t.Fatal(err)}
     r.Validation=validation
     r.ChildUserCPUSeconds=corrResourceCPU(ca.Utime)-corrResourceCPU(cb.Utime);r.ChildSystemCPUSeconds=corrResourceCPU(ca.Stime)-corrResourceCPU(cb.Stime)
     r.GoUserCPUSeconds=corrResourceCPU(pa.Utime)-corrResourceCPU(pb.Utime);r.GoSystemCPUSeconds=corrResourceCPU(pa.Stime)-corrResourceCPU(pb.Stime)
     if inspectErr!=nil {r.Error=inspectErr.Error()}
     if load!=nil {select {case <-load.done:default:r.BackgroundRunningAfter=true}}
     report.Rows=append(report.Rows,r);corrStreamAssertJoined(t,validation.Collection)
     t.Logf("complete profile=%s mode=%s repeat=%d evidence_seconds=%.6f comparison_seconds=%.6f total_seconds=%.6f error=%v",profile.Name,mode,repeat,validation.EvidenceSeconds,validation.ComparisonSeconds,validation.TotalSeconds,inspectErr)
     if inspectErr!=nil || !validation.EvidenceCompleted || !validation.Collection.Video.Equal || !validation.Collection.Audio.Equal || validation.Collection.ChildrenStarted!=2 || len(gate.slot)!=0 {t.Fatal("complete native validation refused",validation,inspectErr)}
     if load!=nil && (!r.BackgroundRunningBefore || !r.BackgroundRunningAfter) {t.Fatal("background did not overlap complete chain")}
     if !lease.held() || borrowed.duration!=original || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("fresh collection replaced stored evidence or changed ownership")}
     afterCapture,err:=corrBoundDigest(original.capture);if err!=nil {t.Fatal(err)}
     afterOutput,err:=corrBoundDigest(original.output.file);if err!=nil {t.Fatal(err)}
     if afterCapture!=captureHash || afterOutput!=outputHash {t.Fatal("complete validation changed media")}
    }
    if load!=nil {load.stop()}
   }
   inventory:=job.inventory;bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("complete validation terminal invariant")}
  })
  if !ok {report.FailedProfiles=append(report.FailedProfiles,profile.Name)}
 }
 if len(report.Rows)!=8 {t.Errorf("incomplete full validation matrix: %d of 8 rows",len(report.Rows))}
}
