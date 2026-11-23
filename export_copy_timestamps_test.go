package main

import (
 "context"
 "encoding/json"
 "fmt"
 "math"
 "os"
 "os/exec"
 "strings"
 "testing"
 "time"
)

type copyTimingPacket struct {
 PTS *int64 `json:"pts"`
 DTS *int64 `json:"dts"`
 Flags string `json:"flags"`
}

func validateCopyVideoTiming(packets []copyTimingPacket) error {
 if len(packets)==0 { return fmt.Errorf("no video packets") }
 if !strings.Contains(packets[0].Flags,"K") { return fmt.Errorf("first video packet is not a keyframe") }
 var last int64
 for i,p:=range packets {
  if p.PTS==nil || p.DTS==nil { return fmt.Errorf("video packet %d has missing PTS or DTS",i) }
  if *p.PTS<*p.DTS { return fmt.Errorf("video packet %d: PTS %d precedes DTS %d",i,*p.PTS,*p.DTS) }
  if i>0 && *p.DTS<=last { return fmt.Errorf("video packet %d: DTS %d does not follow %d",i,*p.DTS,last) }
  last=*p.DTS
 }
 return nil
}

func TestCopyVideoTimingValidator(t *testing.T) {
 n:=func(v int64)*int64{return &v}
 valid:=[]copyTimingPacket{{n(0),n(-2),"K_"},{n(3),n(-1),"__"},{n(1),n(0),"__"},{n(2),n(1),"__"}}
 cases:=[]struct{name string; packets []copyTimingPacket; wantError bool}{
  {"reordered PTS and negative initial DTS",valid,false},
  {"empty",nil,true},
  {"non-keyframe start",[]copyTimingPacket{{n(0),n(0),"__"}},true},
  {"missing PTS",[]copyTimingPacket{{nil,n(0),"K_"}},true},
  {"missing DTS",[]copyTimingPacket{{n(0),nil,"K_"}},true},
  {"PTS before DTS",[]copyTimingPacket{{n(-1),n(0),"K_"}},true},
  {"backward DTS",[]copyTimingPacket{{n(0),n(0),"K_"},{n(1),n(-1),"__"}},true},
  {"duplicate DTS",[]copyTimingPacket{{n(0),n(0),"K_"},{n(1),n(0),"__"}},true},
 }
 for _,tc:=range cases { t.Run(tc.name,func(t *testing.T){
  err:=validateCopyVideoTiming(tc.packets)
  if (err!=nil)!=tc.wantError { t.Fatalf("validation error = %v, wantError %v",err,tc.wantError) }
 }) }
}

func TestKeyframeCopyVideoPacketTiming(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 sources:=[]string{makePreparedBFrameSource(t,ctx,false),makePreparedBFrameSource(t,ctx,false)}
 base:=time.Date(2026,11,22,0,0,0,0,time.UTC)
 var clips []recordingInterval
 for i,path:=range sources {
  file,err:=os.Open(path);if err!=nil{t.Fatal(err)}
  info,statErr:=file.Stat();duration,probeErr:=probeVideoDuration(ctx,file);file.Close()
  if statErr!=nil{t.Fatal(statErr)};if probeErr!=nil{t.Fatal(probeErr)}
  if duration!=6*time.Second{t.Fatalf("fixture duration: %v",duration)}
  start:=base.Add(time.Duration(i)*6*time.Second)
  clips=append(clips,recordingInterval{CameraID:"camera",Path:path,Start:start,End:start.Add(duration),Size:info.Size(),identity:info})
 }
 cases:=[]struct{name string;start,end time.Duration;parts int}{
  {"single-file B-frame cut",1500*time.Millisecond,3500*time.Millisecond,1},
  {"two-file B-frame join",4500*time.Millisecond,7500*time.Millisecond,2},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  requested,err:=buildExportPlan(clips,timelineWindow{Start:base.Add(tc.start),End:base.Add(tc.end)});if err!=nil{t.Fatal(err)}
  pool:=newExportInputPool(1,128);pool.tempDir=t.TempDir()
  pins,err:=pool.prepare(ctx,requested.clips());if err!=nil{t.Fatal(err)};defer pins.Close()
  actual,indices,err:=planKeyframeCopy(ctx,pins,requested);if err!=nil{t.Fatal(err)}
  if len(actual.Parts)!=tc.parts{t.Fatalf("parts: got %d want %d",len(actual.Parts),tc.parts)}
  output:=preparedMediaOutput(t)
  if err:=renderKeyframeCopy(ctx,pins,actual,indices,output,16<<20);err!=nil{t.Fatal(err)}
  // Integer timestamps share the selected video stream's time base. Keep
  // demux order: sorting would hide backward DTS at recording joins.
  raw,err:=exec.CommandContext(ctx,"ffprobe","-v","error","-select_streams","v:0","-show_packets","-show_entries","packet=pts,dts,flags","-of","json",output.Name()).Output();if err!=nil{t.Fatal(err)}
  var data struct{Packets []copyTimingPacket `json:"packets"`}
  if err:=json.Unmarshal(raw,&data);err!=nil{t.Fatal(err)}
  if err:=validateCopyVideoTiming(data.Packets);err!=nil{t.Error(err)}
  // Compare presentation starts in seconds, not raw ticks from different
  // stream time bases. Earliest PTS accounts for B-frame packet reordering.
  raw,err=exec.CommandContext(ctx,"ffprobe","-v","error","-show_packets","-show_entries","packet=codec_type,pts_time","-of","json",output.Name()).Output();if err!=nil{t.Fatal(err)}
  var timing struct{Packets []struct{Kind string `json:"codec_type"`;PTS string `json:"pts_time"`} `json:"packets"`}
  if err:=json.Unmarshal(raw,&timing);err!=nil{t.Fatal(err)}
  starts:=map[string]float64{}
  for i,p:=range timing.Packets{
   if p.Kind!="video" && p.Kind!="audio"{continue}
   pts,err:=preparedMediaSeconds(p.PTS)
   if err!=nil || math.IsNaN(pts) || math.IsInf(pts,0){t.Fatalf("%s packet %d has invalid PTS %q",p.Kind,i,p.PTS)}
   if previous,ok:=starts[p.Kind];!ok || pts<previous{starts[p.Kind]=pts}
  }
  video,hasVideo:=starts["video"];audio,hasAudio:=starts["audio"]
  if !hasVideo || !hasAudio{t.Fatalf("missing packet timestamps: video=%v audio=%v",hasVideo,hasAudio)}
  const tolerance=0.15 // Same AAC edge allowance as the existing join test.
  offset:=audio-video
  t.Logf("presentation starts: video=%.9fs audio=%.9fs audioMinusVideo=%.9fs",video,audio,offset)
  if math.Abs(offset)>tolerance{t.Errorf("audio start misaligned: video=%.9fs audio=%.9fs offset=%.9fs tolerance=%.9fs",video,audio,offset,tolerance)}
 })}
}
