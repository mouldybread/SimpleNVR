package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "runtime"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

type corrBoundProfile struct {Name,Size,Bitrate,Buffer string;Seconds,FPS int}
func corrBoundProfiles()[]corrBoundProfile {return []corrBoundProfile{
 {"360p_14s_1mbps","640x360","1M","2M",14,10},
 {"720p_110s_2mbps","1280x720","2M","4M",110,15},
}}
const corrBoundRequestHeadroomSeconds=1

// Whole-file test assertions outside measurement; never packet hashes or receipts.
func corrBoundDigest(file *os.File)(string,error) {
 info,err:=file.Stat();if err!=nil {return "",err}
 h:=sha256.New()
 if _,err:=io.Copy(h,io.NewSectionReader(file,0,info.Size()));err!=nil {return "",err}
 return fmt.Sprintf("%x",h.Sum(nil)),nil
}
func corrBoundLoadBytes(dir string)(int64,error) {
 entries,err:=os.ReadDir(dir);if err!=nil {return 0,err}
 var total int64
 for _,entry:=range entries {info,err:=entry.Info();if err!=nil {return 0,err};if !info.Mode().IsRegular() {return 0,os.ErrInvalid};total+=info.Size()}
 return total,nil
}

type corrBoundLoad struct {
 cmd *exec.Cmd
 cancel context.CancelFunc
 done chan struct{}
 err error // read only after done closes
}
func (load *corrBoundLoad) stop(){load.cancel();<-load.done}
func corrBoundStartLoad(t *testing.T,binary,source,dir string)*corrBoundLoad {
 t.Helper()
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second)
 cmd:=exec.CommandContext(ctx,binary,"-hide_banner","-loglevel","error","-nostdin","-re","-stream_loop","-1","-i",source,"-t","40","-map","0:v:0","-map","0:a:0","-c","copy","-f","segment","-segment_format","mpegts","-segment_time","1","-reset_timestamps","1",filepath.Join(dir,"load-%03d.ts"))
 cmd.WaitDelay=2*time.Second
 diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes}
 cmd.Stderr=diagnostic
 if err:=cmd.Start();err!=nil {cancel();t.Fatal("load start",err)}
 load:=&corrBoundLoad{cmd:cmd,cancel:cancel,done:make(chan struct{})}
 go func(){defer close(load.done);load.err=cmd.Wait()}()
 t.Cleanup(load.stop)
 timer:=time.NewTimer(5*time.Second);defer timer.Stop()
 tick:=time.NewTicker(20*time.Millisecond);defer tick.Stop()
 for {
  select {
  case <-load.done:t.Fatal("load exited before readiness",load.err,diagnostic.String())
  case <-timer.C:t.Fatal("load did not produce a completed segment")
  case <-tick.C:
   // A second nonempty segment means the first segment has been closed.
   info,err:=os.Stat(filepath.Join(dir,"load-001.ts"))
   if err==nil && info.Size()>0 {return load}
   if err!=nil && !os.IsNotExist(err) {t.Fatal(err)}
  }
 }
}

type corrBoundRow struct {
 Profile,Mode,Outcome,Error string
 Repeat int
 GeneratedSeconds,RequestedSeconds int
 AdapterSeconds,CollectorSeconds float64
 ChildUserCPUSeconds,ChildSystemCPUSeconds,CollectorUserCPUSeconds,CollectorSystemCPUSeconds float64
 ChildInputBlockOps,ChildOutputBlockOps,CollectorInputBlockOps,CollectorOutputBlockOps int64
 BackgroundBytesBefore,BackgroundBytesAfter int64
 BackgroundRunningBefore,BackgroundRunningAfter bool
 Collection corrStreamReport
}
type corrBoundLoadSummary struct {Profile string;Bytes int64;UserCPUSeconds,SystemCPUSeconds float64;Joined bool}

func TestCorrBoundProfileDefinitions(t *testing.T) {
 p:=corrBoundProfiles()
 if len(p)!=2 || p[0].Seconds!=14 || p[1].Seconds!=110 {t.Fatal(p)}
 for _,profile:=range p {if profile.Seconds<=corrBoundRequestHeadroomSeconds || profile.Seconds>maxEventSeconds || profile.FPS<1 {t.Fatal(profile)}}
}

func TestCorrResourceBoundAdapterLoad(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate resource job only")}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT")
 if destination=="" {t.Fatal("explicit report destination required")}
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 binary,err=filepath.Abs(binary);if err!=nil {t.Fatal(err)}
 if _,err:=exec.LookPath("ffprobe");err!=nil {t.Fatal(err)}
 report:=struct{
  Base,Head,GoVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Rows []corrBoundRow
  Background []corrBoundLoadSummary
  FailedProfiles []string
 }{Base:"16edded98d893094a0552e2334ee604e44f4c8e0",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"full admitted adapter; synthetic native H264/AAC with 1024-sample audio input offset at 48 kHz; injected file-copy capture only; generated 14s/110s, requested 13s/109s; no prebuffer; idle then paced private segment writer; two repeats; cache-warm; no recorder-host acceptance"}
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  path:=filepath.Join(filepath.Dir(destination),"bound-adapter.json")
  if err:=os.WriteFile(path,append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 for _,profile:=range corrBoundProfiles() {
  ok:=t.Run(profile.Name,func(t *testing.T){
   seed:=filepath.Join(t.TempDir(),"capture.mp4")
   genCtx,genCancel:=context.WithTimeout(context.Background(),120*time.Second);defer genCancel()
   // Synthetic input alignment only: compensate the native AAC encoder's
   // 1024-sample priming at the fixed 48 kHz fixture rate. Never a runtime fallback.
   cmd:=exec.CommandContext(genCtx,binary,"-hide_banner","-loglevel","error","-nostdin","-f","lavfi","-i",fmt.Sprintf("testsrc2=size=%s:rate=%d:duration=%d",profile.Size,profile.FPS,profile.Seconds),"-f","lavfi","-i","sine=frequency=600:sample_rate=48000","-t",fmt.Sprint(profile.Seconds),"-c:v","libx264","-preset","ultrafast","-threads","1","-g",fmt.Sprint(profile.FPS),"-bf","0","-pix_fmt","yuv420p","-b:v",profile.Bitrate,"-minrate",profile.Bitrate,"-maxrate",profile.Bitrate,"-bufsize",profile.Buffer,"-x264-params","nal-hrd=cbr","-af","asetpts=PTS+1024","-c:a","aac","-b:a","64k","-shortest","-fs",fmt.Sprint(corrPrototypeInputLimit),seed)
   cmd.WaitDelay=2*time.Second
   diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
   if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatal("native generation",err,diagnostic.String())}
   // Substitute only the camera input. All other FFmpeg calls exec the saved
   // native binary; FFprobe, publication and witnesses remain production paths.
   bin:=t.TempDir()
   script:="#!/bin/sh\nfor arg do\n if [ \"$arg\" = -rtsp_transport ]; then\n  exec /bin/cat \"$CORR_BOUND_CAPTURE\" > /proc/self/fd/3\n fi\ndone\nexec \"$CORR_BOUND_FFMPEG\" \"$@\"\n"
   if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
   t.Setenv("CORR_BOUND_CAPTURE",seed);t.Setenv("CORR_BOUND_FFMPEG",binary)
   t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
   bm:=bufferFixture(t)
   event:=bufferTestEvent("native-bound");event.Duration=profile.Seconds-corrBoundRequestHeadroomSeconds
   t.Logf("native fixture generated_seconds=%d requested_seconds=%d",profile.Seconds,event.Duration)
   if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)}
   job:=<-bm.eventChan
   t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
   capture:=bm.recordEventDuration(job);if capture=="" {t.Fatal("native capture registration failed")}
   final:=bm.concatenateEventRecording(job,capture);if final=="" {t.Fatal("native assembly/publication failed")}
   if job.recoveryBorrow==nil {t.Fatal("publication did not retain ownership")}
   borrowed:=job.recoveryBorrow;e:=borrowed.duration;lease:=borrowed.lease
   if e==nil || borrowed.durationErr!=nil {
    corrBoundLogPresentationFailure(t,lease,capture,final,event.Duration)
    t.Fatal("native combined evidence refused",borrowed.durationErr)
   }
   if e.presentation==nil || !job.execution.qualifies(bm,job) || len(job.execution.selection.inputs)!=0 {t.Fatal("native full-adapter prerequisites missing")}
   tree:=eventStorageTree(t,job.dir)
   captureHash,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
   outputHash,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
   gate:=newCorrAdmissionGate()
   for _,mode:=range []string{"idle","paced segment writer"} {
    var load *corrBoundLoad;var loadDir string
    if mode!="idle" {loadDir=t.TempDir();load=corrBoundStartLoad(t,binary,seed,loadDir)}
    for repeat:=1;repeat<=2;repeat++ {
     row:=corrBoundRow{Profile:profile.Name,Mode:mode,Repeat:repeat,GeneratedSeconds:profile.Seconds,RequestedSeconds:event.Duration,Outcome:"matched"}
     if load!=nil {
      select {case <-load.done:t.Fatal("background exited before measurement",load.err);default:row.BackgroundRunningBefore=true}
      row.BackgroundBytesBefore,err=corrBoundLoadBytes(loadDir);if err!=nil {t.Fatal(err)}
     }
     var cb,ca,pb,pa unix.Rusage
     if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&cb);err!=nil {t.Fatal(err)}
     if err:=unix.Getrusage(unix.RUSAGE_SELF,&pb);err!=nil {t.Fatal(err)}
     started:=time.Now()
     collection,inspectErr:=gate.inspect(context.Background(),lease,e,nil,nil)
     row.AdapterSeconds=time.Since(started).Seconds();row.CollectorSeconds=collection.Elapsed.Seconds();row.Collection=collection
     if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&ca);err!=nil {t.Fatal(err)}
     if err:=unix.Getrusage(unix.RUSAGE_SELF,&pa);err!=nil {t.Fatal(err)}
     row.ChildUserCPUSeconds=corrResourceCPU(ca.Utime)-corrResourceCPU(cb.Utime);row.ChildSystemCPUSeconds=corrResourceCPU(ca.Stime)-corrResourceCPU(cb.Stime)
     row.CollectorUserCPUSeconds=corrResourceCPU(pa.Utime)-corrResourceCPU(pb.Utime);row.CollectorSystemCPUSeconds=corrResourceCPU(pa.Stime)-corrResourceCPU(pb.Stime)
     row.ChildInputBlockOps=ca.Inblock-cb.Inblock;row.ChildOutputBlockOps=ca.Oublock-cb.Oublock
     row.CollectorInputBlockOps=pa.Inblock-pb.Inblock;row.CollectorOutputBlockOps=pa.Oublock-pb.Oublock
     if inspectErr!=nil {row.Outcome="refused";row.Error=inspectErr.Error()}
     if load!=nil {
      select {case <-load.done:default:row.BackgroundRunningAfter=true}
      row.BackgroundBytesAfter,err=corrBoundLoadBytes(loadDir);if err!=nil {t.Fatal(err)}
     }
     report.Rows=append(report.Rows,row)
     t.Logf("bound profile=%s mode=%s repeat=%d outcome=%s adapter_seconds=%.6f collector_seconds=%.6f",profile.Name,mode,repeat,row.Outcome,row.AdapterSeconds,row.CollectorSeconds)
     corrStreamAssertJoined(t,collection)
     if inspectErr!=nil || !collection.Video.Equal || !collection.Audio.Equal || collection.ChildrenStarted!=2 || collection.PipeDescriptorsCreated!=8 || len(gate.slot)!=0 {t.Fatal("native admitted adapter failed",collection,inspectErr)}
     if load!=nil && (!row.BackgroundRunningBefore || !row.BackgroundRunningAfter) {t.Fatal("background workload did not overlap full adapter")}
     currentCapture,err:=corrBoundDigest(e.capture);if err!=nil {t.Fatal(err)}
     currentOutput,err:=corrBoundDigest(e.output.file);if err!=nil {t.Fatal(err)}
     if currentCapture!=captureHash || currentOutput!=outputHash || !lease.held() || borrowed.duration!=e || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("inspection changed evidence or ownership")}
    }
    if load!=nil {
     load.stop()
     usage:=load.cmd.ProcessState
     if usage==nil {t.Fatal("background was not joined")}
     bytes,err:=corrBoundLoadBytes(loadDir);if err!=nil {t.Fatal(err)}
     report.Background=append(report.Background,corrBoundLoadSummary{Profile:profile.Name,Bytes:bytes,UserCPUSeconds:usage.UserTime().Seconds(),SystemCPUSeconds:usage.SystemTime().Seconds(),Joined:true})
    }
   }
   inventory:=job.inventory
   bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("native owner terminal teardown failed")}
   for _,path:=range []string{capture,final} {if _,err:=os.Stat(path);err!=nil {t.Fatal("native evidence removed",err)}}
  })
  if !ok {report.FailedProfiles=append(report.FailedProfiles,profile.Name)}
 }
 if len(report.Rows)!=8 {t.Errorf("incomplete bound-adapter matrix: %d of 8 rows",len(report.Rows))}
}
