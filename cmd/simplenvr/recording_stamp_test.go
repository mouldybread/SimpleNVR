package main

import (
 "errors"
 "strings"
 "testing"
 "time"
)

func TestStampedNameRoundTripAndRejects(t *testing.T) {
 start:=time.Date(2026,11,10,12,0,0,0,time.UTC);end:=start.Add(10*time.Minute)
 name:=stampedFileName(start,end,"cam_id_x")
 if name!="20261110_120000_E20261110_121000_cam_id_x.mp4" {t.Fatal(name)}
 got,err:=parseStampedName(name)
 if err!=nil || !got.Start.Equal(start) || !got.End.Equal(end) {t.Fatal(got,err)}
 hkt:=time.FixedZone("HKT",8*3600)
 if stampedFileName(start.In(hkt),end.In(hkt),"x")!=stampedFileName(start,end,"x") {t.Fatal("name depends on zone")}
 if _,err:=parseStampedName("20261110_120000_E20261111_120000_x.mp4");err!=nil {t.Fatal("exactly 24h span rejected",err)}
 for _,bad:=range []string{"","20261110_120000_cam.mp4","20261110_120000_E20261110_121000.mp4","20261110_120000_E20261110_121000_x.mp3","20261110_120000_E20261110_115959_x.mp4","20261110_120000_E20261110_120000_x.mp4","20261110_120000_E20261111_120001_x.mp4","20261310_120000_E20261310_121000_x.mp4","20261110_120000-E20261110_121000_x.mp4","buffer_20261110_120000_nonce.mp4","20261110_120000_E20261110_121000_x.mp4.tmp"} {
  if _,err:=parseStampedName(bad);!errors.Is(err,errStampedName) {t.Fatal("accepted",bad,err)}
 }
}

func TestStampedInWindowEdges(t *testing.T) {
 hkt:=time.FixedZone("HKT",8*3600)
 base:=time.Date(2026,11,10,12,0,0,0,time.UTC)
 for _,zone:=range []*time.Location{time.UTC,hkt} {
  ws:=base.In(zone);we:=base.Add(time.Hour).In(zone)
  cases:=[]struct{name string;start,end time.Time;want bool}{
   {"ends exactly at window start",base.Add(-10*time.Minute),base,false},
   {"ends one second inside",base.Add(-10*time.Minute),base.Add(time.Second),true},
   {"starts just before and runs through",base.Add(-time.Second),base.Add(10*time.Minute),true},
   {"starts exactly at window end",base.Add(time.Hour),base.Add(70*time.Minute),false},
   {"starts one second before window end",base.Add(time.Hour-time.Second),base.Add(70*time.Minute),true},
   {"inside",base.Add(time.Minute),base.Add(11*time.Minute),true},
   {"entirely before",base.Add(-time.Hour),base.Add(-50*time.Minute),false},
   {"entirely after",base.Add(2*time.Hour),base.Add(130*time.Minute),false},
  }
  for _,c:=range cases {
   if got:=stampedInWindow(stampedName{c.start,c.end},ws,we);got!=c.want {t.Fatal(zone,c.name,got)}
  }
 }
}

func TestStampedScanDays(t *testing.T) {
 hkt:=time.FixedZone("HKT",8*3600)
 ws:=time.Date(2026,11,10,12,0,0,0,time.UTC)
 cases:=[]struct{name string;start,end time.Time;want string}{
  {"short window",ws,ws.Add(time.Hour),"20261109,20261110"},
  {"ends exactly at midnight",time.Date(2026,11,10,22,0,0,0,time.UTC),time.Date(2026,11,11,0,0,0,0,time.UTC),"20261109,20261110"},
  {"crosses midnight",time.Date(2026,11,10,22,0,0,0,time.UTC),time.Date(2026,11,11,0,0,1,0,time.UTC),"20261109,20261110,20261111"},
  {"26 hour window",time.Date(2026,11,10,0,0,0,0,time.UTC),time.Date(2026,11,11,2,0,0,0,time.UTC),"20261109,20261110,20261111"},
  {"window given in HKT",time.Date(2026,11,10,7,0,0,0,hkt),time.Date(2026,11,10,8,0,0,0,hkt),"20261108,20261109"},
 }
 for _,c:=range cases {
  days,err:=stampedScanDays(c.start,c.end)
  if err!=nil || strings.Join(days,",")!=c.want {t.Fatal(c.name,days,err)}
 }
 if _,err:=stampedScanDays(ws,ws);!errors.Is(err,errStampedWindow) {t.Fatal(err)}
 if _,err:=stampedScanDays(ws,ws.AddDate(0,0,400));!errors.Is(err,errStampedScanDays) {t.Fatal("unbounded day scan",err)}
}

func TestStampedNameInDay(t *testing.T) {
 n,err:=parseStampedName("20261110_235000_E20261111_001000_x.mp4")
 if err!=nil || !n.inDay("20261110") || n.inDay("20261111") {t.Fatal(n,err)}
}
