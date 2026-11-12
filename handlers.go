package main

import (
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "time"
)

func addCameraHandler(w http.ResponseWriter, r *http.Request) {
 r.ParseForm()
 name := r.FormValue("camera-name")
 rtspUrl := r.FormValue("rtsp-url")
 if name == "" || rtspUrl == "" {http.Error(w,"Name and RTSP URL required",400);return}
 now:=time.Now().UTC()
 id:=generateID(name,now)
 recordDir:=filepath.Join("/recordings",id)
 if err:=os.MkdirAll(recordDir,0755);err!=nil {http.Error(w,"Unable to create recording directory",500);return}
 cam:=&Camera{ID:id,Name:name,RTSPUrl:rtspUrl,RecordDir:recordDir,RetentionDays:7,RecordingMode:ModeContinuous,AddedAt:now}
 nvr.mu.Lock()
 nvr.cameras[id]=cam
 nvr.cameraOrder=append(nvr.cameraOrder,id)
 nvr.mu.Unlock()
 finishCameraUpdate(w,r)
}

func deleteCameraHandler(w http.ResponseWriter,r *http.Request) {
 req,ok:=decodeRecordingRequest(w,r);if !ok {return}
 if err:=deleteManagedCameras(r.Context(),nvr,[]string{req.ID},cameraDeletionDependencies(nvr));err!=nil {cameraDeletionError(w,err);return}
 camerasHandler(w,r)
}

func deleteAllCamerasHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 if err:=deleteManagedCameras(r.Context(),nvr,nil,cameraDeletionDependencies(nvr));err!=nil {cameraDeletionError(w,err);return}
 camerasHandler(w,r)
}

func updateRetentionHandler(w http.ResponseWriter,r *http.Request) {
 var data struct {ID string `json:"id"`;RetentionDays int `json:"retention_days"`}
 if err:=json.NewDecoder(r.Body).Decode(&data);err!=nil {http.Error(w,"Invalid request",400);return}
 if data.RetentionDays<1 || data.RetentionDays>maxCameraRetentionDays {http.Error(w,"Retention days must be between 1 and 365",400);return}
 if err:=setCameraRetention(nvr,data.ID,data.RetentionDays);err!=nil {
  status:=400;if err==errCameraMissing {status=404}
  http.Error(w,err.Error(),status);return
 }
 finishCameraUpdate(w,r)
}

func updateRetentionAllHandler(w http.ResponseWriter,r *http.Request) {
 var data struct {RetentionDays int `json:"retention_days"`}
 if err:=json.NewDecoder(r.Body).Decode(&data);err!=nil {http.Error(w,"Invalid request",400);return}
 if data.RetentionDays<1 || data.RetentionDays>maxCameraRetentionDays {http.Error(w,"Retention days must be between 1 and 365",400);return}
 if err:=setAllCameraRetention(nvr,data.RetentionDays);err!=nil {http.Error(w,err.Error(),400);return}
 finishCameraUpdate(w,r)
}

func updateRecordingModeHandler(w http.ResponseWriter,r *http.Request) {
 req,ok:=decodeRecordingRequest(w,r);if !ok {return}
 if err:=nvr.transitionRecording(req.ID,"mode",req.Mode,runRecordingWorker);err!=nil {recordingError(w,err);return}
 finishCameraUpdate(w,r)
}

func startRecordingHandler(w http.ResponseWriter,r *http.Request) {
 req,ok:=decodeRecordingRequest(w,r);if !ok {return}
 if err:=nvr.transitionRecording(req.ID,"start","",runRecordingWorker);err!=nil {recordingError(w,err);return}
 finishCameraUpdate(w,r)
}

func stopRecordingHandler(w http.ResponseWriter,r *http.Request) {
 req,ok:=decodeRecordingRequest(w,r);if !ok {return}
 if err:=nvr.transitionRecording(req.ID,"stop","",runRecordingWorker);err!=nil {recordingError(w,err);return}
 finishCameraUpdate(w,r)
}

func reorderCamerasHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"Method not allowed",405);return}
 var data struct {Order []string `json:"order"`}
 if err:=json.NewDecoder(r.Body).Decode(&data);err!=nil {http.Error(w,"Invalid request",400);return}
 nvr.mu.Lock()
 seen:=make(map[string]bool)
 for _,id:=range data.Order {
  if _,exists:=nvr.cameras[id];!exists || seen[id] {nvr.mu.Unlock();http.Error(w,"Order must contain each camera exactly once",400);return}
  seen[id]=true
 }
 if len(data.Order)!=len(nvr.cameras) {nvr.mu.Unlock();http.Error(w,"Order must contain each camera exactly once",400);return}
 nvr.cameraOrder=append([]string(nil),data.Order...)
 nvr.mu.Unlock()
 if err:=nvr.save();err!=nil {http.Error(w,"Failed to save camera order",500);return}
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]string{"status":"ok"})
}

func camerasHandler(w http.ResponseWriter,r *http.Request) {
 type CameraView struct {
  ID string
  Name string
  RTSPUrl string
  RecordDir string
  IsRecording bool
  RecordingMode RecordingMode
  RetentionDisplay int
  ModeDisplay string
  DiskUsage string
 }
 nvr.mu.RLock()
 cameras:=make([]CameraView,0,len(nvr.cameraOrder))
 for _,id:=range nvr.cameraOrder {
  cam:=nvr.cameras[id]
  if cam==nil {continue}
  days:=cam.RetentionDays;if days==0 {days=7}
  mode:="Continuous";if cam.RecordingMode==ModeEventBased {mode="Event Based"} else if cam.RecordingMode==ModeNoRecording {mode="No Recording"}
  cameras=append(cameras,CameraView{ID:cam.ID,Name:cam.Name,RTSPUrl:cam.RTSPUrl,RecordDir:cam.RecordDir,IsRecording:cam.IsRecording,RecordingMode:cam.RecordingMode,RetentionDisplay:days,ModeDisplay:mode})
 }
 nvr.mu.RUnlock()
 for i:=range cameras {cameras[i].DiskUsage=calculateDiskUsage(cameras[i].RecordDir)}
 if err:=templates.ExecuteTemplate(w,"cameras.html",cameras);err!=nil {http.Error(w,err.Error(),500)}
}

func calculateDiskUsage(recordDir string) string {
 var totalSize int64
 err:=filepath.Walk(recordDir,func(path string,info os.FileInfo,err error) error {
  if err!=nil || info==nil {return nil}
  if !info.IsDir() && filepath.Ext(path)==".mp4" {totalSize+=info.Size()}
  return nil
 })
 if err!=nil {return "0 B"}
 return formatBytes(totalSize)
}

func formatBytes(bytes int64) string {
 const unit=1024
 if bytes<unit {return fmt.Sprintf("%d B",bytes)}
 div,exp:=int64(unit),0
 for n:=bytes/unit;n>=unit;n/=unit {div*=unit;exp++}
 return fmt.Sprintf("%.1f %cB",float64(bytes)/float64(div),"KMGTPE"[exp])
}
