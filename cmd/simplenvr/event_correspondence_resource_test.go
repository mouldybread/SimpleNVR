package main

// Opt-in CI resource experiment. Never called by recording/finalization.
import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

type corrResourceProfile struct {
 Name,Size,Bitrate,Buffer string
 FPS,PrefixSeconds,CaptureSeconds int
 ExpectStreamCeiling bool
}
func corrResourceProfiles()[]corrResourceProfile {
 return []corrResourceProfile{
  {"360p_14s_1mbps","640x360","1M","2M",10,2,10,false},
  {"720p_110s_2mbps","1280x720","2M","4M",15,10,90,false},
  {"1080p_90s_4mbps_ceiling","1920x1080","4M","8M",25,10,70,true},
 }
}

type corrResourceStreaming struct {
 Outcome,Error string
 Seconds,ChildUserCPUSeconds,ChildSystemCPUSeconds,CollectorUserCPUSeconds,CollectorSystemCPUSeconds float64
 ChildInputBlockOps,ChildOutputBlockOps,CollectorInputBlockOps,CollectorOutputBlockOps int64
 Collection corrStreamReport
}

type corrResourceRow struct {
 Streaming *corrResourceStreaming
 Profile corrResourceProfile
 Repeat int
 Outcome,Error string
 FixtureGenerationSeconds,CollectionSeconds float64
 SourceBytes,LogicalInputPassBytes int64
 ChildUserCPUSeconds,ChildSystemCPUSeconds float64
 ChildInputBlockOps,ChildOutputBlockOps int64
 CollectorUserCPUSeconds,CollectorSystemCPUSeconds float64
 CollectorInputBlockOps,CollectorOutputBlockOps int64
 ChildrenMaxRSSSinceProcessStartLinuxKiB int64
 Collection corrPrototypeReport
}
func corrResourceCPU(v unix.Timeval)float64 {return float64(v.Sec)+float64(v.Usec)/1e6}

// Use the actual green normalizer/comparator, not a second implementation.
// Independent of live-job bindings already tested in the safety matrix.
func corrResourceCollect(ctx context.Context,refs []*os.File,actual *os.File,scratch string)(report corrPrototypeReport,result error) {
 started:=time.Now();defer func(){report.Elapsed=time.Since(started)}()
 report.BorrowedMediaDescriptors=len(refs)+1
 var logical int64
 for _,file:=range append(append([]*os.File(nil),refs...),actual) {
  info,err:=file.Stat();if err!=nil {return report,err}
  if !info.Mode().IsRegular() || info.Size()<=0 || info.Size()>corrPrototypeInputLimit {return report,errCorrPrototype}
  logical+=2*info.Size()
 }
 report.InputSizePassBudget=logical
 if logical>corrPrototypeTotalInputLimit {return report,errCorrPrototype}
 for _,audio:=range []bool{false,true} {
  equal,err:=func()(equal bool,result error){
   a,err:=os.CreateTemp(scratch,"reference-");if err!=nil {return false,err}
   defer func(){result=errors.Join(result,a.Close(),os.Remove(a.Name()))}()
   b,err:=os.CreateTemp(scratch,"actual-");if err!=nil {return false,err}
   defer func(){result=errors.Join(result,b.Close(),os.Remove(b.Name()))}()
   report.PeakScratchDescriptors=2
   if err:=corrPrototypeNormalize(ctx,refs,true,audio,a,&report);err!=nil {return false,err}
   if err:=corrPrototypeNormalize(ctx,[]*os.File{actual},false,audio,b,&report);err!=nil {return false,err}
   equal,count,err:=corrPrototypeEqual(ctx,a,b);report.ComparedBytes+=count
   return equal,err
  }()
  if err!=nil {return report,err}
  if audio {report.AudioEqual=equal} else {report.VideoEqual=equal}
  if !equal {return report,errCorrPrototype}
 }
 return report,ctx.Err()
}

func TestCorrResourceProfileDefinitions(t *testing.T) {
 profiles:=corrResourceProfiles()
 if len(profiles)!=3 {t.Fatal(profiles)}
 if profiles[0].PrefixSeconds*2+profiles[0].CaptureSeconds!=14 || profiles[1].PrefixSeconds*2+profiles[1].CaptureSeconds!=110 || profiles[2].PrefixSeconds*2+profiles[2].CaptureSeconds!=90 {t.Fatal("duration labels drifted")}
 if profiles[0].ExpectStreamCeiling || profiles[1].ExpectStreamCeiling || !profiles[2].ExpectStreamCeiling {t.Fatal("ceiling expectation drifted")}
 if got:=corrResourceCPU(unix.Timeval{Sec:2,Usec:500000});got!=2.5 {t.Fatal(got)}
}

func TestCorrResourceProfiles(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate resource job only")}
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT")
 if destination=="" {t.Fatal("explicit report destination required")}
 report:=struct{
  Base,Head,GoVersion,FFmpegVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Rows []corrResourceRow
 }{Base:"3ad0281b2c54cbb5bd77cbd1819c88c619433f59",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"paired scratch versus streaming measurements; scratch first; cache-warm generated inputs; no runtime acceptance"}
 // Persist completed rows even after t.Fatal in a subtest. Job logs remain
 // available if a hard process/runner timeout prevents this deferred write.
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  if err:=os.WriteFile(destination,append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 for _,profile:=range corrResourceProfiles() {t.Run(profile.Name,func(t *testing.T){
  dir:=t.TempDir();genStarted:=time.Now()
  genCtx,cancelGen:=context.WithTimeout(context.Background(),120*time.Second);defer cancelGen()
  media:=func(args ...string){
   t.Helper()
   cmd:=exec.CommandContext(genCtx,binary,append([]string{"-hide_banner","-loglevel","error","-nostdin"},args...)...)
   cmd.WaitDelay=2*time.Second
   diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes}
   cmd.Stdout=diagnostic;cmd.Stderr=diagnostic
   if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatalf("fixture generation failed: %v %s",err,diagnostic.String())}
  }
  // Capture a bounded version string separately from measured collection.
  if report.FFmpegVersion=="" {
   cmd:=exec.CommandContext(genCtx,binary,"-version");cmd.WaitDelay=2*time.Second
   version:=&eventPresentationProbeBuffer{limit:16<<10};cmd.Stdout=version;cmd.Stderr=version
   if err:=cmd.Run();err!=nil || version.refused {t.Fatal("version unavailable",err)}
   report.FFmpegVersion=version.String()
  }
  paths:=[]string{}
  for i,seconds:=range []int{profile.PrefixSeconds,profile.PrefixSeconds,profile.CaptureSeconds} {
   name:=fmt.Sprintf("input-%d.mp4",i);path:=filepath.Join(dir,name);paths=append(paths,path)
   media("-f","lavfi","-i",fmt.Sprintf("testsrc2=size=%s:rate=%d:duration=%d",profile.Size,profile.FPS,seconds),"-f","lavfi","-i",fmt.Sprintf("sine=frequency=%d:sample_rate=48000",400+100*i),"-t",fmt.Sprint(seconds),"-vf",fmt.Sprintf("hue=h=%d",40*i),"-c:v","libx264","-preset","ultrafast","-threads","1","-g",fmt.Sprint(profile.FPS),"-bf","2","-pix_fmt","yuv420p","-b:v",profile.Bitrate,"-minrate",profile.Bitrate,"-maxrate",profile.Bitrate,"-bufsize",profile.Buffer,"-x264-params","nal-hrd=cbr","-c:a","aac","-b:a","64k","-shortest","-fs",fmt.Sprint(corrPrototypeInputLimit),path)
  }
  // Relative generated basenames avoid any quoting of temp-directory paths.
  plan:=filepath.Join(dir,"concat.txt")
  if err:=os.WriteFile(plan,[]byte("file 'input-0.mp4'\nfile 'input-1.mp4'\nfile 'input-2.mp4'\n"),0600);err!=nil {t.Fatal(err)}
  output:=filepath.Join(dir,"output.mp4")
  media("-f","concat","-safe","0","-i",plan,"-c","copy","-fs",fmt.Sprint(corrPrototypeInputLimit),output)
  generation:=time.Since(genStarted).Seconds()
  refs:=[]*os.File{};var sourceBytes int64
  for _,path:=range paths {
   file,err:=os.Open(path);if err!=nil {t.Fatal(err)};defer file.Close();refs=append(refs,file)
   stat,err:=file.Stat();if err!=nil {t.Fatal(err)};sourceBytes+=stat.Size()
  }
  actual,err:=os.Open(output);if err!=nil {t.Fatal(err)};defer actual.Close()
  for repeat:=1;repeat<=2;repeat++ {
   scratch:=t.TempDir()
   var before,after,selfBefore,selfAfter unix.Rusage
   if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&before);err!=nil {t.Fatal(err)}
   if err:=unix.Getrusage(unix.RUSAGE_SELF,&selfBefore);err!=nil {t.Fatal(err)}
   ctx,cancel:=context.WithTimeout(context.Background(),corrPrototypeBudget)
   measured,collectErr:=corrResourceCollect(ctx,refs,actual,scratch)
   cancel()
   if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&after);err!=nil {t.Fatal(err)}
   if err:=unix.Getrusage(unix.RUSAGE_SELF,&selfAfter);err!=nil {t.Fatal(err)}
   row:=corrResourceRow{Profile:profile,Repeat:repeat,Outcome:"matched",FixtureGenerationSeconds:generation,CollectionSeconds:measured.Elapsed.Seconds(),SourceBytes:sourceBytes,LogicalInputPassBytes:measured.InputSizePassBudget,ChildUserCPUSeconds:corrResourceCPU(after.Utime)-corrResourceCPU(before.Utime),ChildSystemCPUSeconds:corrResourceCPU(after.Stime)-corrResourceCPU(before.Stime),ChildInputBlockOps:after.Inblock-before.Inblock,ChildOutputBlockOps:after.Oublock-before.Oublock,ChildrenMaxRSSSinceProcessStartLinuxKiB:after.Maxrss,CollectorUserCPUSeconds:corrResourceCPU(selfAfter.Utime)-corrResourceCPU(selfBefore.Utime),CollectorSystemCPUSeconds:corrResourceCPU(selfAfter.Stime)-corrResourceCPU(selfBefore.Stime),CollectorInputBlockOps:selfAfter.Inblock-selfBefore.Inblock,CollectorOutputBlockOps:selfAfter.Oublock-selfBefore.Oublock,Collection:measured}
   if collectErr!=nil {row.Outcome="refused";row.Error=collectErr.Error()}
   var sb,sa,pb,pa unix.Rusage
   if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&sb);err!=nil {t.Fatal(err)}
   if err:=unix.Getrusage(unix.RUSAGE_SELF,&pb);err!=nil {t.Fatal(err)}
   streamCtx,streamCancel:=context.WithTimeout(context.Background(),corrPrototypeBudget)
   streamed,streamErr:=corrStreamRun(streamCtx,refs,actual,nil,nil,nil)
   streamCancel()
   if err:=unix.Getrusage(unix.RUSAGE_CHILDREN,&sa);err!=nil {t.Fatal(err)}
   if err:=unix.Getrusage(unix.RUSAGE_SELF,&pa);err!=nil {t.Fatal(err)}
   row.Streaming=&corrResourceStreaming{Outcome:"matched",Seconds:streamed.Elapsed.Seconds(),ChildUserCPUSeconds:corrResourceCPU(sa.Utime)-corrResourceCPU(sb.Utime),ChildSystemCPUSeconds:corrResourceCPU(sa.Stime)-corrResourceCPU(sb.Stime),CollectorUserCPUSeconds:corrResourceCPU(pa.Utime)-corrResourceCPU(pb.Utime),CollectorSystemCPUSeconds:corrResourceCPU(pa.Stime)-corrResourceCPU(pb.Stime),ChildInputBlockOps:sa.Inblock-sb.Inblock,ChildOutputBlockOps:sa.Oublock-sb.Oublock,CollectorInputBlockOps:pa.Inblock-pb.Inblock,CollectorOutputBlockOps:pa.Oublock-pb.Oublock,Collection:streamed}
   if streamErr!=nil {row.Streaming.Outcome="refused";row.Streaming.Error=streamErr.Error()}
   report.Rows=append(report.Rows,row)
   t.Logf("profile=%s repeat=%d outcome=%s seconds=%.3f children=%d normalized=%d compared=%d child_inblock=%d child_oublock=%d",profile.Name,repeat,row.Outcome,row.CollectionSeconds,measured.ProcessesStarted,measured.NormalizedBytes,measured.ComparedBytes,row.ChildInputBlockOps,row.ChildOutputBlockOps)
   entries,err:=os.ReadDir(scratch);if err!=nil || len(entries)!=0 {t.Fatal("scratch leak",entries,err)}
   corrStreamAssertJoined(t,streamed)
   if streamed.ChildrenStarted!=2 || streamed.PipeDescriptorsCreated!=8 {t.Fatal("streaming resource contract",streamed)}
   if profile.ExpectStreamCeiling {
    if !errors.Is(streamErr,errCorrStreamLimit) || errors.Is(streamErr,context.DeadlineExceeded) || (streamed.Video.ReferenceBytes<=corrPrototypeStreamLimit && streamed.Video.ActualBytes<=corrPrototypeStreamLimit) {t.Fatal("streaming must refuse actual byte ceiling",streamed,streamErr)}
   } else if streamErr!=nil || !streamed.Video.Equal || !streamed.Audio.Equal {t.Fatal("streaming content did not match",streamed,streamErr)}
   if streamed.LogicalInputPassBytes*2!=measured.InputSizePassBudget {t.Fatal("logical-pass accounting drift",streamed,measured)}
   t.Logf("streaming profile=%s repeat=%d outcome=%s seconds=%.3f children=%d scratch_bytes=%d",profile.Name,repeat,row.Streaming.Outcome,row.Streaming.Seconds,streamed.ChildrenStarted,streamed.NormalizedScratchBytes)
   if measured.ProcessesStarted>4 || measured.PeakScratchDescriptors!=2 {t.Fatal("resource contract",measured)}
   if profile.ExpectStreamCeiling {
    // Require refusal at the first video stream writer's boundary, not an
    // unrelated missing-codec error, preflight rejection, or deadline expiry.
    if collectErr==nil || !errors.Is(collectErr,errCorrPrototype) || errors.Is(collectErr,context.DeadlineExceeded) || measured.ProcessesStarted!=1 || measured.NormalizedBytes<corrPrototypeStreamLimit-(64<<10) || measured.NormalizedBytes>corrPrototypeStreamLimit {t.Fatal("expected bounded stream refusal",measured,collectErr)}
   } else if collectErr!=nil || measured.ProcessesStarted!=4 || !measured.VideoEqual || !measured.AudioEqual {t.Fatal("normal fixture did not match",measured,collectErr)}
  }
 })}
 if len(report.Rows)!=6 {t.Errorf("incomplete measurement matrix: %d of 6 rows",len(report.Rows))}
}
