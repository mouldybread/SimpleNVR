package main

import (
 "bufio"
 "context"
 "fmt"
 "io"
 "os"
 "os/exec"
 "os/signal"
 "path/filepath"
 "strconv"
 "strings"
 "syscall"
 "testing"
 "time"
)

func requireFFmpeg(t *testing.T) {
 t.Helper()
 for _,name:=range []string{"ffmpeg","ffprobe"} {if _,err:=exec.LookPath(name);err!=nil {
  if os.Getenv("SIMPLENVR_REQUIRE_FFMPEG")=="1" {t.Fatalf("%s required in CI",name)}
  t.Skipf("%s not installed",name)
 }}
}

func TestGracefulFFmpegCancellationPublishesPlayablePartial(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 dir:=t.TempDir();temporary:=filepath.Join(dir,"capture.mp4.tmp");final:=filepath.Join(dir,"capture_partial.mp4")
 cmd:=ffmpegCommand(ctx,"-hide_banner","-loglevel","error","-nostats","-re","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-c:v","mpeg4","-q:v","5","-movflags","faststart","-progress","pipe:1","-f","mp4","-y",temporary)
 stdout,err:=cmd.StdoutPipe();if err!=nil {t.Fatal(err)}
 if err:=cmd.Start();err!=nil {t.Fatal(err)}
 ready:=make(chan struct{},1);readDone:=make(chan struct{})
 go func(){defer close(readDone);scanner:=bufio.NewScanner(stdout);for scanner.Scan(){
  line:=scanner.Text()
  if strings.HasPrefix(line,"frame=") {if frame,err:=strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line,"frame=")));err==nil && frame>=5 {select {case ready<-struct{}{}:default:}}}
 }}()
 done:=make(chan error,1)
 go func(){<-readDone;done<-cmd.Wait()}()
 completed:=false
 defer func(){cancel();if !completed {cmd.Process.Kill();select {case <-done:case <-time.After(5*time.Second):}}}()
 select {case <-ready:case err:=<-done:completed=true;t.Fatalf("FFmpeg exited before producing frames: %v",err);case <-time.After(15*time.Second):t.Fatal("FFmpeg did not produce test frames")}
 cancel()
 select {case <-done:completed=true;case <-time.After(ffmpegStopGrace+5*time.Second):t.Fatal("FFmpeg did not stop within grace")}
 verifyCtx,verifyCancel:=context.WithTimeout(context.Background(),10*time.Second);defer verifyCancel()
 if err:=publishRecording(verifyCtx,temporary,final);err!=nil {t.Fatalf("interrupted MP4 not playable: %v",err)}
 if _,err:=os.Stat(temporary);!os.IsNotExist(err) {t.Fatal("temporary file not published")}
 // Decode the entire synthetic clip, not only its container header.
 decode:=exec.CommandContext(verifyCtx,"ffmpeg","-nostdin","-v","error","-i",final,"-f","null","-")
 if output,err:=decode.CombinedOutput();err!=nil {t.Fatalf("partial clip decode failed: %v %s",err,output)}
}

func TestIgnoreInterruptHelper(t *testing.T) {
 if os.Getenv("SIMPLENVR_SIGNAL_HELPER")!="1" {return}
 signal.Ignore(os.Interrupt)
 fmt.Println("ready")
 for {time.Sleep(time.Hour)}
}

func TestUnresponsiveProcessIsKilledAfterGrace(t *testing.T) {
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 cmd:=interruptibleCommand(ctx,os.Args[0],150*time.Millisecond,"-test.run=^TestIgnoreInterruptHelper$")
 cmd.Env=append(os.Environ(),"SIMPLENVR_SIGNAL_HELPER=1")
 stdout,err:=cmd.StdoutPipe();if err!=nil {t.Fatal(err)}
 if err:=cmd.Start();err!=nil {t.Fatal(err)}
 ready:=make(chan string,1);readDone:=make(chan struct{})
 go func(){defer close(readDone);reader:=bufio.NewReader(stdout);line,_:=reader.ReadString('\n');ready<-line;io.Copy(io.Discard,reader)}()
 done:=make(chan error,1);go func(){<-readDone;done<-cmd.Wait()}()
 completed:=false
 defer func(){cancel();if !completed {cmd.Process.Kill();select {case <-done:case <-time.After(5*time.Second):}}}()
 select {case line:=<-ready:if strings.TrimSpace(line)!="ready" {t.Fatal("helper not ready")};case <-time.After(10*time.Second):t.Fatal("helper startup timeout")}
 cancel()
 select {case err:=<-done:completed=true;if err==nil {t.Fatal("unresponsive helper exited successfully")};case <-time.After(5*time.Second):t.Fatal("forced-kill fallback failed")}
 status,ok:=cmd.ProcessState.Sys().(syscall.WaitStatus)
 if !ok || !status.Signaled() || status.Signal()!=syscall.SIGKILL {t.Fatal("helper was not force-killed")}
}

func TestPublishDoesNotOverwriteOrDeleteUnvalidatedBytes(t *testing.T) {
 dir:=t.TempDir();source:=filepath.Join(dir,"source.tmp");final:=filepath.Join(dir,"final.mp4")
 if err:=os.WriteFile(source,[]byte("unvalidated recording bytes"),0600);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(final,[]byte("existing footage"),0600);err!=nil {t.Fatal(err)}
 if err:=publishRecording(context.Background(),source,final);!os.IsExist(err) {t.Fatalf("existing recording not protected: %v",err)}
 raw,err:=os.ReadFile(final);if err!=nil || string(raw)!="existing footage" {t.Fatal("existing footage changed")}
 removeEmptyRecording(source)
 if _,err:=os.Stat(source);err!=nil {t.Fatal("nonempty unvalidated bytes deleted")}
}

func TestCanceledQueuedPrebufferIsRetained(t *testing.T) {
 bm:=bufferFixture(t)
 dir:=filepath.Join(bm.camera.RecordDir,"buffer");if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 path:=filepath.Join(dir,"closed.mp4")
 if err:=os.WriteFile(path,[]byte(strings.Repeat("x",2048)),0600);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(path,now.Add(-5*time.Second),now)
 if err:=bm.TriggerEvent(bufferTestEvent("interrupted"));err!=nil {t.Fatal(err)}
 bm.cancel();bm.runWorkers(nil,func(job *bufferEvent){t.Error("canceled job executed")})
 entries,err:=os.ReadDir(filepath.Join(bm.camera.RecordDir,"temp"));if err!=nil || len(entries)!=1 {t.Fatal("pinned event staging not retained",err)}
 pinned:=filepath.Join(bm.camera.RecordDir,"temp",entries[0].Name(),"pre_000.mp4")
 if err:=os.Remove(path);err!=nil {t.Fatal(err)}
 if info,err:=os.Stat(pinned);err!=nil || info.Size()!=2048 {t.Fatal("retained prebuffer lost",err)}
 if len(bm.slots)!=0 {t.Fatal("retained job leaked admission slot")}
}
