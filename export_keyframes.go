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
 "sort"
 "time"
)

// Subsection stream copy is deliberately limited to non-reordered video. Open
// or undecodable GOPs are rejected by full output decode before publication.
var errCopyReordered=errors.New("subsection copy cannot safely handle this video timestamp profile; explicitly choose accurate mode")
type copyFrameIndex struct {origin float64;keys []time.Duration;audioAllowance time.Duration}
func inspectCopyFrames(parent context.Context,file *os.File)(copyFrameIndex,error){
 ctx,cancel:=context.WithTimeout(parent,10*time.Second);defer cancel()
 select{case recordingDurations.slots<-struct{}{}:defer func(){<-recordingDurations.slots}();case <-ctx.Done():return copyFrameIndex{},ctx.Err()}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-format_whitelist","mov","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-skip_frame","nokey","-show_frames","-show_streams","-show_entries","stream=start_time,has_b_frames:frame=best_effort_timestamp_time","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second;var raw limitedProbeOutput;cmd.Stdout=&raw;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{if ctx.Err()!=nil{return copyFrameIndex{},ctx.Err()};return copyFrameIndex{},errCopyReordered}
 var data struct{Streams []struct{Start string `json:"start_time"`;Reordered *int `json:"has_b_frames"`} `json:"streams"`;Frames []struct{PTS string `json:"best_effort_timestamp_time"`} `json:"frames"`}
 if json.Unmarshal(raw.Bytes(),&data)!=nil || len(data.Streams)!=1 || (data.Streams[0].Reordered==nil || *data.Streams[0].Reordered!=0) || len(data.Frames)==0{return copyFrameIndex{},errCopyReordered}
 origin,err:=preparedMediaSeconds(data.Streams[0].Start);if err!=nil{return copyFrameIndex{},err}
 index:=copyFrameIndex{origin:origin}
 for _,frame:=range data.Frames{pts,err:=preparedMediaSeconds(frame.PTS);if err!=nil{return index,err};relative:=pts-origin;if relative<0{return index,errCopyReordered};value:=time.Duration(math.Round(relative*float64(time.Second)));if len(index.keys)>0 && value<=index.keys[len(index.keys)-1]{return index,errCopyReordered};index.keys=append(index.keys,value)}
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
 for i,part:=range plan.Parts{if _,err:=fmt.Fprintf(manifest,"file '/proc/self/fd/%d'\ninpoint %.9f\noutpoint %.9f\nduration %s\n",i+4,indices[i].origin+part.In.Seconds(),indices[i].origin+part.Out.Seconds(),trimSeconds(part.End.Sub(part.Start)));err!=nil{return err}}
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
