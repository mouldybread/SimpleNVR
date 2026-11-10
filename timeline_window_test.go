package main

import (
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
 cases:=[]struct{name,start,end string;hours int}{
  {"utc","2026-11-10T00:00:00Z","2026-11-11T00:00:00Z",24},
  {"hong-kong","2026-11-10T00:00:00+08:00","2026-11-11T00:00:00+08:00",24},
  {"chicago-spring","2026-03-08T00:00:00-06:00","2026-03-09T00:00:00-05:00",23},
  {"chicago-fall","2026-11-01T00:00:00-05:00","2026-11-02T00:00:00-06:00",25},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  window,err:=parseTimelineWindow(timelineQuery(tc.start,tc.end),time.Time{})
  if err!=nil {t.Fatal(err)}
  expected,_:=time.Parse(time.RFC3339,tc.start)
  if window.Legacy || window.Start.Location()!=time.UTC || !window.Start.Equal(expected) || window.End.Sub(window.Start)!=time.Duration(tc.hours)*time.Hour {t.Fatalf("wrong window: %+v",window)}
 })}
}

func TestTimelineWindowRejectsAmbiguousAndInvalidRequests(t *testing.T) {
 start,end:="2026-11-10T00:00:00Z","2026-11-11T00:00:00Z"
 cases:=[]url.Values{
  {"start_time":{start}}, {"end_time":{end}}, timelineQuery("",end), timelineQuery(start,""),
  timelineQuery("2026-11-10T00:00:00",end), timelineQuery(start,"nonsense"),
  timelineQuery(start,start), timelineQuery(end,start), timelineQuery(start,"2026-11-12T00:00:00Z"),
  {"start_time":{start,start},"end_time":{end}}, {"start_time":{start},"end_time":{end,end}},
  {"start_time":{start},"end_time":{end},"date":{"2026-11-10"}},
  {"date":{"2026-11-10","2026-11-11"}}, {"date":{"2026-02-30"}},
 }
 for _,query:=range cases {
  if _,err:=parseTimelineWindow(query,time.Now());err==nil {t.Fatalf("invalid request accepted: %v",query)}
  response:=httptest.NewRecorder()
  serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),&NVR{cameras:map[string]*Camera{}})
  if response.Code!=400 {t.Fatalf("expected 400, got %d for %v",response.Code,query)}
 }
}

func TestLegacyTimelineDateCompatibility(t *testing.T) {
 window,err:=parseTimelineWindow(url.Values{"date":{"2026-11-10"}},time.Time{})
 if err!=nil || !window.Legacy || window.Start.Format(time.RFC3339)!="2026-11-09T16:00:00Z" {t.Fatalf("legacy date changed: %+v %v",window,err)}
 now:=time.Date(2026,11,10,20,0,0,0,time.UTC)
 window,err=parseTimelineWindow(nil,now)
 if err!=nil || window.Start.Format(time.RFC3339)!="2026-11-10T16:00:00Z" {t.Fatalf("legacy default is not current HKT day: %+v %v",window,err)}
}

func TestTimelineWindowHandlerBoundariesAndRegularFiles(t *testing.T) {
 dir:=t.TempDir()
 for _,name:=range []string{"20261110_055959_before.mp4","20261110_060000_start.mp4","20261111_055959_inside.mp4","20261111_060000_end.mp4","20261110_070000_active.mp4.tmp"} {
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
 }
 if err:=os.Symlink(filepath.Join(dir,"20261110_060000_start.mp4"),filepath.Join(dir,"20261110_080000_link.mp4"));err!=nil {t.Fatal(err)}
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 response:=httptest.NewRecorder()
 query:=timelineQuery("2026-11-10T00:00:00-06:00","2026-11-11T00:00:00-06:00")
 serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n)
 if response.Code!=200 || response.Header().Get("X-Timeline-Window-Mode")!="absolute" || response.Header().Get("X-Timeline-Duration-Policy")!="estimated-600s" {t.Fatalf("unexpected response: %d %v",response.Code,response.Header())}
 var segments []TimelineSegment
 if err:=json.Unmarshal(response.Body.Bytes(),&segments);err!=nil {t.Fatal(err)}
 if len(segments)!=2 || segments[0].StartTime.Format(time.RFC3339)!="2026-11-10T06:00:00Z" || segments[1].StartTime.Format(time.RFC3339)!="2026-11-11T05:59:59Z" {t.Fatalf("wrong boundary selection: %+v",segments)}
 empty:=httptest.NewRecorder()
 serveTimelineRecordings(empty,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),&NVR{cameras:map[string]*Camera{}})
 if empty.Code!=200 || empty.Body.String()!="[]\n" {t.Fatalf("empty result is not a JSON array: %s",empty.Body.String())}
}
