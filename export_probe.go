package main

import (
 "bytes"
 "context"
 "encoding/json"
 "io"
 "math"
 "os"
 "os/exec"
 "strconv"
 "time"
)

// These helpers remain required by export profile/keyframe inspection and output
// validation. Name-based recording selection does not call them. Keep their
// bounded output, descriptor input and cancellation behavior when removing the
// obsolete recording duration cache.
type limitedProbeOutput struct {bytes.Buffer}
func (b *limitedProbeOutput) Write(data []byte) (int,error) {
 if b.Len()+len(data)>65536 {return 0,errRecordingMetadata}
 return b.Buffer.Write(data)
}
func decodeVideoDuration(raw []byte) (time.Duration,error) {
 var data struct {
  Streams []struct {CodecType string `json:"codec_type"`;Duration string `json:"duration"`} `json:"streams"`
  Format struct {Duration string `json:"duration"`} `json:"format"`
 }
 if json.Unmarshal(raw,&data)!=nil || len(data.Streams)!=1 || data.Streams[0].CodecType!="video" {return 0,errRecordingMetadata}
 for _,value:=range []string{data.Streams[0].Duration,data.Format.Duration} {
  seconds,err:=strconv.ParseFloat(value,64)
  if err!=nil || math.IsNaN(seconds) || math.IsInf(seconds,0) || seconds<=0 || seconds>=float64(math.MaxInt64)/float64(time.Second) {continue}
  duration:=time.Duration(seconds*float64(time.Second))
  if duration>0 {return duration,nil}
 }
 return 0,errRecordingMetadata
}
func probeVideoDuration(ctx context.Context,file *os.File) (time.Duration,error) {
 // Linux runtime: probe the already-open descriptor rather than reopen a
 // pathname that could have been replaced. MOV demuxing covers these MP4s.
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_entries","stream=codec_type,duration:format=duration","-of","json")
 cmd.ExtraFiles=[]*os.File{file}
 cmd.WaitDelay=2*time.Second
 var output limitedProbeOutput
 cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil {if ctx.Err()!=nil {return 0,ctx.Err()};return 0,errRecordingMetadata}
 return decodeVideoDuration(output.Bytes())
}
