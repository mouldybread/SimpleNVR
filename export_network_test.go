package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

// Small kernel socket buffers make non-reading clients exert real TCP/TLS
// backpressure rather than relying on a fake blocking ResponseWriter.
type exportSmallSocketListener struct {net.Listener}
func (l exportSmallSocketListener) Accept() (net.Conn,error) {
 conn,err:=l.Listener.Accept();if err!=nil {return nil,err}
 tcp,ok:=conn.(*net.TCPConn);if !ok {conn.Close();return nil,errors.New("expected TCP listener")}
 if err:=tcp.SetWriteBuffer(4096);err!=nil {conn.Close();return nil,err}
 return conn,nil
}

type exportNetworkWriter struct {
 http.ResponseWriter
 outstanding *atomic.Int32
}
func (w *exportNetworkWriter) Unwrap() http.ResponseWriter {return w.ResponseWriter}
func (w *exportNetworkWriter) Write(p []byte) (int,error) {
 w.outstanding.Add(1);defer w.outstanding.Add(-1)
 return w.ResponseWriter.Write(p)
}

type exportNetworkFixture struct {
 server *httptest.Server
 client *http.Client
 transport *http.Transport
 gate *shutdownGate
 n *NVR
 pool *exportInputPool
 source string
 sourceHash [32]byte
 cookies []*http.Cookie
 mediaReturned chan struct{}
 handlerDone chan struct{}
 cleanupRelease chan struct{}
 releaseOnce sync.Once
 outstanding atomic.Int32
 filesMu sync.Mutex
 opened []*os.File
}

func (f *exportNetworkFixture) releaseCleanup(){f.releaseOnce.Do(func(){close(f.cleanupRelease)})}

func newExportNetworkFixture(t *testing.T,holdCleanup bool) *exportNetworkFixture {
 t.Helper();requireFFmpeg(t)
 dir:=t.TempDir();source:=filepath.Join(dir,"20261110_000000_fixture.mp4")
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=256x144:rate=10","-filter_threads","1","-vf","noise=alls=100:allf=t+u","-t","2","-c:v","mpeg4","-q:v","2","-threads","1","-an","-y",source)
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("network fixture media: %v %s",err,output)}
 info,err:=os.Stat(source);if err!=nil || info.Size()<65536 || info.Size()>8<<20 {t.Fatal("fixture must exceed socket buffers within test cap",err)}
 f:=&exportNetworkFixture{source:source,pool:newExportInputPool(1,1),n:&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}},mediaReturned:make(chan struct{}),handlerDone:make(chan struct{}),cleanupRelease:make(chan struct{})}
 sourceBytes,err:=os.ReadFile(source);if err!=nil {t.Fatal(err)};f.sourceHash=sha256.Sum256(sourceBytes)
 f.pool.tempDir=t.TempDir()
 f.pool.open=func(path string)(*os.File,error){
  file,err:=openExportInput(path);if err==nil {f.filesMu.Lock();f.opened=append(f.opened,file);f.filesMu.Unlock()};return file,err
 }
 if !holdCleanup {f.releaseCleanup()}
 cache:=newDurationCache(2,2,probeVideoDuration)
 auth,_:=authFixture(t)
 route:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/api/download" {http.NotFound(w,r);return}
  defer close(f.handlerDone)
  if r.Header.Get("Cookie")!="" || r.Header.Get("Authorization")!="" {t.Error("auth credentials reached media handler")}
  serveDownload(w,r,f.n,cache,f.pool)
  close(f.mediaReturned)
  <-f.cleanupRelease
 })
 f.gate=newShutdownGate(auth.Handler(route))
 server:=httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/api/download" {f.gate.ServeHTTP(&exportNetworkWriter{ResponseWriter:w,outstanding:&f.outstanding},r)} else {f.gate.ServeHTTP(w,r)}
 }))
 server.Listener=exportSmallSocketListener{server.Listener}
 server.EnableHTTP2=false;server.StartTLS();f.server=server
 transport:=server.Client().Transport.(*http.Transport).Clone();transport.ForceAttemptHTTP2=false
 dialer:=&net.Dialer{Timeout:3*time.Second}
 transport.DialContext=func(ctx context.Context,network,address string)(net.Conn,error){
  conn,err:=dialer.DialContext(ctx,network,address);if err!=nil {return nil,err}
  tcp,ok:=conn.(*net.TCPConn);if !ok {conn.Close();return nil,errors.New("expected TCP client")}
  if holdCleanup {if err:=tcp.SetReadBuffer(1024);err!=nil {conn.Close();return nil,err}};return conn,nil
 }
 f.transport=transport
 f.client=&http.Client{Transport:transport,Timeout:20*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 t.Cleanup(func(){
  f.releaseCleanup();server.CloseClientConnections();transport.CloseIdleConnections()
  drained:=f.gate.beginShutdown()
  select {case <-drained:case <-time.After(12*time.Second):t.Error("export fixture did not drain")}
  server.Close()
 })
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 login,err:=f.client.PostForm(server.URL+"/auth/login",form);if err!=nil {t.Fatal(err)}
 _,_=io.Copy(io.Discard,io.LimitReader(login.Body,4096));login.Body.Close()
 if login.StatusCode!=303 {t.Fatal("HTTPS login failed",login.StatusCode)}
 f.cookies=login.Cookies();if len(f.cookies)==0 {t.Fatal("login returned no session")}
 return f
}

func (f *exportNetworkFixture) request(t *testing.T,trim bool) (*http.Response,context.CancelFunc) {
 t.Helper()
 start,end:="2026-11-10T00:00:00Z","2026-11-10T00:00:02Z"
 if trim {start,end="2026-11-10T00:00:00.2Z","2026-11-10T00:00:01.8Z"}
 query:=url.Values{"camera_id":{"front"},"start_time":{start},"end_time":{end}}
 ctx,cancel:=context.WithCancel(context.Background())
 req,err:=http.NewRequestWithContext(ctx,"GET",f.server.URL+"/api/download?"+query.Encode(),nil);if err!=nil {cancel();t.Fatal(err)}
 for _,cookie:=range f.cookies {req.AddCookie(cookie)}
 response,err:=f.client.Do(req);if err!=nil {cancel();t.Fatal(err)}
 if response.StatusCode!=200 || response.ProtoMajor!=1 || response.Header.Get("Content-Type")!="video/mp4" || response.Header.Get("X-Export-Trim-Applied")!=strconv.FormatBool(trim) {
  body,_:=io.ReadAll(io.LimitReader(response.Body,4096));response.Body.Close();cancel();t.Fatalf("unexpected export: %d %v %s",response.StatusCode,response.Header,body)
 }
 t.Cleanup(func(){cancel();response.Body.Close()})
 return response,cancel
}

func awaitExportNetwork(t *testing.T,ch <-chan struct{},label string) {
 t.Helper();select {case <-ch:case <-time.After(12*time.Second):t.Fatal(label)}
}

func (f *exportNetworkFixture) assertResourcesClosed(t *testing.T) {
 t.Helper()
 if len(f.pool.slots)!=0 {t.Fatal("export admission slot leaked")}
 sourceBytes,err:=os.ReadFile(f.source);if err!=nil || sha256.Sum256(sourceBytes)!=f.sourceHash {t.Fatal("source recording changed",err)}
 f.filesMu.Lock();files:=append([]*os.File(nil),f.opened...);f.filesMu.Unlock()
 if len(files)!=1 {t.Fatalf("expected one pinned input, got %d",len(files))}
 for _,file:=range files {if _,err:=file.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("pinned input descriptor still open",err)}}
 entries,err:=os.ReadDir(f.pool.tempDir);if err!=nil || len(entries)!=0 {t.Fatal("named export staging leaked",err)}
 descriptors,err:=os.ReadDir("/proc/self/fd");if err!=nil {t.Fatal(err)}
 for _,entry:=range descriptors {
  target,err:=os.Readlink(filepath.Join("/proc/self/fd",entry.Name()));if os.IsNotExist(err) {continue};if err!=nil {t.Fatal(err)}
  if strings.HasPrefix(target,f.pool.tempDir+string(os.PathSeparator)) {t.Fatal("anonymous export descriptor leaked",target)}
 }
}

func (f *exportNetworkFixture) awaitBackpressure(t *testing.T) {
 t.Helper();deadline:=time.Now().Add(5*time.Second);stable:=time.Time{}
 for time.Now().Before(deadline) {
  select {case <-f.mediaReturned:t.Fatal("export finished before backpressure was exercised");default:}
  if f.outstanding.Load()>0 && len(f.pool.slots)==1 {
   if stable.IsZero() {stable=time.Now()}
   if time.Since(stable)>=100*time.Millisecond {return}
  } else {stable=time.Time{}}
  time.Sleep(5*time.Millisecond)
 }
 t.Fatal("no sustained outstanding network write observed")
}

// Locate the stream-copy child by its inherited source descriptor. This is a
// Linux test observation, not a production process-control mechanism.
func exportChildUsingSource(t *testing.T,source string) int {
 t.Helper();tasks,err:=os.ReadDir("/proc/self/task");if err!=nil {t.Fatal(err)}
 seen:=make(map[int]bool)
 for _,task:=range tasks {
  raw,err:=os.ReadFile(filepath.Join("/proc/self/task",task.Name(),"children"));if os.IsNotExist(err) {continue};if err!=nil {t.Fatal(err)}
  for _,value:=range strings.Fields(string(raw)) {
   pid,err:=strconv.Atoi(value);if err!=nil || seen[pid] {continue};seen[pid]=true
   target,err:=os.Readlink(fmt.Sprintf("/proc/%d/fd/4",pid));if err==nil && target==source {return pid}
  }
 }
 return 0
}

func TestAuthenticatedHTTPSExportsCompleteForSlowReaders(t *testing.T) {
 for _,trim:=range []bool{false,true} {t.Run(fmt.Sprintf("trim=%t",trim),func(t *testing.T){
  f:=newExportNetworkFixture(t,false)
  for _,bearer:=range []string{"","Bearer 0123456789012345678901234567890123456789012"} {
   request,err:=http.NewRequest("GET",f.server.URL+"/api/download?camera_id=front&start_time=2026-11-10T00:00:00Z&end_time=2026-11-10T00:00:02Z",nil);if err!=nil {t.Fatal(err)}
   request.Header.Set("Authorization",bearer);denied,err:=f.client.Do(request);if err!=nil {t.Fatal(err)}
   _,_=io.Copy(io.Discard,io.LimitReader(denied.Body,4096));denied.Body.Close();if denied.StatusCode!=401 {t.Fatal("sessionless network export accepted",denied.StatusCode)}
  }
  f.filesMu.Lock();unauthenticatedPins:=len(f.opened);f.filesMu.Unlock();if unauthenticatedPins!=0 || len(f.pool.slots)!=0 {t.Fatal("unauthenticated export reached pinning")}
  response,cancel:=f.request(t,trim);defer cancel()
  var received bytes.Buffer;chunk:=make([]byte,4096);reads:=0
  for {
   n,err:=response.Body.Read(chunk)
   if n>0 {reads++;received.Write(chunk[:n]);if received.Len()>8<<20 {t.Fatal("test response cap exceeded")};time.Sleep(time.Millisecond)}
   if err==io.EOF {break};if err!=nil {t.Fatal("slow response failed",err)}
  }
  response.Body.Close();awaitExportNetwork(t,f.handlerDone,"completed export did not return")
  if reads<2 || received.Len()<32 || !bytes.Contains(received.Bytes()[:32],[]byte("ftyp")) {t.Fatal("missing MP4 response")}
  output:=filepath.Join(t.TempDir(),"received.mp4");if err:=os.WriteFile(output,received.Bytes(),0600);err!=nil {t.Fatal(err)}
  ctx,stop:=context.WithTimeout(context.Background(),15*time.Second);defer stop()
  decode:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-i",output,"-f","null","-")
  if result,err:=decode.CombinedOutput();err!=nil {t.Fatalf("received synthetic export did not decode: %v %s",err,result)}
  media,err:=os.Open(output);if err!=nil {t.Fatal(err)}
  duration,probeErr:=probeVideoDuration(ctx,media);closeErr:=media.Close()
  expected:=2*time.Second;if trim {expected=1600*time.Millisecond}
  difference:=duration-expected;if difference<0 {difference=-difference}
  if probeErr!=nil || closeErr!=nil || difference>200*time.Millisecond {t.Fatal("unexpected synthetic response duration",duration,probeErr,closeErr)}
  f.assertResourcesClosed(t)
 })}
}

func TestAuthenticatedHTTPSExportDisconnectAndShutdownDuringBackpressure(t *testing.T) {
 for _,trim:=range []bool{false,true} {for _,shutdown:=range []bool{false,true} {t.Run(fmt.Sprintf("trim=%t/shutdown=%t",trim,shutdown),func(t *testing.T){
  f:=newExportNetworkFixture(t,true);response,cancel:=f.request(t,trim);defer cancel()
  first:=make([]byte,1);if _,err:=io.ReadFull(response.Body,first);err!=nil {t.Fatal(err)}
  f.awaitBackpressure(t)
  pid:=exportChildUsingSource(t,f.source)
  if !trim && pid==0 {t.Fatal("stream-copy child not observed during backpressure")}
  if trim && pid!=0 {t.Fatal("trim renderer still active after video headers")}
  var persisted atomic.Bool;var shutdownDone chan error
  if shutdown {
   shutdownDone=make(chan error,1)
   ctx,stop:=context.WithTimeout(context.Background(),15*time.Second);defer stop()
   go func(){shutdownDone<-shutdownApplication(ctx,f.n,f.gate,nil,func()error{persisted.Store(true);return nil})}()
  } else {cancel();response.Body.Close()}
  awaitExportNetwork(t,f.mediaReturned,"canceled media handler did not release resources")
  if persisted.Load() {t.Fatal("shutdown persisted before tracked handler cleanup")}
  f.assertResourcesClosed(t)
  if pid!=0 {if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("stream-copy child not reaped",err)}}
  f.releaseCleanup();awaitExportNetwork(t,f.handlerDone,"handler cleanup did not finish")
  if shutdown {
   select {case err:=<-shutdownDone:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("shutdown did not join export")}
   if !persisted.Load() {t.Fatal("shutdown persistence did not run")}
  } else {awaitExportNetwork(t,f.gate.beginShutdown(),"disconnected export did not drain")}
  cancel();response.Body.Close()
 })}}
}
