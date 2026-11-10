package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "sync"
 "time"
)

const trimRenderBudget=2*time.Minute
const maxTrimDuration=10*time.Minute
const maxTrimBytes int64=256<<20
var errTrimUnsupported=errors.New("unsupported trim input profile")
var errTrimLimit=errors.New("trim resource limit exceeded")
var errTrimRender=errors.New("trim rendering or validation failed")

type trimProfile struct {Width,Height int;Period time.Duration}
func trimRate(value string) (int64,int64,bool) {
 parts:=strings.Split(value,"/");if len(parts)!=2 {return 0,0,false}
 n,e1:=strconv.ParseInt(parts[0],10,64);d,e2:=strconv.ParseInt(parts[1],10,64)
 return n,d,e1==nil && e2==nil && n>0 && d>0 && n<=1000000 && d<=1000000 && n>=10*d && n<=60*d
}
func decodeTrimProfile(raw []byte) (trimProfile,error) {
 var data struct {Streams []struct {
  Type string `json:"codec_type"`
  Width int `json:"width"`
  Height int `json:"height"`
  Pixel string `json:"pix_fmt"`
  Rate string `json:"r_frame_rate"`
  Average string `json:"avg_frame_rate"`
  Tags struct {Rotate string `json:"rotate"`} `json:"tags"`
  Side []struct {Rotation float64 `json:"rotation"`} `json:"side_data_list"`
 } `json:"streams"`}
 if json.Unmarshal(raw,&data)!=nil {return trimProfile{},errRecordingMetadata}
 if len(data.Streams)!=1 {return trimProfile{},errTrimUnsupported}
 stream:=data.Streams[0]
 if stream.Type!="video" || stream.Pixel!="yuv420p" || stream.Width<2 || stream.Height<2 || stream.Width%2!=0 || stream.Height%2!=0 || stream.Width>4096 || stream.Height>4096 || int64(stream.Width)*int64(stream.Height)>3840*2160 {return trimProfile{},errTrimUnsupported}
 n,d,ok:=trimRate(stream.Rate);an,ad,aok:=trimRate(stream.Average)
 if !ok || !aok || n*ad!=an*d {return trimProfile{},errTrimUnsupported}
 if stream.Tags.Rotate!="" {rotation,err:=strconv.ParseFloat(stream.Tags.Rotate,64);if err!=nil || rotation!=0 {return trimProfile{},errTrimUnsupported}}
 for _,side:=range stream.Side {if side.Rotation!=0 {return trimProfile{},errTrimUnsupported}}
 return trimProfile{stream.Width,stream.Height,time.Duration((int64(time.Second)*d+n-1)/n)},nil
}
func inspectTrimProfile(parent context.Context,file *os.File) (trimProfile,error) {
 ctx,cancel:=context.WithTimeout(parent,5*time.Second);defer cancel()
 if err:=ctx.Err();err!=nil {return trimProfile{},err}
 select {case recordingDurations.slots<-struct{}{}:defer func(){<-recordingDurations.slots}();case <-ctx.Done():return trimProfile{},ctx.Err()}
 if err:=ctx.Err();err!=nil {return trimProfile{},err}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-show_entries","stream=codec_type,width,height,pix_fmt,r_frame_rate,avg_frame_rate:stream_tags=rotate:stream_side_data=rotation","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second
 var output limitedProbeOutput;cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil {if ctx.Err()!=nil {return trimProfile{},ctx.Err()};return trimProfile{},errRecordingMetadata}
 return decodeTrimProfile(output.Bytes())
}
func trimSeconds(duration time.Duration) string {return fmt.Sprintf("%d.%09d",duration/time.Second,duration%time.Second)}
type trimFileWriter struct {file *os.File;limit,written int64;exceeded bool}
func (writer *trimFileWriter) Write(data []byte) (int,error) {
 if int64(len(data))>writer.limit-writer.written {writer.exceeded=true;return 0,errTrimLimit}
 n,err:=writer.file.Write(data);writer.written+=int64(n);return n,err
}
type renderedTrim struct {file *os.File;duration time.Duration;once sync.Once;err error}
func (output *renderedTrim) Close() error {output.once.Do(func(){output.err=output.file.Close()});return output.err}

// Render completely before starting an HTTP video response. Retained frames
// have normalized decoded timestamps in each half-open [In,Out) window.
func renderTrimmedExport(parent context.Context,pins *pinnedExport,plan exportPlan,dir string,limit int64) (*renderedTrim,error) {
 canonical,err:=buildExportPlan(plan.clips(),timelineWindow{Start:plan.Start,End:plan.End})
 if err!=nil {return nil,err}
 if limit<1 || canonical.End.Sub(canonical.Start)>maxTrimDuration {return nil,errTrimLimit}
 if len(canonical.Parts)!=len(pins.files) {return nil,errExportPlanInvalid}
 ctx,cancel:=context.WithTimeout(parent,trimRenderBudget);defer cancel()
 if err:=pins.validate(ctx);err!=nil {return nil,err}
 args:=[]string{"-hide_banner","-loglevel","error","-filter_complex_threads","1"}
 var filters,labels []string
 var first trimProfile;var period time.Duration
 for index,part:=range canonical.Parts {
  if !sameMediaFile(part.Clip.identity,pins.identities[index]) {return nil,errRecordingChanged}
  profile,err:=inspectTrimProfile(ctx,pins.files[index]);if err!=nil {return nil,err}
  if index==0 {first=profile} else if profile.Width!=first.Width || profile.Height!=first.Height {return nil,errTrimUnsupported}
  if part.End.Sub(part.Start)<2*profile.Period {return nil,errTrimUnsupported}
  if profile.Period>period {period=profile.Period}
  args=append(args,"-threads","2","-noautorotate","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i",fmt.Sprintf("/proc/self/fd/%d",index+4))
  label:=fmt.Sprintf("v%d",index)
  filters=append(filters,fmt.Sprintf("[%d:v:0]setpts=PTS-STARTPTS,trim=start=%s:end=%s,setpts=PTS-STARTPTS[%s]",index,trimSeconds(part.In),trimSeconds(part.Out),label))
  labels=append(labels,"["+label+"]")
 }
 filters=append(filters,fmt.Sprintf("%sconcat=n=%d:v=1:a=0[outv]",strings.Join(labels,""),len(labels)))
 args=append(args,"-filter_complex",strings.Join(filters,";"),"-map","[outv]","-an","-sn","-dn","-c:v","libx264","-crf","0","-preset","veryfast","-pix_fmt","yuv420p","-threads","2","-fps_mode","vfr","-f","mp4","-movflags","frag_keyframe+empty_moov","pipe:1")
 file,err:=os.CreateTemp(dir,"simplenvr-trim-*.mp4");if err!=nil {return nil,errTrimRender}
 name:=file.Name()
 if err:=os.Remove(name);err!=nil {file.Close();os.Remove(name);return nil,errTrimRender}
 success:=false;defer func(){if !success {file.Close()}}()
 writer:=&trimFileWriter{file:file,limit:limit}
 cmd:=ffmpegCommand(ctx,args...);cmd.ExtraFiles=append([]*os.File{pins.manifest},pins.files...);cmd.Stdout=writer;cmd.Stderr=io.Discard
 runErr:=cmd.Run()
 if ctx.Err()!=nil {return nil,ctx.Err()}
 if writer.exceeded {return nil,errTrimLimit}
 if runErr!=nil {return nil,errTrimRender}
 if err:=pins.validate(ctx);err!=nil {return nil,err}
 verify,stop:=context.WithTimeout(ctx,5*time.Second)
 duration,err:=probeVideoDuration(verify,file);verifyErr:=verify.Err();stop()
 if verifyErr!=nil {return nil,verifyErr}
 if err!=nil {return nil,errTrimRender}
 difference:=duration-canonical.End.Sub(canonical.Start);if difference<0 {difference=-difference}
 if duration<=0 || difference>2*period {return nil,errTrimRender}
 if _,err:=file.Seek(0,io.SeekStart);err!=nil {return nil,errTrimRender}
 success=true;return &renderedTrim{file:file,duration:duration},nil
}
func (output *renderedTrim) setHeaders(w http.ResponseWriter,plan exportPlan) {
 w.Header().Set("X-Export-Range-Policy","decoded-video-frame-window")
 w.Header().Set("X-Export-Trim-Applied","true")
 w.Header().Set("X-Export-Accuracy","frame-granular-not-wall-clock-exact")
 w.Header().Set("X-Export-Audio-Policy","video-only-inputs")
 w.Header().Set("X-Export-Source-Start",plan.MediaStart.UTC().Format(time.RFC3339Nano))
 w.Header().Set("X-Export-Source-End",plan.MediaEnd.UTC().Format(time.RFC3339Nano))
 w.Header().Del("X-Export-Media-Start");w.Header().Del("X-Export-Media-End")
 w.Header().Set("X-Export-Output-Duration-Seconds",trimSeconds(output.duration))
}
func writeTrimError(w http.ResponseWriter,r *http.Request,err error) {
 if r.Context().Err()!=nil {return}
 status,kind,message:=500,"failed","Trim rendering or validation failed; no export was started"
 switch {
 case errors.Is(err,errTrimUnsupported):status,kind,message=422,"unsupported","Trim requires compatible video-only 8-bit yuv420p inputs with matching reported 10-60 fps rates, no rotation and at least two frames per selected part; audio-containing inputs are not silently stripped"
 case errors.Is(err,errTrimLimit):status,kind,message=413,"limit","Trim duration or output-size limit exceeded; reduce the requested range"
 case errors.Is(err,context.DeadlineExceeded):status,kind,message=504,"timeout","Trim preparation or rendering deadline exceeded; no export was started"
 case errors.Is(err,errRecordingMetadata),errors.Is(err,errRecordingChanged):status,kind,message=503,"metadata-unavailable","Trim input metadata unavailable or changed; no export was started"
 }
 w.Header().Set("X-Export-Trim-Status",kind);http.Error(w,message,status)
}
