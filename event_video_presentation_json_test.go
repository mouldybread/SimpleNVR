package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "strings"
 "testing"
)

const eventPresentationFixture = `{"streams":[{"index":0,"codec_type":"video","time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":1000},{"media_type":"video","stream_index":0,"pts":1000,"duration":1000}]}`

func TestEventVideoPresentationStrictSuccess(t *testing.T) {
 for _, raw := range []string{
  eventPresentationFixture,
  " \n"+eventPresentationFixture+"\t ",
  strings.ReplaceAll(eventPresentationFixture,`"index":0`,`"index":7`),
 } {
  if strings.Contains(raw,`"index":7`) { raw = strings.ReplaceAll(raw,`"stream_index":0`,`"stream_index":7`) }
  before := []byte(raw)
  observation,err := decodeEventVideoPresentation(context.Background(),before)
  if err != nil || observation.continuity.frames != 2 || observation.continuity.first != 0 || observation.continuity.end != 2000 || !observation.continuity.atLeastSeconds(2) || observation.continuity.atLeastSeconds(3) { t.Fatal(observation,err) }
  if !bytes.Equal(before,[]byte(raw)) { t.Fatal("mutated caller JSON") }
 }
 // Object member order is not a timing policy; frame array order IS.
 reordered := `{"frames":[{"duration":1000,"pts":0,"stream_index":0,"media_type":"video"}],"streams":[{"time_base":"1/1000","codec_type":"video","index":0}]}`
 if _,err:=decodeEventVideoPresentation(context.Background(),[]byte(reordered));err!=nil { t.Fatal(err) }
 // Signed origins remain explicit rather than normalized into wall-clock proof.
 shifted:=strings.Replace(eventPresentationFixture,`"pts":0`,`"pts":-1000`,1)
 shifted=strings.Replace(shifted,`"pts":1000`,`"pts":0`,1)
 observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(shifted))
 if err!=nil || observation.continuity.first!=-1000 || !observation.continuity.atLeastSeconds(2) { t.Fatal(observation,err) }
}

func TestEventVideoPresentationRefusesAmbiguousOrMissingFields(t *testing.T) {
 replacements:=[][2]string{
  {`"streams":`, `"Streams":`},
  {`"frames":`, `"Frames":`},
  {`"index":0`, `"index":0,"index":0`},
  {`"codec_type":"video"`, `"codec_type":"video","codec_type":"audio"`},
  {`"time_base":"1/1000"`, `"time_base":"1/1000","time_base":"1/1000"`},
  {`"media_type":"video"`, `"media_type":"video","media_type":"video"`},
  {`"stream_index":0`, `"stream_index":0,"stream_index":0`},
  {`"pts":0`, `"pts":0,"pts":0`},
  {`"duration":1000`, `"duration":1000,"duration":1000`},
  {`"pts":0`, `"p\u0074s":0,"pts":0`},
  {`"index":0,`, ``}, {`"codec_type":"video",`, ``}, {`,"time_base":"1/1000"`, ``},
  {`"media_type":"video",`, ``}, {`"stream_index":0,`, ``}, {`"pts":0,`, ``}, {`,"duration":1000`, ``},
  {`"index":0`, `"index":-1`}, {`"stream_index":0`, `"stream_index":1`},
  {`"codec_type":"video"`, `"codec_type":"audio"`}, {`"media_type":"video"`, `"media_type":"audio"`},
  {`"pts":1000`, `"pts":1001`}, {`"pts":1000`, `"pts":999`}, {`"pts":1000`, `"pts":0`},
  {`"duration":1000`, `"duration":0`}, {`"duration":1000`, `"duration":-1`},
  {`"pts":0`, `"best_effort_timestamp":0`}, {`"duration":1000`, `"pkt_duration":1000`},
  {`"index":0`, `"index":0,"unknown":1`},
  {`"pts":0`, `"pts":0,"unknown":1`},
  {`"streams":`, `"unknown":1,"streams":`},
 }
 for i,pair:=range replacements {
  raw:=strings.Replace(eventPresentationFixture,pair[0],pair[1],1)
  observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(raw))
  if err==nil || observation!=(eventVideoPresentation{}) { t.Fatal("accepted malformed observation",i,raw,observation,err) }
 }
 for _,field:=range []string{`"index":0`,`"stream_index":0`,`"pts":0`,`"duration":1000`} {
  for _,value:=range []string{`null`,`true`,`"0"`,`0.0`,`1e3`,`9223372036854775808`,`-9223372036854775809`,`{}`,`[]`} {
   raw:=strings.Replace(eventPresentationFixture,field,strings.Split(field,":")[0]+":"+value,1)
   observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(raw))
   if err==nil || observation!=(eventVideoPresentation{}) { t.Fatal(field,value,observation,err) }
  }
 }
 for _,base:=range []string{"0/1000","1/0","-1/1000","1/-1","+1/1000","1/+1","1 /1000","1/1000/1","1","1.0/1000","1/9223372036854775808"} {
  raw:=strings.Replace(eventPresentationFixture,"1/1000",base,1)
  if _,err:=decodeEventVideoPresentation(context.Background(),[]byte(raw));err==nil { t.Fatal("invalid base accepted",base) }
 }
}

func TestEventVideoPresentationRequiresWholeDocumentAndSingleStream(t *testing.T) {
 for _,raw:=range []string{
  "", "null", "[]", "{}",
  eventPresentationFixture+"{}", eventPresentationFixture+"null",eventPresentationFixture+"x",
  strings.Replace(eventPresentationFixture,`"frames":[`, `"frames":null,"discard":[`,1),
  strings.Replace(eventPresentationFixture,`"streams":[{"index":0,"codec_type":"video","time_base":"1/1000"}]`,`"streams":[]`,1),
  strings.Replace(eventPresentationFixture,`"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":1000},{"media_type":"video","stream_index":0,"pts":1000,"duration":1000}]`,`"frames":[]`,1),
  strings.Replace(eventPresentationFixture,`"streams":[`,`"streams":[{"index":1,"codec_type":"video","time_base":"1/1000"},`,1),
  strings.Replace(eventPresentationFixture,`"streams":`,`"streams":[],"streams":`,1),
  strings.Replace(eventPresentationFixture,`"frames":`,`"frames":[],"frames":`,1),
  strings.Replace(eventPresentationFixture,`"pts":1000`,`"pts":9223372036854775807`,1),
 } {
  observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(raw))
  if err==nil || observation!=(eventVideoPresentation{}) { t.Fatal(raw,observation,err) }
 }
 for end:=0;end<len(eventPresentationFixture);end++ {
  observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(eventPresentationFixture[:end]))
  if err==nil || observation!=(eventVideoPresentation{}) { t.Fatal("accepted truncated JSON",end,observation,err) }
 }
 // This valid shorter JSON necessarily passes. The parser cannot certify that
 // an upstream producer did not silently omit a frame before closing the array.
 prefix:=`{"streams":[{"index":0,"codec_type":"video","time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":1000}]}`
 observation,err:=decodeEventVideoPresentation(context.Background(),[]byte(prefix))
 if err!=nil || observation.continuity.atLeastSeconds(2) { t.Fatal(observation,err) }
}

type eventPresentationCancelContext struct { context.Context; calls, cancelAt int }
func (ctx *eventPresentationCancelContext) Err() error {
 ctx.calls++
 if ctx.cancelAt>0 && ctx.calls>=ctx.cancelAt { return context.Canceled }
 return nil
}
func TestEventVideoPresentationCancellationAndByteLimit(t *testing.T) {
 raw:=[]byte(eventPresentationFixture)
 counter:=&eventPresentationCancelContext{Context:context.Background()}
 if _,err:=decodeEventVideoPresentation(counter,raw);err!=nil { t.Fatal(err) }
 for call:=1;call<=counter.calls;call++ {
  ctx:=&eventPresentationCancelContext{Context:context.Background(),cancelAt:call}
  observation,err:=decodeEventVideoPresentation(ctx,raw)
  if !errors.Is(err,context.Canceled) || observation!=(eventVideoPresentation{}) { t.Fatal(call,observation,err) }
 }
 padded:=append(append([]byte(nil),raw...),bytes.Repeat([]byte(" "),maxEventVideoPresentationBytes-len(raw))...)
 if _,err:=decodeEventVideoPresentation(context.Background(),padded);err!=nil { t.Fatal("exact byte ceiling refused",err) }
 observation,err:=decodeEventVideoPresentation(context.Background(),append(padded,' '))
 if !errors.Is(err,errEventVideoPresentation) || observation!=(eventVideoPresentation{}) { t.Fatal(observation,err) }
}

func FuzzEventVideoPresentationRefusalHasNoResult(f *testing.F) {
 f.Add([]byte(eventPresentationFixture))
 f.Add([]byte(`{"streams":null,"frames":null}`))
 f.Fuzz(func(t *testing.T,raw []byte){
  observation,err:=decodeEventVideoPresentation(context.Background(),raw)
  if err!=nil && observation!=(eventVideoPresentation{}) { t.Fatal("partial result escaped",fmt.Sprint(err)) }
  if err==nil && (observation.streamIndex<0 || observation.continuity.frames<=0) { t.Fatal("invalid successful result") }
 })
}
