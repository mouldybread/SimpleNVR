package main

import (
 "encoding/json"
 "errors"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "sort"
 "time"
)

const maxTimelineWindow=26*time.Hour

type timelineWindow struct {Start,End time.Time;Legacy bool}

// Absolute boundaries have no dependency on server-local time or time zone.
// The legacy date path is intentionally kept as HKT until clients migrate.
func parseTimelineWindow(query url.Values,now time.Time) (timelineWindow,error) {
 var window timelineWindow
 _,startPresent:=query["start_time"]
 _,endPresent:=query["end_time"]
 if startPresent || endPresent {
  if len(query["date"])!=0 || len(query["start_time"])!=1 || len(query["end_time"])!=1 {return window,errors.New("use one start_time and one end_time without date")}
  start,err:=time.Parse(time.RFC3339Nano,query.Get("start_time"))
  if err!=nil {return window,errors.New("invalid RFC3339 start_time")}
  end,err:=time.Parse(time.RFC3339Nano,query.Get("end_time"))
  if err!=nil {return window,errors.New("invalid RFC3339 end_time")}
  if !end.After(start) || end.Sub(start)>maxTimelineWindow {return window,errors.New("timeline window must be positive and at most 26 hours")}
  return timelineWindow{Start:start.UTC(),End:end.UTC()},nil
 }
 if len(query["date"])>1 {return window,errors.New("use at most one date")}
 zone:=time.FixedZone("HKT",8*60*60)
 day:=query.Get("date")
 if day=="" {day=now.In(zone).Format("2006-01-02")}
 start,err:=time.ParseInLocation("2006-01-02",day,zone)
 if err!=nil {return window,errors.New("invalid legacy date")}
 return timelineWindow{Start:start.UTC(),End:start.AddDate(0,0,1).UTC(),Legacy:true},nil
}

type TimelineSegment struct {
 CameraID string `json:"camera_id"`
 CameraName string `json:"camera_name"`
 StartTime time.Time `json:"start_time"`
 EndTime time.Time `json:"end_time"`
 FilePath string `json:"file_path"`
 Size int64 `json:"size"`
}

type timelineCamera struct {ID,Name,Dir string}

func timelineRecordingsHandler(w http.ResponseWriter,r *http.Request) {serveTimelineRecordings(w,r,nvr)}

func serveTimelineRecordings(w http.ResponseWriter,r *http.Request,n *NVR) {
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {w.Header().Set("Allow","GET, HEAD");http.Error(w,"Method not allowed",405);return}
 window,err:=parseTimelineWindow(r.URL.Query(),time.Now())
 if err!=nil {http.Error(w,err.Error(),400);return}
 w.Header().Set("X-Timeline-Window-Start",window.Start.Format(time.RFC3339Nano))
 w.Header().Set("X-Timeline-Window-End",window.End.Format(time.RFC3339Nano))
 mode:="absolute"
 if window.Legacy {mode="legacy-hkt"}
 w.Header().Set("X-Timeline-Window-Mode",mode)
 w.Header().Set("X-Timeline-Duration-Policy","estimated-600s")
 n.mu.RLock()
 cameras:=make([]timelineCamera,0,len(n.cameras))
 for _,cam:=range n.cameras {cameras=append(cameras,timelineCamera{cam.ID,cam.Name,cam.RecordDir})}
 n.mu.RUnlock()
 segments:=make([]TimelineSegment,0)
 // No camera-state lock is held across directory enumeration or file stats.
 for _,cam:=range cameras {
  if r.Context().Err()!=nil {return}
  files,err:=filepath.Glob(filepath.Join(cam.Dir,"*.mp4"))
  if err!=nil {continue}
  for _,file:=range files {
   if r.Context().Err()!=nil {return}
   info,err:=os.Lstat(file)
   if err!=nil || !info.Mode().IsRegular() {continue}
   name:=filepath.Base(file)
   if len(name)<15 {continue}
   start,err:=time.ParseInLocation("20060102_150405",name[:15],time.UTC)
   if err!=nil || start.Before(window.Start) || !start.Before(window.End) {continue}
   // Real duration and overlap handling are a separate follow-up. Do not
   // interpret this estimate as verified media coverage.
   segments=append(segments,TimelineSegment{
    CameraID:cam.ID,CameraName:cam.Name,StartTime:start,EndTime:start.Add(10*time.Minute),
    FilePath:"/recordings/"+url.PathEscape(cam.ID)+"/"+url.PathEscape(name),Size:info.Size(),
   })
  }
 }
 sort.Slice(segments,func(i,j int) bool {
  if segments[i].StartTime.Equal(segments[j].StartTime) {return segments[i].FilePath<segments[j].FilePath}
  return segments[i].StartTime.Before(segments[j].StartTime)
 })
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(segments)
}
