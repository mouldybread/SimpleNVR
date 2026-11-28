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
 "strings"
 "testing"
 "time"
)

// Qualification fixture only. Observed segment notifications are simulated;
// media, owner descriptors, publication and probe receipts use real code paths.
func TestCorrResourceNativePrebufferQualification(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate native resource job only")}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT")
 if destination=="" {t.Fatal("explicit artifact destination required")}
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 binary,err=filepath.Abs(binary);if err!=nil {t.Fatal(err)}
 type row struct {
  Name,Stage,Error string
  Prefixes,Repeat int
  Seconds float64
  Collection corrStreamReport
 }
 report:=struct{
  Base,Head,GoVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Rows []row
  FailedCases []string
 }{Base:"24f16a34b0dc3f416097d408dfdbf5a8a56f54d7",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"native 128x72 H264/AAC; zero B frames; simulated completion notifications and file-copy camera input; native assembly substitutions for negative controls; no wall-clock coverage or host resource acceptance"}
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  if err:=os.WriteFile(filepath.Join(filepath.Dir(destination),"native-prebuffer.json"),append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 seedDir:=t.TempDir()
 run:=func(args ...string){
  t.Helper();ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
  cmd:=exec.CommandContext(ctx,binary,append([]string{"-hide_banner","-loglevel","error","-nostdin"},args...)...)
  cmd.WaitDelay=2*time.Second
  diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
  if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatal("native fixture generation",err,diagnostic.String())}
 }
 seeds:=make([]string,4)
 for i,duration:=range []string{"1.6","1.6","6.4","3.2"} {
  seeds[i]=filepath.Join(seedDir,fmt.Sprintf("source-%d.mp4",i))
  // Multiples of 1.6s align both 10fps frames and 1024-sample AAC packets at
  // 48kHz. Together with the input offset, this avoids fractional join tails.
  hue,frequency:=i*50,400+i*200;if i==3 {hue,frequency=100,800}
  run("-f","lavfi","-i","testsrc2=size=128x72:rate=10:duration="+duration,"-f","lavfi","-i",fmt.Sprintf("sine=frequency=%d:sample_rate=48000",frequency),"-t",duration,"-vf",fmt.Sprintf("hue=h=%d",hue),"-c:v","libx264","-preset","ultrafast","-threads","1","-g","16","-bf","0","-pix_fmt","yuv420p","-af","asetpts=PTS+1024","-c:a","aac","-b:a","64k","-shortest",seeds[i])
 }
 cases:=[]struct{name,mutation string;prefixes int;paced bool}{
  {"one-prefix-idle","",1,false},{"one-prefix-paced","",1,true},
  {"two-prefix-idle","",2,false},{"two-prefix-paced","",2,true},
  {"omit-first","omit-first",2,false},{"omit-second","omit-second",2,false},
  {"duplicate-first","duplicate-first",2,false},{"reorder","reorder",2,false},
  {"capture-only","capture-only",2,false},{"truncate-capture","truncate-capture",2,false},
 }
 for _,tc:=range cases {
  ok:=t.Run(tc.name,func(t *testing.T){
   bm:=bufferFixture(t)
   now:=time.Now()
   clock:=segmentClockSession{id:"native-prebuffer-fixture",startReturned:now.Add(-5*time.Second)}
   for i:=0;i<tc.prefixes;i++ {
    path:=filepath.Join(bm.camera.RecordDir,fmt.Sprintf("completed-%d.mp4",i))
    data,err:=os.ReadFile(seeds[i]);if err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(path,data,0600);err!=nil {t.Fatal(err)}
    observation:=clock.notice(now.Add(time.Duration(-3+i)*time.Second))
    observation.mediaBegin=float64(i)*1.6;observation.mediaEnd=float64(i+1)*1.6
    start:=now.Add(time.Duration(-tc.prefixes+i)*1600*time.Millisecond)
    if !bm.registerCompletedObserved(path,start,start.Add(1600*time.Millisecond),observation) {t.Fatal("native prefix registration")}
   }
   bin:=t.TempDir()
   plan:=filepath.Join(bin,"negative.txt")
   inputs:=append([]string(nil),seeds[:tc.prefixes]...);inputs=append(inputs,seeds[2])
   switch tc.mutation {
   case "omit-first":inputs=[]string{seeds[1],seeds[2]}
   case "omit-second":inputs=[]string{seeds[0],seeds[2]}
   case "duplicate-first":inputs=[]string{seeds[0],seeds[0],seeds[2]}
   case "reorder":inputs=[]string{seeds[1],seeds[0],seeds[2]}
   case "capture-only":inputs=[]string{seeds[2]}
   case "truncate-capture":inputs=[]string{seeds[0],seeds[1],seeds[3]}
   }
   var text strings.Builder
   for _,input:=range inputs {fmt.Fprintf(&text,"file '%s'\n",input)}
   if err:=os.WriteFile(plan,[]byte(text.String()),0600);err!=nil {t.Fatal(err)}
   script:="#!/bin/sh\nconcat=0\nfor arg do\n if [ \"$arg\" = -rtsp_transport ]; then exec /bin/cat \"$CORR_NATIVE_CAPTURE\" > /proc/self/fd/3; fi\n [ \"$arg\" != concat.txt ] || concat=1\n target=\"$arg\"\ndone\nif [ \"$concat\" = 1 ] && [ \"$CORR_NATIVE_MUTATE\" = 1 ]; then\n exec \"$CORR_NATIVE_FFMPEG\" -hide_banner -loglevel error -nostdin -f concat -safe 0 -i \"$CORR_NATIVE_PLAN\" -c copy -f mp4 -movflags faststart -y \"$target\"\nfi\nexec \"$CORR_NATIVE_FFMPEG\" \"$@\"\n"
   if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
   t.Setenv("CORR_NATIVE_FFMPEG",binary);t.Setenv("CORR_NATIVE_CAPTURE",seeds[2]);t.Setenv("CORR_NATIVE_PLAN",plan)
   mutate:="0";if tc.mutation!="" {mutate="1"};t.Setenv("CORR_NATIVE_MUTATE",mutate)
   t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
   event:=bufferTestEvent("native-prefix");event.Duration=6
   if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)}
   job:=<-bm.eventChan
   t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
   if len(job.preFiles)!=tc.prefixes {t.Fatal("wrong native prefix selection",len(job.preFiles))}
   capture:=bm.recordEventDuration(job);if capture=="" {t.Fatal("native capture failed")}
   final:=bm.concatenateEventRecording(job,capture);if final=="" {t.Fatal("native publication failed")}
   if job.recoveryBorrow==nil {t.Fatal("missing publication ownership")}
   borrowed:=job.recoveryBorrow;e:=borrowed.duration;lease:=borrowed.lease
   if e==nil || borrowed.durationErr!=nil {
    report.Rows=append(report.Rows,row{Name:tc.name,Prefixes:tc.prefixes,Stage:"evidence refusal",Error:fmt.Sprint(borrowed.durationErr)})
    corrBoundLogPresentationFailure(t,lease,capture,final,event.Duration)
    // These controls must reach the comparator, not pass by an earlier refusal.
    if tc.mutation!="capture-only" && tc.mutation!="truncate-capture" {t.Fatal("native evidence prerequisites refused",borrowed.durationErr)}
   } else {
    if tc.mutation=="capture-only" || tc.mutation=="truncate-capture" {t.Fatal("necessary assembly bound accepted missing contribution")}
    tree:=eventStorageTree(t,job.dir)
    outputHash,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
    captureHash,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
    var load *corrBoundLoad
    if tc.paced {load=corrBoundStartLoad(t,binary,seeds[2],t.TempDir())}
    gate:=newCorrAdmissionGate()
    for repeat:=1;repeat<=2;repeat++ {
     started:=time.Now();collection,inspectErr:=gate.inspect(context.Background(),lease,e,nil,nil)
     r:=row{Name:tc.name,Prefixes:tc.prefixes,Repeat:repeat,Stage:"matched",Seconds:time.Since(started).Seconds(),Collection:collection}
     if inspectErr!=nil {r.Stage="comparison refusal";r.Error=inspectErr.Error()}
     report.Rows=append(report.Rows,r);corrStreamAssertJoined(t,collection)
     if tc.mutation=="" {
      if inspectErr!=nil || !collection.Video.Equal || !collection.Audio.Equal || collection.ChildrenStarted!=2 {t.Fatal("native prefix match failed",collection,inspectErr)}
     } else if !errors.Is(inspectErr,errCorrStreamMismatch) {t.Fatal("native content mutation not detected by comparator",collection,inspectErr)}
     if len(gate.slot)!=0 || !lease.held() || borrowed.duration!=e || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("native inspection changed ownership/staging")}
     afterOutput,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
     afterCapture,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
     if afterOutput!=outputHash || afterCapture!=captureHash {t.Fatal("native inspection changed media")}
     if load!=nil {select {case <-load.done:t.Fatal("background exited during comparison",load.err);default:}}
    }
    if load!=nil {load.stop()}
   }
   inventory:=job.inventory;bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("native terminal invariant")}
   for _,path:=range append(append([]string(nil),job.preFiles...),capture,final) {if _,err:=os.Stat(path);err!=nil {t.Fatal("native evidence removed",err)}}
  })
  if !ok {report.FailedCases=append(report.FailedCases,tc.name)}
 }
 if len(report.Rows)!=18 {t.Errorf("incomplete native qualification matrix: %d of 18 rows",len(report.Rows))}
}
