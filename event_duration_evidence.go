package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "math/big"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "time"

 "golang.org/x/sys/unix"
)

var errEventDurationEvidence=errors.New("event video duration evidence unavailable or insufficient")
const eventDurationInspectionBudget=5*time.Second

// Exact video-stream ticks, not rounded seconds or container duration. A span
// reaching a duration lower bound does NOT establish gap-free or wall-clock coverage.
type eventVideoSpan struct {ticks,numerator,denominator int64}
func (span eventVideoSpan) atLeast(seconds int) bool {
 if seconds<1 || seconds>maxEventSeconds || span.ticks<=0 || span.numerator<=0 || span.denominator<=0 {return false}
 left:=new(big.Int).Mul(big.NewInt(span.ticks),big.NewInt(span.numerator))
 right:=new(big.Int).Mul(big.NewInt(int64(seconds)),big.NewInt(span.denominator))
 return left.Cmp(right)>=0
}
func decodeEventVideoSpan(raw []byte) (eventVideoSpan,error) {
 if len(raw)==0 || len(raw)>maxRecoveryManifestBytes {return eventVideoSpan{},errEventDurationEvidence}
 var data struct {Streams []struct {
  CodecType string `json:"codec_type"`
  DurationTicks json.Number `json:"duration_ts"`
  TimeBase string `json:"time_base"`
 } `json:"streams"`}
 if json.Unmarshal(raw,&data)!=nil || len(data.Streams)!=1 || data.Streams[0].CodecType!="video" {return eventVideoSpan{},errEventDurationEvidence}
 stream:=data.Streams[0]
 ticks,err:=stream.DurationTicks.Int64();if err!=nil || ticks<=0 {return eventVideoSpan{},errEventDurationEvidence}
 parts:=strings.Split(stream.TimeBase,"/")
 if len(parts)!=2 {return eventVideoSpan{},errEventDurationEvidence}
 numerator,e1:=strconv.ParseInt(parts[0],10,64);denominator,e2:=strconv.ParseInt(parts[1],10,64)
 if e1!=nil || e2!=nil || numerator<=0 || denominator<=0 {return eventVideoSpan{},errEventDurationEvidence}
 return eventVideoSpan{ticks,numerator,denominator},nil
}
func probeEventVideoSpan(ctx context.Context,file *os.File) (eventVideoSpan,error) {
 if err:=ctx.Err();err!=nil {return eventVideoSpan{},err}
 if file==nil {return eventVideoSpan{},errEventDurationEvidence}
 info,err:=file.Stat();if err!=nil {return eventVideoSpan{},err}
 if !info.Mode().IsRegular() || info.Size()<=0 {return eventVideoSpan{},errEventDurationEvidence}
 // The caller pins the actual owned descriptor. No pathname rediscovery, format
 // duration fallback, shell, or new media-gate acquisition is permitted here.
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_entries","stream=codec_type,duration_ts,time_base","-of","json")
 cmd.ExtraFiles=[]*os.File{file};cmd.WaitDelay=2*time.Second
 var output limitedProbeOutput
 cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil {return eventVideoSpan{},errors.Join(err,ctx.Err())}
 if err:=ctx.Err();err!=nil {return eventVideoSpan{},err}
 return decodeEventVideoSpan(output.Bytes())
}

// Read-only duration evidence collected once by production finalization.
// Still NOT full coverage, a cleanup capability, or permission to delete.
// The existing job owner serializes inspection, checks, and terminal teardown.
// No extra persistent pin, lease, mutable global map or local context is retained.
type eventDurationEvidence struct {
 bm *BufferManager
 job *bufferEvent
 inventory *ownedStagingInventory
 borrowed *eventRecoveryBorrow
 output *eventPublishedOutput
 execution *eventExecutionProvenance
 capture *os.File
 captureID unix.Stat_t
 requestedSeconds int
 requestedStart time.Time
 captureSpan,publishedSpan eventVideoSpan
}
func (e *eventDurationEvidence) bound(ctx context.Context,lease *mediaMutationLease) error {
 if err:=ctx.Err();err!=nil {return err}
 if e==nil || e.bm==nil || e.job==nil || e.inventory==nil || e.borrowed==nil || e.output==nil || e.capture==nil || !lease.held() || lease.gate!=recordingMediaOwnership {return errEventDurationEvidence}
 job:=e.job
 if !e.execution.qualifies(e.bm,job) || job.inventory!=e.inventory || job.recoveryBorrow!=e.borrowed || e.borrowed.lease!=lease || !e.borrowed.terminal || e.borrowed.output!=e.output || job.event.Duration!=e.requestedSeconds || !job.event.StartTime.Equal(e.requestedStart) {return errEventDurationEvidence}
 capture,ok:=e.inventory.entries["event.mp4"]
 if !ok || capture.file!=e.capture || !stagingInventoryIdentity(e.captureID,capture.identity) {return errEventDurationEvidence}
 return nil
}
func (e *eventDurationEvidence) check(ctx context.Context,lease *mediaMutationLease) error {
 if err:=e.bound(ctx,lease);err!=nil {return err}
 if !e.captureSpan.atLeast(e.requestedSeconds) || !e.publishedSpan.atLeast(e.requestedSeconds) {return errEventDurationEvidence}
 if err:=e.inventory.Preflight(ctx,lease);err!=nil {return err}
 if err:=e.output.check(ctx,lease);err!=nil {return err}
 return e.bound(ctx,lease)
}
func inspectEventDurationEvidence(parent context.Context,lease *mediaMutationLease,bm *BufferManager,job *bufferEvent) (*eventDurationEvidence,error) {
 return inspectEventDurationEvidenceWithProbe(parent,lease,bm,job,probeEventVideoSpan)
}
func inspectEventDurationEvidenceWithProbe(parent context.Context,lease *mediaMutationLease,bm *BufferManager,job *bufferEvent,probe func(context.Context,*os.File)(eventVideoSpan,error)) (*eventDurationEvidence,error) {
 // One aggregate deadline covers both probes and the before/after checks. Probe
 // admission cannot renew the budget for each file. WaitDelay also bounds pipe join.
 ctx,cancel:=context.WithTimeout(parent,eventDurationInspectionBudget);defer cancel()
 if err:=ctx.Err();err!=nil {return nil,err}
 if probe==nil || job==nil || bm==nil || job.inventory==nil || job.recoveryBorrow==nil {return nil,errEventDurationEvidence}
 e:=&eventDurationEvidence{bm:bm,job:job,inventory:job.inventory,borrowed:job.recoveryBorrow,output:job.recoveryBorrow.output,execution:job.execution,requestedSeconds:job.event.Duration,requestedStart:job.event.StartTime}
 capture,ok:=e.inventory.entries["event.mp4"];if !ok {return nil,errEventDurationEvidence}
 e.capture=capture.file;e.captureID=capture.identity
 if err:=e.bound(ctx,lease);err!=nil {return nil,err}
 if err:=e.inventory.Preflight(ctx,lease);err!=nil {return nil,err}
 if err:=e.output.check(ctx,lease);err!=nil {return nil,err}
 var err error
 e.captureSpan,err=probe(ctx,e.capture)
 if ctx.Err()!=nil {return nil,ctx.Err()};if err!=nil {return nil,err}
 if !e.captureSpan.atLeast(e.requestedSeconds) {return nil,errEventDurationEvidence}
 if err:=e.bound(ctx,lease);err!=nil {return nil,err}
 e.publishedSpan,err=probe(ctx,e.output.file)
 if ctx.Err()!=nil {return nil,ctx.Err()};if err!=nil {return nil,err}
 if err:=e.check(ctx,lease);err!=nil {return nil,err}
 return e,nil
}
