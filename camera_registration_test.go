package main

import (
 "context"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func registrationFixture() *NVR {return &NVR{cameras:make(map[string]*Camera),cameraOrder:[]string{}}}
func registrationSources() []scannedStream {return []scannedStream{{Name:"front",RTSPURL:"rtsp://camera/front"},{Name:"back",RTSPURL:"rtsp://camera/back"}}}
func registrationIDs() func(string,time.Time)string {
 count:=0
 return func(string,time.Time)string{count++;return fmt.Sprintf("id_registration_%d",count)}
}
func registrationResult(t *testing.T,result <-chan error)error {
 t.Helper()
 select {case err:=<-result:return err;case <-time.After(3*time.Second):t.Fatal("registration did not join");return nil}
}

func TestCameraRegistrationExcludesOrphanCleanupThroughSave(t *testing.T) {
 for _,dedupe:=range []bool{false,true} {t.Run(fmt.Sprint(dedupe),func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir()
  created,finishCreate,saving,finishSave:=make(chan struct{}),make(chan struct{}),make(chan struct{}),make(chan struct{})
  var createOnce,saveOnce sync.Once
  var reads,removes atomic.Int32
  cleanupStarted:=false
  result:=make(chan error,1);registered:=make(chan struct{});cleaned:=make(chan struct{})
  ctx,cancel:=context.WithCancel(context.Background())
  t.Cleanup(func(){cancel();createOnce.Do(func(){close(finishCreate)});saveOnce.Do(func(){close(finishSave)});awaitShutdownStep(t,registered,"registration cleanup did not join");if cleanupStarted {awaitShutdownStep(t,cleaned,"orphan cleanup did not join")}})
  ops:=cameraRegistrationOps{mkdir:func(path string,mode os.FileMode)error{
   if err:=os.Mkdir(path,mode);err!=nil {return err}
   close(created);<-finishCreate;return nil
  },id:registrationIDs()}
  go func(){defer close(registered);result<-registerCameraSources(ctx,n,root,registrationSources()[:1],dedupe,ops,func()error{close(saving);<-finishSave;return nil})}()
  awaitShutdownStep(t,created,"directory creation not reached")
  requireRegistryAvailable(t,n)
  if recordingControlMu.TryLock() {recordingControlMu.Unlock();t.Fatal("creation not serialized with cleanup")}
  cleanupResponse:=httptest.NewRecorder()
  cleanupOps:=orphanCleanupIOFor(context.Background())
  cleanupOps.readDir=func(path string)([]os.DirEntry,error){reads.Add(1);return os.ReadDir(path)}
  cleanupOps.remove=func(path string)error{removes.Add(1);return os.Remove(path)}
  cleanupStarted=true
  go func(){defer close(cleaned);serveOrphanCleanup(cleanupResponse,httptest.NewRequest("POST","/api/recordings/cleanup-orphaned",nil),n,root,cleanupOps)}()
  if reads.Load()!=0 {t.Fatal("cleanup enumerated during unregistered creation")}
  createOnce.Do(func(){close(finishCreate)})
  awaitShutdownStep(t,saving,"registration save not reached")
  requireRegistryAvailable(t,n)
  if recordingControlMu.TryLock() {recordingControlMu.Unlock();t.Fatal("save lost registration serialization")}
  if reads.Load()!=0 {t.Fatal("cleanup entered before registration save completed")}
  saveOnce.Do(func(){close(finishSave)})
  if err:=registrationResult(t,result);err!=nil {t.Fatal(err)}
  awaitShutdownStep(t,cleaned,"cleanup did not run after registration")
  if cleanupResponse.Code!=200 || removes.Load()!=0 {t.Fatal("cleanup treated new camera as orphan",cleanupResponse.Code,removes.Load())}
  n.mu.RLock();cam:=n.cameras["id_registration_1"];n.mu.RUnlock()
  if cam==nil {t.Fatal("camera not committed")}
  if info,err:=os.Lstat(cam.RecordDir);err!=nil || !info.IsDir() {t.Fatal("new camera directory removed",err)}
 })}
}

func TestCameraRegistrationRefusalsPreserveCreatedDirectories(t *testing.T) {
 for _,phase:=range []string{"second-mkdir","cancel","shutdown","registry-replaced","registry-added","id-changed","path-changed","source-changed","save"} {t.Run(phase,func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir()
  original:=&Camera{ID:"existing",RTSPUrl:"rtsp://existing/live",RecordDir:filepath.Join(root,"existing")}
  n.cameras[original.ID]=original;n.cameraOrder=[]string{original.ID}
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  injected:=errors.New("injected failure")
  var replacement *Camera
  creates,saves:=0,0
  ops:=cameraRegistrationOps{id:registrationIDs(),mkdir:func(path string,mode os.FileMode)error{
   creates++
   requireRegistryAvailable(t,n)
   if phase=="second-mkdir" && creates==2 {return injected}
   if err:=os.Mkdir(path,mode);err!=nil {return err}
   if creates==1 {
    switch phase {
    case "cancel":cancel()
    case "shutdown":n.shuttingDown.Store(true)
    case "registry-replaced":replacement=&Camera{ID:"existing"};n.mu.Lock();n.cameras["existing"]=replacement;n.mu.Unlock()
    case "registry-added":n.mu.Lock();n.cameras["foreign"]=&Camera{ID:"foreign"};n.mu.Unlock()
    case "id-changed":n.mu.Lock();original.ID="changed";n.mu.Unlock()
    case "path-changed":n.mu.Lock();original.RecordDir=root+"-changed";n.mu.Unlock()
    case "source-changed":n.mu.Lock();original.RTSPUrl="rtsp://changed/live";n.mu.Unlock()
    }
   }
   return nil
  }}
  err:=registerCameraSources(ctx,n,root,registrationSources(),true,ops,func()error{saves++;requireRegistryAvailable(t,n);return injected})
  if err==nil {t.Fatal("injected failure accepted")}
  switch phase {
  case "second-mkdir":if !errors.Is(err,injected) || !errors.Is(err,errCameraRegistration) {t.Fatal(err)}
  case "cancel":if !errors.Is(err,context.Canceled) {t.Fatal(err)}
  case "shutdown":if !errors.Is(err,errRecorderShuttingDown) {t.Fatal(err)}
  case "registry-replaced","registry-added","id-changed","path-changed","source-changed":if !errors.Is(err,errRecordingStateChanged) {t.Fatal(err)}
  case "save":if !errors.Is(err,errCameraRegistrationPersistence) || !errors.Is(err,injected) {t.Fatal(err)}
  }
  want:=1;if phase=="save" {want=3};if phase=="registry-added" {want=2}
  if len(n.cameras)!=want || saves!=map[bool]int{false:0,true:1}[phase=="save"] {t.Fatal("partial commit or fake persistence rollback",len(n.cameras),saves)}
  if phase=="registry-replaced" && n.cameras["existing"]!=replacement {t.Fatal("replacement overwritten")}
  entries,readErr:=os.ReadDir(root);if readErr!=nil || len(entries)==0 {t.Fatal("created failure evidence removed",readErr)}
  for _,entry:=range entries {if !entry.IsDir() {t.Fatal("unexpected failure fixture")}}
 })}
}

func TestCameraRegistrationIdentityCollisionAndDirectoryRefusal(t *testing.T) {
 for _,kind:=range []string{"registered-id","existing-directory","symlink","pending-id","invalid-id"} {t.Run(kind,func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir()
  id:="id_registration"
  if kind=="registered-id" {n.cameras[id]=&Camera{ID:id,Name:"original"};n.cameraOrder=[]string{id}}
  if kind=="existing-directory" {if err:=os.Mkdir(filepath.Join(root,id),0700);err!=nil {t.Fatal(err)}}
  if kind=="symlink" {if err:=os.Symlink(t.TempDir(),filepath.Join(root,id));err!=nil {t.Fatal(err)}}
  if kind=="invalid-id" {id="../outside"}
  before:=recoveryContractTree(t,root)
  saves:=0
  sources:=registrationSources();if kind!="pending-id" {sources=sources[:1]}
  err:=registerCameraSources(context.Background(),n,root,sources,false,cameraRegistrationOps{os.Mkdir,func(string,time.Time)string{return id}},func()error{saves++;return nil})
  if !errors.Is(err,errCameraRegistration) || saves!=0 {t.Fatal("collision/path admitted",err,saves)}
  if kind!="pending-id" {
   if !reflect.DeepEqual(before,recoveryContractTree(t,root)) {t.Fatal("refusal changed existing directory evidence")}
  } else if info,err:=os.Stat(filepath.Join(root,id));err!=nil || !info.IsDir() {t.Fatal("first pending directory lost",err)}
  want:=0;if kind=="registered-id" {want=1;if n.cameras[id].Name!="original" {t.Fatal("registration overwritten")}}
  if len(n.cameras)!=want {t.Fatal("collision partially registered batch")}
 })}
}

func TestCameraRegistrationDeduplicationAndPersistenceOrdering(t *testing.T) {
 for _,dedupe:=range []bool{false,true} {t.Run(fmt.Sprint(dedupe),func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir();sources:=registrationSources();sources=append(sources,sources[0])
  creates,saves:=0,0
  ops:=cameraRegistrationOps{id:registrationIDs(),mkdir:func(path string,mode os.FileMode)error{creates++;requireRegistryAvailable(t,n);return os.Mkdir(path,mode)}}
  err:=registerCameraSources(context.Background(),n,root,sources,dedupe,ops,func()error{
   saves++;requireRegistryAvailable(t,n)
   if len(n.cameras)!=creates || len(n.cameraOrder)!=creates {t.Error("save preceded full registry commit")}
   return nil
  })
  want:=3;if dedupe {want=2}
  if err!=nil || creates!=want || len(n.cameras)!=want || saves!=1 {t.Fatal(err,creates,len(n.cameras),saves)}
  for _,cam:=range n.cameras {if cam.IsRecording || cam.RecordingMode!=ModeContinuous || cam.RetentionDays!=7 {t.Fatal("registration changed default recording policy")}}
 })}
}

func TestManualCameraRegistrationHTTPBoundary(t *testing.T) {
 cases:=[]struct{name,method,body,phase string;want int;success bool}{
  {"success","POST","camera-name=front&rtsp-url=rtsp%3A%2F%2Fcamera%2Flive","",200,true},
  {"method","GET","","",405,false},
  {"malformed","POST","camera-name=%zz","",400,false},
  {"missing","POST","camera-name=front","",400,false},
  {"shutdown","POST","camera-name=front&rtsp-url=rtsp%3A%2F%2Fcamera%2Flive","shutdown",503,false},
  {"canceled","POST","camera-name=front&rtsp-url=rtsp%3A%2F%2Fcamera%2Flive","cancel",408,false},
  {"save","POST","camera-name=front&rtsp-url=rtsp%3A%2F%2Fcamera%2Flive","save",500,false},
  {"save-canceled","POST","camera-name=front&rtsp-url=rtsp%3A%2F%2Fcamera%2Flive","save-canceled",500,false},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  if tc.phase=="shutdown" {n.shuttingDown.Store(true)};if tc.phase=="cancel" {cancel()}
  r:=httptest.NewRequest(tc.method,"/api/cameras/add",strings.NewReader(tc.body)).WithContext(ctx)
  r.Header.Set("Content-Type","application/x-www-form-urlencoded")
  w:=httptest.NewRecorder();saves:=0
  ok:=serveAddCameraAt(w,r,n,root,cameraRegistrationOps{os.Mkdir,registrationIDs()},func()error{saves++;if tc.phase=="save-canceled" {return context.Canceled};if tc.phase=="save" {return errors.New("private persistence details")};return nil})
  if ok!=tc.success || w.Code!=tc.want || strings.Contains(w.Body.String(),"private persistence details") {t.Fatal("HTTP registration boundary",ok,w.Code,w.Body.String())}
  want:=0;if tc.success || tc.phase=="save" || tc.phase=="save-canceled" {want=1}
  if saves!=want || len(n.cameras)!=want {t.Fatal("rejected request changed registry or saved",saves,len(n.cameras))}
 })}
}

func TestScanRegistrationSaveErrorIsNotReportedAsUpstreamFailure(t *testing.T) {
 source:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");_,_=w.Write([]byte(`{"front":{}}`))}));defer source.Close()
 n:=registrationFixture();root:=t.TempDir()
 values:=url.Values{"url":{source.URL}}
 r:=httptest.NewRequest("POST","/api/go2rtc/scan",strings.NewReader(values.Encode()));r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 w:=httptest.NewRecorder()
 ok:=serveGo2rtcScan(w,r,source.Client(),time.Second,func(ctx context.Context,streams []scannedStream)error{
  return registerCameraSources(ctx,n,root,streams,true,cameraRegistrationOps{os.Mkdir,registrationIDs()},func()error{return errors.New("private save error")})
 })
 if ok || w.Code!=500 || len(n.cameras)!=1 || strings.Contains(w.Body.String(),"private save error") {t.Fatal("scan save error lost committed-state semantics",ok,w.Code,len(n.cameras))}
}

func TestCameraRegistrationCommittedStateStillSavesAfterLateCancellation(t *testing.T) {
 for _,phase:=range []string{"cancel","shutdown","save-failure"} {t.Run(phase,func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir();ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  saves:=0
  err:=registerCameraSources(ctx,n,root,registrationSources(),true,cameraRegistrationOps{os.Mkdir,registrationIDs()},func()error{
   saves++
   if len(n.cameras)!=2 {t.Fatal("save entered before atomic registry commit")}
   requireRegistryAvailable(t,n)
   switch phase {case "cancel":cancel();case "shutdown":n.shuttingDown.Store(true);case "save-failure":return context.Canceled}
   return nil
  })
  if phase=="save-failure" {if !errors.Is(err,errCameraRegistrationPersistence) || !errors.Is(err,context.Canceled) {t.Fatal(err)}} else if err!=nil {t.Fatal(err)}
  if saves!=1 || len(n.cameras)!=2 || len(n.cameraOrder)!=2 {t.Fatal("committed state not preserved",saves,len(n.cameras))}
  for _,cam:=range n.cameras {if _,err:=os.Stat(cam.RecordDir);err!=nil {t.Fatal("committed directory removed",err)}}
 })}
}

func TestCameraRegistrationInvalidInputsDoNotCreateOrSave(t *testing.T) {
 for _,kind:=range []string{"count","root","mkdir","id","registry","name","source"} {t.Run(kind,func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir();sources:=registrationSources()
  creates,saves:=0,0
  ops:=cameraRegistrationOps{mkdir:func(string,os.FileMode)error{creates++;return nil},id:registrationIDs()}
  switch kind {
  case "count":sources=make([]scannedStream,scanStreamLimit+1)
  case "root":root="relative"
  case "mkdir":ops.mkdir=nil
  case "id":ops.id=nil
  case "registry":n=nil
  case "name":sources[0].Name=""
  case "source":sources[0].RTSPURL=""
  }
  err:=registerCameraSources(context.Background(),n,root,sources,true,ops,func()error{saves++;return nil})
  if !errors.Is(err,errCameraRegistration) || creates!=0 || saves!=0 {t.Fatal("invalid registration performed I/O",err,creates,saves)}
 })}
}

func TestScanRegistrationSkipsExistingURLsAndStillPersists(t *testing.T) {
 for _,noNew:=range []bool{false,true} {t.Run(fmt.Sprint(noNew),func(t *testing.T){
  n:=registrationFixture();root:=t.TempDir();sources:=registrationSources()
  original:=&Camera{ID:"existing",Name:"original",RTSPUrl:sources[0].RTSPURL,RecordDir:filepath.Join(root,"existing")}
  n.cameras[original.ID]=original;n.cameraOrder=[]string{original.ID}
  if noNew {sources=sources[:1]}
  creates,saves:=0,0
  err:=registerCameraSources(context.Background(),n,root,sources,true,cameraRegistrationOps{mkdir:func(path string,mode os.FileMode)error{creates++;return os.Mkdir(path,mode)},id:registrationIDs()},func()error{saves++;return nil})
  want:=1;if noNew {want=0}
  if err!=nil || creates!=want || saves!=1 || len(n.cameras)!=want+1 || n.cameras[original.ID]!=original || original.Name!="original" {t.Fatal("scan deduplication changed existing registration",err,creates,saves)}
 })}
}

func TestCameraRegistrationRechecksCancellationAfterControlWait(t *testing.T) {
 n:=registrationFixture();root:=t.TempDir();ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 recordingControlMu.Lock()
 var unlock sync.Once
 done:=make(chan struct{});started:=make(chan struct{});result:=make(chan error,1)
 t.Cleanup(func(){unlock.Do(recordingControlMu.Unlock);awaitShutdownStep(t,done,"canceled registration cleanup did not join")})
 var creates,saves atomic.Int32
 go func(){
  defer close(done);close(started)
  result<-registerCameraSources(ctx,n,root,registrationSources(),true,cameraRegistrationOps{mkdir:func(string,os.FileMode)error{creates.Add(1);return nil},id:registrationIDs()},func()error{saves.Add(1);return nil})
 }()
 awaitShutdownStep(t,started,"registration contender did not start")
 cancel()
 unlock.Do(recordingControlMu.Unlock)
 if err:=registrationResult(t,result);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if creates.Load()!=0 || saves.Load()!=0 || len(n.cameras)!=0 {t.Fatal("canceled control waiter mutated registration")}
}
