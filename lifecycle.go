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
 errRecorderShuttingDown=errors.New("server shutting down; recording control rejected")
)

type recordingWorker func(context.Context,*Camera,RecordingMode)

func runRecordingWorker(ctx context.Context,cam *Camera,mode RecordingMode) {
 switch mode {case ModeContinuous:recordCamera(ctx,cam);case ModeEventBased:recordCameraEventBased(ctx,cam);default:cam.recordWG.Done()}
}

// Caller holds NVR.mu and recordingControlMu. Workers own Done after children join.
func startCameraWorkerLocked(n *NVR,cam *Camera,worker recordingWorker) error {
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 if cam.RecordingMode==ModeNoRecording {return errRecordingDisabled}
 if cam.RecordingMode!=ModeContinuous && cam.RecordingMode!=ModeEventBased {return errRecordingMode}
 // A loaded IsRecording=true with no runtime cancel function is restart intent.
 if cam.IsRecording && cam.recordCancel!=nil {return nil}
 if worker==nil {return errors.New("recording worker unavailable")}
 if err:=checkRecordingStorage(cam.RecordDir);err!=nil {return err}
 ctx,cancel:=context.WithCancel(context.Background())
 mode:=cam.RecordingMode
 cam.recordCancel=cancel
 cam.recordWG.Add(1)
 cam.IsRecording=true
 go worker(ctx,cam,mode)
 return nil
}

func (n *NVR) resumeRecordings(worker recordingWorker) error {
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 n.mu.Lock();defer n.mu.Unlock()
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 var result error
 for _,cam:=range n.cameras {
  if !cam.IsRecording {continue}
  if cam.RecordingMode!=ModeContinuous && cam.RecordingMode!=ModeEventBased {cam.IsRecording=false;continue}
  result=errors.Join(result,startCameraWorkerLocked(n,cam,worker))
 }
 return result
}

// API stops intentionally clear restart intent. Process shutdown does not use this.
func stopManagedCamera(n *NVR,cam *Camera) {
 n.mu.RLock();cancel:=cam.recordCancel;n.mu.RUnlock()
 if cancel!=nil {cancel()}
 cam.recordWG.Wait()
 n.mu.Lock();cam.IsRecording=false;cam.recordCancel=nil;cam.bufferManager=nil;n.mu.Unlock()
}

func (n *NVR) transitionRecording(id,action string,mode RecordingMode,worker recordingWorker) error {
 if action!="start" && action!="stop" && action!="mode" {return errRecordingAction}
 if action=="mode" && mode!=ModeContinuous && mode!=ModeEventBased && mode!=ModeNoRecording {return errRecordingMode}
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 n.mu.Lock()
 if n.shuttingDown.Load() {n.mu.Unlock();return errRecorderShuttingDown}
 cam,exists:=n.cameras[id]
 if !exists {n.mu.Unlock();return errCameraMissing}
 if action=="start" {err:=startCameraWorkerLocked(n,cam,worker);n.mu.Unlock();return err}
 if action=="mode" && cam.RecordingMode==mode {n.mu.Unlock();return nil}
 // Reject a known storage failure before stopping the old mode. A second
 // check at actual worker admission is still necessary; this reserves no space.
 if action=="mode" && mode!=ModeNoRecording {
  if err:=checkRecordingStorage(cam.RecordDir);err!=nil {n.mu.Unlock();return err}
 }
 wasRecording:=cam.IsRecording
 n.mu.Unlock()
 stopManagedCamera(n,cam)
 n.mu.Lock();defer n.mu.Unlock()
 if n.cameras[id]!=cam {return errCameraMissing}
 if action=="mode" {
  if n.shuttingDown.Load() {cam.IsRecording=wasRecording;return errRecorderShuttingDown}
  cam.RecordingMode=mode
  if mode!=ModeNoRecording {return startCameraWorkerLocked(n,cam,worker)}
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
 case errors.Is(err,errCameraMissing):status=404
 case errors.Is(err,errRecordingDisabled):status=409
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
