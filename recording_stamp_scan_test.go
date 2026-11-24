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

func scanTime(day,clock string) time.Time {
 ts,err:=time.ParseInLocation("20060102 150405",day+" "+clock,time.UTC);if err!=nil {panic(err)}
 return ts
}
func scanFixture(t *testing.T,cam string,start,end time.Time,label string) string {
 t.Helper()
 dir:=filepath.Join(cam,stampedDayName(start))
 if err:=os.MkdirAll(dir,0755);err!=nil {t.Fatal(err)}
 path:=filepath.Join(dir,stampedFileName(start,end,label))
 if err:=os.WriteFile(path,[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
 return path
}
func scanCam(dir string) []timelineCamera {return []timelineCamera{{"front","Front",dir}}}

func TestStampedScanWindowEdgesAndPriorDay(t *testing.T) {
 cam:=t.TempDir()
 window:=timelineWindow{Start:scanTime("20261110","060000"),End:scanTime("20261111","060000")}
 scanFixture(t,cam,scanTime("20261110","050000"),scanTime("20261110","060000"),"touch_start")
 scanFixture(t,cam,scanTime("20261109","230000"),scanTime("20261110","070000"),"crosses_midnight")
 scanFixture(t,cam,scanTime("20261110","055959"),scanTime("20261110","060001"),"overlap")
 scanFixture(t,cam,scanTime("20261110","060000"),scanTime("20261110","061000"),"first")
 scanFixture(t,cam,scanTime("20261111","055959"),scanTime("20261111","060000"),"last")
 scanFixture(t,cam,scanTime("20261111","060000"),scanTime("20261111","070000"),"touch_end")
 scan,err:=collectStampedIntervals(context.Background(),scanCam(cam),window)
 if err!=nil || scan.Unparseable!=0 || len(scan.Intervals)!=4 {t.Fatal(scan,err)}
 want:=[]time.Time{scanTime("20261109","230000"),scanTime("20261110","055959"),scanTime("20261110","060000"),scanTime("20261111","055959")}
 for i,clip:=range scan.Intervals {
  if !clip.Start.Equal(want[i]) || clip.identity==nil || clip.Size!=7 || clip.CameraID!="front" {t.Fatalf("wrong interval %d: %+v",i,clip)}
 }
}

func TestStampedScanCountsButNeverReturnsUnparseableNames(t *testing.T) {
 cam:=t.TempDir()
 window:=timelineWindow{Start:scanTime("20261110","000000"),End:scanTime("20261111","000000")}
 scanFixture(t,cam,scanTime("20261110","010000"),scanTime("20261110","011000"),"good")
 day:=filepath.Join(cam,"20261110")
 for _,name:=range []string{"20261110_070000_old.mp4","20261110_070000_E20261110_060000_bad.mp4","notes.tmp"} {
  if err:=os.WriteFile(filepath.Join(day,name),[]byte("x"),0600);err!=nil {t.Fatal(err)}
 }
 other:=filepath.Join(cam,"20261109")
 if err:=os.MkdirAll(other,0755);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(other,stampedFileName(scanTime("20261110","070000"),scanTime("20261110","071000"),"misplaced")),[]byte("x"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(day,stampedFileName(scanTime("20261110","080000"),scanTime("20261110","081000"),"dir")),0755);err!=nil {t.Fatal(err)}
 scan,err:=collectStampedIntervals(context.Background(),scanCam(cam),window)
 if err!=nil || len(scan.Intervals)!=1 || scan.Unparseable!=3 {t.Fatal(scan,err)}
}

func TestStampedScanDoesNotFollowSymlinkedDayOrFile(t *testing.T) {
 outside,cam:=t.TempDir(),t.TempDir()
 target:=scanFixture(t,outside,scanTime("20261110","010000"),scanTime("20261110","011000"),"outside")
 window:=timelineWindow{Start:scanTime("20261110","000000"),End:scanTime("20261111","000000")}
 if err:=os.Symlink(filepath.Dir(target),filepath.Join(cam,"20261110"));err!=nil {t.Fatal(err)}
 scan,err:=collectStampedIntervals(context.Background(),scanCam(cam),window)
 if err!=nil || len(scan.Intervals)!=0 || scan.Unparseable!=0 {t.Fatal(scan,err)}
 cam2:=t.TempDir()
 real:=scanFixture(t,cam2,scanTime("20261110","020000"),scanTime("20261110","021000"),"real")
 link:=filepath.Join(cam2,"20261110",stampedFileName(scanTime("20261110","030000"),scanTime("20261110","031000"),"link"))
 if err:=os.Symlink(real,link);err!=nil {t.Fatal(err)}
 scan,err=collectStampedIntervals(context.Background(),scanCam(cam2),window)
 if err!=nil || len(scan.Intervals)!=1 || scan.Unparseable!=0 {t.Fatal(scan,err)}
}

func TestStampedScanLimitsMissingDirAndCancellation(t *testing.T) {
 window:=timelineWindow{Start:scanTime("20261110","000000"),End:scanTime("20261111","000000")}
 scan,err:=collectStampedIntervals(context.Background(),scanCam(filepath.Join(t.TempDir(),"missing")),window)
 if err!=nil || len(scan.Intervals)!=0 {t.Fatal(scan,err)}
 cam:=t.TempDir();day:=filepath.Join(cam,"20261110")
 if err:=os.MkdirAll(day,0755);err!=nil {t.Fatal(err)}
 for i:=0;i<=maxStampedFolderEntries;i++ {
  file,err:=os.Create(filepath.Join(day,fmt.Sprintf("%05d.mp4",i)));if err!=nil {t.Fatal(err)};file.Close()
 }
 if _,err:=collectStampedIntervals(context.Background(),scanCam(cam),window);!errors.Is(err,errRecordingScanLimit) {t.Fatal(err)}
 wide:=timelineWindow{Start:window.Start,End:window.Start.AddDate(0,0,maxStampedScanDays+1)}
 if _,err:=collectStampedIntervals(context.Background(),scanCam(cam),wide);!errors.Is(err,errStampedScanDays) {t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=collectStampedIntervals(ctx,scanCam(cam),window);!errors.Is(err,context.Canceled) {t.Fatal(err)}
}
