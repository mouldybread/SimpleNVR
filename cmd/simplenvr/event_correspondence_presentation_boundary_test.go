package main

import (
 "context"
 "encoding/json"
 "os"
 "testing"
 "time"
)

// Failure-only observations of newly opened paths are diagnostic, never adopted
// as owner evidence. They run outside measurement and cannot repair a receipt.
func corrBoundLogPresentationFailure(t *testing.T,lease *mediaMutationLease,capture,final string,requested int) {
 t.Helper()
 ctx,cancel:=context.WithTimeout(context.Background(),eventDurationInspectionBudget);defer cancel()
 var spans [2]eventVideoContinuity
 for i,path:=range []string{capture,final} {
  file,err:=os.Open(path)
  if err!=nil {t.Logf("diagnostic_only component=%d open_error=%v",i,err);continue}
  observation,probeErr:=probeEventDurationPresentation(ctx,lease,file)
  closeErr:=file.Close()
  if probeErr!=nil {t.Logf("diagnostic_only component=%d probe_error=%v close_error=%v",i,probeErr,closeErr);continue}
  span:=observation.presentation.continuity;spans[i]=span
  t.Logf("diagnostic_only component=%d requested_seconds=%d first_pts=%d end_pts=%d time_base=%d/%d frames=%d presentation_sufficient=%t stream_duration_sufficient=%t close_error=%v",i,requested,span.first,span.end,span.numerator,span.denominator,span.frames,span.atLeastSeconds(int64(requested)),observation.duration.atLeast(requested),closeErr)
 }
 t.Logf("diagnostic_only no_prebuffer_assembly_compatible=%t",eventAssemblySelectedPrefixCompatible(spans[0],spans[1],0))
}

func TestCorrPresentationMetadataCannotHideShortInterval(t *testing.T) {
 for _,tc:=range []struct{name string;seconds,denominator,offset int64;frames int}{
  {"360p",14,10240,215,140},
  {"720p",110,15360,323,1650},
 } {
  t.Run(tc.name,func(t *testing.T){
   for _,variant:=range []struct{name string;offset,trim int64;enough,compatible bool}{
    {"exact",0,0,true,true},
    {"rebased_same_length",tc.offset,0,true,true},
    {"ci_short_final_frame",tc.offset,tc.offset,false,false},
    {"one_tick_short",0,1,false,false},
   } {
    t.Run(variant.name,func(t *testing.T){
     frames:=make([]map[string]any,tc.frames)
     for i:=range frames {
      duration:=int64(1024);if i==len(frames)-1 {duration-=variant.trim}
      frames[i]=map[string]any{"media_type":"video","stream_index":0,"pts":int64(i)*1024+variant.offset,"duration":duration}
     }
     raw,err:=json.Marshal(map[string]any{"streams":[]any{map[string]any{"index":0,"codec_type":"video","duration_ts":tc.seconds*tc.denominator,"time_base":func()string{if tc.denominator==10240 {return "1/10240"};return "1/15360"}()}},"frames":frames})
     if err!=nil {t.Fatal(err)}
     observation,err:=decodeEventDurationPresentation(context.Background(),raw);if err!=nil {t.Fatal("valid continuous JSON must decode",err)}
     if !observation.duration.atLeast(int(tc.seconds)) {t.Fatal("metadata fixture does not claim requested duration")}
     span:=observation.presentation.continuity
     if got:=span.atLeastSeconds(tc.seconds);got!=variant.enough {t.Fatalf("presentation lower bound=%t want=%t",got,variant.enough)}
     capture:=eventVideoContinuity{first:0,end:tc.seconds*tc.denominator,numerator:1,denominator:tc.denominator,frames:tc.frames}
     if got:=eventAssemblySelectedPrefixCompatible(capture,span,0);got!=variant.compatible {t.Fatalf("assembly compatibility=%t want=%t",got,variant.compatible)}
     if variant.trim==tc.offset && variant.offset==tc.offset && !span.atLeastSeconds(tc.seconds-1) {t.Fatal("shorter request should satisfy duration but must not bypass exact assembly compatibility")}
    })
   }
  })
 }
}

var _ = time.Second
