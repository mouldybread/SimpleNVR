package main

import (
 "context"
 "crypto/sha256"
 "errors"
 "fmt"
 "net/http"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

type exportRenderHTTPResult struct {response *http.Response;err error}
type stoppedExportRenderer struct {pid,fd int}

func enlargeExportRenderFixture(t *testing.T,f *exportNetworkFixture) {
 t.Helper()
 // Build before the first media request, so no reader/cache is using the old
 // fixture. Only this test's temporary source recording is replaced.
 replacement:=f.source+".fixture.tmp"
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=512x288:rate=10","-filter_threads","1","-vf","noise=alls=100:allf=t+u","-t","8","-c:v","mpeg4","-q:v","2","-threads","1","-an","-f","mp4","-y",replacement)
 if result,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("render cancellation fixture: %v %s",err,result)}
 info,err:=os.Stat(replacement)
 if err!=nil || info.Size()<512<<10 || info.Size()>32<<20 {t.Fatal("render fixture outside test bounds",err)}
 if err:=os.Rename(replacement,f.source);err!=nil {t.Fatal(err)}
 data,err:=os.ReadFile(f.source);if err!=nil {t.Fatal(err)};f.sourceHash=sha256.Sum256(data)
}

func startPreheaderTrimRequest(t *testing.T,f *exportNetworkFixture) (<-chan exportRenderHTTPResult,context.CancelFunc) {
 t.Helper()
 query:=url.Values{"camera_id":{"front"},"start_time":{"2026-11-10T00:00:00.2Z"},"end_time":{"2026-11-10T00:00:07.8Z"}}
 ctx,cancel:=context.WithCancel(context.Background())
 request,err:=http.NewRequestWithContext(ctx,"GET",f.server.URL+"/api/download?"+query.Encode(),nil);if err!=nil {cancel();t.Fatal(err)}
 for _,cookie:=range f.cookies {request.AddCookie(cookie)}
 result:=make(chan exportRenderHTTPResult,1);workerDone:=make(chan struct{})
 // Close every returned body in the worker. Even an early-test failure cannot
 // strand a response body in the buffered result channel.
 go func(){defer close(workerDone);response,err:=f.client.Do(request);if response!=nil {response.Body.Close()};result<-exportRenderHTTPResult{response:response,err:err}}()
 t.Cleanup(func(){cancel();select {case <-workerDone:case <-time.After(3*time.Second):t.Error("preheader HTTP worker did not join")}})
 return result,cancel
}

func stopObservedTrimRenderer(t *testing.T,f *exportNetworkFixture,result <-chan exportRenderHTTPResult) stoppedExportRenderer {
 t.Helper();deadline:=time.Now().Add(8*time.Second)
 for time.Now().Before(deadline) {
  select {case completed:=<-result:t.Fatalf("request completed before render observation: %v %v",completed.response,completed.err);default:}
  pid:=exportChildUsingSource(t,f.source)
  if pid==0 {time.Sleep(time.Millisecond);continue}
  command,err:=os.ReadFile(fmt.Sprintf("/proc/%d/cmdline",pid))
  if os.IsNotExist(err) {continue};if err!=nil {t.Fatal(err)}
  if !strings.Contains(string(command),"libx264") || !strings.Contains(string(command),"-filter_complex") {time.Sleep(time.Millisecond);continue}
  fd,err:=unix.PidfdOpen(pid,0)
  if errors.Is(err,unix.ESRCH) {continue};if err!=nil {t.Fatal("pidfd support is required for this Linux test",err)}
  target,err:=os.Readlink(fmt.Sprintf("/proc/%d/fd/4",pid))
  if err!=nil || target!=f.source {unix.Close(fd);continue}
  if err:=unix.PidfdSendSignal(fd,unix.SIGSTOP,nil,0);err!=nil {
   unix.Close(fd);if errors.Is(err,unix.ESRCH) {continue};t.Fatal(err)
  }
  child:=stoppedExportRenderer{pid:pid,fd:fd}
  // A live pidfd prevents cleanup from signaling an unrelated reused PID.
  // Resume before fixture teardown if the test fails before cancellation.
  t.Cleanup(func(){_=unix.PidfdSendSignal(child.fd,unix.SIGCONT,nil,0);_=unix.Close(child.fd)})
  stoppedBy:=time.Now().Add(time.Second)
  for time.Now().Before(stoppedBy) {
   status,err:=os.ReadFile(fmt.Sprintf("/proc/%d/status",pid));if err!=nil {t.Fatal("observed renderer exited before suspension",err)}
   for _,line:=range strings.Split(string(status),"\n") {
    if strings.HasPrefix(line,"State:") && strings.Contains(line,"T (stopped)") {return child}
   }
   time.Sleep(time.Millisecond)
  }
  t.Fatal("renderer did not reach stopped state")
 }
 t.Fatal("real trim renderer was not observed; cancellation case was not exercised")
 return stoppedExportRenderer{}
}

func assertAnonymousTrimPresent(t *testing.T,dir string) {
 t.Helper();entries,err:=os.ReadDir("/proc/self/fd");if err!=nil {t.Fatal(err)}
 for _,entry:=range entries {
  target,err:=os.Readlink(filepath.Join("/proc/self/fd",entry.Name()));if os.IsNotExist(err) {continue};if err!=nil {t.Fatal(err)}
  if strings.HasPrefix(target,dir+string(os.PathSeparator)+"simplenvr-trim-") && strings.HasSuffix(target," (deleted)") {return}
 }
 t.Fatal("anonymous trim output descriptor not observed during rendering")
}

func TestAuthenticatedTrimCancellationBeforeHeadersReapsStoppedRenderer(t *testing.T) {
 for _,shutdown:=range []bool{false,true} {t.Run(fmt.Sprintf("shutdown=%t",shutdown),func(t *testing.T){
  f:=newExportNetworkFixture(t,true)
  enlargeExportRenderFixture(t,f)
  result,cancelRequest:=startPreheaderTrimRequest(t,f);defer cancelRequest()
  child:=stopObservedTrimRenderer(t,f,result)
  if len(f.pool.slots)!=1 {t.Fatal("renderer has no export admission slot")}
  assertAnonymousTrimPresent(t,f.pool.tempDir)
  select {case completed:=<-result:t.Fatalf("response completed while renderer was stopped: %v %v",completed.response,completed.err);case <-time.After(100*time.Millisecond):}
  var persisted atomic.Bool;var shutdownDone chan error
  if shutdown {
   shutdownDone=make(chan error,1)
   ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
   go func(){shutdownDone<-shutdownApplication(ctx,f.n,f.gate,nil,func()error{persisted.Store(true);return nil})}()
  } else {cancelRequest()}
  // SIGINT cannot be handled while SIGSTOP remains active. The existing
  // interruptible-command grace must force-kill and join this real renderer.
  awaitExportNetwork(t,f.mediaReturned,"preheader cancellation did not finish media cleanup")
  if persisted.Load() {t.Fatal("persisted before tracked handler cleanup completed")}
  f.assertResourcesClosed(t)
  if _,err:=os.Stat(fmt.Sprintf("/proc/%d",child.pid));!os.IsNotExist(err) {t.Fatal("observed renderer was not reaped",err)}
  f.releaseCleanup();awaitExportNetwork(t,f.handlerDone,"preheader handler cleanup did not finish")
  if shutdown {
   select {case err:=<-shutdownDone:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("preheader shutdown did not join")}
   if !persisted.Load() {t.Fatal("shutdown persistence missing")}
  } else {awaitExportNetwork(t,f.gate.beginShutdown(),"canceled preheader export did not drain")}
  select {
  case completed:=<-result:
   if completed.response!=nil {
    if strings.HasPrefix(completed.response.Header.Get("Content-Type"),"video/") || completed.response.Header.Get("Content-Disposition")!="" || completed.response.Header.Get("X-Export-Trim-Applied")!="" {t.Fatal("canceled trim started a video response",completed.response.Header)}
   } else if completed.err==nil {t.Fatal("request returned neither response nor error")}
  case <-time.After(3*time.Second):cancelRequest();t.Fatal("client did not observe preheader cancellation/shutdown")
  }
 })}
}
