package main

import (
 "context"
 "crypto/sha256"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

// Probe phases exec real ffprobe on a deliberately empty, still-open pipe.
// Renderer input is paced, never SIGSTOP'd. Neither changes production code.
func installExportPhaseFixture(t *testing.T,phase string) (string,string) {
 t.Helper()
 python,err:=exec.LookPath("python3");if err!=nil {t.Fatal(err)}
 for _,name:=range []string{"ffmpeg","ffprobe"} {
  path,err:=exec.LookPath(name);if err!=nil {t.Fatal(err)}
  path,err=filepath.EvalSymlinks(path);if err!=nil {t.Fatal(err)}
  t.Setenv("SIMPLENVR_PHASE_REAL_"+name,path)
 }
 bin,marker:=t.TempDir(),filepath.Join(t.TempDir(),"pid")
 script:="#!"+python+"\n"+`import os, sys
name = os.path.basename(sys.argv[0])
args = sys.argv[1:]
phase = os.environ["SIMPLENVR_EXPORT_PHASE"]
tracked = False
if name == "ffprobe" and "-show_entries" in args:
    fields = args[args.index("-show_entries") + 1]
    selected = "profile" if "pix_fmt" in fields else "duration"
    if selected == phase:
        readfd, writefd = os.pipe()
        os.dup2(readfd, 3)
        os.set_inheritable(3, True)
        if readfd != 3: os.close(readfd)
        os.set_inheritable(writefd, True)
        tracked = True
if name == "ffmpeg" and phase == "render" and "libx264" in args:
    index = 0
    while index < len(args):
        if args[index] == "-i":
            args[index:index] = ["-re"]
            index += 1
        index += 1
    tracked = True
if tracked:
    with open(os.environ["SIMPLENVR_PHASE_PID"], "x") as f: f.write(str(os.getpid()))
real = os.environ["SIMPLENVR_PHASE_REAL_" + name]
os.execv(real, [real] + args)
`
 for _,name:=range []string{"ffmpeg","ffprobe"} {if err:=os.WriteFile(filepath.Join(bin,name),[]byte(script),0700);err!=nil {t.Fatal(err)}}
 t.Setenv("SIMPLENVR_EXPORT_PHASE",phase);t.Setenv("SIMPLENVR_PHASE_PID",marker)
 t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
 name:="ffprobe";if phase=="render" {name="ffmpeg"}
 return marker,os.Getenv("SIMPLENVR_PHASE_REAL_"+name)
}

func awaitExportPhase(t *testing.T,marker,real string) int {
 t.Helper();pid:=0
 awaitActiveRecorder(t,"export phase did not exec real media tool",func()bool{
  data,err:=os.ReadFile(marker);if err!=nil {return false}
  value,err:=strconv.Atoi(string(data));if err!=nil {return false}
  executable,err:=os.Readlink(fmt.Sprintf("/proc/%d/exe",value));if err!=nil || executable!=real {return false}
  pid=value;return true
 })
 return pid
}

func TestAuthenticatedExportPhaseCancellationAndFullServerShutdown(t *testing.T) {
 for _,phase:=range []string{"duration","profile","render"} {for _,shutdown:=range []bool{false,true} {t.Run(fmt.Sprintf("%s/shutdown=%t",phase,shutdown),func(t *testing.T){
  f:=newExportNetworkFixture(t,true);enlargeExportRenderFixture(t,f)
  marker,real:=installExportPhaseFixture(t,phase)
  result,cancelRequest:=startPreheaderTrimRequest(t,f);defer cancelRequest()
  pid:=awaitExportPhase(t,marker,real)
  wantSlots:=1;if phase=="duration" {wantSlots=0}
  if len(f.pool.slots)!=wantSlots {t.Fatal("unexpected export admission phase",phase,len(f.pool.slots))}
  if phase=="profile" && len(recordingDurations.slots)!=1 {t.Fatal("profile probe admission not occupied")}
  if phase=="render" {
   assertAnonymousTrimPresent(t,f.pool.tempDir)
   awaitActiveRecorder(t,"renderer did not write anonymous output before cancellation",func()bool{
    entries,err:=os.ReadDir("/proc/self/fd");if err!=nil {return false}
    for _,entry:=range entries {
     path:=filepath.Join("/proc/self/fd",entry.Name());target,err:=os.Readlink(path)
     if err==nil && strings.HasPrefix(target,f.pool.tempDir+string(os.PathSeparator)+"simplenvr-trim-") {
      if info,err:=os.Stat(path);err==nil && info.Size()>0 {return true}
     }
    }
    return false
   })
  }
  select {case completed:=<-result:t.Fatalf("phase returned before cancellation: %v %v",completed.response,completed.err);default:}
  var persisted atomic.Bool;done:=make(chan error,1)
  if shutdown {
   ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
   go func(){done<-shutdownApplication(ctx,f.n,f.gate,f.server.Config,func()error{
    if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {return errors.New("persistence preceded media child reaping")}
    persisted.Store(true);return nil
   })}()
  } else {cancelRequest()}
  // Less than the eight-second recorder force-kill grace: no SIGSTOP or
  // deliberately unresponsive renderer is used in this case.
  select {case <-f.mediaReturned:case <-time.After(ffmpegStopGrace-time.Second):t.Fatal("phase cleanup did not complete before renderer force-kill grace")}
  if persisted.Load() {t.Fatal("persisted before tracked handler cleanup release")}
  if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("media child not reaped",err)}
  if phase!="duration" {f.assertResourcesClosed(t)} else {
   f.filesMu.Lock();count:=len(f.opened);f.filesMu.Unlock()
   raw,err:=os.ReadFile(f.source)
   if count!=0 || len(f.pool.slots)!=0 || err!=nil || sha256.Sum256(raw)!=f.sourceHash {t.Fatal("metadata-phase cancellation changed source or reached pinning",err)}
  }
  if len(recordingDurations.slots)!=0 {t.Fatal("profile probe slot leaked")}
  f.releaseCleanup();awaitExportNetwork(t,f.handlerDone,"phase handler did not join")
  if shutdown {
   select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(6*time.Second):t.Fatal("full HTTP phase shutdown did not join")}
   if !persisted.Load() {t.Fatal("phase shutdown did not persist")}
  } else {awaitExportNetwork(t,f.gate.beginShutdown(),"phase cancellation did not drain")}
  select {
  case completed:=<-result:
   if completed.response!=nil {
    headers:=completed.response.Header
    if strings.HasPrefix(headers.Get("Content-Type"),"video/") || headers.Get("Content-Disposition")!="" || headers.Get("X-Export-Trim-Applied")!="" {t.Fatal("canceled preparation started a video response",headers)}
   } else if completed.err==nil {t.Fatal("client returned neither response nor error")}
  case <-time.After(3*time.Second):t.Fatal("client did not observe phase cancellation")
  }
 })}}
}

func TestCanceledDurationProbeReleasesAdmissionSlot(t *testing.T) {
 f:=newExportNetworkFixture(t,false)
 marker,real:=installExportPhaseFixture(t,"duration")
 cache:=newDurationCache(1,1,probeVideoDuration)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 done:=make(chan error,1)
 go func(){_,_,err:=cache.duration(ctx,f.source);done<-err}()
 joined:=false
 defer func(){cancel();if !joined {select {case <-done:case <-time.After(7*time.Second):t.Error("duration cleanup did not join")}}}()
 pid:=awaitExportPhase(t,marker,real)
 if len(cache.slots)!=1 {t.Fatal("duration probe admission not occupied")}
 cancel()
 select {case err:=<-done:joined=true;if !errors.Is(err,context.Canceled) {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("duration probe did not cancel")}
 if len(cache.slots)!=0 {t.Fatal("duration admission slot leaked")}
 if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("duration child not reaped",err)}
}
