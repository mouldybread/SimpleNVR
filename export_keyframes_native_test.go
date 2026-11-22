package main

import (
 "context"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "testing"
 "time"
)

func TestPreparedKeyframeCopyNativeRoundTrip(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),40*time.Second);defer cancel()
 source:=filepath.Join(t.TempDir(),"source.mp4")
 args:=[]string{"-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=128x96:rate=20","-f","lavfi","-i","sine=frequency=440:sample_rate=16000","-t","2","-c:v","libx264","-threads","1","-bf","0","-g","20","-sc_threshold","0","-pix_fmt","yuv420p","-c:a","aac","-y",source}
 if raw,err:=exec.CommandContext(ctx,"ffmpeg",args...).CombinedOutput();err!=nil{t.Fatalf("fixture: %v %s",err,raw)}
 before:=preparedMediaDigest(t,source)
 requested,pins:=preparedMediaTestPlan(t,ctx,source,1,500*time.Millisecond,1500*time.Millisecond)
 actual,indices,err:=planKeyframeCopy(ctx,pins,requested);if err!=nil{t.Fatal(err)}
 if !actual.Start.Equal(requested.MediaStart) || !actual.End.Equal(requested.MediaEnd){t.Fatalf("unexpected keyframe interval: %+v",actual)}
 output:=preparedMediaOutput(t)
 if err:=renderKeyframeCopy(ctx,pins,actual,indices,output,16<<20);err!=nil{t.Fatal(err)}
 decodePreparedMedia(t,ctx,output.Name())
 duration,err:=probeVideoDuration(ctx,output);if err!=nil{t.Fatal(err)}
 if delta:=duration-2*time.Second;delta < -100*time.Millisecond || delta > 100*time.Millisecond{t.Fatal("wrong duration",duration)}
 if preparedMediaDigest(t,source)!=before{t.Fatal("source changed")}
}

// makePreparedBFrameSource makes 6 seconds of 20 fps H.264 with B-frames and an
// IDR frame candidate every second. With open GOP, only the first keyframe is IDR.
func makePreparedBFrameSource(t *testing.T,ctx context.Context,openGOP bool)string{
 t.Helper();file:=filepath.Join(t.TempDir(),"source.mp4")
 gop:="open-gop=0";if openGOP{gop="open-gop=1"}
 args:=[]string{"-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=128x96:rate=20","-f","lavfi","-i","sine=frequency=440:sample_rate=16000","-t","6","-c:v","libx264","-threads","1","-bf","3","-g","20","-sc_threshold","0","-x264-params",gop,"-pix_fmt","yuv420p","-c:a","aac","-y",file}
 if raw,err:=exec.CommandContext(ctx,"ffmpeg",args...).CombinedOutput();err!=nil{t.Fatalf("fixture generation: %v %s",err,raw)}
 return file
}
func countPreparedVideoFrames(t *testing.T,ctx context.Context,path string)int{
 t.Helper()
 raw,err:=exec.CommandContext(ctx,"ffprobe","-v","error","-count_frames","-select_streams","v:0","-show_entries","stream=nb_read_frames","-of","csv=p=0",path).Output();if err!=nil{t.Fatal(err)}
 count,err:=strconv.Atoi(strings.TrimSpace(string(raw)));if err!=nil{t.Fatal(err)}
 return count
}

func TestPreparedKeyframeCopyBFramesCutsAtIDR(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 source:=makePreparedBFrameSource(t,ctx,false);before:=preparedMediaDigest(t,source)
 requested,pins:=preparedMediaTestPlan(t,ctx,source,1,1500*time.Millisecond,3500*time.Millisecond)
 actual,indices,err:=planKeyframeCopy(ctx,pins,requested);if err!=nil{t.Fatalf("B-frame subsection copy refused: %v",err)}
 base:=requested.MediaStart
 if !actual.Start.Equal(base.Add(time.Second)) || !actual.End.Equal(base.Add(4*time.Second)){t.Fatalf("not extended to IDR frames: %+v",actual)}
 output:=preparedMediaOutput(t)
 if err:=renderKeyframeCopy(ctx,pins,actual,indices,output,16<<20);err!=nil{t.Fatal(err)}
 decodePreparedMedia(t,ctx,output.Name())
 duration,err:=probeVideoDuration(ctx,output);if err!=nil{t.Fatal(err)}
 if delta:=duration-3*time.Second;delta < -100*time.Millisecond || delta > 100*time.Millisecond{t.Fatal("wrong duration",duration)}
 if frames:=countPreparedVideoFrames(t,ctx,output.Name());frames!=60{t.Fatalf("expected 60 frames for 3 seconds at 20 fps, got %d",frames)}
 if preparedMediaDigest(t,source)!=before{t.Fatal("source changed")}
}

func TestPreparedKeyframeCopyIgnoresNonIDRKeyframes(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 source:=makePreparedBFrameSource(t,ctx,true)
 requested,pins:=preparedMediaTestPlan(t,ctx,source,1,1500*time.Millisecond,3500*time.Millisecond)
 actual,_,err:=planKeyframeCopy(ctx,pins,requested);if err!=nil{t.Fatal(err)}
 if !actual.Start.Equal(requested.MediaStart) || !actual.End.Equal(requested.MediaEnd){t.Fatalf("open-GOP keyframe used as a cut point: %+v",actual)}
}
