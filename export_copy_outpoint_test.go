package main

import (
 "context"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

func TestCopyDecodeTimeOutpoint150Frames(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 source:=filepath.Join(t.TempDir(),"source.mp4")
 args:=[]string{"-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=320x240:rate=25","-t","20","-c:v","libx264","-threads","1","-pix_fmt","yuv420p","-preset","veryfast","-g","50","-bf","3","-x264-params","open-gop=0:scenecut=0","-y",source}
 if raw,err:=exec.CommandContext(ctx,"ffmpeg",args...).CombinedOutput();err!=nil { t.Fatalf("fixture: %v %s",err,raw) }
 before:=preparedMediaDigest(t,source)
 requested,pins:=preparedMediaTestPlan(t,ctx,source,1,4500*time.Millisecond,9500*time.Millisecond)
 actual,indices,err:=planKeyframeCopy(ctx,pins,requested)
 if err!=nil { t.Fatal(err) }
 if len(indices)!=1 || len(actual.Parts)!=1 || !actual.Start.Equal(requested.MediaStart.Add(4*time.Second)) || !actual.End.Equal(requested.MediaStart.Add(10*time.Second)) { t.Fatal("expected IDR-aligned 4-to-10-second range") }
 index:=indices[0]
 if got:=copyOutpoint(index,actual.Parts[0]);got>=index.origin+10 { t.Fatal("outpoint did not use an earlier decode timestamp") }
 output:=preparedMediaOutput(t)
 if err:=renderKeyframeCopy(ctx,pins,actual,indices,output,16<<20);err!=nil { t.Fatal(err) }
 if frames:=countPreparedVideoFrames(t,ctx,output.Name());frames!=150 { t.Fatalf("decode-time outpoint: got %d frames, want 150 (old presentation-time cut could include 152)",frames) }
 duration,err:=probeVideoDuration(ctx,output)
 if err!=nil { t.Fatal(err) }
 if delta:=duration-6*time.Second;delta < -time.Millisecond || delta > time.Millisecond { t.Fatalf("duration %v, want 6s",duration) }
 if raw,err:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-xerror","-i",output.Name(),"-map","0:v:0","-f","null","-").CombinedOutput();err!=nil { t.Fatalf("decode: %v %s",err,raw) }
 if preparedMediaDigest(t,source)!=before { t.Fatal("source changed") }
}
