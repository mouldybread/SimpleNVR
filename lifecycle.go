package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "sync"
)

// Never acquire recordingControlMu while holding NVR.mu.
var recordingControlMu sync.Mutex

var (
 errCameraMissing=errors.New("camera not found")
 errRecordingDisabled=errors.New("recording is disabled; change the recording mode first")
 errRecordingMode=errors.New("invalid recording mode")
 errRecordingAction=errors.New("invalid recording action")
 errRecordingStateChanged=errors.New("camera state changed during recording admission; retry")
 errRecorderShuttingDown=errors.New("server shutting down; recording control rejected")
)

type recordingWorker func(context.Context,*Camera,RecordingMode)

func runRecordingWorker(ctx context.Context,cam *Camera,mode RecordingMode) {
 switch mode {case ModeContinuous:recordCamera(ctx,cam);case ModeEventBased:recordCameraEventBased(ctx,cam);default:cam.recordWG.Done()}
}

// Caller holds recordingControlMu, but never NVR.mu across admission I/O.
// Workers own Done after children join. The callback is per-call, not global.
func startCameraWorkerWithAdmission(n *NVR,cam *Camera,worker recordingWorker,check func(string) error) error {
 if cam==nil {return errCameraMissing}
 n.mu.RLock()
 if n.shuttingDown.Load() {n.mu.RUnlock();return errRecorderShuttingDown}
 id,dir,mode:=cam.ID,cam.RecordDir,cam.RecordingMode
 if n.cameras[id]!=cam {n.mu.RUnlock();return errCameraMissing}
 if mode==ModeNoRecording {n.mu.RUnlock();return errRecordingDisabled}
 if mode!=ModeContinuous && mode!=ModeEventBased {n.mu.RUnlock();return errRecordingMode}
 active:=cam.IsRecording && cam.recordCancel!=nil
 n.mu.RUnlock()
 if active {return nil}
 if worker==nil || check==nil {return errors.New("recording worker or admission unavailable")}
 admissionErr:=check(dir)
 n.mu.Lock();defer n.mu.Unlock()
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 if n.cameras[id]!=cam {return errCameraMissing}
 if cam.ID!=id || cam.RecordDir!=dir || cam.RecordingMode!=mode {return errRecordingStateChanged}
 if admissionErr!=nil {return admissionErr}
 if cam.IsRecording && cam.recordCancel!=nil {return nil}
 ctx,cancel:=context.WithCancel(context.Background())
 cam.recordCancel=cancel;cam.recordWG.Add(1);cam.IsRecording=true
 go worker(ctx,cam,mode)
 return nil
}

func (n *NVR) resumeRecordings(worker recordingWorker) error {return n.resumeRecordingsWithAdmission(worker,checkRecordingStorage)}
func (n *NVR) resumeRecordingsWithAdmission(worker recordingWorker,check func(string) error) error {
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 n.mu.Lock()
 if n.shuttingDown.Load() {n.mu.Unlock();return errRecorderShuttingDown}
 cameras:=make([]*Camera,0,len(n.cameras))
 for _,cam:=range n.cameras {
  if cam==nil || !cam.IsRecording {continue}
  if cam.RecordingMode!=ModeContinuous && cam.RecordingMode!=ModeEventBased {cam.IsRecording=false;continue}
  cameras=append(cameras,cam)
 }
 n.mu.Unlock()
 var result error
 for _,cam:=range cameras {result=errors.Join(result,startCameraWorkerWithAdmission(n,cam,worker,check))}
 return result
}

// API stops intentionally clear restart intent. Process shutdown does not use this.
func stopManagedCamera(n *NVR,cam *Camera) {
 n.mu.RLock();cancel:=cam.recordCancel;n.mu.RUnlock()
 if cancel!=nil {cancel()}
 cam.recordWG.Wait()
 n.mu.Lock();cam.IsRecording=false;cam.recordCancel=nil;cam.bufferManager=nil;n.mu.Unlock()
}

func (n *NVR) transitionRecording(id,action string,mode RecordingMode,worker recordingWorker) error {return n.transitionRecordingWithAdmission(id,action,mode,worker,checkRecordingStorage)}
func (n *NVR) transitionRecordingWithAdmission(id,action string,mode RecordingMode,worker recordingWorker,check func(string) error) error {
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 return n.transitionRecordingLocked(id,action,mode,worker,check)
}

// Caller holds recordingControlMu; persistence-aware API control uses the same
// transition without releasing serialization before its state has been saved.
func (n *NVR) transitionRecordingLocked(id,action string,mode RecordingMode,worker recordingWorker,check func(string) error) error {
 if action!="start" && action!="stop" && action!="mode" {return errRecordingAction}
 if action=="mode" && mode!=ModeContinuous && mode!=ModeEventBased && mode!=ModeNoRecording {return errRecordingMode}
 n.mu.Lock()
 if n.shuttingDown.Load() {n.mu.Unlock();return errRecorderShuttingDown}
 cam:=n.cameras[id]
 if cam==nil {n.mu.Unlock();return errCameraMissing}
 if action=="start" {n.mu.Unlock();return startCameraWorkerWithAdmission(n,cam,worker,check)}
 if action=="mode" && cam.RecordingMode==mode {n.mu.Unlock();return nil}
 wasRecording,dir,oldMode:=cam.IsRecording,cam.RecordDir,cam.RecordingMode
 n.mu.Unlock()
 // First reject a known failure without disturbing the existing worker.
 // The second check at replacement admission is still not a reservation.
 if action=="mode" && mode!=ModeNoRecording {
  if check==nil {return errors.New("recording admission unavailable")}
  if err:=check(dir);err!=nil {return err}
  n.mu.RLock()
  shuttingDown:=n.shuttingDown.Load()
  same:=n.cameras[id]==cam && cam.ID==id && cam.RecordDir==dir && cam.RecordingMode==oldMode
  n.mu.RUnlock()
  if shuttingDown {return errRecorderShuttingDown}
  if !same {return errRecordingStateChanged}
 }
 stopManagedCamera(n,cam)
 n.mu.Lock()
 if n.cameras[id]!=cam {n.mu.Unlock();return errCameraMissing}
 if action=="mode" {
  if n.shuttingDown.Load() {cam.IsRecording=wasRecording;n.mu.Unlock();return errRecorderShuttingDown}
  cam.RecordingMode=mode
 }
 n.mu.Unlock()
 if action=="mode" && mode!=ModeNoRecording {
  err:=startCameraWorkerWithAdmission(n,cam,worker,check)
  if errors.Is(err,errRecorderShuttingDown) {
   n.mu.Lock();if n.cameras[id]==cam && cam.recordCancel==nil {cam.IsRecording=wasRecording};n.mu.Unlock()
  }
  return err
 }
 return nil
}

type recordingRequest struct {ID string `json:"id"`;Mode RecordingMode `json:"mode,omitempty"`}

func decodeRecordingRequest(w http.ResponseWriter,r *http.Request) (recordingRequest,bool) {
 var req recordingRequest
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return req,false}
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,65536));dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {http.Error(w,"Invalid request",400);return req,false}
 var extra interface{}
 if err:=dec.Decode(&extra);err!=io.EOF || req.ID=="" {http.Error(w,"A single JSON request with a camera id is required",400);return req,false}
 return req,true
}

func recordingError(w http.ResponseWriter,err error) {
 status:=http.StatusInternalServerError
 switch {
 case errors.Is(err,errRecordingPersistence):status=500
 case errors.Is(err,errCameraMissing):status=404
 case errors.Is(err,errRecordingDisabled),errors.Is(err,errRecordingStateChanged):status=409
 case errors.Is(err,errRecordingMode):status=400
 case errors.Is(err,errStorageReserve),errors.Is(err,errEventStagingLimit):status=507
 case errors.Is(err,errRecorderShuttingDown),errors.Is(err,errStorageUnavailable),errors.Is(err,errStoragePolicy),errors.Is(err,errEventStagingUnavailable),errors.Is(err,errEventStagingPolicy):status=503;w.Header().Set("Retry-After","5")
 }
 http.Error(w,err.Error(),status)
}

func finishCameraUpdate(w http.ResponseWriter,r *http.Request) {
 if err:=nvr.save();err!=nil {http.Error(w,"Camera state changed but could not be persisted",500);return}
 camerasHandler(w,r)
}
