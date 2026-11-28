package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "reflect"
 "strconv"
 "strings"
 "testing"
 "time"
)

// Serialization-only template, not capture or cleanup authorization evidence.
func recoveryContractSchemaSeed()([]byte,error) {
 start:=time.Unix(1000,0).UTC();end:=start.Add(30*time.Second)
 first,last:=start,start.Add(time.Second);entered,returned:=start.Add(time.Second),start.Add(2*time.Second)
 m:=eventRecoveryManifest{Version:1,Revision:7,CameraID:"contract-camera",EventID:"contract-event",RequestedStart:start,
  RequestedEnd:end,DurationSeconds:30,CreatedAt:start,ObservedAt:returned,State:"published",CaptureName:"event.mp4",
  PinnedCount:2,OutputTemporary:"contract.mp4.tmp",OutputPublished:"contract.mp4",
  Inputs:[]eventRecoveryInput{{Name:"pre_000.mp4",Start:start.Add(-5*time.Second),End:start.Add(5*time.Second)},
   {Name:"pre_001.mp4",Start:start.Add(10*time.Second),End:start.Add(15*time.Second)}}}
 gaps,invalid:=declaredPrebufferGaps(start,end,m.Inputs)
 m.Coverage=&eventCoverageAssessment{Version:1,Status:"unknown",ObservationPoint:"completed_segment_notification_receipt",
  RequestedStart:start,RequestedEnd:end,SelectionBound:true,DeclaredPrebufferAssessed:true,DeclaredPrebufferGaps:gaps,
  InvalidDeclaredRanges:invalid,Segments:eventCoverageSegments{Inputs:2,Observed:2},FirstNotification:&first,LastNotification:&last,
  CaptureRunState:"succeeded",CaptureRunEntered:&entered,CaptureRunReturned:&returned,Unknowns:eventCoverageUnknowns()}
 return json.Marshal(&m)
}
func recoveryContractEditJSON(data json.RawMessage,path []string,edit func(json.RawMessage)(json.RawMessage,error))(json.RawMessage,error) {
 if len(path)==0 {return edit(data)}
 if index,err:=strconv.Atoi(path[0]);err==nil {
  var entries []json.RawMessage;if err:=json.Unmarshal(data,&entries);err!=nil {return nil,err}
  if index<0 || index>=len(entries) {return nil,fmt.Errorf("fixture array index outside range")}
  updated,err:=recoveryContractEditJSON(entries[index],path[1:],edit);if err!=nil {return nil,err};entries[index]=updated;return json.Marshal(entries)
 }
 var fields map[string]json.RawMessage;if err:=json.Unmarshal(data,&fields);err!=nil {return nil,err}
 value,ok:=fields[path[0]];if !ok {return nil,fmt.Errorf("fixture field absent: %s",path[0])}
 updated,err:=recoveryContractEditJSON(value,path[1:],edit);if err!=nil {return nil,err};fields[path[0]]=updated;return json.Marshal(fields)
}
func TestRecoveryContractNestedSchemaRejectsEveryRequiredFieldMutation(t *testing.T) {
 original,err:=recoveryContractSchemaSeed();if err!=nil {t.Fatal(err)}
 if _,err:=parseRecoveryManifest(original,"contract-camera");err!=nil {t.Fatal("positive schema control rejected",err)}
 sections:=[]struct{name string;path,keys []string}{
  {"coverage",[]string{"coverage_assessment"},[]string{"version","status","complete","observation_point","requested_start","requested_end","selection_bound","declared_prebuffer_assessed","declared_prebuffer_gaps_in_request","invalid_declared_ranges","segments","capture_run_state","unknowns"}},
  {"segments",[]string{"coverage_assessment","segments"},[]string{"inputs","observed","missing","uncertain","backwards","session_changes","sequence_breaks","declared_csv_gaps","declared_csv_overlaps"}},
  {"gap",[]string{"coverage_assessment","declared_prebuffer_gaps_in_request","0"},[]string{"start","end"}},
  {"input",[]string{"prebuffer_inputs","0"},[]string{"name","metadata_start","metadata_end"}},
 }
 for _,section:=range sections {for _,key:=range section.keys {for _,kind:=range []string{"missing","null","case_alias","wrong_type"} {
  t.Run(section.name+"/"+key+"/"+kind,func(t *testing.T){
   mutated,err:=recoveryContractEditJSON(original,section.path,func(raw json.RawMessage)(json.RawMessage,error){
    var fields map[string]json.RawMessage;if err:=json.Unmarshal(raw,&fields);err!=nil {return nil,err};value:=fields[key]
    switch kind {
    case "missing":delete(fields,key)
    case "null":fields[key]=json.RawMessage("null")
    case "case_alias":delete(fields,key);fields[strings.ToUpper(key[:1])+key[1:]]=value
    case "wrong_type":fields[key]=json.RawMessage("[]")
    }
    return json.Marshal(fields)
   })
   if err!=nil {t.Fatal(err)};if _,err:=parseRecoveryManifest(mutated,"contract-camera");err==nil {t.Fatal("nested mutation accepted")}
  })
 }}}
 for _,section:=range sections {for _,kind:=range []string{"unknown_authority","duplicate_key","escaped_duplicate_key"} {
  t.Run(section.name+"/"+kind,func(t *testing.T){
   mutated,err:=recoveryContractEditJSON(original,section.path,func(raw json.RawMessage)(json.RawMessage,error){
    var fields map[string]json.RawMessage;if err:=json.Unmarshal(raw,&fields);err!=nil {return nil,err}
    key:=section.keys[0];encoded,_:=json.Marshal(key);value:=fields[key]
    if kind=="unknown_authority" {encoded=[]byte(`"cleanup_authorized"`);value=[]byte("true")}
    if kind=="escaped_duplicate_key" {encoded=[]byte(fmt.Sprintf(`"\u%04x%s"`,key[0],key[1:]))}
    return append(append(append(append([]byte{'{'},encoded...),':'),append(value,',')...),raw[1:]...),nil
   })
   if err!=nil {t.Fatal(err)};if _,err:=parseRecoveryManifest(mutated,"contract-camera");err==nil {t.Fatal("nested ambiguity/authority field accepted")}
  })
 }}
}
func TestRecoveryContractDiagnosticRelationshipsRejectContradictions(t *testing.T) {
 original,err:=recoveryContractSchemaSeed();if err!=nil {t.Fatal(err)}
 cases:=[]struct{name string;path []string;value string}{
  {"future_version",[]string{"coverage_assessment","version"},"2"},
  {"negative_observed",[]string{"coverage_assessment","segments","observed"},"-1"},
  {"observed_missing_sum",[]string{"coverage_assessment","segments","missing"},"1"},
  {"excess_uncertainty",[]string{"coverage_assessment","segments","uncertain"},"5"},
  {"too_many_backwards",[]string{"coverage_assessment","segments","backwards"},"3"},
  {"too_many_sessions",[]string{"coverage_assessment","segments","session_changes"},"3"},
  {"too_many_sequence_breaks",[]string{"coverage_assessment","segments","sequence_breaks"},"3"},
  {"too_many_declared_gaps",[]string{"coverage_assessment","segments","declared_csv_gaps"},"3"},
  {"negative_overlap",[]string{"coverage_assessment","segments","declared_csv_overlaps"},"-1"},
  {"unbound_observations",[]string{"coverage_assessment","selection_bound"},"false"},
  {"unassessed_gap_credit",[]string{"coverage_assessment","declared_prebuffer_assessed"},"false"},
  {"one_notification_only",[]string{"coverage_assessment","last_notification"},"null"},
  {"notification_order",[]string{"coverage_assessment","last_notification"},`"1970-01-01T00:00:01Z"`},
  {"run_order",[]string{"coverage_assessment","capture_run_returned"},`"1970-01-01T00:00:01Z"`},
  {"missing_entered",[]string{"coverage_assessment","capture_run_entered"},"null"},
  {"run_stage_with_return",[]string{"coverage_assessment","capture_run_state"},`"running"`},
  {"unobserved_with_times",[]string{"coverage_assessment","capture_run_state"},`"not_observed"`},
  {"unsupported_run_state",[]string{"coverage_assessment","capture_run_state"},`"complete"`},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  mutated,err:=recoveryContractEditJSON(original,tc.path,func(json.RawMessage)(json.RawMessage,error){return json.RawMessage(tc.value),nil})
  if err!=nil {t.Fatal(err)};if _,err:=parseRecoveryManifest(mutated,"contract-camera");err==nil {t.Fatal("contradictory diagnostic accepted")}
 })}
}
func FuzzRecoveryContractStrictManifest(f *testing.F) {
 seed,err:=recoveryContractSchemaSeed();if err!=nil {f.Fatal(err)};f.Add(seed)
 var legacy eventRecoveryManifest;if err:=json.Unmarshal(seed,&legacy);err!=nil {f.Fatal(err)};legacy.Coverage=nil
 for _,state:=range []string{"publishing","published","pin_failed"} {
  legacy.State=state
  if state=="pin_failed" {legacy.PinnedCount=1;legacy.FailureCode="link_failed";legacy.OutputTemporary="";legacy.OutputPublished=""}
  raw,err:=json.Marshal(&legacy);if err!=nil {f.Fatal(err)};f.Add(raw)
 }
 f.Add([]byte{});f.Add([]byte{0xff});f.Add(append(append([]byte(nil),seed...),[]byte("{}")...))
 f.Add(bytes.Repeat([]byte{' '},maxRecoveryManifestBytes+1))
 f.Add(append(append([]byte(nil),seed...),bytes.Repeat([]byte{' '},maxRecoveryManifestBytes-len(seed))...))
 f.Add(bytes.Replace(seed,[]byte(`"version":1`),[]byte(`"version":1,"version":1`),1))
 f.Fuzz(func(t *testing.T,data []byte){
  if len(data)>maxRecoveryManifestBytes {if _,err:=parseRecoveryManifest(data,"contract-camera");err==nil {t.Fatal("byte limit bypassed")};return}
  original:=append([]byte(nil),data...);manifest,err:=parseRecoveryManifest(data,"contract-camera")
  if !bytes.Equal(original,data) {t.Fatal("parser mutated input")}
  if err!=nil {if manifest!=nil {t.Fatal("rejected manifest exposed identity")};return}
  if manifest==nil || manifest.CameraID!="contract-camera" {t.Fatal("camera binding lost")}
  if a:=manifest.Coverage;a!=nil && (a.Complete || a.Status!="unknown" || !reflect.DeepEqual(a.Unknowns,eventCoverageUnknowns())) {t.Fatal("diagnostic gained authority")}
  canonical,err:=json.Marshal(manifest);if err!=nil {t.Fatal(err)}
  again,err:=parseRecoveryManifest(canonical,"contract-camera");if err!=nil {t.Fatal("accepted manifest failed round trip",err)}
  repeated,err:=json.Marshal(again);if err!=nil || !bytes.Equal(canonical,repeated) {t.Fatal("canonical representation unstable",err)}
 })
}
