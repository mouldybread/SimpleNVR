package main

import (
 "context"
 "encoding/json"
 "io/fs"
 "net/http/httptest"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)

func eventStorageTree(t *testing.T,root string) map[string]string {
 t.Helper()
 result:=map[string]string{}
 err:=filepath.WalkDir(root,func(path string,entry fs.DirEntry,walkErr error) error {
  if walkErr!=nil {return walkErr}
  relative,err:=filepath.Rel(root,path);if err!=nil {return err}
  if entry.IsDir() {result[relative+"/"]="";return nil}
  raw,err:=os.ReadFile(path);if err!=nil {return err};result[relative]=string(raw);return nil
 })
 if err!=nil {t.Fatal(err)}
 return result
}

func eventStorageRequest() string {
 return `{"camera_name":"Front","label":"Motion","start_time":"2026-11-10T12:00:00Z","duration_seconds":30}`
}

func TestEventStorageRejectionHasNoAdmissionSideEffects(t *testing.T) {
 for _,item:=range []struct{name,policy string;missing bool;status int}{
  {"low","18446744073709551615",false,507},{"invalid","bad",false,503},{"unavailable","1",true,503},
 } {t.Run(item.name,func(t *testing.T){
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES",item.policy)
  root:=t.TempDir();dir:=root;if item.missing {dir=filepath.Join(root,"missing")}
  if err:=os.MkdirAll(filepath.Join(root,"temp","retained"),0700);err!=nil {t.Fatal(err)}
  for _,path:=range []string{filepath.Join(root,"old.mp4.tmp"),filepath.Join(root,"temp","retained","pre_000.mp4")} {if err:=os.WriteFile(path,[]byte("recovery evidence"),0600);err!=nil {t.Fatal(err)}}
  cam:=&Camera{ID:"a",Name:"Front",RecordDir:dir,IsRecording:true,RecordingMode:ModeEventBased}
  bm:=newBufferManager(context.Background(),cam);t.Cleanup(bm.cancel);cam.bufferManager=bm
  source:=filepath.Join(root,"buffer","closed.mp4");if err:=os.MkdirAll(filepath.Dir(source),0700);err!=nil {t.Fatal(err)}
  if err:=os.WriteFile(source,[]byte(strings.Repeat("x",2048)),0600);err!=nil {t.Fatal(err)}
  now:=time.Now().UTC();bm.registerCompleted(source,now.Add(-5*time.Second),now)
  n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}}
  before:=eventStorageTree(t,root);saved:=0
  w:=httptest.NewRecorder();triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(eventStorageRequest())),n,func() error {saved++;return nil})
  if w.Code!=item.status {t.Fatalf("status %d: %s",w.Code,w.Body.String())}
  if len(n.events)!=0 || saved!=0 || len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("rejected capture changed metadata, queued work or consumed capacity")}
  if !reflect.DeepEqual(before,eventStorageTree(t,root)) {t.Fatal("rejected capture created staging or changed recovery artifacts")}
  if strings.Contains(w.Body.String(),root) {t.Fatal("response exposed a filesystem path")}
 })}
}

func TestEventStorageAdmissionAllowsDisabledAndSufficientPolicies(t *testing.T) {
 for _,policy:=range []string{"","0","1"} {t.Run("policy="+policy,func(t *testing.T){
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES",policy)
  cam:=&Camera{ID:"a",Name:"Front",RecordDir:t.TempDir(),IsRecording:true,RecordingMode:ModeEventBased}
  bm:=newBufferManager(context.Background(),cam);defer bm.cancel();cam.bufferManager=bm
  n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}};saved:=0
  w:=httptest.NewRecorder();triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(eventStorageRequest())),n,func() error {saved++;return nil})
  if len(bm.eventChan)>0 {job:=<-bm.eventChan;defer bm.releaseJob(job)}
  if w.Code!=200 {t.Fatalf("status %d: %s",w.Code,w.Body.String())}
  var response struct {Success bool `json:"success"`;Queued bool `json:"recording_queued"`}
  if err:=json.Unmarshal(w.Body.Bytes(),&response);err!=nil {t.Fatal(err)}
  if !response.Success || !response.Queued || len(n.events)!=1 || saved!=1 || len(bm.slots)!=1 {t.Fatal("admitted capture did not retain existing asynchronous behavior")}
 })}
}

func TestMetadataOnlyEventLoggingBypassesCaptureStorageGuard(t *testing.T) {
 for _,mode:=range []RecordingMode{ModeNoRecording,ModeContinuous,ModeEventBased} {t.Run(string(mode),func(t *testing.T){
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES","bad")
  cam:=&Camera{ID:"a",Name:"Front",RecordDir:filepath.Join(t.TempDir(),"missing"),RecordingMode:mode,IsRecording:mode==ModeContinuous}
  n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}};saved:=0
  w:=httptest.NewRecorder();triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(eventStorageRequest())),n,func() error {saved++;return nil})
  if w.Code!=200 || len(n.events)!=1 || saved!=1 {t.Fatalf("metadata-only logging rejected: %d %s",w.Code,w.Body.String())}
  var response struct {Queued bool `json:"recording_queued"`}
  if err:=json.Unmarshal(w.Body.Bytes(),&response);err!=nil {t.Fatal(err)}
  if response.Queued {t.Fatal("metadata-only event claimed a queued recording")}
 })}
}
