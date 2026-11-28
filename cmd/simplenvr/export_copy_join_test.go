package main

import (
 "context"
 "encoding/json"
 "math"
 "os"
 "os/exec"
 "sort"
 "testing"
 "time"
)

type copyJoinPacket struct {
 Stream int `json:"stream_index"`
 PTS string `json:"pts_time"`
 DTS string `json:"dts_time"`
 Duration string `json:"duration_time"`
}

func TestCopyAcrossTwoBFrameRecordingsWithAudio(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel := context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 // Separate files with matching codec configuration; each is six seconds.
 sources := []string{makePreparedBFrameSource(t,ctx,false),makePreparedBFrameSource(t,ctx,false)}
 before := [][32]byte{preparedMediaDigest(t,sources[0]),preparedMediaDigest(t,sources[1])}
 base := time.Date(2026,11,22,0,0,0,0,time.UTC)
 var clips []recordingInterval
 for i,path := range sources {
  file,err := os.Open(path); if err != nil { t.Fatal(err) }
  info,statErr := file.Stat()
  duration,probeErr := probeVideoDuration(ctx,file)
  file.Close()
  if statErr != nil { t.Fatal(statErr) }; if probeErr != nil { t.Fatal(probeErr) }
  if duration != 6*time.Second { t.Fatalf("unexpected fixture duration: %v",duration) }
  start := base.Add(time.Duration(i)*6*time.Second)
  clips = append(clips,recordingInterval{CameraID:"camera",Path:path,Start:start,End:start.Add(duration),Size:info.Size(),identity:info})
 }
 requested,err := buildExportPlan(clips,timelineWindow{Start:base.Add(4500*time.Millisecond),End:base.Add(7500*time.Millisecond)})
 if err != nil { t.Fatal(err) }
 pool := newExportInputPool(1,128); pool.tempDir=t.TempDir()
 pins,err := pool.prepare(ctx,requested.clips()); if err != nil { t.Fatal(err) }
 defer pins.Close()
 actual,indices,err := planKeyframeCopy(ctx,pins,requested); if err != nil { t.Fatal(err) }
 if len(actual.Parts)!=2 || !actual.Start.Equal(base.Add(4*time.Second)) || !actual.End.Equal(base.Add(8*time.Second)) { t.Fatalf("wrong cross-file plan: %+v",actual) }
 var expected []string
 for i,part := range actual.Parts {
  index := indices[i]
  startDTS := 0.0; found := false
  for j,key := range index.keys { if key==part.In { startDTS=index.origin+index.ends[j].Seconds(); found=true; break } }
  if !found { t.Fatal("part does not begin at IDR") }
  endDTS := copyOutpoint(index,part)
  t.Logf("part %d: origin=%.9f in=%.9f out=%.9f startDTS=%.9f endDTS=%.9f audioAllowance=%s",i,index.origin,part.In.Seconds(),part.Out.Seconds(),startDTS,endDTS,index.audioAllowance)
  for _,packet := range copyTestVideoPackets(t,ctx,part.Clip.Path) {
   dts,err := preparedMediaSeconds(packet.DTS); if err != nil { t.Fatal(err) }
   if dts>=startDTS-1e-7 && dts<endDTS-1e-7 { expected=append(expected,packet.Hash) }
  }
 }
 if len(expected)!=80 { t.Fatalf("expected 80 source video packets, got %d",len(expected)) }
 output := preparedMediaOutput(t)
 if err := renderKeyframeCopy(ctx,pins,actual,indices,output,16<<20); err != nil { t.Fatal(err) }
 decodePreparedMedia(t,ctx,output.Name())
 got := copyTestVideoPackets(t,ctx,output.Name())
 if len(got)!=len(expected) { t.Fatalf("video packets: got %d want %d",len(got),len(expected)) }
 for i,packet := range got { if packet.Hash!=expected[i] { t.Fatalf("video packet %d changed, missing or out of order at join",i) } }
 if n:=countPreparedVideoFrames(t,ctx,output.Name()); n!=80 { t.Fatalf("decoded %d frames, want 80",n) }
 raw,err := exec.CommandContext(ctx,"ffprobe","-v","error","-show_packets","-show_entries","packet=stream_index,pts_time,dts_time,duration_time","-of","json",output.Name()).Output()
 if err != nil { t.Fatal(err) }
 var data struct{ Packets []copyJoinPacket `json:"packets"` }
 if err:=json.Unmarshal(raw,&data);err!=nil { t.Fatal(err) }
 var videoPTS,audioPTS,audioEnd []float64
 lastDTS:=map[int]float64{}
 for _,packet := range data.Packets {
  pts,e1:=preparedMediaSeconds(packet.PTS);dts,e2:=preparedMediaSeconds(packet.DTS);duration,e3:=preparedMediaSeconds(packet.Duration)
  if e1!=nil || e2!=nil || e3!=nil || duration<=0 { t.Fatal("invalid packet timestamps") }
  if last,ok:=lastDTS[packet.Stream];ok && dts<=last { t.Fatalf("non-increasing DTS on stream %d",packet.Stream) }
  lastDTS[packet.Stream]=dts
  switch packet.Stream {
  case 0: videoPTS=append(videoPTS,pts)
  case 1: audioPTS=append(audioPTS,pts);audioEnd=append(audioEnd,pts+duration)
  default: t.Fatal("unexpected track")
  }
 }
 if len(videoPTS)!=80 || len(audioPTS)==0 { t.Fatal("missing video or audio") }
 sort.Float64s(videoPTS)
 for i,pts:=range videoPTS { if math.Abs(pts-videoPTS[0]-float64(i)/20)>0.005 { t.Fatalf("video gap or duplicate at frame %d",i) } }
 // AAC packets span 64 ms at 16 kHz. Allow 150 ms at the outside edges,
 // but at most one AAC packet plus 5 ms of gap/overlap inside, including join.
 const edgeTolerance=0.15
 videoStart,videoEnd:=videoPTS[0],videoPTS[0]+4
 startOffset,endOffset:=audioPTS[0]-videoStart,audioEnd[len(audioEnd)-1]-videoEnd
 t.Logf("video packets=%d range=[%.9f,%.9f) audio packets=%d range=[%.9f,%.9f) startOffset=%.9f endOffset=%.9f",len(videoPTS),videoStart,videoEnd,len(audioPTS),audioPTS[0],audioEnd[len(audioEnd)-1],startOffset,endOffset)
 if math.Abs(startOffset)>edgeTolerance || math.Abs(endOffset)>edgeTolerance {
  t.Errorf("audio does not cover video range within AAC boundary allowance: startOffset=%.9fs endOffset=%.9fs tolerance=%.9fs",startOffset,endOffset,edgeTolerance)
 }
 join:=videoStart+2
 for i,start:=range audioPTS {
  if i<3 || i>=len(audioPTS)-3 || (start<=join+0.2 && audioEnd[i]>=join-0.2) {
   t.Logf("audio packet %d: pts=%.9f end=%.9f relativeToJoin=%.9f",i,start,audioEnd[i],start-join)
  }
  if i>0 && math.Abs(start-audioEnd[i-1])>0.069 {
   t.Errorf("audio gap or overlap at packet %d: pts=%.9f previousEnd=%.9f delta=%.9fs",i,start,audioEnd[i-1],start-audioEnd[i-1])
  }
 }
 covered:=false
 for i,start:=range audioPTS { if start<=join && audioEnd[i]>=join { covered=true;break } }
 if !covered { t.Errorf("audio packet coverage missing at recording join %.9f",join) }
 for i,path:=range sources { if preparedMediaDigest(t,path)!=before[i] { t.Fatal("source changed") } }
}
