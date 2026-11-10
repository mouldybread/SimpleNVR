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
}

func newShutdownGate(next http.Handler) *shutdownGate {return &shutdownGate{next:next,drained:make(chan struct{})}}

func (g *shutdownGate) ServeHTTP(w http.ResponseWriter,r *http.Request) {
 g.mu.Lock()
 if g.closing {g.mu.Unlock();w.Header().Set("Retry-After","5");http.Error(w,"Server shutting down",503);return}
 mutation:=mutationPaths[r.URL.Path]
 if mutation {g.mutations.Add(1)}
 g.mu.Unlock()
 if mutation {defer g.mutations.Done()}
 g.next.ServeHTTP(w,r)
}

func (g *shutdownGate) beginShutdown() <-chan struct{} {
 g.once.Do(func(){
  g.mu.Lock();g.closing=true;g.mu.Unlock()
  go func(){g.mutations.Wait();close(g.drained)}()
 })
 return g.drained
}

// The shutdown flag is set before taking the state snapshot. Starts check that
// flag while holding NVR.mu, so a racing start is either rejected or included.
func (n *NVR) cancelAndWaitRecorders() {
 n.mu.RLock()
 cameras:=make([]*Camera,0,len(n.cameras));cancels:=make([]context.CancelFunc,0,len(n.cameras))
 for _,cam:=range n.cameras {cameras=append(cameras,cam);if cam.recordCancel!=nil {cancels=append(cancels,cam.recordCancel)}}
 n.mu.RUnlock()
 for _,cancel:=range cancels {cancel()}
 for _,cam:=range cameras {cam.recordWG.Wait()}
 n.mu.Lock()
 for _,cam:=range cameras {if n.cameras[cam.ID]==cam {cam.recordCancel=nil;cam.bufferManager=nil}}
 // Do not clear IsRecording: at process shutdown it preserves restart intent.
 n.mu.Unlock()
}

func shutdownApplication(ctx context.Context,n *NVR,gate *shutdownGate,server *http.Server,persist func() error) error {
 n.shuttingDown.Store(true)
 mutationsDone:=gate.beginShutdown()
 workersDone:=make(chan struct{})
 go func(){n.cancelAndWaitRecorders();close(workersDone)}()
 httpDone:=make(chan error,1)
 go func(){
  if server==nil {httpDone<-nil;return}
  httpCtx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
  err:=server.Shutdown(httpCtx)
  if err!=nil {
   closeErr:=server.Close()
   if errors.Is(err,context.DeadlineExceeded) && ctx.Err()==nil {err=closeErr} else {err=errors.Join(err,closeErr)}
  }
  httpDone<-err
 }()
 done:=make(chan error,1)
 go func(){
  <-workersDone
  <-mutationsDone
  httpErr:=<-httpDone
  if persist==nil {done<-errors.Join(httpErr,errors.New("shutdown persistence callback missing"));return}
  done<-errors.Join(httpErr,persist())
 }()
 select {
 case err:=<-done:return err
 case <-ctx.Done():if server!=nil {server.Close()};return ctx.Err()
 }
}

func runAuthenticatedServer(ctx context.Context,n *NVR,gate *shutdownGate,server *http.Server,cert,key string) error {
 var serveErr error
 var serveDone chan error
 received:=false
 if ctx.Err()==nil {
  serveDone=make(chan error,1)
  go func(){serveDone<-server.ListenAndServeTLS(cert,key)}()
  select {case <-ctx.Done():case serveErr=<-serveDone:received=true}
 }
 cleanupCtx,cancel:=context.WithTimeout(context.Background(),applicationShutdownGrace);defer cancel()
 cleanupErr:=shutdownApplication(cleanupCtx,n,gate,server,n.save)
 if serveDone!=nil && !received {select {case serveErr=<-serveDone:case <-cleanupCtx.Done():cleanupErr=errors.Join(cleanupErr,cleanupCtx.Err())}}
 if errors.Is(serveErr,http.ErrServerClosed) {serveErr=nil}
 return errors.Join(serveErr,cleanupErr)
}

// Main uses this context-aware entry before starting recorders.
func recoverBeforeStartup(ctx context.Context,n *NVR) {
 n.mu.RLock()
 cameras:=make([]*Camera,0,len(n.cameras));for _,cam:=range n.cameras {cameras=append(cameras,cam)}
 n.mu.RUnlock()
 for _,cam:=range cameras {
  if ctx.Err()!=nil {return}
  if err:=os.RemoveAll(filepath.Join(cam.RecordDir,"buffer"));err!=nil {log.Printf("Unable to clear transient buffer for camera %s",cam.ID)}
  if entries,err:=os.ReadDir(filepath.Join(cam.RecordDir,"temp"));err==nil && len(entries)>0 {log.Printf("Preserved unassembled event staging for camera %s; manual recovery/cleanup required",cam.ID)}
  files,_:=filepath.Glob(filepath.Join(cam.RecordDir,"*.tmp"))
  for _,file:=range files {
   if ctx.Err()!=nil {return}
   final:=partialRecordingName(strings.TrimSuffix(file,".tmp"))
   probeCtx,cancel:=context.WithTimeout(ctx,recordingFinalizeGrace)
   err:=publishRecording(probeCtx,file,final);cancel()
   if err!=nil {log.Printf("Partial recording for camera %s retained for inspection",cam.ID)}
  }
 }
}
