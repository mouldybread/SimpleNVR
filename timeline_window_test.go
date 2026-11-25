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
func TestTimelineWindowHandlerStampedNames(t *testing.T) {
 base:=t.TempDir();front,side:=filepath.Join(base,"front"),filepath.Join(base,"side")
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:front},"side":{ID:"side",Name:"Side",RecordDir:side}}}
 scanFixture(t,front,scanTime("20261110","055959"),scanTime("20261110","060001"),"continuous")
 scanFixture(t,front,scanTime("20261110","050000"),scanTime("20261110","060000"),"continuous")
 scanFixture(t,side,scanTime("20261110","070000"),scanTime("20261110","071000"),"event")
 if err:=os.WriteFile(filepath.Join(front,"20261110","20261110_080000_old.mp4"),[]byte("x"),0600);err!=nil {t.Fatal(err)}
 query:=timelineQuery("2026-11-10T00:00:00-06:00","2026-11-11T00:00:00-06:00").Encode()
 get:=func(n *NVR,extra string) *httptest.ResponseRecorder {
  response:=httptest.NewRecorder();serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query+extra,nil),n);return response
 }
 decode:=func(response *httptest.ResponseRecorder) []TimelineSegment {
  var segments []TimelineSegment;if err:=json.Unmarshal(response.Body.Bytes(),&segments);err!=nil {t.Fatal(err,response.Body.String())};return segments
 }
 all:=get(n,"")
 if all.Code!=200 || all.Header().Get("X-Timeline-Duration-Policy")!="name-end" || all.Header().Get("X-Timeline-Unparseable-Count")!="1" {t.Fatal(all.Code,all.Header(),all.Body.String())}
 segments:=decode(all)
 if len(segments)!=2 || segments[0].CameraID!="front" || segments[0].StartTime.Format(time.RFC3339)!="2026-11-10T05:59:59Z" || segments[0].EndTime.Sub(segments[0].StartTime)!=2*time.Second || segments[0].Size!=7 {t.Fatalf("wrong segments: %+v",segments)}
 if segments[0].FilePath!="/recordings/front/20261110/20261110_055959_E20261110_060001_continuous.mp4" {t.Fatal(segments[0].FilePath)}
 only:=decode(get(n,"&camera_id=side"))
 if len(only)!=1 || only[0].CameraID!="side" {t.Fatalf("camera filter failed: %+v",only)}
 if len(decode(get(n,"&camera_id=front&camera_id=side&camera_id=front")))!=2 {t.Fatal("repeated camera_id failed")}
 if code:=get(n,"&camera_id=nope").Code;code!=404 {t.Fatal(code)}
 if code:=get(n,"&camera_id=").Code;code!=400 {t.Fatal(code)}
 if empty:=get(&NVR{cameras:map[string]*Camera{}},"");empty.Body.String()!="[]\n" {t.Fatal(empty.Body.String())}
}
