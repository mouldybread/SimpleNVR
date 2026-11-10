package main

import (
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "os"
 "os/signal"
 "strings"
 "sync"
 "testing"
 "time"
)

func waitHTTPJob(t *testing.T,ch <-chan struct{},label string) {
 t.Helper();select {case <-ch:case <-time.After(5*time.Second):t.Fatal(label)}
}

func TestHTTPJobHelperProcess(t *testing.T) {
 if os.Getenv("SIMPLENVR_HTTP_JOB_HELPER")!="1" {return}
 signal.Ignore(os.Interrupt)
 fmt.Fprintln(os.Stdout,"ready")
 for {time.Sleep(time.Hour)}
}

type jobReadyWriter struct {once sync.Once;ready chan struct{}}
func (w *jobReadyWriter) Write(p []byte) (int,error) {
 if strings.Contains(string(p),"ready") {w.once.Do(func(){close(w.ready)})}
 return len(p),nil
}

func TestExportJobShutdownReapsChildBeforeDrain(t *testing.T) {
 ready:=make(chan struct{});reaped:=make(chan struct{});finished:=make(chan struct{})
 release:=make(chan struct{});var once sync.Once
 defer once.Do(func(){close(release)})
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  cmd:=interruptibleCommand(r.Context(),os.Args[0],100*time.Millisecond,"-test.run=^TestHTTPJobHelperProcess$")
  cmd.Env=append(os.Environ(),"SIMPLENVR_HTTP_JOB_HELPER=1")
  cmd.Stdout=&jobReadyWriter{ready:ready}
  err:=cmd.Run()
  if err==nil || cmd.ProcessState==nil {t.Errorf("child was not canceled and reaped: %v",err)}
  close(reaped)
  <-release
 }))
 go func(){defer close(finished);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/download",nil))}()
 waitHTTPJob(t,ready,"child did not start")
 drained:=gate.beginShutdown()
 waitHTTPJob(t,reaped,"child was not killed and reaped")
 select {case <-drained:t.Fatal("drained before handler cleanup");default:}
 once.Do(func(){close(release)})
 waitHTTPJob(t,finished,"export handler did not return")
 waitHTTPJob(t,drained,"export job did not drain")
 w:=httptest.NewRecorder();gate.ServeHTTP(w,httptest.NewRequest("GET","/api/download",nil))
 if w.Code!=503 {t.Fatal("late export was admitted")}
}

func TestProxyJobShutdownCancelsHeadersAndBody(t *testing.T) {
 for _,bodyStall:=range []bool{false,true} {t.Run(fmt.Sprint(bodyStall),func(t *testing.T){
  entered:=make(chan struct{});disconnected:=make(chan struct{})
  upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   if bodyStall {w.WriteHeader(200);io.WriteString(w,"partial");w.(http.Flusher).Flush()}
   close(entered);<-r.Context().Done();close(disconnected)
  }));defer upstream.Close()
  gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){relayLiveStream(w,r,upstream.URL,upstream.Client())}))
  done:=make(chan struct{})
  go func(){defer close(done);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/stream/front",nil))}()
  waitHTTPJob(t,entered,"proxy did not contact upstream")
  drained:=gate.beginShutdown()
  waitHTTPJob(t,done,"proxy handler did not return")
  waitHTTPJob(t,disconnected,"proxy upstream was not canceled")
  waitHTTPJob(t,drained,"proxy job did not drain")
 })}
}

func TestProxyJobClientDisconnectCancelsUpstream(t *testing.T) {
 entered:=make(chan struct{});disconnected:=make(chan struct{})
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-r.Context().Done();close(disconnected)}));defer upstream.Close()
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){relayLiveStream(w,r,upstream.URL,upstream.Client())}))
 server:=httptest.NewServer(gate);defer server.Close()
 req,err:=http.NewRequest("GET",server.URL+"/api/stream/front",nil)
 if err!=nil {t.Fatal(err)}
 client:=&http.Client{Timeout:300*time.Millisecond}
 done:=make(chan struct{})
 go func(){defer close(done);resp,_:=client.Do(req);if resp!=nil {resp.Body.Close()}}()
 waitHTTPJob(t,entered,"proxy did not contact upstream")
 waitHTTPJob(t,done,"client did not time out")
 waitHTTPJob(t,disconnected,"client disconnect did not cancel upstream")
 waitHTTPJob(t,gate.beginShutdown(),"disconnected proxy did not drain")
}

func TestProxyFiltersHeadersAndPreservesSourceQuery(t *testing.T) {
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Cookie")!="" || r.Header.Get("Authorization")!="" || r.Header.Get("Connection")!="" {t.Error("sensitive or hop-by-hop request headers forwarded")}
  if r.URL.Query().Get("src")!="front & back" {t.Errorf("source query corrupted: %s",r.URL.RawQuery)}
  w.Header().Set("Set-Cookie","upstream=secret")
  w.Header().Set("Connection","close")
  w.Header().Set("Content-Type","video/mp4")
  io.WriteString(w,"video")
 }));defer upstream.Close()
 r:=httptest.NewRequest("GET","/api/stream/front",nil)
 r.Header.Set("Cookie","session=secret");r.Header.Set("Authorization","Bearer secret");r.Header.Set("Connection","keep-alive")
 w:=httptest.NewRecorder()
 relayLiveStream(w,r,upstream.URL+"?src=front+%26+back",upstream.Client())
 if w.Code!=200 || w.Body.String()!="video" || w.Header().Get("Set-Cookie")!="" || w.Header().Get("Connection")!="" {t.Fatal("invalid proxy response or leaked headers")}
}
