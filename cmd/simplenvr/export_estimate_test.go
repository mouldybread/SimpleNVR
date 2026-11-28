package main

import (
 "math"
 "testing"
 "time"
)

func TestEstimateExportBytes(t *testing.T) {
 base:=time.Date(2026,11,21,0,0,0,0,time.UTC)
 clip:=recordingInterval{Start:base,End:base.Add(10*time.Second),Size:10<<20}
 whole:=exportPlan{Parts:[]exportPlanPart{{Clip:clip,In:0,Out:10*time.Second}}}
 half:=exportPlan{Parts:[]exportPlanPart{{Clip:clip,In:2*time.Second,Out:7*time.Second}}}
 two:=exportPlan{Parts:[]exportPlanPart{{Clip:clip,In:0,Out:10*time.Second},{Clip:clip,In:0,Out:10*time.Second}}}
 for _,tc:=range []struct{name string;plan exportPlan;mode string;want float64}{
  {"whole copy",whole,"copy",float64(10<<20)*1.15+float64(8<<20)},
  {"half of a recording copies half of the file",half,"copy",float64(5<<20)*1.15+float64(8<<20)},
  {"two recordings add up",two,"copy",float64(20<<20)*1.15+float64(8<<20)},
  {"accurate leaves room for re-encoding",whole,"accurate",float64(40<<20)},
 } {t.Run(tc.name,func(t *testing.T){
  got:=estimateExportBytes(tc.plan,tc.mode)
  if math.Abs(float64(got)-tc.want)>2 {t.Fatalf("got %d want %.0f",got,tc.want)}
 })}
}

func TestEstimateExportBytesUnknownSizeReservesMaximum(t *testing.T) {
 base:=time.Date(2026,11,21,0,0,0,0,time.UTC)
 unknownSize:=recordingInterval{Start:base,End:base.Add(10*time.Second)}
 noSpan:=recordingInterval{Start:base,End:base,Size:1<<20}
 for name,plan:=range map[string]exportPlan{
  "no parts":{},
  "unknown file size":{Parts:[]exportPlanPart{{Clip:unknownSize,In:0,Out:10*time.Second}}},
  "empty clip span":{Parts:[]exportPlanPart{{Clip:noSpan,In:0,Out:time.Second}}},
  "empty used range":{Parts:[]exportPlanPart{{Clip:recordingInterval{Start:base,End:base.Add(10*time.Second),Size:1<<20},In:time.Second,Out:time.Second}}},
 } {if got:=estimateExportBytes(plan,"copy");got!=0 && name!="no parts" {t.Fatalf("%s: estimate %d, want 0",name,got)}}
}
