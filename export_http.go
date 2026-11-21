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
const(exportCreatePath="/api/exports/create";exportCancelPath="/api/exports/cancel";exportStatusPath="/api/exports/status";exportDownloadPath="/api/exports/download")
func init(){mutationPaths[exportCreatePath]=true;mutationPaths[exportCancelPath]=true}
type exportCreateRequest struct{
 CameraID string `json:"camera_id"`
 Start string `json:"start_time"`
 End string `json:"end_time"`
 Mode string `json:"mode,omitempty"`
 AcceptedStart string `json:"accepted_start,omitempty"`
 AcceptedEnd string `json:"accepted_end,omitempty"`
}
type exportHTTPView struct{exportJobView;DownloadURL string `json:"download_url,omitempty"`}
type exportPlanHTTPError struct{status int;message string}
func(e *exportPlanHTTPError)Error()string{return e.message}
type exportCopyProposal struct{
 Code string `json:"code"`
 RequestedStart string `json:"requested_start"`
 RequestedEnd string `json:"requested_end"`
 ExportStart string `json:"export_start"`
 ExportEnd string `json:"export_end"`
 AudioAllowanceMS int64 `json:"audio_allowance_ms"`
}
func(e *exportCopyProposal)Error()string{return "copy range approval required"}
type exportHTTPPlanner func(context.Context,exportCreateRequest)(exportJobRenderer,error)
type exportJobHTTP struct{auth *authManager;store *exportJobStore;plan exportHTTPPlanner;ctx context.Context;cancel context.CancelFunc;mu sync.Mutex;closing bool;requests sync.WaitGroup;once sync.Once;done chan struct{};closeErr error}
func newExportJobHTTP(a *authManager,store *exportJobStore,plan exportHTTPPlanner)*exportJobHTTP{ctx,cancel:=context.WithCancel(context.Background());return &exportJobHTTP{auth:a,store:store,plan:plan,ctx:ctx,cancel:cancel,done:make(chan struct{})}}
func configuredExportJobHTTP(a *authManager,n *NVR)(*exportJobHTTP,error){
 limits,err:=loadExportJobLimits(os.Getenv);if err!=nil{return nil,err};directory:=os.Getenv("NVR_EXPORT_SCRATCH_DIR");if directory==""{return newExportJobHTTP(a,nil,nil),nil}
 if !filepath.IsAbs(directory){return nil,errors.New("NVR_EXPORT_SCRATCH_DIR must be absolute")};directory=filepath.Clean(directory)
 resolved,err:=filepath.EvalSymlinks(directory);if err!=nil{return nil,errors.New("export scratch directory unavailable")}
 if resolved!=directory || directory=="/recordings" || strings.HasPrefix(directory,"/recordings/"){return nil,errors.New("export scratch must be outside recordings and not symlinked")}
 info,err:=os.Stat(directory);if err!=nil || !info.IsDir(){return nil,errors.New("export scratch must be an existing directory")}
 n.mu.RLock();overlap:=false
 for _,cam:=range n.cameras{if cam==nil{continue};path,err:=filepath.Abs(cam.RecordDir);if err!=nil{overlap=true;break};path=filepath.Clean(path);if path==directory || strings.HasPrefix(directory,path+string(os.PathSeparator)) || strings.HasPrefix(path,directory+string(os.PathSeparator)){overlap=true;break}}
 n.mu.RUnlock();if overlap{return nil,errors.New("export scratch overlaps a camera directory")}
 store,err:=newExportJobStore(context.Background(),directory,limits);if err!=nil{return nil,err};service:=newExportJobHTTP(a,store,nil)
 service.plan=func(ctx context.Context,req exportCreateRequest)(exportJobRenderer,error){return planExistingExportJob(ctx,n,store.directory,req)};return service,nil
}
func exportJobRoute(path string)bool{return path==exportCreatePath || path==exportCancelPath || path==exportStatusPath || path==exportDownloadPath}
func(s *exportJobHTTP)owner(r *http.Request)(string,error){
 if s.auth==nil{return "",errExportJobMissing};cfg,stamp,err:=readAuthConfig(s.auth.path);if err!=nil{return "",err}
 user:=s.auth.sessions.GetString(r.Context(),"user");version:=s.auth.sessions.GetString(r.Context(),"credential_version");token:=s.auth.sessions.Token(r.Context())
 if user!=cfg.Username || version!=stamp || token==""{return "",errExportJobMissing};digest:=sha256.Sum256([]byte(user+"\x00"+version+"\x00"+token));return hex.EncodeToString(digest[:]),nil
}
func(s *exportJobHTTP)Handler(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 if !exportJobRoute(r.URL.Path){next.ServeHTTP(w,r);return}
 w.Header().Set("Cache-Control","private, no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer")
 owner,err:=s.owner(r);if err!=nil{http.Error(w,"Unauthorized",401);return}
 allowed:="GET, HEAD";if r.URL.Path==exportCreatePath || r.URL.Path==exportCancelPath{allowed="POST"}
 if (allowed=="POST" && r.Method!="POST") || (allowed!="POST" && r.Method!="GET" && r.Method!="HEAD"){w.Header().Set("Allow",allowed);http.Error(w,"Method not allowed",405);return}
 s.mu.Lock();if s.closing || s.store==nil{s.mu.Unlock();http.Error(w,"Prepared exports unavailable; configure export scratch storage or retry after restart",503);return};s.requests.Add(1);s.mu.Unlock();defer s.requests.Done()
 ctx,cancel:=context.WithCancel(r.Context());defer cancel();stopService:=context.AfterFunc(s.ctx,cancel);defer stopService();r=r.WithContext(ctx)
 writeDone:=make(chan struct{});stopWrite:=context.AfterFunc(ctx,func(){_=http.NewResponseController(w).SetWriteDeadline(time.Now());close(writeDone)});defer func(){if !stopWrite(){<-writeDone}}()
 switch r.URL.Path{
 case exportCreatePath:s.create(w,r,owner)
 case exportCancelPath:s.cancelRequest(w,r,owner)
 case exportStatusPath:id,ok:=exportRequestID(w,r);if !ok{return};view,err:=s.store.status(owner,id);if err!=nil{writeExportJobHTTPError(w,err);return};writeExportJobView(w,r,200,view)
 case exportDownloadPath:id,ok:=exportRequestID(w,r);if !ok{return};reader,release,err:=s.store.acquire(owner,id);if err!=nil{writeExportJobHTTPError(w,err);return};defer release();w.Header().Set("Content-Type","video/mp4");w.Header().Set("Content-Disposition",`attachment; filename="export-`+id+`.mp4"`);http.ServeContent(w,r,"export-"+id+".mp4",time.Time{},reader)
 }
})}
func validExportJobID(id string)bool{if len(id)!=64{return false};raw,err:=hex.DecodeString(id);return err==nil && len(raw)==32}
func exportRequestID(w http.ResponseWriter,r *http.Request)(string,bool){q:=r.URL.Query();v:=q["id"];if len(q)!=1 || len(v)!=1 || !validExportJobID(v[0]){http.Error(w,"One valid export id is required",400);return "",false};return v[0],true}
func decodeExportJobJSON(w http.ResponseWriter,r *http.Request,value interface{})bool{
 if r.URL.RawQuery!=""{http.Error(w,"Query parameters not accepted",400);return false};kind,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"));if err!=nil || kind!="application/json"{http.Error(w,"Content-Type must be application/json",415);return false}
 d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));d.DisallowUnknownFields();if err:=d.Decode(value);err!=nil{http.Error(w,"Invalid export request",400);return false};if err:=d.Decode(&struct{}{});err!=io.EOF{http.Error(w,"One export JSON object is required",400);return false};return true
}
func writeExportJobView(w http.ResponseWriter,r *http.Request,status int,view exportJobView){body:=exportHTTPView{exportJobView:view};if view.State=="ready"{body.DownloadURL=exportDownloadPath+"?id="+view.ID};w.Header().Set("Content-Type","application/json");w.WriteHeader(status);if r.Method!="HEAD"{json.NewEncoder(w).Encode(body)}}
func writeExportJobHTTPError(w http.ResponseWriter,err error){status,message:=404,"Export unavailable, expired, or not ready";if errors.Is(err,errExportJobsBusy){status,message=429,"Export capacity exhausted; release prepared exports or wait for expiry";w.Header().Set("Retry-After","5")};if errors.Is(err,errExportJobsClosed){status,message=503,"Export service stopping"};http.Error(w,message,status)}
func validExportSelection(req exportCreateRequest)bool{
 start,e1:=time.Parse(time.RFC3339Nano,req.Start);end,e2:=time.Parse(time.RFC3339Nano,req.End)
 if req.CameraID=="" || len(req.CameraID)>256 || e1!=nil || e2!=nil || !end.After(start) || (req.Mode!="copy" && req.Mode!="accurate"){return false}
 if req.AcceptedStart!="" || req.AcceptedEnd!=""{a,e1:=time.Parse(time.RFC3339Nano,req.AcceptedStart);b,e2:=time.Parse(time.RFC3339Nano,req.AcceptedEnd);if req.Mode!="copy" || e1!=nil || e2!=nil || a.After(start) || b.Before(end) || !b.After(a){return false}}
 return true
}
func(s *exportJobHTTP)create(w http.ResponseWriter,r *http.Request,owner string){
 var req exportCreateRequest;if !decodeExportJobJSON(w,r,&req){return};if req.Mode==""{req.Mode="copy"};if !validExportSelection(req){http.Error(w,"Valid camera, interval, copy/accurate mode and matching approval fields required",400);return}
 if s.plan==nil{http.Error(w,"Export renderer unavailable",503);return};render,err:=s.plan(r.Context(),req)
 if err!=nil{var proposal *exportCopyProposal;var requestErr *exportPlanHTTPError
  if errors.As(err,&proposal){w.Header().Set("Content-Type","application/json");w.WriteHeader(409);json.NewEncoder(w).Encode(proposal)}else if errors.As(err,&requestErr){http.Error(w,requestErr.message,requestErr.status)}else if errors.Is(err,errCopyReordered) || errors.Is(err,errPreparedMediaProfile){message:="Unsupported or incompatible export tracks";if errors.Is(err,errCopyReordered){message=errCopyReordered.Error()};http.Error(w,message,422)}else if errors.Is(err,errExportPlanGap) || errors.Is(err,errExportPlanOverlap) || errors.Is(err,errExportInputLimit) || errors.Is(err,errExportPlanInvalid){writeExportPlanError(w,err)}else{writeRecordingIntervalError(w,r,err)};return
 }
 if r.Context().Err()!=nil{return};view,err:=s.store.submit(owner,render);if err!=nil{writeExportJobHTTPError(w,err);return};w.Header().Set("Location",exportStatusPath+"?id="+view.ID);writeExportJobView(w,r,202,view)
}
func(s *exportJobHTTP)cancelRequest(w http.ResponseWriter,r *http.Request,owner string){var req struct{ID string `json:"id"`};if !decodeExportJobJSON(w,r,&req){return};if !validExportJobID(req.ID){http.Error(w,"Valid export id required",400);return};if err:=s.store.cancelJob(owner,req.ID);err!=nil{writeExportJobHTTPError(w,err);return};w.WriteHeader(204)}

func copyProposal(req exportCreateRequest,requested,actual exportPlan,indices []copyFrameIndex)error{
 a,b:=actual.Start.UTC().Format(time.RFC3339Nano),actual.End.UTC().Format(time.RFC3339Nano)
 approvedA,e1:=time.Parse(time.RFC3339Nano,req.AcceptedStart);approvedB,e2:=time.Parse(time.RFC3339Nano,req.AcceptedEnd)
 if req.AcceptedStart!="" && e1==nil && e2==nil && approvedA.Equal(actual.Start) && approvedB.Equal(actual.End){return nil}
 // Partial copy also requires approval of possible AAC packet-boundary overhang.
 var allowance time.Duration;for _,index:=range indices{if index.audioAllowance>allowance{allowance=index.audioAllowance}}
 return &exportCopyProposal{Code:"copy_range_approval_required",RequestedStart:requested.Start.UTC().Format(time.RFC3339Nano),RequestedEnd:requested.End.UTC().Format(time.RFC3339Nano),ExportStart:a,ExportEnd:b,AudioAllowanceMS:int64((allowance+time.Millisecond-1)/time.Millisecond)}
}
func planExistingExportJob(parent context.Context,n *NVR,_ string,req exportCreateRequest)(exportJobRenderer,error){
 if req.Mode==""{req.Mode="copy"};if !validExportSelection(req){return nil,errExportPlanInvalid}
 start,_:=time.Parse(time.RFC3339Nano,req.Start);end,_:=time.Parse(time.RFC3339Nano,req.End)
 n.mu.RLock();cam:=n.cameras[req.CameraID];var camera timelineCamera;if cam!=nil{camera=timelineCamera{cam.ID,cam.Name,cam.RecordDir}};n.mu.RUnlock();if cam==nil{return nil,&exportPlanHTTPError{404,"Camera not found"}}
 ctx,cancel:=context.WithTimeout(parent,recordingMetadataBudget);defer cancel()
 clips,err:=collectRecordingIntervals(ctx,[]timelineCamera{camera},timelineWindow{Start:start.UTC(),End:end.UTC()},recordingDurations);if err!=nil{return nil,err}
 requested,err:=buildExportPlan(clips,timelineWindow{Start:start.UTC(),End:end.UTC()});if err!=nil{return nil,err};plan:=requested
 pins,err:=exportInputPins.prepare(ctx,requested.clips());if err!=nil{return nil,err};defer pins.Close()
 profiles:=make([]preparedMediaProfile,len(pins.files));for i,file:=range pins.files{profiles[i],err=inspectPreparedMediaProfile(ctx,file);if err!=nil{return nil,err}}
 var indices []copyFrameIndex
 if req.Mode=="copy" && requested.NeedsTrim{
  plan,indices,err=planKeyframeCopy(ctx,pins,requested);if err!=nil{return nil,err}
  // Reinspect expanded coverage, so overlapping media outside the original
  // selection cannot become an implicit source choice after approval.
  expanded,err:=collectRecordingIntervals(ctx,[]timelineCamera{camera},timelineWindow{Start:plan.Start,End:plan.End},recordingDurations);if err!=nil{return nil,err};checked,err:=buildExportPlan(expanded,timelineWindow{Start:plan.Start,End:plan.End});if err!=nil{return nil,err}
  if len(checked.Parts)!=len(plan.Parts){return nil,errRecordingChanged};for i:=range checked.Parts{if checked.Parts[i].Clip.Path!=plan.Parts[i].Clip.Path || !sameMediaFile(checked.Parts[i].Clip.identity,plan.Parts[i].Clip.identity){return nil,errRecordingChanged}}
  if err:=copyProposal(req,requested,plan,indices);err!=nil{return nil,err}
 }else if req.Mode=="copy" && req.AcceptedStart!=""{a,_:=time.Parse(time.RFC3339Nano,req.AcceptedStart);b,_:=time.Parse(time.RFC3339Nano,req.AcceptedEnd);if !a.Equal(plan.Start) || !b.Equal(plan.End){return nil,errExportPlanInvalid}}
 if req.Mode=="copy"{for i:=1;i<len(profiles);i++{if !compatiblePreparedCopy(profiles[0],profiles[i]){return nil,errPreparedMediaProfile}}}else{if _,err:=preparedAccurateArguments(profiles,plan);err!=nil{return nil,err}}
 return func(ctx context.Context,w io.Writer)error{
  output,ok:=w.(*boundedExportJobWriter);if !ok{return errors.New("export output unavailable")}
  held,err:=exportInputPins.prepare(ctx,plan.clips());if err!=nil{return err};defer held.Close()
  if len(indices)>0{err=renderKeyframeCopy(ctx,held,plan,indices,output.file,output.limit)}else{err=renderPreparedMedia(ctx,held,plan,req.Mode,output.file,output.limit)}
  if err!=nil{return err};info,err:=output.file.Stat();if err!=nil{return err};if info.Size()>output.limit{return errExportJobSize};output.written=info.Size();return ctx.Err()
 },nil
}
func(s *exportJobHTTP)Close(ctx context.Context)error{s.once.Do(func(){s.mu.Lock();s.closing=true;s.cancel();if s.store!=nil{s.store.cancel()};s.mu.Unlock();go func(){s.requests.Wait();if s.store!=nil{s.closeErr=s.store.Close(context.Background())};close(s.done)}()});select{case <-s.done:return s.closeErr;case <-ctx.Done():return ctx.Err()}}
func(s *exportJobHTTP)runLifecycle(ctx context.Context){<-ctx.Done();_=s.Close(context.Background())}
