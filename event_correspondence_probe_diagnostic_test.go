package main

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "sort"
 "testing"
 "time"
)

// Diagnostics only. Never used as a receipt, fallback or qualification.
// The existing failing full-adapter assertions remain required and unchanged.
type corrProbeDiagnostic struct {
 Profile,File,Entries string
 Stdout,Stderr,ProcessError,ContextError,DecodeError string
 StdoutRefused,StderrRefused bool
 StrictDecodeAttempted,StrictDecodeAccepted bool
 RootFields,FrameFields []string
 FrameCount,FramesMissingDuration int
}

func corrProbeDiagnosticRun(binary,path,profile,kind,entries string,strict bool)corrProbeDiagnostic {
 row:=corrProbeDiagnostic{Profile:profile,File:kind,Entries:entries}
 ctx,cancel:=context.WithTimeout(context.Background(),eventDurationInspectionBudget);defer cancel()
 file,err:=os.Open(path);if err!=nil {row.ProcessError=err.Error();return row};defer file.Close()
 cmd:=exec.CommandContext(ctx,binary,"-v","error","-err_detect","explode","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_frames","-show_entries",entries,"-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second
 stdout:=&eventPresentationProbeBuffer{limit:maxEventVideoPresentationBytes}
 stderr:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes}
 cmd.Stdout=stdout;cmd.Stderr=stderr
 if err:=cmd.Run();err!=nil {row.ProcessError=err.Error()}
 if err:=ctx.Err();err!=nil {row.ContextError=err.Error()}
 row.Stdout=stdout.String();row.Stderr=stderr.String()
 row.StdoutRefused=stdout.refused;row.StderrRefused=stderr.refused
 if strict && row.ProcessError=="" && row.ContextError=="" && !stdout.refused && !stderr.refused && stderr.Len()==0 {
  row.StrictDecodeAttempted=true
  _,err:=decodeEventDurationPresentation(ctx,stdout.Bytes())
  row.StrictDecodeAccepted=err==nil
  if err!=nil {row.DecodeError=err.Error()}
 }
 // Field summaries are explanatory only; they do not normalize or repair JSON.
 var root map[string]json.RawMessage
 if json.Unmarshal(stdout.Bytes(),&root)==nil {
  for key:=range root {row.RootFields=append(row.RootFields,key)}
  sort.Strings(row.RootFields)
  var frames []map[string]json.RawMessage
  if json.Unmarshal(root["frames"],&frames)==nil {
   row.FrameCount=len(frames);keys:=make(map[string]bool)
   for _,frame:=range frames {
    if _,exists:=frame["duration"];!exists {row.FramesMissingDuration++}
    for key:=range frame {keys[key]=true}
   }
   for key:=range keys {row.FrameFields=append(row.FrameFields,key)}
   sort.Strings(row.FrameFields)
  }
 }
 return row
}

func TestCorrResourceNativeProbeDiagnostics(t *testing.T) {
 if os.Getenv("SIMPLENVR_CORRESPONDENCE_RESOURCE")!="1" {t.Skip("separate resource job only")}
 destination:=os.Getenv("SIMPLENVR_CORRESPONDENCE_REPORT")
 if destination=="" {t.Fatal("explicit report destination required")}
 report:=struct{
  Base,Head,GoVersion,Scope string
  RuntimeQualified,CleanupAuthorized bool
  Versions map[string]string
  Rows []corrProbeDiagnostic
  FailedProfiles []string
 }{Base:"50a2198286ad6e3dc4e637489bb7f3ddbef6419d",Head:os.Getenv("GITHUB_SHA"),GoVersion:runtime.Version(),Scope:"diagnostic native fixtures only; no live-job binding or performance acceptance; supplementary packet fields are never a fallback",Versions:make(map[string]string)}
 defer func(){
  data,err:=json.MarshalIndent(report,"","  ");if err!=nil {t.Error(err);return}
  path:=filepath.Join(filepath.Dir(destination),"native-probe-diagnostics.json")
  if err:=os.WriteFile(path,append(data,'\n'),0600);err!=nil {t.Error(err)}
 }()
 tools:=make(map[string]string)
 for _,name:=range []string{"ffmpeg","ffprobe"} {
  binary,err:=exec.LookPath(name);if err!=nil {t.Fatal(err)}
  tools[name]=binary
  ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
  cmd:=exec.CommandContext(ctx,binary,"-version");cmd.WaitDelay=2*time.Second
  output:=&eventPresentationProbeBuffer{limit:16<<10};diagnostic:=&eventPresentationProbeBuffer{limit:16<<10}
  cmd.Stdout=output;cmd.Stderr=diagnostic
  err=cmd.Run();cancel()
  report.Versions[name]=output.String()+diagnostic.String()
  if err!=nil || output.refused || diagnostic.refused {t.Fatal("tool version unavailable",name,err)}
 }
 for _,profile:=range corrBoundProfiles() {
  ok:=t.Run(profile.Name,func(t *testing.T){
   dir:=t.TempDir();seed:=filepath.Join(dir,"capture.mp4")
   ctx,cancel:=context.WithTimeout(context.Background(),120*time.Second);defer cancel()
   run:=func(args ...string){
    t.Helper()
    cmd:=exec.CommandContext(ctx,tools["ffmpeg"],append([]string{"-hide_banner","-loglevel","error","-nostdin"},args...)...);cmd.Dir=dir;cmd.WaitDelay=2*time.Second
    diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
    if err:=cmd.Run();err!=nil || diagnostic.refused || diagnostic.Len()!=0 {t.Fatal("diagnostic fixture generation",err,diagnostic.String())}
   }
   run("-f","lavfi","-i",fmt.Sprintf("testsrc2=size=%s:rate=%d:duration=%d",profile.Size,profile.FPS,profile.Seconds),"-f","lavfi","-i","sine=frequency=600:sample_rate=48000","-t",fmt.Sprint(profile.Seconds),"-c:v","libx264","-preset","ultrafast","-threads","1","-g",fmt.Sprint(profile.FPS),"-bf","0","-pix_fmt","yuv420p","-b:v",profile.Bitrate,"-minrate",profile.Bitrate,"-maxrate",profile.Bitrate,"-bufsize",profile.Buffer,"-x264-params","nal-hrd=cbr","-c:a","aac","-b:a","64k","-shortest","-fs",fmt.Sprint(corrPrototypeInputLimit),seed)
   if err:=os.WriteFile(filepath.Join(dir,"concat.txt"),[]byte("file 'capture.mp4'\n"),0600);err!=nil {t.Fatal(err)}
   remuxed:=filepath.Join(dir,"published.mp4")
   run("-nostats","-f","concat","-safe","1","-i","concat.txt","-c","copy","-f","mp4","-movflags","faststart","-y",remuxed)
   for _,file:=range []struct{name,path string}{{"capture",seed},{"remuxed",remuxed}} {
    for _,supplementary:=range []bool{false,true} {
     entries:=eventCombinedPresentationEntries
     if supplementary {entries="stream=index,codec_type,duration_ts,time_base:frame=media_type,stream_index,pts,duration,pkt_duration,pkt_duration_time:frame_side_data="}
     row:=corrProbeDiagnosticRun(tools["ffprobe"],file.path,profile.Name,file.name,entries,!supplementary)
     report.Rows=append(report.Rows,row)
     t.Logf("probe profile=%s file=%s supplementary=%t process_error=%q context_error=%q stderr_bytes=%d stdout_refused=%t strict_attempted=%t strict_accepted=%t decode_error=%q frames=%d missing_duration=%d fields=%v",profile.Name,file.name,supplementary,row.ProcessError,row.ContextError,len(row.Stderr),row.StdoutRefused,row.StrictDecodeAttempted,row.StrictDecodeAccepted,row.DecodeError,row.FrameCount,row.FramesMissingDuration,row.FrameFields)
     // Preserve every observation before reporting command/capture failures.
     // A decoder refusal is diagnostic data, not accepted evidence. The
     // unchanged full-adapter test still fails if native evidence refuses.
     if row.ProcessError!="" || row.ContextError!="" || row.StdoutRefused || row.StderrRefused || row.Stderr!="" {t.Error("native diagnostic command refused; inspect retained JSON")}
    }
   }
  })
  if !ok {report.FailedProfiles=append(report.FailedProfiles,profile.Name)}
 }
 if len(report.Rows)!=8 {t.Errorf("incomplete native diagnostics: %d of 8 rows",len(report.Rows))}
}
