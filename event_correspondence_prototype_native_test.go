package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

// Native descriptor/concat fixtures; not live-camera or runtime acceptance.
func TestCorrPrototypeNativeOrderedContent(t *testing.T) {
 binary,err:=exec.LookPath("ffmpeg");if err!=nil {t.Skip("native fixture requires stock ffmpeg")}
 dir:=t.TempDir()
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel()
 media:=func(args ...string){
  t.Helper()
  cmd:=exec.CommandContext(ctx,binary,append([]string{"-hide_banner","-loglevel","error","-nostdin"},args...)...)
  cmd.WaitDelay=2*time.Second
  if output,err:=cmd.CombinedOutput();err!=nil || len(output)!=0 {t.Fatalf("native fixture: %v %s",err,output)}
 }
 sources:=[]string{}
 for i,seconds:=range []int{1,1,2} {
  path:=filepath.Join(dir,fmt.Sprintf("source-%d.mp4",i));sources=append(sources,path)
  media("-f","lavfi","-i",fmt.Sprintf("testsrc2=size=128x72:rate=10:duration=%d",seconds),"-f","lavfi","-i",fmt.Sprintf("sine=frequency=%d:sample_rate=48000",400+i*100),"-t",fmt.Sprint(seconds),"-vf",fmt.Sprintf("hue=h=%d",i*40),"-c:v","libx264","-preset","veryfast","-threads","1","-g","10","-bf","2","-pix_fmt","yuv420p","-c:a","aac","-b:a","64k","-shortest","-movflags","+faststart",path)
 }
 short:=filepath.Join(dir,"short.mp4")
 media("-i",sources[2],"-t","1","-c","copy",short)
 plans:=map[string][]string{
  "normal":sources,
  "omitted first":{sources[1],sources[2]},
  "omitted second":{sources[0],sources[2]},
  "capture only":{sources[2]},
  "duplicated":{sources[0],sources[0],sources[2]},
  "reordered":{sources[1],sources[0],sources[2]},
  "truncated":{sources[0],sources[1],short},
 }
 outputs:=map[string]string{}
 for name,inputs:=range plans {
  plan:=filepath.Join(dir,name+".txt");text:=""
  for _,input:=range inputs {text+=fmt.Sprintf("file '%s'\n",input)}
  if err:=os.WriteFile(plan,[]byte(text),0600);err!=nil {t.Fatal(err)}
  output:=filepath.Join(dir,name+".mp4");outputs[name]=output
  media("-f","concat","-safe","0","-i",plan,"-c","copy",output)
 }
 shifted:=filepath.Join(dir,"shifted.mp4")
 media("-i",outputs["normal"],"-c","copy","-output_ts_offset","5",shifted)
 outputs["timestamp shift"]=shifted
 for _,kind:=range []string{"video only","audio only"} {
  video,audio:=outputs["normal"],outputs["reordered"]
  if kind=="video only" {video,audio=audio,video}
  output:=filepath.Join(dir,kind+".mp4");outputs[kind]=output
  media("-i",video,"-i",audio,"-map","0:v:0","-map","1:a:0","-c","copy",output)
 }
 refs:=[]*os.File{}
 for _,source:=range sources {
  file,err:=os.Open(source);if err!=nil {t.Fatal(err)};defer file.Close();refs=append(refs,file)
 }
 for name,output:=range outputs {t.Run(name,func(t *testing.T){
  actual,err:=os.Open(output);if err!=nil {t.Fatal(err)};defer actual.Close()
  report:=corrPrototypeReport{}
  for _,audio:=range []bool{false,true} {
   a,err:=os.CreateTemp(t.TempDir(),"reference");if err!=nil {t.Fatal(err)};defer a.Close()
   b,err:=os.CreateTemp(t.TempDir(),"output");if err!=nil {t.Fatal(err)};defer b.Close()
   if err:=corrPrototypeNormalize(ctx,refs,true,audio,a,&report);err!=nil {t.Fatal("descriptor concat",err)}
   if err:=corrPrototypeNormalize(ctx,[]*os.File{actual},false,audio,b,&report);err!=nil {t.Fatal("actual",err)}
   equal,count,err:=corrPrototypeEqual(ctx,a,b);report.ComparedBytes+=count
   want:=name=="normal" || name=="timestamp shift" || (audio && name=="video only") || (!audio && name=="audio only")
   if err!=nil || equal!=want {t.Fatal("content",audio,equal,want,err)}
  }
  if report.ProcessesStarted!=4 {t.Fatal(report)}
  streamed,streamErr:=corrStreamRun(ctx,refs,actual,nil,nil,nil)
  corrStreamAssertJoined(t,streamed)
  if streamed.ChildrenStarted!=2 || streamed.PipeDescriptorsCreated!=8 {t.Fatal("streaming resource contract",streamed)}
  wantMatch:=name=="normal" || name=="timestamp shift"
  if wantMatch {
   if streamErr!=nil || !streamed.Video.Equal || !streamed.Audio.Equal {t.Fatal("streaming match",streamed,streamErr)}
  } else if !errors.Is(streamErr,errCorrStreamMismatch) {t.Fatal("streaming fault not detected",name,streamed,streamErr)}
  t.Logf("%s: processes=%d normalized_bytes=%d compared_bytes=%d streaming_children=%d streaming_seconds=%.3f",name,report.ProcessesStarted,report.NormalizedBytes,report.ComparedBytes,streamed.ChildrenStarted,streamed.Elapsed.Seconds())
 })}
}
