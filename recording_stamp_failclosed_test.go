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

// Every failure below must discard the whole result: no partial interval list, no
// partial timeline body, no export started. Four valid candidates sit in the window.

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

func TestFailClosedTimelineHasNoPartialResult(t *testing.T) {
 n,f:=stampedFailNVR(t)
 query:=url.Values{"start_time":{f.window.Start.Format(time.RFC3339)},"end_time":{f.window.End.Format(time.RFC3339)}}
 response:=httptest.NewRecorder()
 serveTimelineRecordings(response,httptest.NewRequest("GET","/api/timeline/recordings?"+query.Encode(),nil),n)
 body:=response.Body.String()
 if response.Code!=503 || response.Header().Get("X-Recording-Metadata-Status")!="unavailable" || response.Header().Get("X-Timeline-Unparseable-Count")!="" || strings.Contains(body,"file_path") || strings.HasPrefix(strings.TrimSpace(body),"[") {
  t.Fatal("failed scan produced a partial timeline",response.Code,response.Header(),body)
 }
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
