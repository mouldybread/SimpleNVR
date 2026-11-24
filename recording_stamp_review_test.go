package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// All 32 names fit in the first 256-name batch. Failure on inspection 1 or 3
// must still account for the entire returned batch, independent of name order.
func TestStampedScanCountsReadAheadOnInspectionFailure(t *testing.T) {
 for _,failAt:=range []int{1,3} {
  t.Run(fmt.Sprintf("inspection_%d",failAt),func(t *testing.T) {
   cam:=t.TempDir();base:=scanTime("20261110","000000")
   day:=stampedDayDir(t,cam,base)
   for i:=0;i<32;i++ {
    start:=base.Add(time.Duration(i)*time.Minute)
    stampedPutFile(t,day,start,start.Add(time.Minute),"batch",true)
   }
   calls:=0
   inspect:=func(dir *os.File,name string) (os.FileInfo,error) {
    calls++
    if calls==failAt {return nil,errRecordingMetadata}
    return stampedCandidateInfo(dir,name)
   }
   scan,stats,err:=collectStampedIntervalsObserved(context.Background(),scanCam(cam),timelineWindow{Start:base,End:base.Add(time.Hour)},inspect)
   want:=stampedScanStats{DayLookups:2,DaysOpened:1,EntriesRead:32,MetadataCalls:failAt}
   if !errors.Is(err,errRecordingMetadata) || scan.Intervals!=nil || scan.Unparseable!=0 || stats!=want || calls!=failAt {t.Fatal(scan,stats,err,calls)}
  })
 }
}

// Camera slices have a defined order, unlike the NVR map in the HTTP test.
// Establish that healthy intervals really were accumulated before inspecting the
// failing camera, then prove none escape in the returned result.
func TestStampedScanDiscardsEarlierHealthyCamera(t *testing.T) {
 healthy:=newStampedFailFixture(t)
 bad:=t.TempDir();start,end:=healthy.badName()
 badDay:=stampedDayDir(t,bad,start)
 stampedPutFile(t,badDay,start,end,"empty",false)
 cams:=[]timelineCamera{{"healthy","Healthy",healthy.cam},{"bad","Bad",bad}}
 healthyCalls,badCalls:=0,0
 inspect:=func(dir *os.File,name string) (os.FileInfo,error) {
  switch dir.Name() {
  default:
   // Directory handles may expose only the opened basename. Use the held
   // directory identity instead of a pathname label to identify the camera.
   info,err:=dir.Stat();if err!=nil {return nil,err}
   healthyInfo,err:=os.Stat(healthy.day);if err!=nil {return nil,err}
   if os.SameFile(info,healthyInfo) {healthyCalls++} else {
    badCalls++
    if healthyCalls!=4 {t.Errorf("bad camera inspected before all healthy candidates: %d",healthyCalls)}
   }
  }
  return stampedCandidateInfo(dir,name)
 }
 scan,stats,err:=collectStampedIntervalsObserved(context.Background(),cams,healthy.window,inspect)
 want:=stampedScanStats{DayLookups:4,DaysOpened:2,EntriesRead:5,MetadataCalls:5}
 if !errors.Is(err,errRecordingMetadata) || scan.Intervals!=nil || scan.Unparseable!=0 || stats!=want || healthyCalls!=4 || badCalls!=1 {t.Fatal(scan,stats,err,healthyCalls,badCalls)}
 // The failing fixture itself is a real empty in-window file, not an injected error.
 info,statErr:=os.Stat(filepath.Join(badDay,stampedFileName(start,end,"empty")))
 if statErr!=nil || info.Size()!=0 {t.Fatal(info,statErr)}
}
