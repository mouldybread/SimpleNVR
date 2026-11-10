package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)

func trimHashes(t *testing.T,ctx context.Context,file *os.File,filter string) []string {
 t.Helper();args:=[]string{"-nostdin","-v","error","-i","/proc/self/fd/3","-map","0:v:0"}
 if filter!="" {args=append(args,"-vf",filter)}
 args=append(args,"-an","-fps_mode","vfr","-f","framemd5","-")
 cmd:=exec.CommandContext(ctx,"ffmpeg",args...);cmd.ExtraFiles=[]*os.File{file}
 raw,err:=cmd.Output();if err!=nil {t.Fatal(err)}
 var hashes []string
 for _,line:=range strings.Split(string(raw),"\n") {if line!="" && !strings.HasPrefix(line,"#") {parts:=strings.Split(line,",");hashes=append(hashes,strings.TrimSpace(parts[len(parts)-1]))}}
 return hashes
}
func TestTrimProfilesRejectUnsupportedStreams(t *testing.T) {
 valid:=`{"streams":[{"codec_type":"video","width":64,"height":64,"pix_fmt":"yuv420p","r_frame_rate":"10/1","avg_frame_rate":"10/1"}]}`
 if _,err:=decodeTrimProfile([]byte(valid));err!=nil {t.Fatal(err)}
 cases:=[]string{strings.Replace(valid,"yuv420p","yuv420p10le",1),strings.Replace(valid,`"avg_frame_rate":"10/1"`,`"avg_frame_rate":"11/1"`,1),strings.Replace(valid,`"height":64`,`"height":65`,1),`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}]}`,strings.Replace(valid,`"pix_fmt"`,`"tags":{"rotate":"90"},"pix_fmt"`,1)}
 for _,raw:=range cases {if _,err:=decodeTrimProfile([]byte(raw));!errors.Is(err,errTrimUnsupported) {t.Fatal("unsupported profile accepted",raw,err)}}
}
func TestDecodedTrimMatchesSourceFramesAcrossClips(t *testing.T) {
 requireFFmpeg(t);dir:=t.TempDir();source:=filepath.Join(dir,"source.mp4")
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-t","4","-c:v","mpeg4","-g","100","-bf","2","-q:v","3","-an","-y",source)
 if raw,err:=cmd.CombinedOutput();err!=nil {t.Fatal(err,string(raw))}
 file,err:=os.Open(source);if err!=nil {t.Fatal(err)};defer file.Close();info,err:=file.Stat();if err!=nil {t.Fatal(err)}
 base:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 clips:=[]recordingInterval{{CameraID:"front",Path:source,Start:base,End:base.Add(4*time.Second),Size:info.Size(),identity:info},{CameraID:"front",Path:source,Start:base.Add(4*time.Second),End:base.Add(8*time.Second),Size:info.Size(),identity:info}}
 for _,tc:=range []struct{start,end time.Duration;frames int}{
  {1300*time.Millisecond,6200*time.Millisecond,49},
  {1250*time.Millisecond,2800*time.Millisecond,15},
 } {
  plan,err:=buildExportPlan(clips,timelineWindow{Start:base.Add(tc.start),End:base.Add(tc.end)});if err!=nil {t.Fatal(err)}
  pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir();pins,err:=pool.prepare(ctx,plan.clips());if err!=nil {t.Fatal(err)}
  output,err:=renderTrimmedExport(ctx,pins,plan,pool.tempDir,maxTrimBytes)
  if err!=nil {pins.Close();t.Fatal(err)}
  var expected []string
  for _,part:=range plan.Parts {filter:=fmt.Sprintf("setpts=PTS-STARTPTS,trim=start=%s:end=%s,setpts=PTS-STARTPTS",trimSeconds(part.In),trimSeconds(part.Out));expected=append(expected,trimHashes(t,ctx,file,filter)...)}
  actual:=trimHashes(t,ctx,output.file,"")
  if len(actual)!=tc.frames || !reflect.DeepEqual(actual,expected) {t.Fatal("trimmed decoded frames differ from source-window sequence",len(actual),len(expected))}
  if err:=output.Close();err!=nil {t.Fatal(err)};if err:=pins.Close();err!=nil {t.Fatal(err)}
  if entries,err:=os.ReadDir(pool.tempDir);err!=nil || len(entries)!=0 {t.Fatal("trim staging leaked",err)}
 }
}
func TestTrimLimitsCancellationAndHandlerPolicy(t *testing.T) {
 dir:=t.TempDir();clips:=syntheticExportClips(t,dir)
 for index,name:=range []string{"20261110_000000_a.mp4","20261110_000002_b.mp4"} {if err:=os.Rename(clips[index].Path,filepath.Join(dir,name));err!=nil {t.Fatal(err)}}
 base:=time.Date(2026,11,10,0,0,0,0,time.UTC);ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 cache:=newDurationCache(2,2,probeVideoDuration);camera:=timelineCamera{"front","Front",dir};window:=timelineWindow{Start:base.Add(500*time.Millisecond),End:base.Add(3500*time.Millisecond)}
 selected,err:=collectRecordingIntervals(ctx,[]timelineCamera{camera},window,cache);if err!=nil {t.Fatal(err)}
 plan,err:=buildExportPlan(selected,window);if err!=nil {t.Fatal(err)}
 pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir();pins,err:=pool.prepare(ctx,plan.clips());if err!=nil {t.Fatal(err)};defer pins.Close()
 if output,err:=renderTrimmedExport(ctx,pins,plan,pool.tempDir,1);!errors.Is(err,errTrimLimit) {if output!=nil {output.Close()};t.Fatal("output limit not enforced",err)}
 canceled,stop:=context.WithCancel(ctx);stop()
 if output,err:=renderTrimmedExport(canceled,pins,plan,pool.tempDir,maxTrimBytes);!errors.Is(err,context.Canceled) {if output!=nil {output.Close()};t.Fatal(err)}
 if entries,err:=os.ReadDir(pool.tempDir);err!=nil || len(entries)!=0 {t.Fatal("failed render leaked staging",err)}
 pins.Close()
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 request:=httptest.NewRequest("GET","/api/download?camera_id=front&start_time=2026-11-10T00:00:00.5Z&end_time=2026-11-10T00:00:03.5Z",nil).WithContext(ctx)
 response:=httptest.NewRecorder();serveDownload(response,request,n,cache,pool)
 if response.Code!=200 || response.Header().Get("X-Export-Range-Policy")!="decoded-video-frame-window" || response.Header().Get("X-Export-Trim-Applied")!="true" || response.Header().Get("X-Export-Media-End")!="" || response.Header().Get("X-Export-Audio-Policy")!="video-only-inputs" {t.Fatal(response.Code,response.Header(),response.Body.String())}
 if !bytes.Contains(response.Body.Bytes(),[]byte("ftyp")) || len(pool.slots)!=0 {t.Fatal("trim response or cleanup failed")}
 rejected:=httptest.NewRecorder();writeTrimError(rejected,request,errTrimUnsupported)
 if rejected.Code!=422 || strings.HasPrefix(rejected.Header().Get("Content-Type"),"video/") {t.Fatal("unsupported trim reported as video")}
}
