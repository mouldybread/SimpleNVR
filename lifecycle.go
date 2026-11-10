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
// Startup completes before the HTTP server accepts control requests.
var recordingControlMu sync.Mutex

var (
 errCameraMissing = errors.New("camera not found")
 errRecordingDisabled = errors.New("recording is disabled; change the recording mode first")
 errRecordingMode = errors.New("invalid recording mode")
 errRecordingAction = errors.New("invalid recording action")
)

// Workers own the matching recordWG.Done call, after joining their child work.
type recordingWorker func(context.Context, *Camera, RecordingMode)

func runRecordingWorker(ctx context.Context, cam *Camera, mode RecordingMode) {
 switch mode {
 case ModeContinuous: recordCamera(ctx,cam)
 case ModeEventBased: recordCameraEventBased(ctx,cam)
 default: cam.recordWG.Done()
 }
}

// Caller holds NVR.mu and recordingControlMu (except single-threaded startup).
func startCameraWorkerLocked(cam *Camera, worker recordingWorker) error {
 if cam.RecordingMode==ModeNoRecording {return errRecordingDisabled}
 if cam.RecordingMode!=ModeContinuous && cam.RecordingMode!=ModeEventBased {return errRecordingMode}
 if cam.IsRecording {return nil}
 if worker==nil {return errors.New("recording worker unavailable")}
 ctx,cancel:=context.WithCancel(context.Background())
 mode:=cam.RecordingMode
 cam.recordCancel=cancel
 cam.recordWG.Add(1)
 cam.IsRecording=true
 go worker(ctx,cam,mode)
 return nil
}

// Caller holds recordingControlMu, not NVR.mu. Event workers transitively join
// maintenance and event jobs before completing their top-level WaitGroup task.
func stopManagedCamera(n *NVR, cam *Camera) {
 n.mu.RLock()
 cancel:=cam.recordCancel
 n.mu.RUnlock()
 if cancel!=nil {cancel()}
 cam.recordWG.Wait()
 n.mu.Lock()
 cam.IsRecording=false
 cam.recordCancel=nil
 cam.bufferManager=nil
 n.mu.Unlock()
}

func (n *NVR) transitionRecording(id, action string, mode RecordingMode, worker recordingWorker) error {
 if action!="start" && action!="stop" && action!="mode" {return errRecordingAction}
 if action=="mode" && mode!=ModeContinuous && mode!=ModeEventBased && mode!=ModeNoRecording {return errRecordingMode}
 recordingControlMu.Lock()
 defer recordingControlMu.Unlock()
 n.mu.Lock()
 cam,exists:=n.cameras[id]
 if !exists {n.mu.Unlock();return errCameraMissing}
 if action=="start" {
  err:=startCameraWorkerLocked(cam,worker)
  n.mu.Unlock()
  return err
 }
 if action=="mode" && cam.RecordingMode==mode {n.mu.Unlock();return nil}
 n.mu.Unlock()
 stopManagedCamera(n,cam)
 n.mu.Lock()
 defer n.mu.Unlock()
 if n.cameras[id]!=cam {return errCameraMissing}
 if action=="mode" {
  cam.RecordingMode=mode
  if mode!=ModeNoRecording {return startCameraWorkerLocked(cam,worker)}
 }
 return nil
}

type recordingRequest struct {
 ID string `json:"id"`
 Mode RecordingMode `json:"mode,omitempty"`
}

func decodeRecordingRequest(w http.ResponseWriter,r *http.Request) (recordingRequest,bool) {
 var req recordingRequest
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return req,false}
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,65536))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {http.Error(w,"Invalid request",400);return req,false}
 var extra interface{}
 if err:=dec.Decode(&extra);err!=io.EOF || req.ID=="" {http.Error(w,"A single JSON request with a camera id is required",400);return req,false}
 return req,true
}

func recordingError(w http.ResponseWriter,err error) {
 status:=http.StatusInternalServerError
 switch {
 case errors.Is(err,errCameraMissing):status=http.StatusNotFound
 case errors.Is(err,errRecordingDisabled):status=http.StatusConflict
 case errors.Is(err,errRecordingMode):status=http.StatusBadRequest
 }
 http.Error(w,err.Error(),status)
}

func finishCameraUpdate(w http.ResponseWriter,r *http.Request) {
 if err:=nvr.save();err!=nil {http.Error(w,"Camera state changed but could not be persisted",500);return}
 camerasHandler(w,r)
}
