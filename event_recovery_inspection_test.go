package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func recoveryInspectionFixture(t *testing.T) (string,string,[]byte) {
 t.Helper()
 root:=t.TempDir();id:="event-fixture";dir:=filepath.Join(root,"temp",id)
 if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 m:=eventRecoveryManifest{Version:1,Revision:2,CameraID:"a",EventID:"event-a",RequestedStart:now,RequestedEnd:now.Add(30*time.Second),DurationSeconds:30,CreatedAt:now,ObservedAt:now,State:"pinned",Inputs:[]eventRecoveryInput{},CaptureName:"event.mp4"}
 data,err:=json.Marshal(m);if err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(dir,data);err!=nil {t.Fatal(err)}
 return root,id,data
}

func TestStrictRecoveryParserRejectsAmbiguityAndInvalidSchema(t *testing.T) {
 _,_,data:=recoveryInspectionFixture(t)
 if _,err:=parseRecoveryManifest(data,"a");err!=nil {t.Fatal(err)}
 if _,err:=parseRecoveryManifest(data,"other");err==nil {t.Fatal("cross-camera manifest accepted")}
 invalid:=[][]byte{nil,append(append([]byte{},data...),[]byte("{}")...),bytes.Replace(data,[]byte(`"version":1`),[]byte(`"version":1,"version":1`),1),make([]byte,maxRecoveryManifestBytes+1),[]byte("\xff")}
 for _,mutation:=range []func(map[string]json.RawMessage){
  func(m map[string]json.RawMessage){m["version"]=json.RawMessage(`2`)},
  func(m map[string]json.RawMessage){m["revision"]=json.RawMessage(`0`)},
  func(m map[string]json.RawMessage){m["revision"]=json.RawMessage(`1000001`)},
  func(m map[string]json.RawMessage){m["Version"]=json.RawMessage(`1`)},
  func(m map[string]json.RawMessage){delete(m,"partial")},
  func(m map[string]json.RawMessage){m["partial"]=json.RawMessage(`null`)},
  func(m map[string]json.RawMessage){m["prebuffer_inputs"]=json.RawMessage(`null`)},
  func(m map[string]json.RawMessage){m["state"]=json.RawMessage(`"recovered"`)},
  func(m map[string]json.RawMessage){m["duration_seconds"]=json.RawMessage(`31`)},
  func(m map[string]json.RawMessage){m["capture_name"]=json.RawMessage(`"../event.mp4"`)},
  func(m map[string]json.RawMessage){m["pinned_count"]=json.RawMessage(`1`)},
  func(m map[string]json.RawMessage){m["failure_code"]=json.RawMessage(`"invented"`)},
  func(m map[string]json.RawMessage){m["output_temporary"]=json.RawMessage(`"../output.mp4.tmp"`)},
 } {
  var raw map[string]json.RawMessage;if err:=json.Unmarshal(data,&raw);err!=nil {t.Fatal(err)};mutation(raw)
  changed,err:=json.Marshal(raw);if err!=nil {t.Fatal(err)};invalid=append(invalid,changed)
 }
 for i,item:=range invalid {if _,err:=parseRecoveryManifest(item,"a");err==nil {t.Fatalf("invalid case %d accepted",i)}}
 var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
 m.Inputs=[]eventRecoveryInput{{Name:"pre_000.mp4",Start:m.CreatedAt,End:m.CreatedAt.Add(time.Second)}};m.PinnedCount=1
 nested,err:=json.Marshal(m);if err!=nil {t.Fatal(err)}
 nested=bytes.Replace(nested,[]byte(`"name":"pre_000.mp4"`),[]byte(`"name":"pre_000.mp4","name":"pre_000.mp4"`),1)
 if _,err:=parseRecoveryManifest(nested,"a");err==nil {t.Fatal("nested duplicate accepted")}
}

func TestRecoveryReaderAcceptsWriterStatesAndPublicationIntent(t *testing.T) {
 bm:=bufferFixture(t);job,err:=bm.pinBuffer(bufferTestEvent("writer-roundtrip"));if err!=nil {t.Fatal(err)}
 for _,state:=range []string{"pinning","pinned","capturing","captured","assembling","capture_failed","publishing","published","publication_failed"} {
  job.recovery.FailureCode="";job.recovery.OutputTemporary="";job.recovery.OutputPublished="";job.recovery.Partial=false
  code:=""
  if state=="capture_failed" {code="process_failed_or_canceled"}
  if state=="publishing" || state=="published" || state=="publication_failed" {job.recovery.OutputTemporary="output.mp4.tmp";job.recovery.OutputPublished="output_partial.mp4";job.recovery.Partial=true}
  if state=="publication_failed" {code="validation_or_publication_failed"}
  if err:=job.recoveryCheckpoint(state,code);err!=nil {t.Fatal(err)}
  data,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
  if _,err:=parseRecoveryManifest(data,bm.camera.ID);err!=nil {t.Fatalf("writer state %s rejected: %v",state,err)}
 }
}

func TestRecoveryInspectionIsReadOnlyAndNeverCertifiesMedia(t *testing.T) {
 root,id,data:=recoveryInspectionFixture(t);dir:=filepath.Join(root,"temp",id)
 evidence:=[]byte("not necessarily valid media")
 if err:=os.WriteFile(filepath.Join(dir,"event.mp4"),evidence,0600);err!=nil {t.Fatal(err)}
 report:=inspectEventRecovery(context.Background(),root,"a",id)
 if !report.Complete || report.Classification!="inflight_or_interrupted" || report.EventID!="event-a" || len(report.Artifacts)!=2 || !report.Artifacts[0].Present || report.Artifacts[1].Present {t.Fatalf("bad inspection: %+v",report)}
 got,err:=os.ReadFile(filepath.Join(dir,"recovery.json"));if err!=nil || !bytes.Equal(got,data) {t.Fatal("manifest changed",err)}
 got,err=os.ReadFile(filepath.Join(dir,"event.mp4"));if err!=nil || !bytes.Equal(got,evidence) {t.Fatal("media changed",err)}
 entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=2 {t.Fatal("inspection created artifacts",err)}
 encoded,err:=json.Marshal(report);if err!=nil {t.Fatal(err)}
 if strings.Contains(string(encoded),root) || strings.Contains(string(encoded),"event.mp4") || strings.Contains(string(encoded),"valid_media") {t.Fatal("paths or invented validity leaked")}
 if err:=os.Remove(filepath.Join(dir,"recovery.json"));err!=nil {t.Fatal(err)}
 report=inspectEventRecovery(context.Background(),root,"a",id)
 if !report.Complete || report.Classification!="legacy_or_missing_manifest" || report.Artifacts!=nil || report.EventID!="" {t.Fatal("missing manifest invented identity/usage")}
}

func TestRecoveryInspectionRejectsSymlinksSpecialFilesAndCancellation(t *testing.T) {
 for _,kind:=range []string{"manifest_symlink","manifest_fifo","manifest_directory","artifact_symlink","temp_symlink","staging_symlink"} {
  t.Run(kind,func(t *testing.T){
   root,id,_:=recoveryInspectionFixture(t);dir:=filepath.Join(root,"temp",id);manifest:=filepath.Join(dir,"recovery.json")
   switch kind {
   case "manifest_symlink","manifest_fifo","manifest_directory":
    if err:=os.Remove(manifest);err!=nil {t.Fatal(err)}
    var err error
    if kind=="manifest_symlink" {err=os.Symlink(filepath.Join(t.TempDir(),"outside"),manifest)} else if kind=="manifest_fifo" {err=unix.Mkfifo(manifest,0600)} else {err=os.Mkdir(manifest,0700)}
    if err!=nil {t.Fatal(err)}
   case "artifact_symlink":if err:=os.Symlink(filepath.Join(t.TempDir(),"outside"),filepath.Join(dir,"event.mp4"));err!=nil {t.Fatal(err)}
   case "temp_symlink":if err:=os.Rename(filepath.Join(root,"temp"),filepath.Join(root,"saved"));err!=nil {t.Fatal(err)};if err:=os.Symlink(filepath.Join(root,"saved"),filepath.Join(root,"temp"));err!=nil {t.Fatal(err)}
   case "staging_symlink":if err:=os.Rename(dir,dir+"-saved");err!=nil {t.Fatal(err)};if err:=os.Symlink(dir+"-saved",dir);err!=nil {t.Fatal(err)}
   }
   report:=inspectEventRecovery(context.Background(),root,"a",id);if report.Complete || len(report.Errors)==0 {t.Fatalf("unsafe %s accepted",kind)}
  })
 }
 root,id,_:=recoveryInspectionFixture(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
 if report:=inspectEventRecovery(ctx,root,"a",id);report.Complete {t.Fatal("cancellation ignored")}
 for _,name:=range []string{"../event-fixture","/event-fixture","event-","event-x/y"} {if report:=inspectEventRecovery(context.Background(),root,"a",name);report.Complete {t.Fatal("invalid staging name accepted")}}
}

func TestRecoveryStatusValidationSaturationAndRegistryChange(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:"unused"}}};slots:=make(chan struct{},2);calls:=0
 inspect:=func(ctx context.Context,_,_,_ string) eventRecoveryInspection {calls++;if _,ok:=ctx.Deadline();!ok {t.Error("missing deadline")};return eventRecoveryInspection{Complete:true,Classification:"inflight_or_interrupted"}}
 for _,tc:=range []struct{method,query string;status int}{{"POST","camera_id=a&staging_id=event-x",405},{"GET","",400},{"GET","camera_id=a",400},{"GET","camera_id=a&staging_id=../x",400},{"GET","camera_id=a&staging_id=event-x&staging_id=event-y",400},{"GET","camera_id=a&staging_id=event-x&extra=1",400},{"GET","camera_id=%zz&staging_id=event-x",400},{"GET","camera_id=missing&staging_id=event-x",404}} {
  r:=httptest.NewRequest(tc.method,"/api/recovery/status",nil);r.URL.RawQuery=tc.query;w:=httptest.NewRecorder();recoveryStatusHandlerWithState(w,r,n,inspect,slots)
  if w.Code!=tc.status {t.Fatalf("%q: %d",tc.query,w.Code)}
 }
 if calls!=0 {t.Fatal("invalid request inspected disk")}
 slots<-struct{}{};slots<-struct{}{};w:=httptest.NewRecorder()
 request:=func()*http.Request{return httptest.NewRequest("GET","/api/recovery/status?camera_id=a&staging_id=event-x",nil)}
 recoveryStatusHandlerWithState(w,request(),n,inspect,slots);if w.Code!=503 || calls!=0 {t.Fatal("capacity bound ignored")};<-slots;<-slots
 w=httptest.NewRecorder();recoveryStatusHandlerWithState(w,request(),n,inspect,slots)
 if w.Code!=200 || len(slots)!=0 || w.Header().Get("Cache-Control")!="no-store" {t.Fatal("slot/header/success failure")}
 changed:=func(context.Context,string,string,string) eventRecoveryInspection {n.mu.Lock();n.cameras["a"]=&Camera{ID:"a",RecordDir:"replacement"};n.mu.Unlock();return eventRecoveryInspection{Complete:true}}
 w=httptest.NewRecorder();recoveryStatusHandlerWithState(w,request(),n,changed,slots);if w.Code!=503 {t.Fatal("camera replacement ignored")}
}

func TestRecoveryStatusRequiresSessionNotEventBearer(t *testing.T) {
 auth,_:=authFixture(t);n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a"}}};calls:=0
 h:=auth.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){recoveryStatusHandlerWithState(w,r,n,func(context.Context,string,string,string)eventRecoveryInspection{calls++;return eventRecoveryInspection{Complete:true}},make(chan struct{},2))}))
 for _,bearer:=range []string{"","Bearer 0123456789012345678901234567890123456789012"} {
  r:=httptest.NewRequest("GET","https://nvr.test/api/recovery/status?camera_id=a&staging_id=event-x",nil);r.Header.Set("Authorization",bearer);w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=401 {t.Fatal("sessionless inspection accepted",w.Code)}
 }
 if calls!=0 {t.Fatal("unauthenticated filesystem access")}
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 r:=httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()));r.Header.Set("Content-Type","application/x-www-form-urlencoded");login:=httptest.NewRecorder();h.ServeHTTP(login,r)
 if login.Code!=303 {t.Fatal("login failed",login.Code)}
 r=httptest.NewRequest("GET","https://nvr.test/api/recovery/status?camera_id=a&staging_id=event-x",nil);for _,cookie:=range login.Result().Cookies(){r.AddCookie(cookie)}
 w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=200 || calls!=1 {t.Fatal("session inspection failed",w.Code)}
}

func TestShutdownJoinsRecoveryInspectionBeforePersistence(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a"}}};slots:=make(chan struct{},2)
 entered,canceled,release,requestDone:=make(chan struct{}),make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 inspect:=func(ctx context.Context,_,_,_ string)eventRecoveryInspection{close(entered);<-ctx.Done();close(canceled);<-release;return eventRecoveryInspection{Errors:[]string{"inspection_canceled"}}}
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){recoveryStatusHandlerWithState(w,r,n,inspect,slots)}))
 go func(){defer close(requestDone);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/recovery/status?camera_id=a&staging_id=event-x",nil))}()
 awaitShutdownStep(t,entered,"recovery inspection not entered")
 var persisted atomic.Bool;done:=make(chan error,1);ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 go func(){done<-shutdownApplication(ctx,n,gate,nil,func()error{persisted.Store(true);return nil})}()
 awaitShutdownStep(t,canceled,"recovery inspection not canceled");if persisted.Load(){t.Fatal("persisted before inspection joined")}
 once.Do(func(){close(release)});awaitShutdownStep(t,requestDone,"recovery request not joined")
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("shutdown did not complete")}
 if !persisted.Load() || len(slots)!=0 {t.Fatal("join/slot release failed")}
}

func TestRecoveryInspectionPublicationStateIsNotMediaValidity(t *testing.T) {
 for _,state:=range []string{"publishing","published"} {
  root,id,data:=recoveryInspectionFixture(t);dir:=filepath.Join(root,"temp",id)
  var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
  m.State=state;m.Partial=true;m.OutputTemporary="planned.mp4.tmp";m.OutputPublished="planned_partial.mp4"
  changed,err:=json.Marshal(m);if err!=nil {t.Fatal(err)};if err:=writeEventRecovery(dir,changed);err!=nil {t.Fatal(err)}
  report:=inspectEventRecovery(context.Background(),root,"a",id)
  expected:="publication_outcome_ambiguous";if state=="published" {expected="published_checkpoint_unverified"}
  if !report.Complete || report.Classification!=expected || report.Partial==nil || !*report.Partial || len(report.Artifacts)!=4 || report.Artifacts[3].Present {t.Fatalf("publication inference unsafe: %+v",report)}
 }
}

func TestRecoverySnapshotIdentityDetectsChanges(t *testing.T) {
 var before unix.Stat_t;before.Dev=1;before.Ino=2;before.Mode=unix.S_IFREG|0600;before.Size=10
 after:=before;if !sameRecoverySnapshot(&before,&after) {t.Fatal("equal metadata differs")}
 after.Ino++;if sameRecoverySnapshot(&before,&after) {t.Fatal("inode replacement ignored")}
 after=before;after.Size++;if sameRecoverySnapshot(&before,&after) {t.Fatal("growth ignored")}
 after=before;after.Mtim.Nsec++;if sameRecoverySnapshot(&before,&after) {t.Fatal("mtime change ignored")}
}
