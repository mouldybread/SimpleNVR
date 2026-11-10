package main

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "sort"
 "time"
)

func triggerEventHandler(w http.ResponseWriter,r *http.Request) {
 triggerEventHandlerWithState(w,r,nvr,nvr.save)
}

// Inject persistence for isolated handler tests; production uses the same NVR
// instance for admission, event metadata and the save callback.
func triggerEventHandlerWithState(w http.ResponseWriter,r *http.Request,n *NVR,persist func() error) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 var req struct {CameraName string `json:"camera_name"`;Label string `json:"label"`;StartTime string `json:"start_time"`;DurationSeconds int `json:"duration_seconds"`}
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,65536))
 if err:=dec.Decode(&req);err!=nil {http.Error(w,"Invalid event JSON",400);return}
 var extra interface{}
 if err:=dec.Decode(&extra);err!=io.EOF {http.Error(w,"One event JSON object is required",400);return}
 if req.CameraName=="" || len(req.CameraName)>256 || req.Label=="" || len(req.Label)>128 || req.DurationSeconds<1 || req.DurationSeconds>maxEventSeconds {http.Error(w,"Camera, label and duration of 1 to 3600 seconds are required",400);return}
 start,err:=time.Parse(time.RFC3339,req.StartTime)
 if err!=nil {http.Error(w,"Invalid RFC3339 start_time",400);return}
 event:=Event{ID:generateID("event",time.Now()),CameraName:req.CameraName,Label:req.Label,StartTime:start.UTC(),Duration:req.DurationSeconds,CreatedAt:time.Now().UTC()}
 queued:=false
 status:=0
 var storageErr error
 func(){
  recordingControlMu.Lock();defer recordingControlMu.Unlock()
  n.mu.RLock()
  var cam *Camera
  ambiguous:=false
  for _,candidate:=range n.cameras {if candidate.Name==req.CameraName {if cam!=nil {ambiguous=true;break};cam=candidate}}
  active:=false;var bm *BufferManager
  if cam!=nil {active=cam.IsRecording && cam.RecordingMode==ModeEventBased;bm=cam.bufferManager}
  n.mu.RUnlock()
  if ambiguous {status=409;return}
  if cam==nil {status=404;return}
  if active {
   if bm==nil {status=503;return}
   // Both checks precede slot reservation, staging creation and prebuffer pinning.
   // Metadata-only logging intentionally bypasses capture admission policies.
   if storageErr=checkRecordingStorage(cam.RecordDir);storageErr!=nil {return}
   if storageErr=checkEventStagingAdmission(r.Context(),cam.RecordDir);storageErr!=nil {return}
   if err:=bm.TriggerEvent(event);err!=nil {
    switch {case errors.Is(err,errEventBusy):status=429;case errors.Is(err,errEventStopped):status=503;default:status=500}
    return
   }
   queued=true
  }
  n.mu.Lock();n.events=append(n.events,event);n.mu.Unlock()
 }()
 if storageErr!=nil {recordingError(w,storageErr);return}
 if status!=0 {
  if status==429 || status==503 {w.Header().Set("Retry-After","1")}
  http.Error(w,"Event not accepted: camera missing, ambiguous, recorder unavailable, at capacity or storage failed",status);return
 }
 if err:=persist();err!=nil {http.Error(w,"Event state changed but metadata could not be persisted",500);return}
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]interface{}{"success":true,"event_id":event.ID,"recording_queued":queued,"message":"Event logged; queued captures are asynchronous and not yet confirmed saved"})
}

func listEventsHandler(w http.ResponseWriter,r *http.Request) {
 name:=r.URL.Query().Get("camera_name")
 nvr.mu.RLock()
 events:=make([]Event,0)
 for _,event:=range nvr.events {if name=="" || event.CameraName==name {events=append(events,event)}}
 nvr.mu.RUnlock()
 sort.Slice(events,func(i,j int) bool {return events[i].StartTime.After(events[j].StartTime)})
 w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(events)
}

func clearAllEventsHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"Method not allowed",405);return}
 nvr.mu.Lock();count:=len(nvr.events);nvr.events=[]Event{};nvr.mu.Unlock()
 if err:=nvr.save();err!=nil {http.Error(w,"Event state changed but could not be persisted",500);return}
 w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(map[string]interface{}{"success":true,"count":count})
}

func clearCameraEventsHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"Method not allowed",405);return}
 var req struct {CameraName string `json:"camera_name"`}
 if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil || req.CameraName=="" {http.Error(w,"camera_name required",400);return}
 nvr.mu.Lock();kept:=make([]Event,0,len(nvr.events));count:=0
 for _,event:=range nvr.events {if event.CameraName==req.CameraName {count++} else {kept=append(kept,event)}}
 nvr.events=kept;nvr.mu.Unlock()
 if err:=nvr.save();err!=nil {http.Error(w,"Event state changed but could not be persisted",500);return}
 w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(map[string]interface{}{"success":true,"count":count})
}
