package main

import (
 "bytes"
 "context"
 "encoding/binary"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "os/exec"
 "sort"
 "strconv"
 "strings"
 "time"
)

// Subsection stream copy starts and ends only at IDR frames. An IDR frame
// resets the decoder, so H.264 with B-frames can be cut there without
// re-encoding. Other keyframes (open GOP, recovery points) are ignored. A full
// decode of the output before publication remains as a final safety check.
// The error text is unchanged so the existing HTTP mapping keeps working.
var errCopyReordered=errors.New("subsection copy cannot safely handle this video timestamp profile; explicitly choose accurate mode")

const copyProbeLimit=16<<20
const copyKeyPacketLimit=16<<20

// keys are the presentation times of IDR frames and ends are their decode times,
// both relative to origin. A cut that ends on an IDR frame must stop at its decode
// time, which is earlier than its presentation time when the stream has B-frames.
type copyFrameIndex struct {origin float64;keys []time.Duration;ends []time.Duration;audioAllowance time.Duration}

type copyProbeOutput struct{bytes.Buffer}
func (b *copyProbeOutput) Write(data []byte)(int,error){
 if b.Len()+len(data)>copyProbeLimit{return 0,errCopyReordered}
 return b.Buffer.Write(data)
}

// nalHasIDR reports whether an H.264 sample with 4-byte NAL length prefixes
// contains an IDR slice (NAL type 5). Malformed samples are never IDR.
func nalHasIDR(sample []byte) bool {
 idr:=false
 for i:=0;i<len(sample);{
  if len(sample)-i<4{return false}
  size:=int(binary.BigEndian.Uint32(sample[i:i+4]));i+=4
  if size<1 || size>len(sample)-i{return false}
  if sample[i]&0x1f==5{idr=true}
  i+=size
 }
 return idr
}

func copyPacketFields(line string) map[string]string {
 line=strings.TrimSpace(line)
 if line==""{return nil}
 fields:=make(map[string]string,5)
 for _,part:=range strings.Split(line,"|"){
  key,value,ok:=strings.Cut(part,"=")
  if ok{fields[key]=value}
 }
 return fields
}

func copyVideoOrigin(ctx context.Context,file *os.File)(float64,error){
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_entries","stream=codec_name,start_time","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second;var raw limitedProbeOutput;cmd.Stdout=&raw;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{if ctx.Err()!=nil{return 0,ctx.Err()};return 0,errCopyReordered}
 var data struct{Streams []struct{Codec string `json:"codec_name"`;Start string `json:"start_time"`} `json:"streams"`}
 if json.Unmarshal(raw.Bytes(),&data)!=nil || len(data.Streams)!=1{return 0,errCopyReordered}
 if data.Streams[0].Codec!="h264"{return 0,errPreparedMediaProfile}
 return preparedMediaSeconds(data.Streams[0].Start)
}

func inspectCopyFrames(parent context.Context,file *os.File)(copyFrameIndex,error){
 ctx,cancel:=context.WithTimeout(parent,15*time.Second);defer cancel()
 select{case recordingDurations.slots<-struct{}{}:defer func(){<-recordingDurations.slots}();case <-ctx.Done():return copyFrameIndex{},ctx.Err()}
 origin,err:=copyVideoOrigin(ctx,file);if err!=nil{return copyFrameIndex{},err}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_entries","packet=pts_time,dts_time,size,pos,flags","-of","compact=p=0")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second;var raw copyProbeOutput;cmd.Stdout=&raw;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{if ctx.Err()!=nil{return copyFrameIndex{},ctx.Err()};return copyFrameIndex{},errCopyReordered}
 index:=copyFrameIndex{origin:origin}
 for _,line:=range strings.Split(raw.String(),"\n"){
  fields:=copyPacketFields(line)
  if fields==nil || !strings.Contains(fields["flags"],"K"){continue}
  pts,err:=preparedMediaSeconds(fields["pts_time"]);if err!=nil{return index,errCopyReordered}
  dts,err:=preparedMediaSeconds(fields["dts_time"]);if err!=nil{return index,errCopyReordered}
  pos,posErr:=strconv.ParseInt(fields["pos"],10,64);size,sizeErr:=strconv.ParseInt(fields["size"],10,64)
  if posErr!=nil || sizeErr!=nil || pos<0 || size<5 || size>copyKeyPacketLimit{return index,errCopyReordered}
  sample:=make([]byte,size)
  if _,err:=file.ReadAt(sample,pos);err!=nil{return index,errCopyReordered}
  if !nalHasIDR(sample){continue}
  relative:=pts-origin
  if relative<0 || dts>pts{return index,errCopyReordered}
  value:=time.Duration(math.Round(relative*float64(time.Second)))
  if len(index.keys)>0 && value<=index.keys[len(index.keys)-1]{return index,errCopyReordered}
  index.keys=append(index.keys,value)
  index.ends=append(index.ends,time.Duration(math.Round((dts-origin)*float64(time.Second))))
 }
 if len(index.keys)==0{return index,errCopyReordered}
 return index,nil
}
func copyBoundary(keys []time.Duration,target,limit time.Duration,later bool)(time.Duration,error){
 if target<0 || target>limit{return 0,errExportPlanInvalid}
 if target==limit{return limit,nil}
 pos:=sort.Search(len(keys),func(i int)bool{return keys[i]>=target})
 if pos<len(keys) && keys[pos]==target{return target,nil}
 if later{if pos<len(keys) && keys[pos]<=limit{return keys[pos],nil};return limit,nil}
 if pos==0{return 0,errCopyReordered};return keys[pos-1],nil
}

// copyOutpoint is the media time where the concat demuxer stops reading a part.
// When the cut ends on an IDR frame, that is the frame's decode time, so the IDR
// frame and the frames after it are not included. Otherwise the end of the part.
func copyOutpoint(index copyFrameIndex,part exportPlanPart)float64{
 if part.Out<part.Clip.End.Sub(part.Clip.Start){
  pos:=sort.Search(len(index.keys),func(i int)bool{return index.keys[i]>=part.Out})
  if pos<len(index.keys) && index.keys[pos]==part.Out && pos<len(index.ends){return index.origin+index.ends[pos].Seconds()}
 }
 return index.origin+part.Out.Seconds()
}
func planKeyframeCopy(ctx context.Context,pins *pinnedExport,requested exportPlan)(exportPlan,[]copyFrameIndex,error){
 if err:=pins.validate(ctx);err!=nil{return exportPlan{},nil,err}
 indices:=make([]copyFrameIndex,len(requested.Parts));parts:=requested.clips()
 start,end:=requested.Start,requested.End
 for i,part:=range requested.Parts{
  index,err:=inspectCopyFrames(ctx,pins.files[i]);if err!=nil{return exportPlan{},nil,err}
  profile,err:=inspectPreparedMediaProfile(ctx,pins.files[i]);if err!=nil{return exportPlan{},nil,err}
  if len(profile.Streams)==2{rate,err:=preparedMediaSeconds(profile.Streams[1].SampleRate);if err!=nil || rate<=0{return exportPlan{},nil,errPreparedMediaProfile};index.audioAllowance=time.Duration(math.Ceil(1024/rate*float64(time.Second)))}
  duration:=part.Clip.End.Sub(part.Clip.Start)
  in,err:=copyBoundary(index.keys,part.In,duration,false);if err!=nil{return exportPlan{},nil,err}
  out,err:=copyBoundary(index.keys,part.Out,duration,true);if err!=nil{return exportPlan{},nil,err}
  if i==0{start=part.Clip.Start.Add(in)}else if in!=0{return exportPlan{},nil,errCopyReordered}
  if i==len(indices)-1{end=part.Clip.Start.Add(out)}else if out!=duration{return exportPlan{},nil,errCopyReordered}
  indices[i]=index
 }
 plan,err:=buildExportPlan(parts,timelineWindow{Start:start,End:end});if err!=nil{return plan,nil,err}
 return plan,indices,pins.validate(ctx)
}
func renderKeyframeCopy(ctx context.Context,pins *pinnedExport,plan exportPlan,indices []copyFrameIndex,output *os.File,limit int64)error{
 if len(indices)!=len(plan.Parts) || len(pins.files)!=len(indices){return errExportPlanInvalid}
 if err:=pins.validate(ctx);err!=nil{return err}
 info,err:=output.Stat();if err!=nil || info.Size()!=0 || !info.Mode().IsRegular(){return errExportPlanInvalid}
 profiles:=make([]preparedMediaProfile,len(indices))
 for i:=range profiles{profiles[i],err=inspectPreparedMediaProfile(ctx,pins.files[i]);if err!=nil{return err};if i>0 && !compatiblePreparedCopy(profiles[0],profiles[i]){return errPreparedMediaProfile}}
 manifest,err:=os.CreateTemp("","simplenvr-keycopy-*.ffconcat");if err!=nil{return err};defer manifest.Close();defer os.Remove(manifest.Name())
 if err:=os.Remove(manifest.Name());err!=nil{return err}
 if _,err:=io.WriteString(manifest,"ffconcat version 1.0\n");err!=nil{return err}
 for i,part:=range plan.Parts{if _,err:=fmt.Fprintf(manifest,"file '/proc/self/fd/%d'\ninpoint %.9f\noutpoint %.9f\nduration %s\n",i+4,indices[i].origin+part.In.Seconds(),copyOutpoint(indices[i],part),trimSeconds(part.End.Sub(part.Start)));err!=nil{return err}}
 if _,err:=manifest.Seek(0,io.SeekStart);err!=nil{return err}
 args:=[]string{"-hide_banner","-loglevel","error","-nostdin","-protocol_whitelist","file,pipe","-format_whitelist","concat,mov","-f","concat","-safe","0","-auto_convert","0","-i","/proc/self/fd/3","-map","0:v:0"}
 if len(profiles[0].Streams)==2{args=append(args,"-map","0:a:0")}
 args=append(args,"-c","copy","-t",trimSeconds(plan.End.Sub(plan.Start)),"-avoid_negative_ts","disabled","-use_editlist","1","-f","mp4","-movflags","frag_keyframe+delay_moov","pipe:1")
 writer:=&boundedExportJobWriter{ctx:ctx,file:output,limit:limit}
 cmd:=ffmpegCommand(ctx,args...);cmd.ExtraFiles=append([]*os.File{manifest},pins.files...);cmd.Stdout=writer;cmd.Stderr=io.Discard
 err=cmd.Run();if writer.exceeded{return errExportJobSize};if ctx.Err()!=nil{return ctx.Err()};if err!=nil{return err}
 produced,err:=inspectPreparedMediaProfile(ctx,output);if err!=nil{return err};if len(produced.Streams)!=len(profiles[0].Streams){return errPreparedMediaProfile}
 duration,err:=probeVideoDuration(ctx,output);if err!=nil{return err};delta:=duration-plan.End.Sub(plan.Start);if delta<0{delta=-delta};if delta>100*time.Millisecond{return errCopyReordered}
 // Decode the candidate without changing it. No undecodable random-access
 // start is advertised as a ready download. Time/resource limits still apply.
 validate:=ffmpegCommand(ctx,"-hide_banner","-loglevel","error","-nostdin","-xerror","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i","/proc/self/fd/3","-map","0:v:0","-map","0:a:0?","-f","null","-")
 validate.ExtraFiles=[]*os.File{output};validate.Stdout=io.Discard;validate.Stderr=io.Discard
 if err:=validate.Run();err!=nil{if ctx.Err()!=nil{return ctx.Err()};return errCopyReordered}
 return pins.validate(ctx)
}
