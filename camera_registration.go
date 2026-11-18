package main

import (
 "context"
 "errors"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"
)

var errCameraRegistration=errors.New("camera registration refused or directory creation failed")
var errCameraRegistrationPersistence=errors.New("camera registration changed but could not be persisted")

type cameraRegistrationOps struct {
 mkdir func(string,os.FileMode) error
 id func(string,time.Time) string
}
func realCameraRegistrationOps() cameraRegistrationOps {return cameraRegistrationOps{os.Mkdir,generateID}}

type registrationSnapshot struct {camera *Camera;id,dir,source string}

// Serialize creation, registration and optional persistence with cleanup and
// recording controls. Never hold NVR.mu across filesystem I/O or persistence.
// On refusal, preserve created directories; pathname cleanup is not ownership.
func registerCameraSources(ctx context.Context,n *NVR,root string,sources []scannedStream,dedupe bool,ops cameraRegistrationOps,persist func()error) error {
 if n==nil || ops.mkdir==nil || ops.id==nil || len(sources)>scanStreamLimit || !filepath.IsAbs(root) || filepath.Clean(root)!=root || root==string(filepath.Separator) {return errCameraRegistration}
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 check:=func()error{if err:=ctx.Err();err!=nil {return err};if n.shuttingDown.Load() {return errRecorderShuttingDown};return nil}
 if err:=check();err!=nil {return err}
 n.mu.RLock()
 snapshot:=make(map[string]registrationSnapshot,len(n.cameras))
 existing:=make(map[string]bool,len(n.cameras))
 ids:=make(map[string]bool,len(n.cameras))
 for id,cam:=range n.cameras {
  ids[id]=true
  item:=registrationSnapshot{camera:cam}
  if cam!=nil {item.id=cam.ID;item.dir=cam.RecordDir;item.source=cam.RTSPUrl;existing[cam.RTSPUrl]=true}
  snapshot[id]=item
 }
 n.mu.RUnlock()
 pending:=make([]*Camera,0,len(sources))
 now:=time.Now().UTC()
 for _,source:=range sources {
  if err:=check();err!=nil {return err}
  if source.Name=="" || source.RTSPURL=="" {return errCameraRegistration}
  if dedupe && existing[source.RTSPURL] {continue}
  id:=ops.id(source.Name,now)
  if id=="" || id=="." || id==".." || strings.ContainsAny(id,"/\\\x00") || ids[id] {return errCameraRegistration}
  ids[id]=true
  dir:=filepath.Join(root,id)
  // Exclusive creation: never adopt an existing, unregistered directory.
  if err:=ops.mkdir(dir,0755);err!=nil {return errors.Join(errCameraRegistration,err)}
  pending=append(pending,&Camera{ID:id,Name:source.Name,RTSPUrl:source.RTSPURL,RecordDir:dir,RetentionDays:7,RecordingMode:ModeContinuous,AddedAt:now})
  existing[source.RTSPURL]=true
 }
 commitErr:=func()error{
  n.mu.Lock();defer n.mu.Unlock()
  if err:=check();err!=nil {return err}
  if len(n.cameras)!=len(snapshot) {return errRecordingStateChanged}
  for id,want:=range snapshot {
   cam,exists:=n.cameras[id]
   if !exists || cam!=want.camera {return errRecordingStateChanged}
   if cam!=nil && (cam.ID!=want.id || cam.RecordDir!=want.dir || cam.RTSPUrl!=want.source) {return errRecordingStateChanged}
  }
  for _,cam:=range pending {if _,exists:=n.cameras[cam.ID];exists {return errRecordingStateChanged}}
  if len(pending)>0 && n.cameras==nil {n.cameras=make(map[string]*Camera)}
  for _,cam:=range pending {n.cameras[cam.ID]=cam;n.cameraOrder=append(n.cameraOrder,cam.ID)}
  return nil
 }()
 if commitErr!=nil {return commitErr}
 // A committed registry change must attempt persistence even if the caller is
 // canceled just afterward. Failure does not pretend to roll back disk/memory.
 if persist!=nil {if err:=persist();err!=nil {return errors.Join(errCameraRegistrationPersistence,err)}}
 return nil
}

// Per-call seams keep tests in disposable roots and avoid mutable global hooks.
// Production wraps this with authenticated middleware and camera rendering.
func serveAddCameraAt(w http.ResponseWriter,r *http.Request,n *NVR,root string,ops cameraRegistrationOps,persist func()error) bool {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return false}
 if persist==nil {http.Error(w,"Camera registration persistence unavailable",500);return false}
 if err:=r.ParseForm();err!=nil {http.Error(w,"Invalid camera request",400);return false}
 name,source:=r.FormValue("camera-name"),r.FormValue("rtsp-url")
 if name=="" || source=="" {http.Error(w,"Name and RTSP URL required",400);return false}
 err:=registerCameraSources(r.Context(),n,root,[]scannedStream{{Name:name,RTSPURL:source}},false,ops,persist)
 if err==nil {return true}
 switch {
 case errors.Is(err,errCameraRegistrationPersistence):http.Error(w,"Camera state changed but could not be persisted",500)
 case errors.Is(err,errRecorderShuttingDown):http.Error(w,"Server shutting down",503)
 case errors.Is(err,errRecordingStateChanged):http.Error(w,"Camera registry changed; retry",409)
 case errors.Is(err,context.Canceled):http.Error(w,"Camera request canceled",408)
 case errors.Is(err,context.DeadlineExceeded):http.Error(w,"Camera request timed out",504)
 default:http.Error(w,"Unable to create camera registration",500)
 }
 return false
}
