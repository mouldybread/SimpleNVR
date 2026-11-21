package main

import (
 "context"
 "os/exec"
 "path/filepath"
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

func TestPreparedKeyframeCopyRefusesReorderedVideo(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 source:=makePreparedMediaSource(t,ctx,false)
 requested,pins:=preparedMediaTestPlan(t,ctx,source,1,500*time.Millisecond,1500*time.Millisecond)
 if _,_,err:=planKeyframeCopy(ctx,pins,requested);err!=errCopyReordered{t.Fatalf("reordered subsection copy not refused: %v",err)}
}
