package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "os"
 "os/exec"
 "time"

 "golang.org/x/sys/unix"
)

// Opt-in component only: no production call site is added. A future caller MUST
// supply the existing held lease, owned descriptor and shared aggregate context.
// This function never acquires/releases ownership or opens media by its name.
const eventPresentationProbeDiagnosticBytes = 16 << 10

// Named storage prevents promoted ReadFrom/WriteString from bypassing Write.
type eventPresentationProbeBuffer struct {
 data bytes.Buffer
 limit int
 refused bool
}
func (b *eventPresentationProbeBuffer) Len() int {return b.data.Len()}
func (b *eventPresentationProbeBuffer) Bytes() []byte {return b.data.Bytes()}
func (b *eventPresentationProbeBuffer) String() string {return b.data.String()}
func (b *eventPresentationProbeBuffer) Write(p []byte) (int,error) {
 if b.refused || len(p)>b.limit-b.Len() { b.refused=true;return 0,errEventVideoPresentation }
 return b.data.Write(p)
}

// FFprobe can emit empty program/stream-group envelopes alongside show_entries.
// Accept ONLY these two known empty arrays. The canonical decoder remains strict;
// unknown/nonempty/null/duplicate envelopes do not get silently discarded.
func decodeEventPresentationProbeJSON(ctx context.Context,raw []byte) (eventVideoPresentation,error) {
 var zero eventVideoPresentation
 if err:=ctx.Err();err!=nil {return zero,err}
 if len(raw)==0 || len(raw)>maxEventVideoPresentationBytes {return zero,errEventVideoPresentation}
 d:=json.NewDecoder(bytes.NewReader(raw))
 token,err:=d.Token();if err!=nil || token!=json.Delim('{') {return zero,errEventVideoPresentation}
 fields:=make(map[string]json.RawMessage,4)
 for d.More() {
  if err:=ctx.Err();err!=nil {return zero,err}
  token,err:=d.Token();if err!=nil {return zero,errEventVideoPresentation}
  key,ok:=token.(string);if !ok {return zero,errEventVideoPresentation}
  if key!="streams" && key!="frames" && key!="programs" && key!="stream_groups" {return zero,errEventVideoPresentation}
  if _,exists:=fields[key];exists {return zero,errEventVideoPresentation}
  var value json.RawMessage
  if err:=d.Decode(&value);err!=nil {return zero,errEventVideoPresentation}
  if key=="programs" || key=="stream_groups" {
   trimmed:=bytes.TrimSpace(value)
   var entries []json.RawMessage
   if len(trimmed)==0 || trimmed[0]!='[' || json.Unmarshal(value,&entries)!=nil || len(entries)!=0 {return zero,errEventVideoPresentation}
  }
  fields[key]=value
 }
 token,err=d.Token()
 if err!=nil || token!=json.Delim('}') || !eventPresentationEOF(d) || fields["streams"]==nil || fields["frames"]==nil {return zero,errEventVideoPresentation}
 canonical,err:=json.Marshal(struct {
  Streams json.RawMessage `json:"streams"`
  Frames json.RawMessage `json:"frames"`
 }{Streams:fields["streams"],Frames:fields["frames"]})
 if err!=nil {return zero,errEventVideoPresentation}
 return decodeEventVideoPresentation(ctx,canonical)
}

func eventPresentationDescriptorStat(ctx context.Context,lease *mediaMutationLease,file *os.File) (unix.Stat_t,error) {
 var stat unix.Stat_t
 if err:=ctx.Err();err!=nil {return stat,err}
 if file==nil || !lease.held() || lease.gate!=recordingMediaOwnership {return stat,errEventVideoPresentation}
 if err:=unix.Fstat(int(file.Fd()),&stat);err!=nil {return unix.Stat_t{},err}
 if stat.Mode&unix.S_IFMT!=unix.S_IFREG || stat.Size<=0 || stat.Nlink==0 {return unix.Stat_t{},errEventVideoPresentation}
 return stat,nil
}
func eventPresentationDescriptorUnchanged(before,after unix.Stat_t) bool {
 return before.Dev==after.Dev && before.Ino==after.Ino && before.Mode==after.Mode && before.Size==after.Size && before.Nlink==after.Nlink && before.Mtim==after.Mtim && before.Ctim==after.Ctim
}

// A successful result binds this individual inspection to the borrowed file for
// the duration of the call, NOT to a job, inventory, requested interval or future
// release. It is not a reusable receipt or positive cleanup authorization.
func probeEventVideoPresentation(parent context.Context,lease *mediaMutationLease,file *os.File) (eventVideoPresentation,error) {
 var zero eventVideoPresentation
 // This can only shorten a caller's shared aggregate deadline, never extend it.
 ctx,cancel:=context.WithTimeout(parent,eventDurationInspectionBudget);defer cancel()
 before,err:=eventPresentationDescriptorStat(ctx,lease,file);if err!=nil {return zero,err}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-err_detect","explode","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_frames","-show_entries","stream=index,codec_type,time_base:frame=media_type,stream_index,pts,duration:frame_side_data=","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second
 stdout:=&eventPresentationProbeBuffer{limit:maxEventVideoPresentationBytes}
 stderr:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes}
 cmd.Stdout=stdout;cmd.Stderr=stderr
 runErr:=cmd.Run()
 if err:=ctx.Err();err!=nil {return zero,errors.Join(runErr,err)}
 if runErr!=nil {return zero,runErr}
 // Error diagnostics disqualify even a zero-exit, syntactically valid prefix.
 if stdout.refused || stderr.refused || stderr.Len()!=0 {return zero,errEventVideoPresentation}
 after,err:=eventPresentationDescriptorStat(ctx,lease,file);if err!=nil {return zero,err}
 if !eventPresentationDescriptorUnchanged(before,after) {return zero,errEventVideoPresentation}
 observation,err:=decodeEventPresentationProbeJSON(ctx,stdout.Bytes());if err!=nil {return zero,err}
 after,err=eventPresentationDescriptorStat(ctx,lease,file);if err!=nil {return zero,err}
 if !eventPresentationDescriptorUnchanged(before,after) {return zero,errEventVideoPresentation}
 if err:=ctx.Err();err!=nil {return zero,err}
 return observation,nil
}
