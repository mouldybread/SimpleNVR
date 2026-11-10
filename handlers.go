package main

import (
 "strings"
 "net/url"
 "encoding/json"
 "fmt"
 "io"
 "log"
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
 err:=func() error {
  recordingControlMu.Lock();defer recordingControlMu.Unlock()
  nvr.mu.RLock();cam,exists:=nvr.cameras[req.ID];nvr.mu.RUnlock()
  if !exists {return errCameraMissing}
  if err:=validateCameraPath(cam.ID,cam.RecordDir);err!=nil {return err}
  stopManagedCamera(nvr,cam)
  if err:=os.RemoveAll(cam.RecordDir);err!=nil {return err}
  nvr.mu.Lock()
  delete(nvr.cameras,req.ID)
  order:=make([]string,0,len(nvr.cameraOrder))
  for _,id:=range nvr.cameraOrder {if id!=req.ID {order=append(order,id)}}
  nvr.cameraOrder=order
  nvr.mu.Unlock()
  return nil
 }()
 if err!=nil {recordingError(w,err);return}
 finishCameraUpdate(w,r)
}

func deleteAllCamerasHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 err:=func() error {
  recordingControlMu.Lock();defer recordingControlMu.Unlock()
  nvr.mu.RLock()
  cameras:=make([]*Camera,0,len(nvr.cameras))
  for _,cam:=range nvr.cameras {cameras=append(cameras,cam)}
  nvr.mu.RUnlock()
  for _,cam:=range cameras {if err:=validateCameraPath(cam.ID,cam.RecordDir);err!=nil {return err}}
  for _,cam:=range cameras {
   nvr.mu.RLock();cancel:=cam.recordCancel;nvr.mu.RUnlock()
   if cancel!=nil {cancel()}
  }
  var firstErr error
  for _,cam:=range cameras {
   stopManagedCamera(nvr,cam)
   if err:=os.RemoveAll(cam.RecordDir);err!=nil {if firstErr==nil {firstErr=err};continue}
   nvr.mu.Lock()
   if nvr.cameras[cam.ID]==cam {delete(nvr.cameras,cam.ID)}
   nvr.mu.Unlock()
  }
  nvr.mu.Lock()
  order:=make([]string,0,len(nvr.cameraOrder))
  for _,id:=range nvr.cameraOrder {if _,exists:=nvr.cameras[id];exists {order=append(order,id)}}
  nvr.cameraOrder=order
  nvr.mu.Unlock()
  return firstErr
 }()
 // Bulk deletion can partially succeed; persist the remaining state even on error.
 if saveErr:=nvr.save();saveErr!=nil {http.Error(w,"Camera state changed but could not be persisted",500);return}
 if err!=nil {recordingError(w,err);return}
 camerasHandler(w,r)
}

func updateRetentionHandler(w http.ResponseWriter,r *http.Request) {
 var data struct {ID string `json:"id"`;RetentionDays int `json:"retention_days"`}
 if err:=json.NewDecoder(r.Body).Decode(&data);err!=nil {http.Error(w,"Invalid request",400);return}
 if data.RetentionDays<1 {http.Error(w,"Retention days must be at least 1",400);return}
 nvr.mu.Lock()
 cam,exists:=nvr.cameras[data.ID]
 if exists {cam.RetentionDays=data.RetentionDays}
 nvr.mu.Unlock()
 if !exists {http.Error(w,"Camera not found",404);return}
 finishCameraUpdate(w,r)
}

func updateRetentionAllHandler(w http.ResponseWriter,r *http.Request) {
 var data struct {RetentionDays int `json:"retention_days"`}
 if err:=json.NewDecoder(r.Body).Decode(&data);err!=nil {http.Error(w,"Invalid request",400);return}
 if data.RetentionDays<1 {http.Error(w,"Retention days must be at least 1",400);return}
 nvr.mu.Lock()
 for _,cam:=range nvr.cameras {cam.RetentionDays=data.RetentionDays}
 nvr.mu.Unlock()
 finishCameraUpdate(w,r)
}

func updateRecordingModeHandler(w http.ResponseWriter,r *http.Request) {
 req,ok:=decodeRecordingRequest(w,r);if !ok {return}
 if err:=nvr.transitionRecording(req.ID,"mode",req.Mode,runRecordingWorker);err!=nil {recordingError(w,err);return}
 finishCameraUpdate(w,r)
}

func scanGo2rtcHandler(w http.ResponseWriter, r *http.Request) {
 var go2rtcUrl string
 contentType:=r.Header.Get("Content-Type")
 if strings.Contains(contentType,"multipart/form-data") {
  if err:=r.ParseMultipartForm(10<<20);err!=nil {log.Printf("Failed to parse multipart form: %v",err)}
  if r.MultipartForm!=nil {defer r.MultipartForm.RemoveAll()}
 } else {if err:=r.ParseForm();err!=nil {log.Printf("Failed to parse form: %v",err)}}
 for _,key:=range []string{"go2rtc-url","go2rtc_url","url","go2rtcURL"} {if val:=r.Form.Get(key);val!="" {go2rtcUrl=val;break}}
 if go2rtcUrl=="" && r.Body!=nil {
  bodyBytes,err:=io.ReadAll(r.Body)
  if err==nil && len(bodyBytes)>0 {
   var data map[string]interface{}
   if json.Unmarshal(bodyBytes,&data)==nil {for _,key:=range []string{"go2rtc-url","go2rtc_url","url","go2rtcURL"} {if val,ok:=data[key].(string);ok && val!="" {go2rtcUrl=val;break}}}
  }
 }
 if go2rtcUrl=="" {go2rtcUrl="http://go2rtc:1984"}
 go2rtcUrl=strings.TrimSuffix(go2rtcUrl,"/")
 parsedUrl,err:=url.Parse(go2rtcUrl)
 if err!=nil || (parsedUrl.Scheme!="http" && parsedUrl.Scheme!="https") || parsedUrl.Host=="" || parsedUrl.Port()=="" {http.Error(w,"Invalid format. Must be in the form http://websiteaddress:port",400);return}
 host:=parsedUrl.Hostname()
 resp,err:=http.Get(go2rtcUrl+"/api/streams")
 if err!=nil {http.Error(w,"Failed to connect to go2rtc: "+err.Error(),502);return}
 defer resp.Body.Close()
 body,_:=io.ReadAll(resp.Body)
 var streams map[string]interface{}
 json.Unmarshal(body,&streams)
 now:=time.Now().UTC()
 for streamName:=range streams {
  rtspUrl:=fmt.Sprintf("rtsp://%s:8554/%s",host,streamName)
  nvr.mu.RLock()
  exists:=false
  for _,cam:=range nvr.cameras {if cam.RTSPUrl==rtspUrl {exists=true;break}}
  nvr.mu.RUnlock()
  if exists {continue}
  id:=generateID(streamName,now)
  recordDir:=filepath.Join("/recordings",id)
  if err:=os.MkdirAll(recordDir,0755);err!=nil {http.Error(w,"Unable to create recording directory",500);return}
  cam:=&Camera{ID:id,Name:streamName,RTSPUrl:rtspUrl,RecordDir:recordDir,RetentionDays:7,RecordingMode:ModeContinuous,AddedAt:now}
  nvr.mu.Lock()
  nvr.cameras[id]=cam
  nvr.cameraOrder=append(nvr.cameraOrder,id)
  nvr.mu.Unlock()
 }
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
