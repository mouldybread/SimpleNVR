package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net/url"
 "path/filepath"
 "strconv"
 "time"
)

const maxTimelineWindow=26*time.Hour

type timelineWindow struct {Start,End time.Time;Legacy bool}

func parseTimelineWindow(query url.Values,now time.Time) (timelineWindow,error) {
 var window timelineWindow
 _,startPresent:=query["start_time"];_,endPresent:=query["end_time"]
 if startPresent || endPresent {
  if len(query["date"])!=0 || len(query["start_time"])!=1 || len(query["end_time"])!=1 {return window,errors.New("use one start_time and one end_time without date")}
  start,err:=time.Parse(time.RFC3339Nano,query.Get("start_time"));if err!=nil {return window,errors.New("invalid RFC3339 start_time")}
  end,err:=time.Parse(time.RFC3339Nano,query.Get("end_time"));if err!=nil {return window,errors.New("invalid RFC3339 end_time")}
  if !end.After(start) || end.Sub(start)>maxTimelineWindow {return window,errors.New("timeline window must be positive and at most 26 hours")}
  return timelineWindow{Start:start.UTC(),End:end.UTC()},nil
 }
 if len(query["date"])>1 {return window,errors.New("use at most one date")}
 zone:=time.FixedZone("HKT",8*60*60);day:=query.Get("date")
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
func serveTimelineRecordings(w http.ResponseWriter,r *http.Request,n *NVR) {serveTimelineWithDurations(w,r,n,recordingDurations)}

// Intervals come from the stamped file names only; the duration cache is unused here
// and kept in the signature for callers that still pass it. Optional repeated
// camera_id limits the result; an unknown id is a 404, an empty one a 400.
func serveTimelineWithDurations(w http.ResponseWriter,r *http.Request,n *NVR,_ *durationCache) {
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {w.Header().Set("Allow","GET, HEAD");http.Error(w,"Method not allowed",405);return}
 window,err:=parseTimelineWindow(r.URL.Query(),time.Now())
 if err!=nil {http.Error(w,err.Error(),400);return}
 ids:=r.URL.Query()["camera_id"]
 for _,id:=range ids {if id=="" {http.Error(w,"camera_id must not be empty",400);return}}
 n.mu.RLock()
 cameras:=make([]timelineCamera,0,len(n.cameras))
 missing:=false
 if len(ids)==0 {
  for _,cam:=range n.cameras {if cam!=nil {cameras=append(cameras,timelineCamera{cam.ID,cam.Name,cam.RecordDir})}}
 } else {
  seen:=make(map[string]bool,len(ids))
  for _,id:=range ids {
   if seen[id] {continue};seen[id]=true
   cam:=n.cameras[id]
   if cam==nil {missing=true;break}
   cameras=append(cameras,timelineCamera{cam.ID,cam.Name,cam.RecordDir})
  }
 }
 n.mu.RUnlock()
 if missing {http.Error(w,"Camera not found",404);return}
 w.Header().Set("X-Timeline-Window-Start",window.Start.Format(time.RFC3339Nano));w.Header().Set("X-Timeline-Window-End",window.End.Format(time.RFC3339Nano))
 mode:="absolute";if window.Legacy {mode="legacy-hkt"}
 w.Header().Set("X-Timeline-Window-Mode",mode);w.Header().Set("X-Timeline-Duration-Policy","name-end")
 ctx,cancel:=context.WithTimeout(r.Context(),recordingMetadataBudget);defer cancel()
 scan,err:=collectStampedIntervals(ctx,cameras,window)
 if err!=nil {writeRecordingIntervalError(w,r,err);return}
 w.Header().Set("X-Timeline-Unparseable-Count",strconv.Itoa(scan.Unparseable))
 segments:=make([]TimelineSegment,0,len(scan.Intervals))
 for _,clip:=range scan.Intervals {
  day:=filepath.Base(filepath.Dir(clip.Path))
  segments=append(segments,TimelineSegment{CameraID:clip.CameraID,CameraName:clip.CameraName,StartTime:clip.Start,EndTime:clip.End,FilePath:"/recordings/"+url.PathEscape(clip.CameraID)+"/"+url.PathEscape(day)+"/"+url.PathEscape(filepath.Base(clip.Path)),Size:clip.Size})
 }
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(segments)
}
