package main

import (
 "bytes"
 "html/template"
 "strings"
 "testing"
)

func TestCameraTemplateRecordingStatus(t *testing.T) {
 tmpl,err:=template.ParseFiles("web/templates/cameras.html")
 if err!=nil {t.Fatal(err)}
 failure:=continuousRecordingNotice{Code:"upstream_rtsp_unavailable",Message:"Could not open upstream stream",Detail:"DESCRIBE 404",PublicationMessage:"Check recording storage",Retrying:true}
 notRetrying:=failure;notRetrying.Retrying=false
 for _,tc:=range []struct {
  name string
  enabled bool
  mode RecordingMode
  health continuousRecordingNotice
  status,class string
  failed bool
 }{
  {"continuous enabled",true,ModeContinuous,continuousRecordingNotice{},"Recording enabled","status-recording",false},
  {"continuous failed retrying",true,ModeContinuous,failure,"Recording failed — retrying","status-failed recording",true},
  {"continuous failed without retry claim",true,ModeContinuous,notRetrying,"Recording failed","status-failed recording",true},
  {"stopped",false,ModeContinuous,continuousRecordingNotice{},"Recording stopped","status-idle",false},
  {"stopped ignores stale failure",false,ModeContinuous,failure,"Recording stopped","status-idle",false},
  {"event enabled unchanged",true,ModeEventBased,continuousRecordingNotice{},"Recording enabled","status-recording",false},
  {"event ignores continuous failure",true,ModeEventBased,failure,"Recording enabled","status-recording",false},
  {"no recording",false,ModeNoRecording,failure,"Recording stopped","status-idle",false},
 } {t.Run(tc.name,func(t *testing.T){
  view:=map[string]interface{}{
   "ID":"test","Name":"Test camera","RTSPUrl":"rtsp://example.invalid/live",
   "RecordDir":"/recordings/test","DiskUsage":"0 B","IsRecording":tc.enabled,
   "RecordingMode":tc.mode,"ModeDisplay":string(tc.mode),"RetentionDisplay":7,"RecordingHealth":tc.health,
  }
  var out bytes.Buffer
  if err:=tmpl.ExecuteTemplate(&out,"cameras.html",[]map[string]interface{}{view});err!=nil {t.Fatal(err)}
  body:=out.String()
  action,absentAction:="Start Recording","Stop Recording"
  if tc.enabled {action,absentAction=absentAction,action}
  for _,want:=range []string{`class="`+tc.class+`">`+tc.status+`</span>`,">"+action+"</button>"} {
   if !strings.Contains(body,want) {t.Fatalf("missing %q in %s",want,body)}
  }
  for _,absent:=range []string{"Recording state reflects intent", "camera-status-explanation",">Recording</span>",">Idle</span>",">"+absentAction+"</button>"} {
   if strings.Contains(body,absent) {t.Fatalf("unexpected %q",absent)}
  }
  if tc.failed {
   for _,want:=range []string{"Recording setting: Enabled",tc.health.Message,tc.health.Detail,tc.health.PublicationMessage} {
    if !strings.Contains(body,want) {t.Fatalf("missing failure context %q",want)}
   }
   if strings.Contains(body,">Recording enabled</span>") {t.Fatal("failure masked by enabled primary status")}
  } else {
   for _,absent:=range []string{"Recording failed","camera-recording-error","Recording setting: Enabled"} {
    if strings.Contains(body,absent) {t.Fatalf("unexpected failure context %q",absent)}
   }
  }
 })}
}

func TestCameraTemplateRecordingRecoveryAndEscaping(t *testing.T) {
 tmpl,err:=template.ParseFiles("web/templates/cameras.html");if err!=nil {t.Fatal(err)}
 view:=map[string]interface{}{
  "ID":"test","Name":"Test camera","RTSPUrl":"","RecordDir":"/recordings/test","DiskUsage":"0 B",
  "IsRecording":true,"RecordingMode":ModeContinuous,"ModeDisplay":"Continuous","RetentionDisplay":7,
  "RecordingHealth":continuousRecordingNotice{Code:"failed",Message:`<script>alert("message")</script>`,Detail:`<img src=x onerror=alert(1)>`,PublicationMessage:`<svg onload=alert(1)>`,Retrying:true},
 }
 render:=func()string {t.Helper();var out bytes.Buffer;if err:=tmpl.ExecuteTemplate(&out,"cameras.html",[]map[string]interface{}{view});err!=nil {t.Fatal(err)};return out.String()}
 failed:=render()
 for _,raw:=range []string{"<script>","<img ","<svg "} {if strings.Contains(failed,raw) {t.Fatalf("unescaped diagnostic: %s",failed)}}
 if !strings.Contains(failed,"&lt;script&gt;") || !strings.Contains(failed,"Recording failed — retrying") {t.Fatal("escaped failure context missing")}
 view["RecordingHealth"]=continuousRecordingNotice{}
 recovered:=render()
 if !strings.Contains(recovered,`class="status-recording">Recording enabled</span>`) || strings.Contains(recovered,"Recording failed") || strings.Contains(recovered,"camera-recording-error") {t.Fatal("cleared health did not restore enabled status")}
 if !strings.Contains(recovered,">Stop Recording</button>") {t.Fatal("recovery changed recording control")}
}
