package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "sync/atomic"
 "testing"
 "time"
)

// Only the sentinel input is replaced. Capture outputs and recorder control
// flow are production code; the input is a looped local file, NOT RTSP.
func installActiveRecorderFixture(t *testing.T,source string) (string,string) {
 t.Helper();requireFFmpeg(t)
 real,err:=exec.LookPath("ffmpeg");if err!=nil {t.Fatal(err)}
 real,err=filepath.EvalSymlinks(real);if err!=nil {t.Fatal(err)}
 python,err:=exec.LookPath("python3");if err!=nil {t.Fatal("python3 required for active-recorder fixture",err)}
 bin,pids:=t.TempDir(),t.TempDir()
 script:="#!"+python+"\n"+`import os, sys
args = sys.argv[1:]
tracked = False
if "-i" in args and args[args.index("-i") + 1] == "rtsp://simplenvr-fixture":
    i = args.index("-rtsp_transport")
    if args[i + 1] != "tcp":
        raise SystemExit("unexpected fixture transport")
    del args[i:i + 2]
    i = args.index("-i")
    args[i:i + 2] = ["-re", "-stream_loop", "-1", "-i", os.environ["SIMPLENVR_RECORDER_FIXTURE"]]
    tracked = True
elif os.environ.get("SIMPLENVR_ASSEMBLY_FIXTURE") == "1" and "concat.txt" in args:
    i = args.index("-i")
    args[i:i] = ["-re"]
    tracked = True
if tracked:
    with open(os.path.join(os.environ["SIMPLENVR_RECORDER_PIDS"], str(os.getpid())), "x") as f:
        f.write("fixture media process\n")
os.execv(os.environ["SIMPLENVR_REAL_FFMPEG"], [os.environ["SIMPLENVR_REAL_FFMPEG"]] + args)
`
 if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
 t.Setenv("SIMPLENVR_REAL_FFMPEG",real)
 t.Setenv("SIMPLENVR_RECORDER_FIXTURE",source)
 t.Setenv("SIMPLENVR_RECORDER_PIDS",pids)
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","0")
 t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
 return pids,real
}

func awaitActiveRecorder(t *testing.T,label string,ready func() bool) {
 t.Helper();deadline:=time.Now().Add(12*time.Second)
 for time.Now().Before(deadline) {if ready() {return};time.Sleep(10*time.Millisecond)}
 t.Fatal(label)
}

func activeRecorderWriting(pattern string) bool {
 files,_:=filepath.Glob(pattern)
 for _,path:=range files {if info,err:=os.Stat(path);err==nil && info.Mode().IsRegular() && info.Size()>1024 {return true}}
 return false
}

func activeRecorderPIDs(t *testing.T,dir,real string,count int) []int {
 t.Helper();var pids []int
 awaitActiveRecorder(t,"capture processes did not exec real FFmpeg",func()bool{
  entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=count {return false}
  observed:=make([]int,0,count)
  for _,entry:=range entries {
   pid,err:=strconv.Atoi(entry.Name());if err!=nil {return false}
   executable,err:=os.Readlink(fmt.Sprintf("/proc/%d/exe",pid));if err!=nil || executable!=real {return false}
   observed=append(observed,pid)
  }
  pids=observed;return true
 })
 return pids
}

func assertActiveRecorderPartial(t *testing.T,dir string) {
 t.Helper()
 files,err:=filepath.Glob(filepath.Join(dir,"*_partial.mp4"));if err!=nil || len(files)!=1 {t.Fatal("expected one interrupted continuous output",files,err)}
 temporary,err:=filepath.Glob(filepath.Join(dir,"*.tmp"));if err!=nil || len(temporary)!=0 {t.Fatal("synthetic continuous publication left temporary output",temporary,err)}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-i",files[0],"-f","null","-")
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("synthetic continuous partial decode: %v %s",err,output)}
}

func TestActiveRecordersAndExportJoinBeforeFullHTTPShutdownPersistence(t *testing.T) {
 f:=newExportNetworkFixture(t,true)
 pidsDir,real:=installActiveRecorderFixture(t,f.source)
 old:=nvr;nvr=f.n
 // Cleanup joins recorders before restoring the global registry or PATH.
 t.Cleanup(func(){
  f.n.shuttingDown.Store(true)
  joined:=make(chan struct{});go func(){f.n.cancelAndWaitRecorders();close(joined)}()
  select {case <-joined:nvr=old;case <-time.After(25*time.Second):t.Error("active-recorder cleanup did not join")}
 })
 continuous:=&Camera{ID:"continuous",Name:"Continuous",RecordDir:t.TempDir(),RecordingMode:ModeContinuous,RTSPUrl:"rtsp://simplenvr-fixture"}
 eventCamera:=&Camera{ID:"event",Name:"Event",RecordDir:t.TempDir(),RecordingMode:ModeEventBased,RTSPUrl:"rtsp://simplenvr-fixture"}
 stopped:=&Camera{ID:"stopped",Name:"Stopped",RecordDir:t.TempDir(),RecordingMode:ModeContinuous,RTSPUrl:"rtsp://simplenvr-fixture"}
 f.n.mu.Lock()
 for _,cam:=range []*Camera{continuous,eventCamera,stopped} {f.n.cameras[cam.ID]=cam;f.n.cameraOrder=append(f.n.cameraOrder,cam.ID)}
 f.n.mu.Unlock()
 if err:=f.n.transitionRecording(stopped.ID,"start","",runRecordingWorker);err!=nil {t.Fatal(err)}
 awaitActiveRecorder(t,"explicit-stop recorder never wrote output",func()bool{return activeRecorderWriting(filepath.Join(stopped.RecordDir,"*.tmp"))})
 stoppedPIDs:=activeRecorderPIDs(t,pidsDir,real,1)
 if err:=f.n.transitionRecording(stopped.ID,"stop","",runRecordingWorker);err!=nil {t.Fatal(err)}
 if stopped.IsRecording || stopped.recordCancel!=nil {t.Fatal("explicit stop retained runtime state or restart intent")}
 for _,pid:=range stoppedPIDs {
  if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("explicit-stop child not reaped",err)}
  if err:=os.Remove(filepath.Join(pidsDir,strconv.Itoa(pid)));err!=nil {t.Fatal(err)}
 }
 assertActiveRecorderPartial(t,stopped.RecordDir)
 for _,cam:=range []*Camera{continuous,eventCamera} {
  if err:=f.n.transitionRecording(cam.ID,"start","",runRecordingWorker);err!=nil {t.Fatal(err)}
 }
 var bm *BufferManager
 awaitActiveRecorder(t,"event manager not installed",func()bool{f.n.mu.RLock();bm=eventCamera.bufferManager;f.n.mu.RUnlock();return bm!=nil})
 seed:=filepath.Join(eventCamera.RecordDir,"buffer","fixture-seed.mp4")
 raw,err:=os.ReadFile(f.source);if err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(seed,raw,0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(seed,now.Add(-2*time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("active-shutdown"));err!=nil {t.Fatal(err)}
 awaitActiveRecorder(t,"continuous capture never wrote output",func()bool{return activeRecorderWriting(filepath.Join(continuous.RecordDir,"*.tmp"))})
 awaitActiveRecorder(t,"event capture never wrote output",func()bool{return activeRecorderWriting(filepath.Join(eventCamera.RecordDir,"temp","event-*","event.mp4"))})
 pids:=activeRecorderPIDs(t,pidsDir,real,3)
 response,cancel:=f.request(t,false);defer cancel()
 first:=make([]byte,1);if _,err:=io.ReadFull(response.Body,first);err!=nil {t.Fatal(err)}
 f.awaitBackpressure(t)
 exportPID:=exportChildUsingSource(t,f.source);if exportPID==0 {t.Fatal("active export child not observed")}
 pids=append(pids,exportPID)
 saved:=filepath.Join(t.TempDir(),"cameras.json")
 var persisted atomic.Bool
 ctx,stop:=context.WithTimeout(context.Background(),30*time.Second);defer stop()
 done:=make(chan error,1)
 go func(){done<-shutdownApplication(ctx,f.n,f.gate,f.server.Config,func()error{
  for _,pid:=range pids {if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {return fmt.Errorf("persistence preceded child reaping: %d: %v",pid,err)}}
  select {case <-f.handlerDone:default:return errors.New("persistence preceded export cleanup")}
  // TempDir cameras deliberately cannot pass the production /recordings
  // path validator. Exercise snapshot/atomic persistence without changing it.
  f.n.mu.RLock();data,err:=json.Marshal(persistedState{f.n.cameras,f.n.cameraOrder,f.n.events});f.n.mu.RUnlock()
  if err!=nil {return err}
  err=atomicWrite(saved,append(data,'\n'));if err==nil {persisted.Store(true)};return err
 })}()
 awaitExportNetwork(t,f.mediaReturned,"server shutdown did not cancel blocked export")
 if persisted.Load() {t.Fatal("persisted before tracked handler release")}
 f.releaseCleanup()
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(30*time.Second):t.Fatal("full server shutdown did not join")}
 if !persisted.Load() {t.Fatal("final configuration not persisted")}
 cancel();response.Body.Close();f.assertResourcesClosed(t)
 conn,err:=net.DialTimeout("tcp",f.server.Listener.Addr().String(),time.Second)
 if err==nil {conn.Close();t.Fatal("HTTP listener still accepting after shutdown")}
 if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("event workers leaked capacity")}
 for _,cam:=range []*Camera{continuous,eventCamera} {if !cam.IsRecording || cam.recordCancel!=nil || cam.bufferManager!=nil {t.Fatal("shutdown lost restart intent or retained runtime state",cam.ID)}}
 assertActiveRecorderPartial(t,continuous.RecordDir)
 dirs,err:=filepath.Glob(filepath.Join(eventCamera.RecordDir,"temp","event-*"));if err!=nil || len(dirs)!=1 {t.Fatal("active event staging lost",dirs,err)}
 m:=readRecoveryTest(t,dirs[0])
 if m.EventID!="active-shutdown" || m.State!="capture_failed" || m.FailureCode!="process_failed_or_canceled" || m.PinnedCount<1 || m.OutputPublished!="" {t.Fatalf("unexpected interrupted acquisition checkpoint: %+v",m)}
 if info,err:=os.Stat(filepath.Join(dirs[0],"event.mp4"));err!=nil || info.Size()<=1024 {t.Fatal("interrupted capture evidence lost",err)}
 pins,err:=filepath.Glob(filepath.Join(dirs[0],"pre_*.mp4"));if err!=nil {t.Fatal(err)}
 seedRetained:=false
 for _,pin:=range pins {data,err:=os.ReadFile(pin);if err!=nil {t.Fatal(err)};if sha256.Sum256(data)==f.sourceHash {seedRetained=true}}
 if !seedRetained {t.Fatal("seed prebuffer lost during stopped buffer cleanup")}
 data,err:=os.ReadFile(saved);if err!=nil {t.Fatal(err)}
 var snapshot persistedState;if err:=json.Unmarshal(data,&snapshot);err!=nil {t.Fatal(err)}
 if !snapshot.Cameras[continuous.ID].IsRecording || !snapshot.Cameras[eventCamera.ID].IsRecording || snapshot.Cameras[stopped.ID].IsRecording {t.Fatal("persisted shutdown/explicit-stop intent incorrect")}
}


func TestActiveEventAssemblyCancellationRetainsOriginalInputs(t *testing.T) {
 f:=newExportNetworkFixture(t,false)
 pidsDir,real:=installActiveRecorderFixture(t,f.source)
 t.Setenv("SIMPLENVR_ASSEMBLY_FIXTURE","1")
 bm:=bufferFixture(t)
 raw,err:=os.ReadFile(f.source);if err!=nil {t.Fatal(err)}
 seed:=filepath.Join(bm.camera.RecordDir,"seed.mp4")
 if err:=os.WriteFile(seed,raw,0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(seed,now.Add(-2*time.Second),now)
 job,err:=bm.pinBuffer(bufferTestEvent("assembly-shutdown"));if err!=nil {t.Fatal(err)}
 capture:=filepath.Join(job.dir,"event.mp4")
 if err:=os.WriteFile(capture,raw,0600);err!=nil {t.Fatal(err)}
 bm.slots<-struct{}{}
 done:=make(chan string,1)
 joined:=false
 defer func(){bm.cancel();if !joined {select {case <-done:case <-time.After(25*time.Second):t.Error("assembly cleanup did not join")}}}()
 go func(){final:=bm.concatenateEventRecording(job,capture);bm.releaseJob(job);done<-final}()
 awaitActiveRecorder(t,"assembly never wrote temporary output",func()bool{return activeRecorderWriting(filepath.Join(bm.camera.RecordDir,"*.tmp"))})
 pids:=activeRecorderPIDs(t,pidsDir,real,1)
 bm.cancel()
 var final string
 select {case final=<-done:joined=true;case <-time.After(25*time.Second):t.Fatal("active assembly cancellation did not join")}
 for _,pid:=range pids {if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("assembly child not reaped",err)}}
 if final=="" || final!=partialRecordingName(final) {t.Fatal("synthetic interrupted assembly not published as partial",final)}
 if !job.keepStaging || len(bm.slots)!=0 {t.Fatal("assembly canceled without evidence retention/capacity release")}
 for _,input:=range []string{capture,job.preFiles[0]} {
  data,err:=os.ReadFile(input);if err!=nil || sha256.Sum256(data)!=f.sourceHash {t.Fatal("original assembly input changed",err)}
 }
 if _,err:=os.Stat(filepath.Join(job.dir,"concat.txt"));err!=nil {t.Fatal("assembly concat evidence lost",err)}
 m:=readRecoveryTest(t,job.dir)
 if m.State!="published" || !m.Partial || m.OutputPublished!=filepath.Base(final) {t.Fatalf("interrupted publication checkpoint incorrect: %+v",m)}
 assertActiveRecorderPartial(t,bm.camera.RecordDir)
}
