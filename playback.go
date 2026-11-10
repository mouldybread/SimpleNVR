package main

import (
 "context"
 "errors"
 "net"
 "net/url"
 "encoding/json"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "time"
)

func listRecordingsHandler(w http.ResponseWriter,r *http.Request) {
 cameraID:=r.URL.Query().Get("camera_id")
 if cameraID=="" {http.Error(w,"Camera ID required",400);return}
 nvr.mu.RLock();cam,exists:=nvr.cameras[cameraID];nvr.mu.RUnlock()
 if !exists {http.Error(w,"Camera not found",404);return}
 files,err:=filepath.Glob(filepath.Join(cam.RecordDir,"*.mp4"))
 if err!=nil {http.Error(w,"Failed to list recordings",500);return}
 type Recording struct {Filename string `json:"filename"`;Path string `json:"path"`;Timestamp time.Time `json:"timestamp"`;Size int64 `json:"size"`}
 var recordings []Recording
 for _,file:=range files {info,err:=os.Stat(file);if err!=nil {continue};name:=filepath.Base(file);recordings=append(recordings,Recording{name,"/recordings/"+filepath.Join(filepath.Base(cam.RecordDir),name),info.ModTime(),info.Size()})}
 sort.Slice(recordings,func(i,j int) bool {return recordings[i].Timestamp.After(recordings[j].Timestamp)})
 w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(recordings)
}

func serveRecordingHandler(w http.ResponseWriter,r *http.Request) {
 path:=r.URL.Path[len("/recordings/"):];fullPath:=filepath.Join("/recordings",path)
 log.Printf("Serving recording: %s",fullPath)
 info,err:=os.Stat(fullPath)
 if err!=nil {http.Error(w,"Recording not found",404);return}
 if info.IsDir() {http.Error(w,"Invalid recording path",400);return}
 w.Header().Set("Content-Type","video/mp4");w.Header().Set("Accept-Ranges","bytes");w.Header().Set("Cache-Control","no-cache")
 http.ServeFile(w,r,fullPath)
}

func deleteAllRecordingsHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!="POST" {http.Error(w,"Method not allowed",405);return}
 nvr.mu.RLock();defer nvr.mu.RUnlock()
 count:=0;var size int64
 for _,cam:=range nvr.cameras {
  files,err:=filepath.Glob(filepath.Join(cam.RecordDir,"*.mp4"));if err!=nil {continue}
  for _,file:=range files {info,err:=os.Stat(file);if err!=nil {continue};size+=info.Size();if err:=os.Remove(file);err==nil {count++} else {log.Printf("Failed to delete recording %s: %v",file,err)}}
 }
 w.Header().Set("Content-Type","application/json");json.NewEncoder(w).Encode(map[string]interface{}{"success":true,"deleted":count,"size_mb":float64(size)/(1024*1024)})
}

func handleDownload(w http.ResponseWriter,r *http.Request) {serveDownload(w,r,nvr,recordingDurations,exportInputPins)}
func serveDownload(w http.ResponseWriter,r *http.Request,n *NVR,cache *durationCache,pool *exportInputPool) {
 if r.Context().Err()!=nil {return}
 query:=r.URL.Query()
 if len(query["camera_id"])!=1 || len(query["start_time"])!=1 || len(query["end_time"])!=1 || query.Get("camera_id")=="" {http.Error(w,"One camera_id, start_time and end_time are required",400);return}
 start,err:=time.Parse(time.RFC3339Nano,query.Get("start_time"));if err!=nil {http.Error(w,"Invalid start_time",400);return}
 end,err:=time.Parse(time.RFC3339Nano,query.Get("end_time"));if err!=nil || !end.After(start) {http.Error(w,"Invalid end_time or empty/reversed range",400);return}
 n.mu.RLock()
 cam,exists:=n.cameras[query.Get("camera_id")]
 var camera timelineCamera
 if exists {camera=timelineCamera{cam.ID,cam.Name,cam.RecordDir}}
 n.mu.RUnlock()
 if !exists {http.Error(w,"Camera not found",404);return}
 ctx,cancel:=context.WithTimeout(r.Context(),recordingMetadataBudget);defer cancel()
 clips,err:=collectRecordingIntervals(ctx,[]timelineCamera{camera},timelineWindow{Start:start.UTC(),End:end.UTC()},cache)
 if err!=nil {writeRecordingIntervalError(w,r,err);return}
 if len(clips)==0 {http.Error(w,"No recordings found in time range",404);return}
 pins,err:=pool.prepare(ctx,clips)
 cancel()
 if err!=nil {
  if errors.Is(err,errExportInputLimit) {http.Error(w,"Too many export clips; reduce the requested time range",413)} else {writeRecordingIntervalError(w,r,err)}
  return
 }
 // Close only after Run returns, including failure/cancellation paths. The
 // shutdown gate joins the handler after this deferred cleanup has completed.
 defer pins.Close()
 cmd,err:=pins.command(r.Context())
 if err!=nil {writeRecordingIntervalError(w,r,err);return}
 name:=fmt.Sprintf("%s_%s.mp4",sanitizeFilename(camera.Name),start.UTC().Format("20060102_150405"))
 w.Header().Set("Content-Type","video/mp4");w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=%q",name))
 w.Header().Set("X-Recording-Duration-Policy","probed-media");w.Header().Set("X-Export-Input-Policy","pinned-descriptors")
 cmd.Stdout=w;cmd.Stderr=os.Stderr
 if err:=cmd.Run();err!=nil {log.Printf("FFmpeg export ended for camera %s: %v",camera.ID,err)}
}

var liveProxyClient=&http.Client{
 Transport:&http.Transport{Proxy:http.ProxyFromEnvironment,DialContext:(&net.Dialer{Timeout:10*time.Second,KeepAlive:30*time.Second}).DialContext,TLSHandshakeTimeout:10*time.Second,ResponseHeaderTimeout:10*time.Second,IdleConnTimeout:90*time.Second,ForceAttemptHTTP2:true},
 CheckRedirect:func(_ *http.Request,_ []*http.Request) error {return http.ErrUseLastResponse},
}
func relayLiveStream(w http.ResponseWriter,r *http.Request,target string,client *http.Client) {
 req,err:=http.NewRequestWithContext(r.Context(),r.Method,target,nil)
 if err!=nil {http.Error(w,"Failed to create proxy request",500);return}
 for _,key:=range []string{"Accept","Range"} {if value:=r.Header.Get(key);value!="" {req.Header.Set(key,value)}}
 resp,err:=client.Do(req)
 if err!=nil {if r.Context().Err()==nil {http.Error(w,"Failed to connect to stream",502)};return}
 defer resp.Body.Close()
 for _,key:=range []string{"Content-Type","Content-Length","Content-Range","Accept-Ranges"} {if value:=resp.Header.Get(key);value!="" {w.Header().Set(key,value)}}
 w.Header().Set("Cache-Control","no-store");w.WriteHeader(resp.StatusCode)
 if r.Method!=http.MethodHead {_,_=io.Copy(w,resp.Body)}
}
func streamProxyHandler(w http.ResponseWriter,r *http.Request) {
 name:=strings.TrimPrefix(r.URL.Path,"/api/stream/")
 if name=="" {http.Error(w,"Camera name required",400);return}
 nvr.mu.RLock();raw:="";found:=false
 for _,cam:=range nvr.cameras {if cam.Name==name {raw=cam.RTSPUrl;found=true;break}}
 nvr.mu.RUnlock()
 if !found {http.Error(w,"Camera not found",404);return}
 host:="go2rtc";if parsed,err:=url.Parse(raw);err==nil && parsed.Hostname()!="" {host=parsed.Hostname()}
 target:=url.URL{Scheme:"http",Host:net.JoinHostPort(host,"1984"),Path:"/api/stream.mp4",RawQuery:url.Values{"src":{name}}.Encode()}
 relayLiveStream(w,r,target.String(),liveProxyClient)
}
