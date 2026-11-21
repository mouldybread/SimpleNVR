package main

import (
 "context"
 "crypto/sha256"
 "encoding/binary"
 "encoding/json"
 "errors"
 "io"
 "math"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestPreparedMediaCameraProfile(t *testing.T){
 raw:=`{"streams":[{"codec_name":"h264","codec_type":"video","width":2688,"height":1520,"pix_fmt":"yuvj420p","r_frame_rate":"20/1","avg_frame_rate":"1200000/60001","start_time":"0.000000"},{"codec_name":"aac","codec_type":"audio","sample_rate":"16000","channels":1,"start_time":"0.000000"}]}`
 profile,err:=decodePreparedMediaProfile([]byte(raw));if err!=nil || len(profile.Streams)!=2{t.Fatalf("camera profile refused: %+v %v",profile,err)}
 for _,invalid:=range []string{strings.Replace(raw,"yuvj420p","yuv420p10le",1),strings.Replace(raw,`"height":1520`,`"height":1521`,1),strings.Replace(raw,`"channels":1`,`"channels":0`,1),strings.Replace(raw,`"codec_name":"aac"`,`"codec_name":"unknown"`,1)}{
  if _,err:=decodePreparedMediaProfile([]byte(invalid));err==nil{t.Fatal("unsupported input accepted",invalid)}
 }
}

func makePreparedMediaSource(t *testing.T,ctx context.Context,delayed bool)string{
 t.Helper();file:=filepath.Join(t.TempDir(),"source.mp4")
 args:=[]string{"-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=128x96:rate=20"}
 if delayed{args=append(args,"-itsoffset","0.25")}
 args=append(args,"-f","lavfi","-i","sine=frequency=440:sample_rate=16000","-t","2","-c:v","libx264","-threads","1","-g","20","-pix_fmt","yuvj420p","-color_range","pc","-c:a","aac","-ac","1","-y",file)
 if raw,err:=exec.CommandContext(ctx,"ffmpeg",args...).CombinedOutput();err!=nil{t.Fatalf("fixture generation: %v %s",err,raw)}
 return file
}
func preparedMediaTestPlan(t *testing.T,ctx context.Context,source string,count int,start,end time.Duration)(exportPlan,*pinnedExport){
 t.Helper();file,err:=os.Open(source);if err!=nil{t.Fatal(err)};defer file.Close()
 info,err:=file.Stat();if err!=nil{t.Fatal(err)};duration,err:=probeVideoDuration(ctx,file);if err!=nil{t.Fatal(err)}
 base:=time.Date(2026,11,21,0,0,0,0,time.UTC);clips:=make([]recordingInterval,count)
 for i:=range clips{begin:=base.Add(time.Duration(i)*duration);clips[i]=recordingInterval{CameraID:"camera",Path:source,Start:begin,End:begin.Add(duration),Size:info.Size(),identity:info}}
 plan,err:=buildExportPlan(clips,timelineWindow{Start:base.Add(start),End:base.Add(end)});if err!=nil{t.Fatal(err)}
 pool:=newExportInputPool(1,128);pool.tempDir=t.TempDir();pins,err:=pool.prepare(ctx,plan.clips());if err!=nil{t.Fatal(err)};t.Cleanup(func(){if err:=pins.Close();err!=nil{t.Error(err)}})
 return plan,pins
}
func preparedMediaOutput(t *testing.T)*os.File{
 t.Helper();file,err:=os.CreateTemp(t.TempDir(),"export-*.mp4");if err!=nil{t.Fatal(err)};t.Cleanup(func(){file.Close()});return file
}
func preparedMediaDigest(t *testing.T,path string)[32]byte{t.Helper();raw,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)};return sha256.Sum256(raw)}
func decodePreparedMedia(t *testing.T,ctx context.Context,path string){
 t.Helper();raw,err:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-xerror","-i",path,"-map","0:v:0","-map","0:a:0","-f","null","-").CombinedOutput();if err!=nil{t.Fatalf("external A/V decode: %v %s",err,raw)}
}
func preparedPacketHashes(t *testing.T,ctx context.Context,path string)[]string{
 t.Helper();raw,err:=exec.CommandContext(ctx,"ffprobe","-v","error","-show_packets","-show_data_hash","sha256","-show_entries","packet=stream_index,data_hash","-of","json",path).Output();if err!=nil{t.Fatal(err)}
 var data struct{Packets []struct{Index int `json:"stream_index"`;Hash string `json:"data_hash"`} `json:"packets"`};if err:=json.Unmarshal(raw,&data);err!=nil{t.Fatal(err)}
 hashes:=make([]string,2)
 for _,packet:=range data.Packets{if packet.Index<0 || packet.Index>1 || packet.Hash==""{t.Fatal("unexpected packet")};hashes[packet.Index]+=packet.Hash+"\n"}
 return hashes
}

func TestPreparedMediaCopyAndAccurateAcrossClips(t *testing.T){
 requireFFmpeg(t);ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 source:=makePreparedMediaSource(t,ctx,false);before:=preparedMediaDigest(t,source)
 plan,pins:=preparedMediaTestPlan(t,ctx,source,1,0,2*time.Second)
 original:=preparedMediaOutput(t)
 if err:=renderPreparedMedia(ctx,pins,plan,"copy",original,16<<20);err!=nil{t.Fatal(err)}
 if preparedMediaDigest(t,original.Name())!=before{t.Fatal("single complete copy changed original bytes")}
 plan,pins=preparedMediaTestPlan(t,ctx,source,2,0,4*time.Second)
 copied:=preparedMediaOutput(t);if err:=renderPreparedMedia(ctx,pins,plan,"copy",copied,16<<20);err!=nil{t.Fatal(err)}
 expected:=preparedPacketHashes(t,ctx,source);actual:=preparedPacketHashes(t,ctx,copied.Name())
 if actual[0]!=expected[0]+expected[0] || actual[1]!=expected[1]+expected[1]{t.Fatal("stream copy changed/dropped compressed video or audio packets")}
 decodePreparedMedia(t,ctx,copied.Name())
 plan,pins=preparedMediaTestPlan(t,ctx,source,2,500*time.Millisecond,3500*time.Millisecond)
 refused:=preparedMediaOutput(t);if err:=renderPreparedMedia(ctx,pins,plan,"copy",refused,16<<20);!errors.Is(err,errPreparedCopyBoundary){t.Fatalf("partial copy silently changed boundaries: %v",err)}
 accurate:=preparedMediaOutput(t);if err:=renderPreparedMedia(ctx,pins,plan,"accurate",accurate,16<<20);err!=nil{t.Fatal(err)}
 decodePreparedMedia(t,ctx,accurate.Name());profile,err:=inspectPreparedMediaProfile(ctx,accurate);if err!=nil{t.Fatal(err)}
 if len(profile.Streams)!=2 || profile.Streams[0].Pixel!="yuv420p" || profile.Streams[1].Codec!="aac" || profile.Streams[1].SampleRate!="16000" || profile.Streams[1].Channels!=1{t.Fatalf("export track/profile mismatch: %+v",profile)}
 if preparedMediaDigest(t,source)!=before{t.Fatal("export modified source")}
}

func TestPreparedAccurateAudioOffsetAndLimits(t *testing.T){
 requireFFmpeg(t);ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 source:=makePreparedMediaSource(t,ctx,true);before:=preparedMediaDigest(t,source)
 plan,pins:=preparedMediaTestPlan(t,ctx,source,1,0,2*time.Second)
 output:=preparedMediaOutput(t);if err:=renderPreparedMedia(ctx,pins,plan,"accurate",output,16<<20);err!=nil{t.Fatal(err)}
 raw,err:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-i",output.Name(),"-map","0:a:0","-ac","1","-ar","16000","-f","f32le","pipe:1").Output();if err!=nil{t.Fatal(err)}
 first:=-1
 for i:=0;i+4<=len(raw);i+=4{value:=math.Float32frombits(binary.LittleEndian.Uint32(raw[i:i+4]));if math.Abs(float64(value))>0.02{first=i/4;break}}
 if first<0 || math.Abs(float64(first)/16000-0.25)>0.04{t.Fatalf("audio lost its relative 250ms start offset: first sample %d",first)}
 tiny:=preparedMediaOutput(t);if err:=renderPreparedMedia(ctx,pins,plan,"accurate",tiny,16);!errors.Is(err,errExportJobSize){t.Fatalf("output size not bounded: %v",err)}
 stopped,stop:=context.WithCancel(ctx);stop();canceled:=preparedMediaOutput(t)
 if err:=renderPreparedMedia(stopped,pins,plan,"accurate",canceled,16<<20);!errors.Is(err,context.Canceled){t.Fatal(err)}
 data:=preparedMediaOutput(t);if _,err:=io.WriteString(data,"not empty");err!=nil{t.Fatal(err)}
 if err:=renderPreparedMedia(ctx,pins,plan,"accurate",data,16<<20);err==nil{t.Fatal("existing nonempty output overwritten")}
 if preparedMediaDigest(t,source)!=before{t.Fatal("source changed after failure/cancellation")}
}
