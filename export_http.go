package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
)

const (
 exportCreatePath="/api/exports/create"
 exportCancelPath="/api/exports/cancel"
 exportStatusPath="/api/exports/status"
 exportDownloadPath="/api/exports/download"
)

// Extend the existing authentication and shutdown mutation policy. The route
// handler is installed INSIDE auth.Handler, so event tokens never reach it.
func init() {mutationPaths[exportCreatePath]=true;mutationPaths[exportCancelPath]=true}

type exportCreateRequest struct {
 CameraID string `json:"camera_id"`
 Start string `json:"start_time"`
 End string `json:"end_time"`
}
type exportHTTPView struct {
 exportJobView
 DownloadURL string `json:"download_url,omitempty"`
}
type exportPlanHTTPError struct {status int;message string}
func (e *exportPlanHTTPError) Error()string{return e.message}
type exportHTTPPlanner func(context.Context,exportCreateRequest)(exportJobRenderer,error)

type exportJobHTTP struct {
 auth *authManager
 store *exportJobStore
 plan exportHTTPPlanner
 ctx context.Context
 cancel context.CancelFunc
 mu sync.Mutex
 closing bool
 requests sync.WaitGroup
 once sync.Once
 done chan struct{}
 closeErr error
}

func newExportJobHTTP(auth *authManager,store *exportJobStore,plan exportHTTPPlanner)*exportJobHTTP {
 ctx,cancel:=context.WithCancel(context.Background())
 return &exportJobHTTP{auth:auth,store:store,plan:plan,ctx:ctx,cancel:cancel,done:make(chan struct{})}
}

// Opt-in scratch storage avoids silently putting GiB exports in the container
// writable layer. The operator must mount trusted private storage at this path.
func configuredExportJobHTTP(auth *authManager,n *NVR)(*exportJobHTTP,error) {
 limits,err:=loadExportJobLimits(os.Getenv);if err!=nil{return nil,err}
 directory:=os.Getenv("NVR_EXPORT_SCRATCH_DIR")
 if directory=="" {return newExportJobHTTP(auth,nil,nil),nil}
 if !filepath.IsAbs(directory){return nil,errors.New("NVR_EXPORT_SCRATCH_DIR must be absolute")}
 directory=filepath.Clean(directory)
 resolved,err:=filepath.EvalSymlinks(directory);if err!=nil{return nil,errors.New("export scratch directory unavailable")}
 if resolved!=directory || directory=="/recordings" || strings.HasPrefix(directory,"/recordings/"){return nil,errors.New("export scratch must be outside recordings and not symlinked")}
 info,err:=os.Stat(directory);if err!=nil || !info.IsDir(){return nil,errors.New("export scratch must be an existing directory")}
 n.mu.RLock()
 overlap:=false
 for _,cam:=range n.cameras {if cam==nil{continue};path,err:=filepath.Abs(cam.RecordDir);if err!=nil{overlap=true;break};path=filepath.Clean(path);if path==directory || strings.HasPrefix(directory,path+string(os.PathSeparator)) || strings.HasPrefix(path,directory+string(os.PathSeparator)){overlap=true;break}}
 n.mu.RUnlock()
 if overlap{return nil,errors.New("export scratch overlaps a camera directory")}
 store,err:=newExportJobStore(context.Background(),directory,limits);if err!=nil{return nil,err}
 service:=newExportJobHTTP(auth,store,nil)
 service.plan=func(ctx context.Context,req exportCreateRequest)(exportJobRenderer,error){return planExistingExportJob(ctx,n,store.directory,req)}
 return service,nil
}

func exportJobRoute(path string)bool {return path==exportCreatePath || path==exportCancelPath || path==exportStatusPath || path==exportDownloadPath}

// The identity comes only from a validated session, not a header, query or
// JSON owner. A new session or changed credentials cannot inherit old jobs.
func (s *exportJobHTTP) owner(r *http.Request)(string,error) {
 if s.auth==nil{return "",errExportJobMissing}
 cfg,stamp,err:=readAuthConfig(s.auth.path);if err!=nil{return "",err}
 user:=s.auth.sessions.GetString(r.Context(),"user")
 version:=s.auth.sessions.GetString(r.Context(),"credential_version")
 token:=s.auth.sessions.Token(r.Context())
 if user!=cfg.Username || version!=stamp || token==""{return "",errExportJobMissing}
 digest:=sha256.Sum256([]byte(user+"\x00"+version+"\x00"+token))
 return hex.EncodeToString(digest[:]),nil
}

func (s *exportJobHTTP) Handler(next http.Handler)http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if !exportJobRoute(r.URL.Path){next.ServeHTTP(w,r);return}
  w.Header().Set("Cache-Control","private, no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer")
  owner,err:=s.owner(r);if err!=nil{http.Error(w,"Unauthorized",401);return}
  allowed:="GET, HEAD"
  if r.URL.Path==exportCreatePath || r.URL.Path==exportCancelPath {allowed="POST"}
  if (allowed=="POST" && r.Method!=http.MethodPost) || (allowed!="POST" && r.Method!=http.MethodGet && r.Method!=http.MethodHead){w.Header().Set("Allow",allowed);http.Error(w,"Method not allowed",405);return}
  s.mu.Lock()
  if s.closing || s.store==nil {s.mu.Unlock();http.Error(w,"Prepared exports unavailable; configure export scratch storage or retry after restart",503);return}
  s.requests.Add(1);s.mu.Unlock();defer s.requests.Done()
  ctx,cancel:=context.WithCancel(r.Context());defer cancel()
  stopService:=context.AfterFunc(s.ctx,cancel);defer stopService()
  r=r.WithContext(ctx)
  // This also releases readers blocked behind a slow/disconnected client.
  writeDone:=make(chan struct{})
  stopWrite:=context.AfterFunc(ctx,func(){_ =http.NewResponseController(w).SetWriteDeadline(time.Now());close(writeDone)})
  defer func(){if !stopWrite(){<-writeDone}}()
  switch r.URL.Path {
  case exportCreatePath:s.create(w,r,owner)
  case exportCancelPath:s.cancelRequest(w,r,owner)
  case exportStatusPath:
   id,ok:=exportRequestID(w,r);if !ok{return}
   view,err:=s.store.status(owner,id);if err!=nil{writeExportJobHTTPError(w,err);return}
   writeExportJobView(w,r,200,view)
  case exportDownloadPath:
   id,ok:=exportRequestID(w,r);if !ok{return}
   reader,release,err:=s.store.acquire(owner,id);if err!=nil{writeExportJobHTTPError(w,err);return};defer release()
   w.Header().Set("Content-Type","video/mp4");w.Header().Set("Content-Disposition",`attachment; filename="export-`+id+`.mp4"`)
   http.ServeContent(w,r,"export-"+id+".mp4",time.Time{},reader)
  }
 })
}

func exportRequestID(w http.ResponseWriter,r *http.Request)(string,bool) {
 query:=r.URL.Query();values:=query["id"]
 if len(query)!=1 || len(values)!=1 || !validExportJobID(values[0]){http.Error(w,"One valid export id is required",400);return "",false}
 return values[0],true
}
func validExportJobID(id string)bool {if len(id)!=64{return false};raw,err:=hex.DecodeString(id);return err==nil && len(raw)==32}
func decodeExportJobJSON(w http.ResponseWriter,r *http.Request,value interface{})bool {
 if r.URL.RawQuery!=""{http.Error(w,"Query parameters not accepted",400);return false}
 kind,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"));if err!=nil || kind!="application/json"{http.Error(w,"Content-Type must be application/json",415);return false}
 decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));decoder.DisallowUnknownFields()
 if err:=decoder.Decode(value);err!=nil{http.Error(w,"Invalid export request",400);return false}
 if err:=decoder.Decode(&struct{}{});err!=io.EOF{http.Error(w,"One export JSON object is required",400);return false}
 return true
}
func writeExportJobView(w http.ResponseWriter,r *http.Request,status int,view exportJobView) {
 body:=exportHTTPView{exportJobView:view}
 if view.State=="ready"{body.DownloadURL=exportDownloadPath+"?id="+view.ID}
 w.Header().Set("Content-Type","application/json");w.WriteHeader(status)
 if r.Method!=http.MethodHead{json.NewEncoder(w).Encode(body)}
}
func writeExportJobHTTPError(w http.ResponseWriter,err error) {
 status,message:=404,"Export unavailable, expired, or not ready"
 if errors.Is(err,errExportJobsBusy){status,message=429,"Export capacity exhausted; release prepared exports or wait for expiry";w.Header().Set("Retry-After","5")}
 if errors.Is(err,errExportJobsClosed){status,message=503,"Export service stopping"}
 http.Error(w,message,status)
}
func (s *exportJobHTTP) create(w http.ResponseWriter,r *http.Request,owner string) {
 var req exportCreateRequest;if !decodeExportJobJSON(w,r,&req){return}
 start,e1:=time.Parse(time.RFC3339Nano,req.Start);end,e2:=time.Parse(time.RFC3339Nano,req.End)
 if req.CameraID=="" || len(req.CameraID)>256 || e1!=nil || e2!=nil || !end.After(start){http.Error(w,"Valid camera_id and ordered RFC3339 start_time/end_time required",400);return}
 if s.plan==nil{http.Error(w,"Export renderer unavailable",503);return}
 render,err:=s.plan(r.Context(),req)
 if err!=nil{
  var requestErr *exportPlanHTTPError
  if errors.As(err,&requestErr){http.Error(w,requestErr.message,requestErr.status)}else if errors.Is(err,errExportPlanGap) || errors.Is(err,errExportPlanOverlap) || errors.Is(err,errExportInputLimit) || errors.Is(err,errExportPlanInvalid){writeExportPlanError(w,err)}else{writeRecordingIntervalError(w,r,err)}
  return
 }
 if r.Context().Err()!=nil{return}
 view,err:=s.store.submit(owner,render);if err!=nil{writeExportJobHTTPError(w,err);return}
 w.Header().Set("Location",exportStatusPath+"?id="+view.ID);writeExportJobView(w,r,202,view)
}
func (s *exportJobHTTP) cancelRequest(w http.ResponseWriter,r *http.Request,owner string) {
 var req struct{ID string `json:"id"`};if !decodeExportJobJSON(w,r,&req){return}
 if !validExportJobID(req.ID){http.Error(w,"Valid export id required",400);return}
 if err:=s.store.cancelJob(owner,req.ID);err!=nil{writeExportJobHTTPError(w,err);return}
 w.WriteHeader(http.StatusNoContent)
}

// This integration deliberately retains the existing render restrictions.
// Stream-copy compatibility and audio-preserving accurate trim are next steps.
func planExistingExportJob(parent context.Context,n *NVR,scratch string,req exportCreateRequest)(exportJobRenderer,error) {
 start,_:=time.Parse(time.RFC3339Nano,req.Start);end,_:=time.Parse(time.RFC3339Nano,req.End)
 n.mu.RLock();cam:=n.cameras[req.CameraID];var camera timelineCamera;if cam!=nil{camera=timelineCamera{cam.ID,cam.Name,cam.RecordDir}};n.mu.RUnlock()
 if cam==nil{return nil,&exportPlanHTTPError{404,"Camera not found"}}
 ctx,cancel:=context.WithTimeout(parent,recordingMetadataBudget);defer cancel()
 window:=timelineWindow{Start:start.UTC(),End:end.UTC()}
 clips,err:=collectRecordingIntervals(ctx,[]timelineCamera{camera},window,recordingDurations);if err!=nil{return nil,err}
 plan,err:=buildExportPlan(clips,window);if err!=nil{return nil,err}
 return func(ctx context.Context,w io.Writer)error {
  pins,err:=exportInputPins.prepare(ctx,plan.clips());if err!=nil{return err};defer pins.Close()
  if plan.NeedsTrim {
   output,err:=renderTrimmedExport(ctx,pins,plan,scratch,maxTrimBytes);if err!=nil{return err};defer output.Close()
   if _,err=io.Copy(w,output.file);err!=nil{return err}
  }else{
   cmd,err:=pins.command(ctx);if err!=nil{return err};cmd.Stdout=w;cmd.Stderr=io.Discard
   if err=cmd.Run();err!=nil{return err}
  }
  if err:=pins.validate(ctx);err!=nil{return err}
  output,ok:=w.(*boundedExportJobWriter);if !ok{return errors.New("export output descriptor unavailable")}
  verify,stop:=context.WithTimeout(ctx,5*time.Second);defer stop()
  duration,err:=probeVideoDuration(verify,output.file);if err!=nil{return err}
  difference:=duration-plan.End.Sub(plan.Start);if difference<0{difference=-difference}
  if duration<=0 || difference>250*time.Millisecond{return errors.New("prepared export duration validation failed")}
  return ctx.Err()
 },nil
}

func (s *exportJobHTTP) Close(ctx context.Context)error {
 s.once.Do(func(){
  s.mu.Lock();s.closing=true;s.cancel();if s.store!=nil{s.store.cancel()};s.mu.Unlock()
  go func(){s.requests.Wait();if s.store!=nil{s.closeErr=s.store.Close(context.Background())};close(s.done)}()
 })
 select{case <-s.done:return s.closeErr;case <-ctx.Done():return ctx.Err()}
}
func (s *exportJobHTTP) runLifecycle(ctx context.Context){<-ctx.Done();_ =s.Close(context.Background())}
