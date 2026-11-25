package main

import (
 "context"
 "errors"
 "fmt"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

// Downloads, exports and the strict scan fail closed: every failure below must discard
// the whole result, with no partial interval list and no export started. The timeline
// is the one caller that degrades instead; see the skip-and-report tests further down.
// Four valid candidates sit in the window.

type stampedFailFixture struct {cam,day string;base time.Time;window timelineWindow}

func newStampedFailFixture(t *testing.T) stampedFailFixture {
 t.Helper()
 base:=scanTime("20261110","000000");cam:=t.TempDir()
 f:=stampedFailFixture{cam:cam,base:base,window:timelineWindow{Start:base.Add(6*time.Hour),End:base.Add(18*time.Hour)}}
 f.day=stampedDayDir(t,cam,base)
 for i,hour:=range []time.Duration{7,9,13,15} {
  start:=base.Add(hour*time.Hour)
  stampedPutFile(t,f.day,start,start.Add(10*time.Minute),fmt.Sprintf("good%d",i),true)
 }
 return f
}

// The name every bad in-window candidate uses: valid, in the right folder, overlapping.
func (f stampedFailFixture) badName() (time.Time,time.Time) {
 start:=f.base.Add(10*time.Hour)
 return start,start.Add(10*time.Minute)
}

func TestFailClosedInvalidInWindowCandidates(t *testing.T) {
 cases:=map[string]func(t *testing.T,f stampedFailFixture){
  "empty file":func(t *testing.T,f stampedFailFixture) {
   start,end:=f.badName();stampedPutFile(t,f.day,start,end,"bad",false)
  },
  "directory":func(t *testing.T,f stampedFailFixture) {
   start,end:=f.badName()
   if err:=os.Mkdir(filepath.Join(f.day,stampedFileName(start,end,"bad")),0755);err!=nil {t.Fatal(err)}
  },
  "symlink":func(t *testing.T,f stampedFailFixture) {
   start,end:=f.badName()
   target:=filepath.Join(f.day,stampedFileName(f.base.Add(7*time.Hour),f.base.Add(7*time.Hour+10*time.Minute),"good0"))
   if err:=os.Symlink(target,filepath.Join(f.day,stampedFileName(start,end,"bad")));err!=nil {t.Fatal(err)}
  },
  "fifo":func(t *testing.T,f stampedFailFixture) {
   start,end:=f.badName()
   if err:=unix.Mkfifo(filepath.Join(f.day,stampedFileName(start,end,"bad")),0600);err!=nil {t.Fatal(err)}
  },
 }
 for name,setup:=range cases {
  t.Run(name,func(t *testing.T) {
   f:=newStampedFailFixture(t);setup(t,f)
   scan,stats,err:=collectStampedIntervalsObserved(context.Background(),scanCam(f.cam),f.window,nil)
   if !errors.Is(err,errRecordingMetadata) || len(scan.Intervals)!=0 || scan.Unparseable!=0 || stats.MetadataCalls<1 {t.Fatal(scan,stats,err)}
   public,publicErr:=collectStampedIntervals(context.Background(),scanCam(f.cam),f.window)
   if !errors.Is(publicErr,errRecordingMetadata) || len(public.Intervals)!=0 || public.Unparseable!=0 {t.Fatal(public,publicErr)}
  })
 }
}

func TestFailClosedInspectionFailuresDiscardEverything(t *testing.T) {
 t.Run("metadata error on third candidate",func(t *testing.T) {
  f:=newStampedFailFixture(t);calls:=0
  inspect:=func(dir *os.File,name string) (os.FileInfo,error) {
   calls++
   if calls==3 {return nil,errRecordingMetadata}
   return stampedCandidateInfo(dir,name)
  }
  scan,stats,err:=collectStampedIntervalsObserved(context.Background(),scanCam(f.cam),f.window,inspect)
  if !errors.Is(err,errRecordingMetadata) || len(scan.Intervals)!=0 || stats.MetadataCalls!=3 {t.Fatal(scan,stats,err)}
 })
 t.Run("file disappears after enumeration",func(t *testing.T) {
  f:=newStampedFailFixture(t);removed:=false
  inspect:=func(dir *os.File,name string) (os.FileInfo,error) {
   if !removed {
    removed=true
    if err:=os.Remove(filepath.Join(f.day,name));err!=nil {t.Error(err)}
   }
   return stampedCandidateInfo(dir,name)
  }
  scan,stats,err:=collectStampedIntervalsObserved(context.Background(),scanCam(f.cam),f.window,inspect)
  if !errors.Is(err,errRecordingMetadata) || len(scan.Intervals)!=0 || stats.MetadataCalls!=1 {t.Fatal(scan,stats,err)}
 })
 t.Run("canceled before the scan",func(t *testing.T) {
  f:=newStampedFailFixture(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
  scan,stats,err:=collectStampedIntervalsObserved(ctx,scanCam(f.cam),f.window,nil)
  if !errors.Is(err,context.Canceled) || len(scan.Intervals)!=0 || stats!=(stampedScanStats{}) {t.Fatal(scan,stats,err)}
 })
 for _,cancelAt:=range []int{1,4} {
  t.Run(fmt.Sprintf("canceled during candidate %d",cancelAt),func(t *testing.T) {
   f:=newStampedFailFixture(t);ctx,cancel:=context.WithCancel(context.Background());defer cancel();calls:=0
   inspect:=func(dir *os.File,name string) (os.FileInfo,error) {
    calls++
    if calls==cancelAt {cancel()}
    return stampedCandidateInfo(dir,name)
   }
   scan,stats,err:=collectStampedIntervalsObserved(ctx,scanCam(f.cam),f.window,inspect)
   if !errors.Is(err,context.Canceled) || len(scan.Intervals)!=0 || stats.MetadataCalls!=cancelAt {t.Fatal(scan,stats,err)}
  })
 }
}

// One healthy camera and one camera with an empty in-window candidate.
func stampedFailNVR(t *testing.T) (*NVR,stampedFailFixture) {
 t.Helper()
 bad:=newStampedFailFixture(t)
 start,end:=bad.badName();stampedPutFile(t,bad.day,start,end,"bad",false)
 healthy:=newStampedFailFixture(t)
 n:=&NVR{cameras:map[string]*Camera{
  "healthy":{ID:"healthy",Name:"Healthy",RecordDir:healthy.cam},
  "front":{ID:"front",Name:"Front",RecordDir:bad.cam},
 }}
 return n,bad
}

// The timeline degrades instead of failing: the unusable recording is left out and
// counted, so one corrupt file cannot hide every other recording. Both cameras'
// good segments (four each) are returned.
func TestTimelineSkipsUnreadableCandidateAndReportsCount(t *testing.T) {
 n,f:=stampedFailNVR(t)
 query:=url.Values{"start_time":{f.window.Start.Format(time.RFC3339)},"end_time":{f.window.End.Format(time.RFC3339)}}
 response:=httptest.NewRecorder()
 serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n)
 body:=response.Body.String()
 if response.Code!=200 || response.Header().Get("X-Timeline-Unreadable-Count")!="1" || response.Header().Get("X-Timeline-Unparseable-Count")!="0" || response.Header().Get("X-Recording-Metadata-Status")!="" || strings.Count(body,"\"file_path\"")!=8 {
  t.Fatal("timeline did not skip and report the unreadable recording",response.Code,response.Header(),body)
 }
}

func TestTimelineScanSkipsEveryKindOfUnusableCandidate(t *testing.T) {
 for _,kind:=range []string{"empty file","directory","symlink","fifo"} {t.Run(kind,func(t *testing.T) {
  f:=newStampedFailFixture(t);start,end:=f.badName();bad:=filepath.Join(f.day,stampedFileName(start,end,"bad"))
  switch kind {
  case "empty file":stampedPutFile(t,f.day,start,end,"bad",false)
  case "directory":if err:=os.Mkdir(bad,0755);err!=nil {t.Fatal(err)}
  case "symlink":
   target:=filepath.Join(f.day,stampedFileName(f.base.Add(7*time.Hour),f.base.Add(7*time.Hour+10*time.Minute),"good0"))
   if err:=os.Symlink(target,bad);err!=nil {t.Fatal(err)}
  case "fifo":if err:=unix.Mkfifo(bad,0600);err!=nil {t.Fatal(err)}
  }
  scan,err:=collectTimelineIntervals(context.Background(),scanCam(f.cam),f.window)
  if err!=nil || len(scan.Intervals)!=4 || scan.Unreadable!=1 || scan.Unparseable!=0 {t.Fatal(len(scan.Intervals),scan.Unreadable,scan.Unparseable,err)}
  _,stats,err:=collectStampedIntervalsWith(context.Background(),scanCam(f.cam),f.window,nil,stampedScanOptions{SkipUnreadable:true})
  if err!=nil || stats.RelevantCandidates!=5 || stats.MetadataCalls!=5 {t.Fatal("skipped candidate was not counted and inspected",stats,err)}
 })}
}

// Only an unusable recording is skipped. Cancellation and any other error still fail
// the whole call, as do folder-level failures.
func TestTimelineScanStillFailsOnOtherErrors(t *testing.T) {
 t.Run("other inspection error",func(t *testing.T) {
  f:=newStampedFailFixture(t)
  inspect:=func(*os.File,string) (os.FileInfo,error) {return nil,os.ErrPermission}
  scan,_,err:=collectStampedIntervalsWith(context.Background(),scanCam(f.cam),f.window,inspect,stampedScanOptions{SkipUnreadable:true})
  if !errors.Is(err,os.ErrPermission) || len(scan.Intervals)!=0 || scan.Unreadable!=0 {t.Fatal(scan,err)}
 })
 t.Run("canceled while skipping",func(t *testing.T) {
  f:=newStampedFailFixture(t);ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  inspect:=func(*os.File,string) (os.FileInfo,error) {cancel();return nil,errRecordingMetadata}
  scan,_,err:=collectStampedIntervalsWith(ctx,scanCam(f.cam),f.window,inspect,stampedScanOptions{SkipUnreadable:true})
  if !errors.Is(err,context.Canceled) || len(scan.Intervals)!=0 {t.Fatal(scan,err)}
 })
 t.Run("folder over its entry bound",func(t *testing.T) {
  f:=newStampedFailFixture(t)
  for i:=0;i<maxStampedFolderEntries;i++ {
   if err:=os.WriteFile(filepath.Join(f.day,fmt.Sprintf("%05d.mp4",i)),nil,0600);err!=nil {t.Fatal(err)}
  }
  scan,err:=collectTimelineIntervals(context.Background(),scanCam(f.cam),f.window)
  if !errors.Is(err,errRecordingScanLimit) || len(scan.Intervals)!=0 || scan.Unreadable!=0 {t.Fatal(scan,err)}
 })
}

// Skipped candidates spend the call-wide candidate budget. Four cameras of 5,001
// empty (skippable) in-window files are 20,004 candidates, so the call must fail at
// candidate 20,001 even though every one of them could have been skipped.
func TestTimelineScanCountsSkippedCandidatesAgainstTheBudget(t *testing.T) {
 root:=t.TempDir();base:=scanTime("20261110","000000")
 window:=timelineWindow{Start:base.Add(6*time.Hour),End:base.Add(18*time.Hour)}
 const cameras,each=4,maxStampedCandidates/4+1
 cams:=make([]timelineCamera,cameras)
 for c:=range cams {
  id:=fmt.Sprintf("cam%d",c);dir:=filepath.Join(root,id);cams[c]=timelineCamera{id,id,dir}
  today:=stampedDayDir(t,dir,base)
  for i:=0;i<each;i++ {
   start:=window.Start.Add(time.Duration(i)*time.Second)
   stampedPutFile(t,today,start,start.Add(time.Minute),"c",false)
  }
 }
 scan,stats,err:=collectStampedIntervalsWith(context.Background(),cams,window,nil,stampedScanOptions{SkipUnreadable:true})
 if !errors.Is(err,errRecordingScanLimit) || len(scan.Intervals)!=0 || scan.Unreadable!=0 || stats.RelevantCandidates!=maxStampedCandidates+1 || stats.MetadataCalls!=maxStampedCandidates {t.Fatal(scan,stats,err)}
}

func TestFailClosedDownloadStartsNoExport(t *testing.T) {
 n,f:=stampedFailNVR(t)
 pool:=newExportInputPool(1,2);opened:=false
 pool.open=func(string) (*os.File,error) {opened=true;return nil,os.ErrNotExist}
 query:=url.Values{"camera_id":{"front"},"start_time":{f.window.Start.Format(time.RFC3339)},"end_time":{f.window.End.Format(time.RFC3339)}}
 response:=httptest.NewRecorder()
 serveDownload(response,httptest.NewRequest("GET","/api/download?"+query.Encode(),nil),n,pool)
 if response.Code!=503 || opened || strings.HasPrefix(response.Header().Get("Content-Type"),"video/") || response.Header().Get("Content-Disposition")!="" || len(pool.slots)!=0 {
  t.Fatal("failed scan reached pinning or streaming",response.Code,response.Header(),opened)
 }
}

func TestFailClosedExportPlanningReturnsNoRenderer(t *testing.T) {
 n,f:=stampedFailNVR(t)
 render,err:=planExistingExportJob(context.Background(),n,"",exportCreateRequest{CameraID:"front",Start:f.window.Start.Format(time.RFC3339),End:f.window.End.Format(time.RFC3339),Mode:"copy"})
 var httpErr *exportPlanHTTPError
 if render!=nil || !errors.Is(err,errRecordingMetadata) || errors.As(err,&httpErr) {t.Fatal("failed scan produced an export plan",err)}
}
