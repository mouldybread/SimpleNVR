package main

import (
 "bytes"
 "context"
 "errors"
 "io"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func exportClipFixture(t *testing.T,dir,name,data string) recordingInterval {
 t.Helper();path:=filepath.Join(dir,name)
 if err:=os.WriteFile(path,[]byte(data),0600);err!=nil {t.Fatal(err)}
 info,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
 return recordingInterval{Path:path,Size:info.Size(),identity:info}
}
func assertPinsClosed(t *testing.T,pool *exportInputPool,files []*os.File) {
 t.Helper();if len(pool.slots)!=0 {t.Fatal("export admission slot leaked")}
 for _,file:=range files {if _,err:=file.Stat();!errors.Is(err,os.ErrClosed) {t.Fatalf("descriptor not closed: %v",err)}}
}
func TestPinnedExportSurvivesUnlinkAndPathReplacement(t *testing.T) {
 dir:=t.TempDir();clip:=exportClipFixture(t,dir,"camera's\\clip\n.mp4","original media")
 pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir()
 pins,err:=pool.prepare(context.Background(),[]recordingInterval{clip});if err!=nil {t.Fatal(err)}
 defer pins.Close()
 replacement:=filepath.Join(dir,"replacement");if err:=os.WriteFile(replacement,[]byte("different media"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Rename(replacement,clip.Path);err!=nil {t.Fatal(err)}
 if err:=os.Remove(clip.Path);err!=nil {t.Fatal(err)}
 raw:=make([]byte,len("original media"));if _,err:=pins.files[0].ReadAt(raw,0);err!=nil || string(raw)!="original media" {t.Fatal(string(raw),err)}
 manifest,err:=io.ReadAll(pins.manifest);if err!=nil {t.Fatal(err)}
 if string(manifest)!="ffconcat version 1.0\nfile '/proc/self/fd/4'\n" {t.Fatalf("unsafe manifest: %q",manifest)}
 if entries,err:=os.ReadDir(pool.tempDir);err!=nil || len(entries)!=0 {t.Fatal("named manifest retained",err)}
 files:=append([]*os.File{pins.manifest},pins.files...)
 if err:=pins.Close();err!=nil {t.Fatal(err)}
 if err:=pins.Close();err!=nil {t.Fatal(err)}
 assertPinsClosed(t,pool,files)
}
func TestExportPinPreparationFailuresCloseAllInputs(t *testing.T) {
 for _,failure:=range []string{"replacement","missing","symlink","manifest","cancel"} {t.Run(failure,func(t *testing.T){
  dir:=t.TempDir();a:=exportClipFixture(t,dir,"a.mp4","first");b:=exportClipFixture(t,dir,"b.mp4","second")
  pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  var opened []*os.File
  pool.open=func(path string)(*os.File,error){file,err:=openExportInput(path);if err==nil {opened=append(opened,file);if failure=="cancel" {cancel()}};return file,err}
  switch failure {
  case "replacement":
   other:=exportClipFixture(t,dir,"other","second");if err:=os.Chtimes(other.Path,b.identity.ModTime(),b.identity.ModTime());err!=nil {t.Fatal(err)}
   if err:=os.Rename(other.Path,b.Path);err!=nil {t.Fatal(err)}
  case "missing":if err:=os.Remove(b.Path);err!=nil {t.Fatal(err)}
  case "symlink":if err:=os.Remove(b.Path);err!=nil {t.Fatal(err)};if err:=os.Symlink(a.Path,b.Path);err!=nil {t.Fatal(err)}
  case "manifest":pool.tempDir=filepath.Join(dir,"absent")
  }
  if pins,err:=pool.prepare(ctx,[]recordingInterval{a,b});err==nil {pins.Close();t.Fatal("preparation failure accepted")}
  assertPinsClosed(t,pool,opened)
 })}
}
func TestExportPinCapacityAndCanceledAdmission(t *testing.T) {
 clip:=exportClipFixture(t,t.TempDir(),"clip.mp4","media");pool:=newExportInputPool(2,1);pool.tempDir=t.TempDir()
 if _,err:=pool.prepare(context.Background(),[]recordingInterval{clip,clip});!errors.Is(err,errExportInputLimit) {t.Fatal(err)}
 a,err:=pool.prepare(context.Background(),[]recordingInterval{clip});if err!=nil {t.Fatal(err)};defer a.Close()
 b,err:=pool.prepare(context.Background(),[]recordingInterval{clip});if err!=nil {t.Fatal(err)};defer b.Close()
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Millisecond);defer cancel()
 if _,err:=pool.prepare(ctx,[]recordingInterval{clip});!errors.Is(err,context.DeadlineExceeded) {t.Fatal(err)}
 if len(pool.slots)!=2 {t.Fatal("admission bound changed")}
}
func TestIntervalSelectionCarriesExportIdentity(t *testing.T) {
 dir:=t.TempDir();clip:=exportClipFixture(t,dir,"20261110_000000_clip.mp4","original")
 cache:=newDurationCache(1,1,func(context.Context,*os.File)(time.Duration,error){return time.Second,nil})
 start:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 clips,err:=collectRecordingIntervals(context.Background(),[]timelineCamera{{"front","Front",dir}},timelineWindow{Start:start,End:start.Add(time.Minute)},cache)
 if err!=nil || len(clips)!=1 || !sameMediaFile(clip.identity,clips[0].identity) {t.Fatal("selected identity missing",err)}
 replacement:=exportClipFixture(t,dir,"replacement","original")
 if err:=os.Chtimes(replacement.Path,clip.identity.ModTime(),clip.identity.ModTime());err!=nil {t.Fatal(err)}
 if err:=os.Rename(replacement.Path,clip.Path);err!=nil {t.Fatal(err)}
 pool:=newExportInputPool(1,1)
 if pins,err:=pool.prepare(context.Background(),clips);!errors.Is(err,errRecordingChanged) {if pins!=nil {pins.Close()};t.Fatal("replacement passed identity verification",err)}
 if len(pool.slots)!=0 {t.Fatal("slot leaked")}
}
func syntheticExportClips(t *testing.T,dir string) []recordingInterval {
 t.Helper();requireFFmpeg(t)
 a:=filepath.Join(dir,"a.mp4");ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-t","2","-c:v","mpeg4","-an","-y",a)
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatal(err,string(output))}
 b:=filepath.Join(dir,"b.mp4");if err:=os.Link(a,b);err!=nil {t.Fatal(err)}
 var clips []recordingInterval
 for _,path:=range []string{a,b} {info,err:=os.Stat(path);if err!=nil {t.Fatal(err)};clips=append(clips,recordingInterval{Path:path,Size:info.Size(),identity:info})}
 return clips
}
func TestFFmpegReadsPinnedInputsAfterOriginalPathsDisappear(t *testing.T) {
 dir:=t.TempDir();clips:=syntheticExportClips(t,dir);pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir()
 pins,err:=pool.prepare(context.Background(),clips);if err!=nil {t.Fatal(err)};defer pins.Close()
 if err:=os.Remove(clips[0].Path);err!=nil {t.Fatal(err)}
 replacement:=filepath.Join(dir,"replacement");if err:=os.WriteFile(replacement,[]byte("not an MP4"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Rename(replacement,clips[1].Path);err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 cmd,err:=pins.command(ctx);if err!=nil {t.Fatal(err)}
 var output,stderr bytes.Buffer;cmd.Stdout=&output;cmd.Stderr=&stderr
 if err:=cmd.Run();err!=nil {t.Fatal(err,stderr.String())}
 final:=filepath.Join(dir,"output.mp4");if err:=os.WriteFile(final,output.Bytes(),0600);err!=nil {t.Fatal(err)}
 file,err:=os.Open(final);if err!=nil {t.Fatal(err)};defer file.Close()
 duration,err:=probeVideoDuration(ctx,file)
 if err!=nil || duration<3900*time.Millisecond || duration>4100*time.Millisecond {t.Fatal("wrong pinned concat output duration",duration,err)}
 decode:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-i",final,"-f","null","-")
 if output,err:=decode.CombinedOutput();err!=nil {t.Fatal(err,string(output))}
 files:=append([]*os.File{pins.manifest},pins.files...);if err:=pins.Close();err!=nil {t.Fatal(err)};assertPinsClosed(t,pool,files)
}
type canceledExportWriter struct {ctx context.Context;ready chan struct{};once sync.Once}
func (w *canceledExportWriter) Write(_ []byte)(int,error){w.once.Do(func(){close(w.ready)});<-w.ctx.Done();return 0,w.ctx.Err()}
func TestCanceledFFmpegExportClosesPinsAfterRun(t *testing.T) {
 dir:=t.TempDir();clips:=syntheticExportClips(t,dir);pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir()
 pins,err:=pool.prepare(context.Background(),clips);if err!=nil {t.Fatal(err)};defer pins.Close()
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 cmd,err:=pins.command(ctx);if err!=nil {t.Fatal(err)}
 ready:=make(chan struct{});cmd.Stdout=&canceledExportWriter{ctx:ctx,ready:ready};cmd.Stderr=io.Discard
 files:=append([]*os.File{pins.manifest},pins.files...);done:=make(chan error,1)
 go func(){err:=cmd.Run();done<-errors.Join(err,pins.Close())}()
 select {case <-ready:case <-time.After(5*time.Second):cancel();t.Fatal("export did not produce data")}
 cancel()
 select {case err:=<-done:if err==nil {t.Fatal("canceled export succeeded")};case <-time.After(ffmpegStopGrace+5*time.Second):t.Fatal("canceled export did not finish")}
 assertPinsClosed(t,pool,files)
}
func TestExportPinFailureDoesNotStartVideoResponse(t *testing.T) {
 dir:=t.TempDir();exportClipFixture(t,dir,"20261110_000000_clip.mp4","media")
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 cache:=newDurationCache(1,1,func(context.Context,*os.File)(time.Duration,error){return time.Second,nil})
 pool:=newExportInputPool(1,1);pool.open=func(string)(*os.File,error){return nil,os.ErrNotExist}
 query:="/api/download?camera_id=front&start_time=2026-11-10T00:00:00Z&end_time=2026-11-10T00:01:00Z"
 response:=httptest.NewRecorder();serveDownload(response,httptest.NewRequest("GET",query,nil),n,cache,pool)
 if response.Code!=503 || strings.HasPrefix(response.Header().Get("Content-Type"),"video/") || response.Header().Get("Content-Disposition")!="" {t.Fatal("pin failure started an export response",response.Code,response.Header())}
 if len(pool.slots)!=0 {t.Fatal("slot leaked")}
}
