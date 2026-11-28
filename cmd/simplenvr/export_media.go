package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "os/exec"
 "reflect"
 "strconv"
 "strings"
 "time"
)

var errPreparedMediaProfile=errors.New("unsupported or incompatible export tracks")
var errPreparedCopyBoundary=errors.New("stream copy requires complete recording boundaries; use accurate mode or explicitly choose a different interval")

type preparedMediaStream struct {
 Type string `json:"codec_type"`
 Codec string `json:"codec_name"`
 Tag string `json:"codec_tag_string"`
 Profile string `json:"profile"`
 Width int `json:"width"`
 Height int `json:"height"`
 Pixel string `json:"pix_fmt"`
 Range string `json:"color_range"`
 SampleRate string `json:"sample_rate"`
 Channels int `json:"channels"`
 Layout string `json:"channel_layout"`
 TimeBase string `json:"time_base"`
 ExtraHash string `json:"extradata_hash"`
 Start string `json:"start_time"`
 Duration string `json:"duration"`
 Tags struct {Rotate string `json:"rotate"`} `json:"tags"`
 Side []struct {Rotation float64 `json:"rotation"`} `json:"side_data_list"`
}
type preparedMediaProfile struct {Streams []preparedMediaStream `json:"streams"`}
func preparedMediaSeconds(value string)(float64,error){
 n,err:=strconv.ParseFloat(value,64)
 if err!=nil || math.IsNaN(n) || math.IsInf(n,0) || math.Abs(n)>86400*366{return 0,errPreparedMediaProfile}
 return n,nil
}
func decodePreparedMediaProfile(raw []byte)(preparedMediaProfile,error){
 var profile preparedMediaProfile
 if len(raw)>65536 || json.Unmarshal(raw,&profile)!=nil{return profile,errPreparedMediaProfile}
 if len(profile.Streams)<1 || len(profile.Streams)>2{return profile,errPreparedMediaProfile}
 video:=profile.Streams[0]
 if video.Type!="video" || video.Width<2 || video.Height<2 || video.Width%2!=0 || video.Height%2!=0 || video.Width>4096 || video.Height>4096 || int64(video.Width)*int64(video.Height)>3840*2160{return profile,errPreparedMediaProfile}
 if video.Pixel!="yuv420p" && video.Pixel!="yuvj420p"{return profile,errPreparedMediaProfile}
 if video.Range!="" && video.Range!="unknown" && video.Range!="tv" && video.Range!="pc"{return profile,errPreparedMediaProfile}
 for _,stream:=range profile.Streams{
  if stream.Codec==""{return profile,errPreparedMediaProfile}
  if stream.Tags.Rotate!=""{angle,err:=preparedMediaSeconds(stream.Tags.Rotate);if err!=nil || angle!=0{return profile,errPreparedMediaProfile}}
  for _,side:=range stream.Side{if side.Rotation!=0{return profile,errPreparedMediaProfile}}
 }
 if len(profile.Streams)==2{
  audio:=profile.Streams[1];rate,err:=strconv.Atoi(audio.SampleRate)
  if audio.Type!="audio" || audio.Codec!="aac" || err!=nil || rate<8000 || rate>96000 || audio.Channels<1 || audio.Channels>8{return profile,errPreparedMediaProfile}
 }
 return profile,nil
}
func inspectPreparedMediaProfile(parent context.Context,file *os.File)(preparedMediaProfile,error){
 ctx,cancel:=context.WithTimeout(parent,10*time.Second);defer cancel()
 if err:=ctx.Err();err!=nil{return preparedMediaProfile{},err}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i","/proc/self/fd/3","-show_data_hash","sha256","-show_entries","stream=codec_type,codec_name,codec_tag_string,profile,width,height,pix_fmt,color_range,sample_rate,channels,channel_layout,time_base,extradata_hash,start_time,duration:stream_tags=rotate:stream_side_data=rotation","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second
 var output limitedProbeOutput;cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{if ctx.Err()!=nil{return preparedMediaProfile{},ctx.Err()};return preparedMediaProfile{},errPreparedMediaProfile}
 return decodePreparedMediaProfile(output.Bytes())
}

// Copy compatibility compares the actual codec configuration, not nominal or
// average frame-rate strings. Differing track counts/configurations are refused
// instead of selecting one audio track or falling back to a silent conversion.
func compatiblePreparedCopy(a,b preparedMediaProfile)bool{
 if len(a.Streams)!=len(b.Streams){return false}
 for i,left:=range a.Streams{
  right:=b.Streams[i]
  left.Start="";left.Duration="";left.Tags.Rotate="";left.Side=nil
  right.Start="";right.Duration="";right.Tags.Rotate="";right.Side=nil
  if left.ExtraHash=="" || right.ExtraHash=="" || !reflect.DeepEqual(left,right){return false}
 }
 return true
}

func preparedAccurateArguments(profiles []preparedMediaProfile,plan exportPlan)([]string,error){
 if len(profiles)!=len(plan.Parts) || len(profiles)==0{return nil,errExportPlanInvalid}
 args:=[]string{"-hide_banner","-loglevel","error","-nostdin","-copyts","-filter_complex_threads","1"}
 audio:=len(profiles[0].Streams)==2
 first:=profiles[0].Streams[0]
 var filters,labels []string
 for index,part:=range plan.Parts{
  profile:=profiles[index];video:=profile.Streams[0]
  if video.Width!=first.Width || video.Height!=first.Height || (len(profile.Streams)==2)!=audio{return nil,errPreparedMediaProfile}
  if audio && (profile.Streams[1].SampleRate!=profiles[0].Streams[1].SampleRate || profile.Streams[1].Channels!=profiles[0].Streams[1].Channels || profile.Streams[1].Layout!=profiles[0].Streams[1].Layout){return nil,errPreparedMediaProfile}
  origin,err:=preparedMediaSeconds(video.Start);if err!=nil{return nil,err}
  if audio{if _,err:=preparedMediaSeconds(profile.Streams[1].Start);err!=nil{return nil,err}}
  args=append(args,"-threads","2","-noautorotate","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i",fmt.Sprintf("/proc/self/fd/%d",index+4))
  span:=part.End.Sub(part.Start)
  if span<=0{return nil,errExportPlanInvalid}
  rangeFilter:="format=yuv420p"
  if video.Pixel=="yuvj420p" || video.Range=="pc"{rangeFilter="scale=in_range=pc:out_range=tv,format=yuv420p"}
  // Both tracks use the VIDEO origin and the same requested cut origin. Do not
  // independently reset audio/video to their first retained packet/frame.
  filters=append(filters,fmt.Sprintf("[%d:v:0]setpts=PTS-(%.9f)/TB,trim=start=%s:end=%s,setpts=PTS-(%s)/TB,%s[v%d]",index,origin,trimSeconds(part.In),trimSeconds(part.Out),trimSeconds(part.In),rangeFilter,index))
  labels=append(labels,fmt.Sprintf("[v%d]",index))
  if audio{
   rate:=profile.Streams[1].SampleRate
   filters=append(filters,fmt.Sprintf("[%d:a:0]asetpts=PTS-(%.9f)/TB,atrim=start=%s:end=%s,asetpts=PTS-(%s)/TB,aresample=%s:async=0:first_pts=0,apad=whole_dur=%s,atrim=duration=%s[a%d]",index,origin,trimSeconds(part.In),trimSeconds(part.Out),trimSeconds(part.In),rate,trimSeconds(span),trimSeconds(span),index))
   labels=append(labels,fmt.Sprintf("[a%d]",index))
  }
 }
 if audio{filters=append(filters,fmt.Sprintf("%sconcat=n=%d:v=1:a=1[outv][outa]",strings.Join(labels,""),len(plan.Parts)))}else{filters=append(filters,fmt.Sprintf("%sconcat=n=%d:v=1:a=0[outv]",strings.Join(labels,""),len(plan.Parts)))}
 args=append(args,"-filter_complex",strings.Join(filters,";"),"-map","[outv]")
 if audio{args=append(args,"-map","[outa]","-c:a","aac","-b:a","128k")}else{args=append(args,"-an")}
 args=append(args,"-sn","-dn","-c:v","libx264","-bf","0","-crf","18","-preset","veryfast","-pix_fmt","yuv420p","-color_range","tv","-threads","2","-fps_mode","vfr","-avoid_negative_ts","disabled","-use_editlist","1","-f","mp4","-movflags","frag_keyframe+delay_moov","pipe:1")
 return args,nil
}

// Backend rendering primitive, not yet selected by the HTTP/UI planner. mode
// is exactly "copy" or "accurate"; there is no automatic conversion fallback.
// Output must be a new empty file, never an existing recording. The caller owns
// its lifetime and must not expose it until this function validates success.
func renderPreparedMedia(ctx context.Context,pins *pinnedExport,plan exportPlan,mode string,output *os.File,limit int64)error{
 if err:=ctx.Err();err!=nil{return err}
 if mode!="copy" && mode!="accurate"{return errExportPlanInvalid}
 canonical,err:=buildExportPlan(plan.clips(),timelineWindow{Start:plan.Start,End:plan.End});if err!=nil{return err}
 if pins==nil || output==nil || len(pins.files)!=len(canonical.Parts) || len(pins.identities)!=len(pins.files) || limit<1{return errExportPlanInvalid}
 info,err:=output.Stat();if err!=nil || !info.Mode().IsRegular() || info.Size()!=0{return errors.New("export output must be a new empty regular file")}
 if _,err:=output.Seek(0,io.SeekStart);err!=nil{return err}
 if mode=="copy" && canonical.NeedsTrim{return errPreparedCopyBoundary}
 if err:=pins.validate(ctx);err!=nil{return err}
 profiles:=make([]preparedMediaProfile,len(pins.files))
 for i,file:=range pins.files{
  if !sameMediaFile(canonical.Parts[i].Clip.identity,pins.identities[i]){return errRecordingChanged}
  profiles[i],err=inspectPreparedMediaProfile(ctx,file);if err!=nil{return err}
 }
 writer:=&boundedExportJobWriter{ctx:ctx,file:output,limit:limit}
 if mode=="copy" && len(pins.files)==1{
  _,err=io.Copy(writer,io.NewSectionReader(pins.files[0],0,pins.identities[0].Size()))
 }else{
  var args []string
  if mode=="copy"{
   for i:=1;i<len(profiles);i++{if !compatiblePreparedCopy(profiles[0],profiles[i]){return errPreparedMediaProfile}}
   if _,err:=pins.manifest.Seek(0,io.SeekStart);err!=nil{return err}
   args=[]string{"-hide_banner","-loglevel","error","-nostdin","-protocol_whitelist","file,pipe","-format_whitelist","concat,mov","-f","concat","-safe","0","-auto_convert","0","-i","/proc/self/fd/3","-map","0:v:0"}
   if len(profiles[0].Streams)==2{args=append(args,"-map","0:a:0")}
   args=append(args,"-c","copy","-f","mp4","-movflags","frag_keyframe+empty_moov","pipe:1")
  }else{args,err=preparedAccurateArguments(profiles,canonical);if err!=nil{return err}}
  cmd:=ffmpegCommand(ctx,args...);cmd.ExtraFiles=append([]*os.File{pins.manifest},pins.files...);cmd.Stdout=writer;cmd.Stderr=io.Discard
  err=cmd.Run()
 }
 if writer.exceeded{return errExportJobSize}
 if ctx.Err()!=nil{return ctx.Err()}
 if err!=nil{return err}
 if err:=pins.validate(ctx);err!=nil{return err}
 produced,err:=inspectPreparedMediaProfile(ctx,output);if err!=nil{return err}
 if len(produced.Streams)!=len(profiles[0].Streams){return errors.New("export lost or added a track")}
 if len(produced.Streams)==2{
  audio:=produced.Streams[1]
  if audio.SampleRate!=profiles[0].Streams[1].SampleRate || audio.Channels!=profiles[0].Streams[1].Channels{return errors.New("export audio configuration changed")}
 }
 duration,err:=probeVideoDuration(ctx,output);if err!=nil{return err}
 difference:=duration-canonical.End.Sub(canonical.Start);if difference<0{difference=-difference}
 if duration<=0 || difference>250*time.Millisecond{return errors.New("export video duration differs from selected interval")}
 if len(produced.Streams)==2{
  seconds,err:=preparedMediaSeconds(produced.Streams[1].Duration);if err!=nil{return err}
  if math.Abs(seconds-canonical.End.Sub(canonical.Start).Seconds())>0.25{return errors.New("export audio duration differs from selected interval")}
 }
 if _,err:=output.Seek(0,io.SeekStart);err!=nil{return err}
 return ctx.Err()
}
