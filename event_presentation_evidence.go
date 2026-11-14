package main

import (
 "bytes"
 "context"
 "encoding/json"
 "os"

 "golang.org/x/sys/unix"
)

// Both observations come from ONE query and the SAME selected video stream.
// A scalar descriptor snapshot is kept; no descriptor, context or lease is added.
type eventVideoProbeObservation struct {
 duration eventVideoSpan
 presentation eventVideoPresentation
 stat unix.Stat_t
}
const eventCombinedPresentationEntries="stream=index,codec_type,duration_ts,time_base:frame=media_type,stream_index,pts,duration:frame_side_data="

func decodeEventDurationPresentation(ctx context.Context,raw []byte) (eventVideoProbeObservation,error) {
 var zero eventVideoProbeObservation
 root,err:=eventPresentationProbeFields(ctx,raw);if err!=nil {return zero,err}
 d:=json.NewDecoder(bytes.NewReader(root["streams"]))
 token,err:=d.Token();if err!=nil || token!=json.Delim('[') || !d.More() {return zero,errEventVideoPresentation}
 stream,err:=eventPresentationObject(ctx,d,"index","codec_type","duration_ts","time_base");if err!=nil {return zero,err}
 if d.More() {return zero,errEventVideoPresentation}
 token,err=d.Token();if err!=nil || token!=json.Delim(']') || !eventPresentationEOF(d) {return zero,errEventVideoPresentation}
 ticks,ok:=eventPresentationInteger(stream["duration_ts"]);if !ok || ticks<=0 {return zero,errEventVideoPresentation}
 numerator,denominator,ok:=eventPresentationTimeBase(stream["time_base"]);if !ok {return zero,errEventVideoPresentation}
 frames,err:=canonicalEventProbeFrames(ctx,root["frames"]);if err!=nil {return zero,err}
 // Remove ONLY the already-validated stream duration field for the canonical
 // frame decoder; unknown/duplicate keys and missing fields have already refused.
 canonical,err:=json.Marshal(struct {
  Streams []map[string]json.RawMessage `json:"streams"`
  Frames json.RawMessage `json:"frames"`
 }{Streams:[]map[string]json.RawMessage{{"index":stream["index"],"codec_type":stream["codec_type"],"time_base":stream["time_base"]}},Frames:frames})
 if err!=nil {return zero,errEventVideoPresentation}
 presentation,err:=decodeEventVideoPresentation(ctx,canonical);if err!=nil {return zero,err}
 return eventVideoProbeObservation{duration:eventVideoSpan{ticks,numerator,denominator},presentation:presentation},nil
}
func probeEventDurationPresentation(ctx context.Context,lease *mediaMutationLease,file *os.File) (eventVideoProbeObservation,error) {
 return runEventVideoPresentationProbe(ctx,lease,file,eventCombinedPresentationEntries,decodeEventDurationPresentation)
}

// Bound to the exact duration/job/inventory/execution/request binding, plus the
// exact successful capture/output observations. Still NOT complete event coverage.
type eventPresentationEvidence struct {
 duration *eventDurationEvidence
 capture,published eventVideoPresentation
 captureStat,publishedStat unix.Stat_t
}
func (p *eventPresentationEvidence) check(ctx context.Context,lease *mediaMutationLease,e *eventDurationEvidence) error {
 if p==nil || e==nil || p.duration!=e || e.presentation!=p {return errEventVideoPresentation}
 if err:=e.bound(ctx,lease);err!=nil {return err}
 if !e.captureSpan.atLeast(e.requestedSeconds) || !e.publishedSpan.atLeast(e.requestedSeconds) {return errEventDurationEvidence}
 for _,pair:=range []struct {presentation eventVideoPresentation;span eventVideoSpan}{{p.capture,e.captureSpan},{p.published,e.publishedSpan}} {
  c:=pair.presentation.continuity
  if pair.presentation.streamIndex<0 || c.numerator!=pair.span.numerator || c.denominator!=pair.span.denominator || !c.atLeastSeconds(int64(e.requestedSeconds)) {return errEventVideoPresentation}
 }
 capture,err:=eventPresentationDescriptorStat(ctx,lease,e.capture);if err!=nil {return err}
 output,err:=eventPresentationDescriptorStat(ctx,lease,e.output.file);if err!=nil {return err}
 if !eventPresentationDescriptorUnchanged(p.captureStat,capture) || !eventPresentationDescriptorUnchanged(p.publishedStat,output) {return errEventVideoPresentation}
 return e.bound(ctx,lease)
}

func inspectEventDurationPresentationEvidence(parent context.Context,lease *mediaMutationLease,bm *BufferManager,job *bufferEvent) (*eventDurationEvidence,error) {
 // Outer budget includes collection AND attaching/checking the bound receipt.
 // Inner collector and children can only shorten it. Metadata uses other budgets.
 ctx,cancel:=context.WithTimeout(parent,eventDurationInspectionBudget);defer cancel()
 var observations [2]eventVideoProbeObservation
 calls:=0
 e,err:=inspectEventDurationEvidenceWithProbe(ctx,lease,bm,job,func(ctx context.Context,file *os.File)(eventVideoSpan,error){
  if calls>=len(observations) {return eventVideoSpan{},errEventVideoPresentation}
  observation,err:=probeEventDurationPresentation(ctx,lease,file);if err!=nil {return eventVideoSpan{},err}
  if !observation.presentation.continuity.atLeastSeconds(int64(job.event.Duration)) {return eventVideoSpan{},errEventVideoPresentation}
  observations[calls]=observation;calls++
  return observation.duration,nil
 })
 if err!=nil {return nil,err}
 if calls!=2 {return nil,errEventVideoPresentation}
 e.presentation=&eventPresentationEvidence{duration:e,capture:observations[0].presentation,published:observations[1].presentation,captureStat:observations[0].stat,publishedStat:observations[1].stat}
 if err:=e.presentation.check(ctx,lease,e);err!=nil {return nil,err}
 return e,nil
}

// The query explicitly selects NO side-data fields. Some codecs still emit a
// side_data_list containing empty objects. Accept only that known empty envelope;
// no nonempty side-data object, alias, duplicate or other frame field is ignored.
func canonicalEventProbeFrames(ctx context.Context,raw []byte) (json.RawMessage,error) {
 d:=json.NewDecoder(bytes.NewReader(raw))
 token,err:=d.Token();if err!=nil || token!=json.Delim('[') {return nil,errEventVideoPresentation}
 var frames []map[string]json.RawMessage
 for d.More() {
  if err:=ctx.Err();err!=nil {return nil,err}
  if len(frames)>=maxEventVideoContinuityFrames {return nil,errEventVideoPresentation}
  token,err:=d.Token();if err!=nil || token!=json.Delim('{') {return nil,errEventVideoPresentation}
  fields:=make(map[string]json.RawMessage,5)
  for d.More() {
   if err:=ctx.Err();err!=nil {return nil,err}
   token,err:=d.Token();if err!=nil {return nil,errEventVideoPresentation}
   key,ok:=token.(string);if !ok {return nil,errEventVideoPresentation}
   if key!="media_type" && key!="stream_index" && key!="pts" && key!="duration" && key!="side_data_list" {return nil,errEventVideoPresentation}
   if _,exists:=fields[key];exists {return nil,errEventVideoPresentation}
   var value json.RawMessage;if err:=d.Decode(&value);err!=nil {return nil,errEventVideoPresentation}
   fields[key]=value
  }
  token,err=d.Token();if err!=nil || token!=json.Delim('}') {return nil,errEventVideoPresentation}
  if value,exists:=fields["side_data_list"];exists {
   var entries []json.RawMessage;trimmed:=bytes.TrimSpace(value)
   if len(trimmed)==0 || trimmed[0]!='[' || json.Unmarshal(value,&entries)!=nil || len(entries)>16 {return nil,errEventVideoPresentation}
   for _,entry:=range entries {
    if err:=ctx.Err();err!=nil {return nil,err}
    var object map[string]json.RawMessage;trimmed:=bytes.TrimSpace(entry)
    if len(trimmed)==0 || trimmed[0]!='{' || json.Unmarshal(entry,&object)!=nil || len(object)!=0 {return nil,errEventVideoPresentation}
   }
   delete(fields,"side_data_list")
  }
  if len(fields)!=4 {return nil,errEventVideoPresentation}
  frames=append(frames,fields)
 }
 token,err=d.Token();if err!=nil || token!=json.Delim(']') || !eventPresentationEOF(d) {return nil,errEventVideoPresentation}
 if len(frames)==0 {return nil,errEventVideoPresentation}
 if err:=ctx.Err();err!=nil {return nil,err}
 encoded,err:=json.Marshal(frames);if err!=nil {return nil,errEventVideoPresentation}
 return encoded,nil
}
