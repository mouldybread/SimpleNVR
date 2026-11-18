package main

import (
 "context"
 "encoding/json"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "testing"
 "time"
)
func timelineQuery(start,end string) url.Values {return url.Values{"start_time":{start},"end_time":{end}}}
func TestAbsoluteTimelineWindows(t *testing.T) {
 for _,tc:=range []struct{start,end string;hours int}{
  {"2026-11-10T00:00:00Z","2026-11-11T00:00:00Z",24},
  {"2026-11-10T00:00:00+08:00","2026-11-11T00:00:00+08:00",24},
  {"2026-03-08T00:00:00-06:00","2026-03-09T00:00:00-05:00",23},
  {"2026-11-01T00:00:00-05:00","2026-11-02T00:00:00-06:00",25},
 } {
  window,err:=parseTimelineWindow(timelineQuery(tc.start,tc.end),time.Time{});if err!=nil {t.Fatal(err)}
  expected,_:=time.Parse(time.RFC3339,tc.start)
  if window.Legacy || window.Start.Location()!=time.UTC || !window.Start.Equal(expected) || window.End.Sub(window.Start)!=time.Duration(tc.hours)*time.Hour {t.Fatalf("wrong window: %+v",window)}
 }
}
func TestTimelineWindowRejectsAmbiguousAndInvalidRequests(t *testing.T) {
 start,end:="2026-11-10T00:00:00Z","2026-11-11T00:00:00Z"
 for _,query:=range []url.Values{
  {"start_time":{start}},{"end_time":{end}},timelineQuery("",end),timelineQuery(start,""),timelineQuery("2026-11-10T00:00:00",end),timelineQuery(start,"nonsense"),timelineQuery(start,start),timelineQuery(end,start),timelineQuery(start,"2026-11-12T00:00:00Z"),
  {"start_time":{start,start},"end_time":{end}},{"start_time":{start},"end_time":{end,end}},{"start_time":{start},"end_time":{end},"date":{"2026-11-10"}},{"date":{"2026-11-10","2026-11-11"}},{"date":{"2026-02-30"}},
 } {
  response:=httptest.NewRecorder();serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),&NVR{cameras:map[string]*Camera{}})
  if response.Code!=400 {t.Fatalf("invalid request accepted: %v (%d)",query,response.Code)}
 }
}
func TestLegacyTimelineDateCompatibility(t *testing.T) {
 window,err:=parseTimelineWindow(url.Values{"date":{"2026-11-10"}},time.Time{})
 if err!=nil || !window.Legacy || window.Start.Format(time.RFC3339)!="2026-11-09T16:00:00Z" {t.Fatal(window,err)}
 window,err=parseTimelineWindow(nil,time.Date(2026,11,10,20,0,0,0,time.UTC))
 if err!=nil || window.Start.Format(time.RFC3339)!="2026-11-10T16:00:00Z" {t.Fatal(window,err)}
}
func TestTimelineWindowHandlerBoundariesAndRegularFiles(t *testing.T) {
 dir:=t.TempDir()
 for _,name:=range []string{"20261110_055958_ends_at_start.mp4","20261110_055959_overlap.mp4","20261110_060000_start.mp4","20261111_055959_inside.mp4","20261111_060000_end.mp4","20261110_070000_active.mp4.tmp"} {if err:=os.WriteFile(filepath.Join(dir,name),[]byte("fixture"),0600);err!=nil {t.Fatal(err)}}
 if err:=os.Symlink(filepath.Join(dir,"20261110_060000_start.mp4"),filepath.Join(dir,"20261110_080000_link.mp4"));err!=nil {t.Fatal(err)}
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 cache:=newDurationCache(10,1,func(context.Context,*os.File)(time.Duration,error){if !n.mu.TryLock() {t.Error("probe holds NVR lock")} else {n.mu.Unlock()};return 2*time.Second,nil})
 query:=timelineQuery("2026-11-10T00:00:00-06:00","2026-11-11T00:00:00-06:00")
 response:=httptest.NewRecorder();serveTimelineWithDurations(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n,cache)
 if response.Code!=200 || response.Header().Get("X-Timeline-Duration-Policy")!="probed-media" {t.Fatal(response.Code,response.Body.String())}
 var segments []TimelineSegment;if err:=json.Unmarshal(response.Body.Bytes(),&segments);err!=nil {t.Fatal(err)}
 if len(segments)!=3 || segments[0].StartTime.Format(time.RFC3339)!="2026-11-10T05:59:59Z" || segments[0].EndTime.Sub(segments[0].StartTime)!=2*time.Second {t.Fatalf("wrong overlap selection: %+v",segments)}
 empty:=httptest.NewRecorder();serveTimelineWithDurations(empty,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),&NVR{cameras:map[string]*Camera{}},cache)
 if empty.Body.String()!="[]\n" {t.Fatal(empty.Body.String())}
 failed:=httptest.NewRecorder();bad:=newDurationCache(1,1,func(context.Context,*os.File)(time.Duration,error){return 0,errRecordingMetadata})
 serveTimelineWithDurations(failed,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n,bad)
 if failed.Code!=503 || failed.Header().Get("X-Recording-Metadata-Status")!="unavailable" {t.Fatal("probe failure returned apparent coverage")}
}
