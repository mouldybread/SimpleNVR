package main

import (
 "context"
 "errors"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
)

const applicationShutdownGrace=60*time.Second

type shutdownGate struct {
 next http.Handler
 mu sync.Mutex
 closing bool
 mutations sync.WaitGroup
 once sync.Once
 drained chan struct{}
 jobs map[uint64]context.CancelFunc
 nextJob uint64
}
func newShutdownGate(next http.Handler) *shutdownGate {return &shutdownGate{next:next,drained:make(chan struct{}),jobs:make(map[uint64]context.CancelFunc)}}
func (g *shutdownGate) ServeHTTP(w http.ResponseWriter,r *http.Request) {
 g.mu.Lock()
 if g.closing {g.mu.Unlock();w.Header().Set("Retry-After","5");http.Error(w,"Server shutting down",503);return}
 job:=r.URL.Path=="/api/download" || r.URL.Path=="/api/timeline/recordings" || r.URL.Path=="/api/storage/status" || r.URL.Path=="/api/recovery/status" || strings.HasPrefix(r.URL.Path,"/api/stream/")
 tracked:=mutationPaths[r.URL.Path] || job
 if tracked {g.mutations.Add(1)}
 var cancel context.CancelFunc;var id uint64
 if job {var ctx context.Context;ctx,cancel=context.WithCancel(r.Context());r=r.WithContext(ctx);g.nextJob++;id=g.nextJob;g.jobs[id]=cancel}
 g.mu.Unlock()
 if tracked {defer g.mutations.Done()}
 if job {
  callbackDone:=make(chan struct{})
  stop:=context.AfterFunc(r.Context(),func(){_=http.NewResponseController(w).SetWriteDeadline(time.Now());close(callbackDone)})
  defer func(){if !stop() {<-callbackDone};cancel();g.mu.Lock();delete(g.jobs,id);g.mu.Unlock()}()
 }
 g.next.ServeHTTP(w,r)
}
func (g *shutdownGate) beginShutdown() <-chan struct{} {
 g.once.Do(func(){
  g.mu.Lock();g.closing=true
  cancels:=make([]context.CancelFunc,0,len(g.jobs));for _,cancel:=range g.jobs {cancels=append(cancels,cancel)}
  g.mu.Unlock()
  for _,cancel:=range cancels {cancel()}
  go func(){g.mutations.Wait();close(g.drained)}()
 })
 return g.drained
}
func (n *NVR) cancelAndWaitRecorders() {
 n.mu.RLock()
 cameras:=make([]*Camera,0,len(n.cameras));cancels:=make([]context.CancelFunc,0,len(n.cameras))
 for _,cam:=range n.cameras {cameras=append(cameras,cam);if cam.recordCancel!=nil {cancels=append(cancels,cam.recordCancel)}}
 n.mu.RUnlock()
 for _,cancel:=range cancels {cancel()}
 for _,cam:=range cameras {cam.recordWG.Wait()}
 n.mu.Lock()
 for _,cam:=range cameras {if n.cameras[cam.ID]==cam {cam.recordCancel=nil;cam.bufferManager=nil}}
 n.mu.Unlock()
}
func shutdownApplication(ctx context.Context,n *NVR,gate *shutdownGate,server *http.Server,persist func() error) error {
 n.shuttingDown.Store(true);mutationsDone:=gate.beginShutdown()
 workersDone:=make(chan struct{});go func(){n.cancelAndWaitRecorders();close(workersDone)}()
 httpDone:=make(chan error,1)
 go func(){
  if server==nil {httpDone<-nil;return}
  httpCtx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
  err:=server.Shutdown(httpCtx)
  if err!=nil {closeErr:=server.Close();if errors.Is(err,context.DeadlineExceeded) && ctx.Err()==nil {err=closeErr} else {err=errors.Join(err,closeErr)}}
  httpDone<-err
 }()
 done:=make(chan error,1)
 go func(){<-workersDone;<-mutationsDone;httpErr:=<-httpDone;if persist==nil {done<-errors.Join(httpErr,errors.New("shutdown persistence callback missing"));return};done<-errors.Join(httpErr,persist())}()
 select {case err:=<-done:return err;case <-ctx.Done():if server!=nil {server.Close()};return ctx.Err()}
}
func runAuthenticatedServer(ctx context.Context,n *NVR,gate *shutdownGate,server *http.Server,cert,key string) error {
 var serveErr error;var serveDone chan error;received:=false
 if ctx.Err()==nil {
  serveDone=make(chan error,1);go func(){serveDone<-server.ListenAndServeTLS(cert,key)}()
  select {case <-ctx.Done():case serveErr=<-serveDone:received=true}
 }
 cleanupCtx,cancel:=context.WithTimeout(context.Background(),applicationShutdownGrace);defer cancel()
 cleanupErr:=shutdownApplication(cleanupCtx,n,gate,server,n.save)
 if serveDone!=nil && !received {select {case serveErr=<-serveDone:case <-cleanupCtx.Done():cleanupErr=errors.Join(cleanupErr,cleanupCtx.Err())}}
 if errors.Is(serveErr,http.ErrServerClosed) {serveErr=nil}
 return errors.Join(serveErr,cleanupErr)
}
func recoverBeforeStartup(ctx context.Context,n *NVR) {
 n.mu.RLock();cameras:=make([]*Camera,0,len(n.cameras));for _,cam:=range n.cameras {cameras=append(cameras,cam)};n.mu.RUnlock()
 for _,cam:=range cameras {
  if ctx.Err()!=nil {return}
  if err:=os.RemoveAll(filepath.Join(cam.RecordDir,"buffer"));err!=nil {log.Printf("Unable to clear transient buffer for camera %s",cam.ID)}
  if entries,err:=os.ReadDir(filepath.Join(cam.RecordDir,"temp"));err==nil && len(entries)>0 {log.Printf("Preserved unassembled event staging for camera %s; manual recovery/cleanup required",cam.ID)}
  files,_:=filepath.Glob(filepath.Join(cam.RecordDir,"*.tmp"))
  for _,file:=range files {
   if ctx.Err()!=nil {return}
   final:=partialRecordingName(strings.TrimSuffix(file,".tmp"));probeCtx,cancel:=context.WithTimeout(ctx,recordingFinalizeGrace)
   err:=publishRecording(probeCtx,file,final);cancel()
   if err!=nil {log.Printf("Partial recording for camera %s retained for inspection",cam.ID)}
  }
 }
}
