package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "reflect"
 "testing"
 "time"
)

// Scale and boundary regressions assert the per-call work counters, never timing.
// Fixtures are tens of thousands of real directory entries, not the 1.79 million
// files of the production target; no production-scale run is claimed.

func stampedDayDir(t *testing.T,cam string,start time.Time) string {
 t.Helper()
 dir:=filepath.Join(cam,stampedDayName(start))
 if err:=os.MkdirAll(dir,0755);err!=nil {t.Fatal(err)}
 return dir
}
func stampedPutFile(t *testing.T,dir string,start,end time.Time,label string,nonempty bool) string {
 t.Helper()
 var data []byte
 if nonempty {data=[]byte("x")}
 path:=filepath.Join(dir,stampedFileName(start,end,label))
 if err:=os.WriteFile(path,data,0600);err!=nil {t.Fatal(err)}
 return path
}
func stampedObserve(t *testing.T,cams []timelineCamera,window timelineWindow) (stampedScan,stampedScanStats) {
 t.Helper()
 scan,stats,err:=collectStampedIntervalsObserved(context.Background(),cams,window,nil)
 if err!=nil {t.Fatal(err)}
 return scan,stats
}
func stampedPaths(scan stampedScan) []string {
 paths:=make([]string,len(scan.Intervals))
 for i,clip:=range scan.Intervals {paths[i]=clip.Path}
 return paths
}

// 30 retained days of 700 files each is more than 20,000 physical files, far past
// the 8,192 per-folder bound in total. A one-hour request still succeeds, looks up
// only the two day folders that can hold overlap, and inspects only its candidates.
func TestStampedScanLargeRetentionNarrowWindow(t *testing.T) {
 cam:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base.Add(10*time.Hour),End:base.Add(11*time.Hour)}
 const retentionDays,perDay=30,700
 physical:=0
 for d:=0;d<retentionDays;d++ {
  dayStart:=base.AddDate(0,0,-d);dir:=stampedDayDir(t,cam,dayStart)
  for i:=0;i<perDay;i++ {
   start:=dayStart.Add(time.Duration(i)*time.Second)
   stampedPutFile(t,dir,start,start.Add(time.Minute),"old",false);physical++
  }
 }
 crossStart,crossEnd:=base.Add(-10*time.Minute),base.Add(10*time.Hour+30*time.Minute)
 cross:=stampedPutFile(t,stampedDayDir(t,cam,crossStart),crossStart,crossEnd,"cross",true)
 today:=stampedDayDir(t,cam,base)
 for i,offset:=range []time.Duration{10*time.Hour,10*time.Hour+20*time.Minute,10*time.Hour+40*time.Minute} {
  start:=base.Add(offset)
  stampedPutFile(t,today,start,start.Add(20*time.Minute),fmt.Sprintf("in%d",i),true)
 }
 physical+=4
 if physical<=maxStampedCandidates || physical<=maxStampedFolderEntries {t.Fatal("fixture is not larger than the scan bounds",physical)}
 scan,stats:=stampedObserve(t,scanCam(cam),window)
 want:=stampedScanStats{DayLookups:2,DaysOpened:2,EntriesRead:1404,RelevantCandidates:4,MetadataCalls:4}
 if len(scan.Intervals)!=4 || scan.Unparseable!=0 || stats!=want || scan.Intervals[0].Path!=cross {t.Fatal(len(scan.Intervals),scan.Unparseable,stats)}

 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:cam}}}
 query:=url.Values{"start_time":{window.Start.Format(time.RFC3339)},"end_time":{window.End.Format(time.RFC3339)}}
 response:=httptest.NewRecorder()
 serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n)
 if response.Code!=200 || response.Header().Get("X-Timeline-Duration-Policy")!="name-end" || response.Header().Get("X-Timeline-Unparseable-Count")!="0" {t.Fatal(response.Code,response.Header())}
 var segments []TimelineSegment
 if err:=json.Unmarshal(response.Body.Bytes(),&segments);err!=nil || len(segments)!=4 {t.Fatal(err,response.Body.String())}
 if segments[0].FilePath!="/recordings/front/20261109/"+stampedFileName(crossStart,crossEnd,"cross") {t.Fatal(segments[0].FilePath)}
}

// All 34 cameras are scanned in one call. Growing every camera from three retained
// days to a full year, plus a future folder, changes neither the lookups, the
// entries read, nor the candidate inspections, and returns identical intervals.
func TestStampedScan34CamerasIgnoresOlderRetention(t *testing.T) {
 root:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base.Add(6*time.Hour),End:base.Add(18*time.Hour)}
 cams:=make([]timelineCamera,34)
 for c:=range cams {
  id:=fmt.Sprintf("cam%02d",c);dir:=filepath.Join(root,id)
  cams[c]=timelineCamera{id,"Camera "+id,dir}
  today:=stampedDayDir(t,dir,base)
  stampedPutFile(t,today,base.Add(6*time.Hour),base.Add(6*time.Hour+10*time.Minute),"a",true)
  stampedPutFile(t,today,base.Add(12*time.Hour),base.Add(12*time.Hour+10*time.Minute),"b",true)
  stampedPutFile(t,stampedDayDir(t,dir,base.Add(-time.Hour)),base.Add(-time.Hour),base.Add(7*time.Hour),"cross",true)
 }
 first,firstStats:=stampedObserve(t,cams,window)
 want:=stampedScanStats{DayLookups:68,DaysOpened:68,EntriesRead:102,RelevantCandidates:102,MetadataCalls:102}
 if len(first.Intervals)!=102 || firstStats!=want {t.Fatal(len(first.Intervals),firstStats)}
 perCamera:=map[string]int{}
 for _,clip:=range first.Intervals {perCamera[clip.CameraID]++}
 if len(perCamera)!=34 {t.Fatal("cameras returned:",len(perCamera))}
 for id,count:=range perCamera {if count!=3 {t.Fatal(id,count)}}

 for _,cam:=range cams {
  for d:=2;d<365;d++ {
   dayStart:=base.AddDate(0,0,-d)
   stampedPutFile(t,stampedDayDir(t,cam.Dir,dayStart),dayStart.Add(time.Hour),dayStart.Add(time.Hour+10*time.Minute),"old",false)
  }
  later:=base.AddDate(0,0,1)
  stampedPutFile(t,stampedDayDir(t,cam.Dir,later),later.Add(time.Hour),later.Add(time.Hour+10*time.Minute),"later",false)
 }
 second,secondStats:=stampedObserve(t,cams,window)
 if secondStats!=want || !reflect.DeepEqual(stampedPaths(first),stampedPaths(second)) {t.Fatal("older retention changed the work or the result",secondStats)}
}

// The candidate bound is one budget for the whole call: four cameras of 5,000 each
// reach exactly 20,000 and succeed; one more candidate anywhere fails the call before
// that candidate is inspected. Malformed and out-of-window names spend none of it.
func TestStampedScanCandidateLimitIsCallWide(t *testing.T) {
 root:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base.Add(6*time.Hour),End:base.Add(18*time.Hour)}
 const cameras,each=4,maxStampedCandidates/4
 cams:=make([]timelineCamera,cameras)
 for c:=range cams {
  id:=fmt.Sprintf("cam%d",c);dir:=filepath.Join(root,id);cams[c]=timelineCamera{id,id,dir}
  today:=stampedDayDir(t,dir,base)
  for i:=0;i<each;i++ {
   start:=window.Start.Add(time.Duration(i)*time.Second)
   stampedPutFile(t,today,start,start.Add(time.Minute),"c",true)
  }
 }
 noise:=stampedDayDir(t,cams[0].Dir,base)
 for i:=0;i<500;i++ {
  if err:=os.WriteFile(filepath.Join(noise,fmt.Sprintf("bad%04d.mp4",i)),nil,0600);err!=nil {t.Fatal(err)}
  early:=base.Add(time.Hour+time.Duration(i)*time.Second)
  stampedPutFile(t,noise,early,early.Add(30*time.Second),"early",false)
  late:=base.Add(19*time.Hour+time.Duration(i)*time.Second)
  stampedPutFile(t,noise,late,late.Add(30*time.Second),"late",false)
 }
 scan,stats:=stampedObserve(t,cams,window)
 want:=stampedScanStats{DayLookups:8,DaysOpened:4,EntriesRead:4*each+1500,RelevantCandidates:maxStampedCandidates,MetadataCalls:maxStampedCandidates}
 if len(scan.Intervals)!=maxStampedCandidates || scan.Unparseable!=500 || stats!=want {t.Fatal(len(scan.Intervals),scan.Unparseable,stats)}

 extra:=window.Start.Add(each*time.Second)
 stampedPutFile(t,stampedDayDir(t,cams[cameras-1].Dir,base),extra,extra.Add(time.Minute),"c",true)
 failed,failedStats,err:=collectStampedIntervalsObserved(context.Background(),cams,window,nil)
 if !errors.Is(err,errRecordingScanLimit) || len(failed.Intervals)!=0 || failed.Unparseable!=0 || failedStats.RelevantCandidates!=maxStampedCandidates+1 || failedStats.MetadataCalls!=maxStampedCandidates {t.Fatal(err,len(failed.Intervals),failedStats)}
}

// The physical bound is per opened folder, not per call: two folders of exactly
// 8,192 malformed entries pass; one more entry in either fails the call.
func TestStampedScanFolderBoundIsPerFolder(t *testing.T) {
 cam:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base,End:base.Add(24*time.Hour)}
 for _,day:=range []time.Time{base.AddDate(0,0,-1),base} {
  dir:=stampedDayDir(t,cam,day)
  for i:=0;i<maxStampedFolderEntries;i++ {
   if err:=os.WriteFile(filepath.Join(dir,fmt.Sprintf("%05d.mp4",i)),nil,0600);err!=nil {t.Fatal(err)}
  }
 }
 scan,stats:=stampedObserve(t,scanCam(cam),window)
 want:=stampedScanStats{DayLookups:2,DaysOpened:2,EntriesRead:2*maxStampedFolderEntries}
 if len(scan.Intervals)!=0 || scan.Unparseable!=2*maxStampedFolderEntries || stats!=want {t.Fatal(len(scan.Intervals),scan.Unparseable,stats)}
 if err:=os.WriteFile(filepath.Join(cam,"20261110","99999.mp4"),nil,0600);err!=nil {t.Fatal(err)}
 failed,failedStats,err:=collectStampedIntervalsObserved(context.Background(),scanCam(cam),window,nil)
 if !errors.Is(err,errRecordingScanLimit) || failed.Unparseable!=0 || failedStats.EntriesRead!=2*maxStampedFolderEntries+1 || failedStats.RelevantCandidates!=0 || failedStats.MetadataCalls!=0 {t.Fatal(err,failed.Unparseable,failedStats)}
}

type stampedBoundaryFixture struct {label string;start,end time.Time;want bool}

func stampedRunBoundary(t *testing.T,window timelineWindow,fixtures []stampedBoundaryFixture) stampedScan {
 t.Helper()
 cam:=t.TempDir();var want []string
 for _,f:=range fixtures {
  path:=stampedPutFile(t,stampedDayDir(t,cam,f.start),f.start,f.end,f.label,true)
  if f.want {want=append(want,path)}
 }
 scan,stats:=stampedObserve(t,scanCam(cam),window)
 got:=map[string]bool{}
 for _,path:=range stampedPaths(scan) {got[path]=true}
 if len(got)!=len(want) || len(scan.Intervals)!=len(want) || stats.RelevantCandidates!=len(want) || stats.MetadataCalls!=len(want) {t.Fatal("wrong selection",stampedPaths(scan),stats)}
 for _,path:=range want {if !got[path] {t.Fatal("missing",path)}}
 return scan
}

func TestStampedScanBoundaryInstants(t *testing.T) {
 day:=scanTime("20261110","000000")
 t.Run("whole second window",func(t *testing.T) {
  window:=timelineWindow{Start:day.Add(6*time.Hour),End:day.Add(18*time.Hour)}
  stampedRunBoundary(t,window,[]stampedBoundaryFixture{
   {"ends_at_start",day.Add(5*time.Hour),day.Add(6*time.Hour),false},
   {"ends_one_second_after_start",day.Add(5*time.Hour),day.Add(6*time.Hour+time.Second),true},
   {"starts_one_second_before_end",day.Add(18*time.Hour-time.Second),day.Add(18*time.Hour+time.Minute),true},
   {"starts_at_end",day.Add(18*time.Hour),day.Add(18*time.Hour+time.Minute),false},
   {"spans_window",day.Add(5*time.Hour),day.Add(19*time.Hour),true},
   {"inside",day.Add(12*time.Hour),day.Add(12*time.Hour+time.Second),true},
  })
 })
 t.Run("sub-second window",func(t *testing.T) {
  window:=timelineWindow{Start:day.Add(6*time.Hour+500*time.Millisecond),End:day.Add(18*time.Hour+500*time.Millisecond)}
  stampedRunBoundary(t,window,[]stampedBoundaryFixture{
   {"ends_before_start",day.Add(5*time.Hour),day.Add(6*time.Hour),false},
   {"ends_after_start",day.Add(5*time.Hour),day.Add(6*time.Hour+time.Second),true},
   {"starts_before_end",day.Add(18*time.Hour),day.Add(18*time.Hour+time.Minute),true},
   {"starts_after_end",day.Add(18*time.Hour+time.Second),day.Add(18*time.Hour+time.Minute),false},
  })
 })
}

// A stamped span is at most 24 hours, which is what makes the one-day lookback
// sufficient. A longer name is malformed: counted, never returned, never inspected.
func TestStampedScanSpanAssumption(t *testing.T) {
 day:=scanTime("20261110","000000");prior:=day.AddDate(0,0,-1)
 window:=timelineWindow{Start:day.Add(6*time.Hour),End:day.Add(7*time.Hour)}
 scan:=stampedRunBoundary(t,window,[]stampedBoundaryFixture{
  {"exactly_24h_overlaps",prior.Add(6*time.Hour+30*time.Minute),day.Add(6*time.Hour+30*time.Minute),true},
  {"exactly_24h_ends_at_start",prior.Add(6*time.Hour),day.Add(6*time.Hour),false},
  {"over_24h",prior.Add(6*time.Hour),day.Add(6*time.Hour+time.Second),false},
 })
 if scan.Unparseable!=1 {t.Fatal("over-long name not counted as malformed",scan.Unparseable)}
}

func TestStampedScanMidnightWindowAndDayLookups(t *testing.T) {
 cam:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base,End:base.Add(24*time.Hour)}
 crossStart,crossEnd:=base.Add(-30*time.Minute),base.Add(30*time.Minute)
 cross:=stampedPutFile(t,stampedDayDir(t,cam,crossStart),crossStart,crossEnd,"cross",true)
 next:=base.Add(24*time.Hour)
 stampedPutFile(t,stampedDayDir(t,cam,next),next,next.Add(time.Minute),"next_day",true)
 stampedPutFile(t,stampedDayDir(t,cam,base),base.Add(-15*time.Minute),base.Add(15*time.Minute),"misfiled",true)
 scan,stats:=stampedObserve(t,scanCam(cam),window)
 want:=stampedScanStats{DayLookups:2,DaysOpened:2,EntriesRead:2,RelevantCandidates:1,MetadataCalls:1}
 if len(scan.Intervals)!=1 || scan.Intervals[0].Path!=cross || scan.Unparseable!=1 || stats!=want {t.Fatal(stampedPaths(scan),scan.Unparseable,stats)}
}

func TestStampedScanEquivalentWindowOffsets(t *testing.T) {
 cam:=t.TempDir();day:=scanTime("20261110","000000")
 stampedPutFile(t,stampedDayDir(t,cam,day.Add(-time.Hour)),day.Add(-time.Hour),day.Add(7*time.Hour),"cross",true)
 stampedPutFile(t,stampedDayDir(t,cam,day),day.Add(8*time.Hour),day.Add(9*time.Hour),"inside",true)
 stampedPutFile(t,stampedDayDir(t,cam,day),day.Add(18*time.Hour),day.Add(19*time.Hour),"after",true)
 utc:=timelineWindow{Start:day.Add(6*time.Hour),End:day.Add(18*time.Hour)}
 base,baseStats:=stampedObserve(t,scanCam(cam),utc)
 if len(base.Intervals)!=2 {t.Fatal(stampedPaths(base))}
 for _,zone:=range []*time.Location{time.FixedZone("east",8*3600),time.FixedZone("west",-6*3600),time.FixedZone("odd",5*3600+1800)} {
  shifted:=timelineWindow{Start:utc.Start.In(zone),End:utc.End.In(zone)}
  got,stats:=stampedObserve(t,scanCam(cam),shifted)
  if !reflect.DeepEqual(stampedPaths(got),stampedPaths(base)) || stats!=baseStats {t.Fatal(zone,stampedPaths(got),stats)}
 }
}
