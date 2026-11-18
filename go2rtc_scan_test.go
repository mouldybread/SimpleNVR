package main

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
 "time"
)

func scanTestRequest(target string) *http.Request {
 values:=url.Values{"go2rtc-url":{target}}
 r:=httptest.NewRequest("POST","/api/go2rtc/scan",strings.NewReader(values.Encode()))
 r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 return r
}

func TestScanRejectsInvalidUpstreamBeforeMutation(t *testing.T) {
 cases:=[]struct{name,body string;status int}{
  {"status",`{}`,503}, {"malformed",`{"front":`,200}, {"null",`null`,200},
  {"array",`[]`,200}, {"invalid-entry",`{"front":null}`,200},
  {"invalid-name",`{"../front":{}}`,200}, {"trailing",`{} {}`,200},
  {"oversized",strings.Repeat(" ",scanResponseLimit+1),200},
 }
 entries:=make(map[string]interface{})
 for i:=0;i<=scanStreamLimit;i++ {entries[fmt.Sprint(i)]=map[string]interface{}{}}
 tooMany,_:=json.Marshal(entries)
 cases=append(cases,struct{name,body string;status int}{"too-many",string(tooMany),200})
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(tc.status);io.WriteString(w,tc.body)}));defer server.Close()
  n:=&NVR{cameras:map[string]*Camera{"existing":{ID:"existing",Name:"unchanged"}}}
  called:=false
  w:=httptest.NewRecorder()
  ok:=serveGo2rtcScan(w,scanTestRequest(server.URL),server.Client(),time.Second,func(ctx context.Context,streams []scannedStream) error {called=true;return n.applyScanStreams(ctx,streams)})
  if ok || w.Code!=502 || called || len(n.cameras)!=1 || n.cameras["existing"].Name!="unchanged" {t.Fatalf("failure changed state or returned success: code=%d applied=%v",w.Code,called)}
 })}
}

func TestScanStalledHeadersAndBodyHaveDeadlines(t *testing.T) {
 for _,bodyStall:=range []bool{false,true} {t.Run(fmt.Sprint(bodyStall),func(t *testing.T){
  entered:=make(chan struct{});disconnected:=make(chan struct{})
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   if bodyStall {w.WriteHeader(200);io.WriteString(w,`{"front":`);w.(http.Flusher).Flush()}
   close(entered);<-r.Context().Done();close(disconnected)
  }));defer server.Close()
  w:=httptest.NewRecorder();applied:=false;done:=make(chan bool,1)
  go func(){done<-serveGo2rtcScan(w,scanTestRequest(server.URL),server.Client(),200*time.Millisecond,func(context.Context,[]scannedStream) error {applied=true;return nil})}()
  select {case <-entered:case <-time.After(3*time.Second):t.Fatal("upstream was not contacted")}
  select {case ok:=<-done:if ok || w.Code!=504 || applied {t.Fatalf("code=%d applied=%v",w.Code,applied)};case <-time.After(3*time.Second):t.Fatal("scan exceeded deadline")}
  select {case <-disconnected:case <-time.After(3*time.Second):t.Fatal("upstream was not canceled")}
 })}
}

func TestScanRequestCancellationReleasesGate(t *testing.T) {
 entered:=make(chan struct{});disconnected:=make(chan struct{})
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-r.Context().Done();close(disconnected)}));defer server.Close()
 applied:=false
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){serveGo2rtcScan(w,r,server.Client(),time.Minute,func(context.Context,[]scannedStream) error {applied=true;return nil})}))
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 r:=scanTestRequest(server.URL).WithContext(ctx)
 w:=httptest.NewRecorder();done:=make(chan struct{})
 go func(){defer close(done);gate.ServeHTTP(w,r)}()
 select {case <-entered:case <-time.After(3*time.Second):t.Fatal("upstream was not contacted")}
 drained:=gate.beginShutdown()
 select {case <-drained:t.Fatal("gate drained while scan was blocked");default:}
 cancel()
 select {case <-done:case <-time.After(3*time.Second):t.Fatal("canceled scan blocked")}
 select {case <-drained:case <-time.After(3*time.Second):t.Fatal("mutation did not drain")}
 select {case <-disconnected:case <-time.After(3*time.Second):t.Fatal("upstream was not canceled")}
 if w.Code!=408 || applied {t.Fatalf("code=%d applied=%v",w.Code,applied)}
}

func TestScanSuccessAndInputLimits(t *testing.T) {
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/api/streams" {t.Errorf("unexpected path %s",r.URL.Path)}
  io.WriteString(w,`{"front":{"producers":[]},"back":{}}`)
 }));defer server.Close()
 var got []scannedStream
 w:=httptest.NewRecorder()
 if !serveGo2rtcScan(w,scanTestRequest(server.URL),server.Client(),time.Second,func(_ context.Context,streams []scannedStream) error {got=streams;return nil}) {t.Fatalf("scan failed: %s",w.Body.String())}
 if len(got)!=2 || got[0].Name!="back" || got[1].Name!="front" || got[1].RTSPURL!="rtsp://127.0.0.1:8554/front" {t.Fatalf("unexpected streams: %+v",got)}
 for _,body:=range []string{`{"url":`, `null`, `{} {}`,strings.Repeat(" ",scanRequestLimit+1)} {
  r:=httptest.NewRequest("POST","/api/go2rtc/scan",strings.NewReader(body));r.Header.Set("Content-Type","application/json")
  w:=httptest.NewRecorder();called:=false
  if serveGo2rtcScan(w,r,server.Client(),time.Second,func(context.Context,[]scannedStream) error {called=true;return nil}) || called || (w.Code!=400 && w.Code!=413) {t.Fatalf("invalid input accepted: %d",w.Code)}
 }
}

func TestScanRejectsShutdownBeforeCreatingDirectories(t *testing.T) {
 n:=&NVR{cameras:make(map[string]*Camera)};n.shuttingDown.Store(true)
 if err:=n.applyScanStreams(context.Background(),[]scannedStream{{Name:"front",RTSPURL:"rtsp://localhost:8554/front"}});err!=errRecorderShuttingDown || len(n.cameras)!=0 {t.Fatalf("shutdown scan changed state: %v",err)}
}
