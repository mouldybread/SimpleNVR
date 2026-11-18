package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

// This checks both readers and writers while the injected filesystem phase
// is blocked. It does not claim to interrupt a real blocked kernel syscall.
func requireRegistryAvailable(t *testing.T,n *NVR) {
 t.Helper();done:=make(chan struct{})
 go func(){n.mu.RLock();n.mu.RUnlock();n.mu.Lock();n.mu.Unlock();close(done)}()
 select {case <-done:case <-time.After(2*time.Second):t.Fatal("registry lock held across filesystem phase")}
}

func TestAdmissionFilesystemPhaseDoesNotHoldRegistryAndShutdownPreservesIntent(t *testing.T) {
 for _,phase:=range []string{"start","resume","mode-first","mode-second"} {t.Run(phase,func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous)
  var starts atomic.Int32
  started:=make(chan struct{})
  worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();if starts.Add(1)==1 {close(started)};<-ctx.Done()}
  defer n.cancelAndWaitRecorders()
  if phase=="resume" {n.mu.Lock();cam.IsRecording=true;n.mu.Unlock()}
  if phase=="mode-first" || phase=="mode-second" {
   if err:=n.transitionRecording(cam.ID,"start","",worker);err!=nil {t.Fatal(err)}
   waitHTTPJob(t,started,"old recorder did not start")
  }
  entered,release:=make(chan struct{}),make(chan struct{});var once sync.Once
  defer once.Do(func(){close(release)})
  calls:=0;blockAt:=1;if phase=="mode-second" {blockAt=2}
  check:=func(string) error {calls++;if calls==blockAt {close(entered);<-release};return nil}
  done:=make(chan error,1)
  go func(){switch phase {
  case "resume":done<-n.resumeRecordingsWithAdmission(worker,check)
  case "start":done<-n.transitionRecordingWithAdmission(cam.ID,"start","",worker,check)
  default:done<-n.transitionRecordingWithAdmission(cam.ID,"mode",ModeEventBased,worker,check)
  }}()
  waitHTTPJob(t,entered,"admission phase not reached")
  requireRegistryAvailable(t,n)
  n.shuttingDown.Store(true)
  joined:=make(chan struct{});go func(){n.cancelAndWaitRecorders();close(joined)}()
  waitHTTPJob(t,joined,"recorder shutdown could not access registry during admission")
  once.Do(func(){close(release)})
  select {case err:=<-done:if !errors.Is(err,errRecorderShuttingDown) {t.Fatal("late admission was not rejected",err)};case <-time.After(2*time.Second):t.Fatal("control did not finish")}
  expectedStarts:=int32(0);if phase=="mode-first" || phase=="mode-second" {expectedStarts=1}
  if starts.Load()!=expectedStarts {t.Fatal("replacement recorder launched during shutdown")}
  n.mu.RLock();intent:=cam.IsRecording;cancel:=cam.recordCancel;n.mu.RUnlock()
  expectedIntent:=phase!="start"
  if intent!=expectedIntent || cancel!=nil {t.Fatal("shutdown changed restart intent or left runtime cancel",intent,expectedIntent,cancel!=nil)}
 })}
}

func TestAdmissionRevalidatesDirectoryAndRegistryIdentity(t *testing.T) {
 for _,replace:=range []bool{false,true} {t.Run(map[bool]string{false:"directory",true:"identity"}[replace],func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);starts:=0
  worker:=func(_ context.Context,cam *Camera,_ RecordingMode){starts++;cam.recordWG.Done()}
  check:=func(string) error {n.mu.Lock();defer n.mu.Unlock();if replace {n.cameras[cam.ID]=&Camera{ID:cam.ID,RecordingMode:ModeContinuous}} else {cam.RecordDir="changed"};return nil}
  err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,check)
  expected:=errRecordingStateChanged;if replace {expected=errCameraMissing}
  if !errors.Is(err,expected) || starts!=0 {t.Fatal("stale admission launched a worker",err,starts)}
 })}
}

func TestModePreflightFailureLeavesOldWorkerRunningAndSecondFailureJoinsIt(t *testing.T) {
 for _,failAt:=range []int{1,2} {t.Run(map[int]string{1:"preflight",2:"replacement"}[failAt],func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);started,stopped:=make(chan struct{}),make(chan struct{})
  worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();close(started);<-ctx.Done();close(stopped)}
  defer n.cancelAndWaitRecorders()
  if err:=n.transitionRecording(cam.ID,"start","",worker);err!=nil {t.Fatal(err)}
  waitHTTPJob(t,started,"old recorder did not start")
  calls:=0;check:=func(string) error {requireRegistryAvailable(t,n);calls++;if calls==failAt {return errStorageReserve};return nil}
  err:=n.transitionRecordingWithAdmission(cam.ID,"mode",ModeEventBased,worker,check)
  if !errors.Is(err,errStorageReserve) || calls!=failAt {t.Fatal("admission error lost",err,calls)}
  n.mu.RLock();active,mode:=cam.IsRecording,cam.RecordingMode;n.mu.RUnlock()
  if failAt==1 {if !active || mode!=ModeContinuous {t.Fatal("preflight failure changed old recorder")};select {case <-stopped:t.Fatal("old recorder canceled before preflight passed");default:}} else {waitHTTPJob(t,stopped,"old recorder was not joined");if active || mode!=ModeEventBased {t.Fatal("failed replacement admission reported active")}}
 })}
}

func TestPublishedDeletionReleasesRegistryAndReportsPartialFailures(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.RecordDir=t.TempDir()
 entered,release:=make(chan struct{}),make(chan struct{});var once sync.Once;defer once.Do(func(){close(release)})
 w:=httptest.NewRecorder();done:=make(chan struct{})
 go func(){defer close(done);serveDeletePublishedRecordings(w,httptest.NewRequest("POST","/api/recordings/delete-all",nil),n,func(context.Context,string) publishedDeleteResult {close(entered);<-release;return publishedDeleteResult{Count:1,Bytes:1048576,Failures:2}})}()
 waitHTTPJob(t,entered,"deletion phase not reached");requireRegistryAvailable(t,n)
 once.Do(func(){close(release)});waitHTTPJob(t,done,"deletion did not return")
 var result struct {Success bool `json:"success"`;Deleted int `json:"deleted"`;SizeMB float64 `json:"size_mb"`;Failures int `json:"failures"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&result);err!=nil {t.Fatal(err)}
 if w.Code!=200 || result.Success || result.Deleted!=1 || result.SizeMB!=1 || result.Failures!=2 {t.Fatal("partial deletion was reported as complete",w.Code,result)}
}

func TestPublishedDeletionPreservesStagingSpecialEntriesAndCountsOnlySuccessfulUnlinks(t *testing.T) {
 dir,outside:=t.TempDir(),t.TempDir()
 fixtures:=map[string]string{"a.mp4":"deleted","b.mp4":"keep on error","active.mp4.tmp":"capture","temp/event/input.mp4":"event input","buffer/pre.mp4":"prebuffer"}
 for name,data:=range fixtures {putCleanupFixture(t,filepath.Join(dir,name),data)}
 target:=filepath.Join(outside,"target.mp4");putCleanupFixture(t,target,"outside")
 if err:=os.Symlink(target,filepath.Join(dir,"c.mp4"));err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(dir,"d.mp4"),0700);err!=nil {t.Fatal(err)}
 result:=deletePublishedRecordingsWithRemove(context.Background(),dir,func(path string) error {if filepath.Base(path)=="b.mp4" {return os.ErrPermission};return os.Remove(path)})
 if result.Count!=1 || result.Bytes!=int64(len(fixtures["a.mp4"])) || result.Failures!=1 {t.Fatal("unsuccessful bytes counted or error hidden",result)}
 for name,data:=range fixtures {if name=="a.mp4" {continue};raw,err:=os.ReadFile(filepath.Join(dir,name));if err!=nil || string(raw)!=data {t.Fatal("retained input changed",name,err)}}
 if raw,err:=os.ReadFile(target);err!=nil || string(raw)!="outside" {t.Fatal("outside target changed")}
 if _,err:=os.Lstat(filepath.Join(dir,"c.mp4"));err!=nil {t.Fatal("stable symlink removed")}
 if info,err:=os.Stat(filepath.Join(dir,"d.mp4"));err!=nil || !info.IsDir() {t.Fatal("directory removed")}
 missing:=deletePublishedRecordings(context.Background(),filepath.Join(dir,"missing"));if missing.Failures!=1 {t.Fatal("unavailable directory reported success",missing)}
}

func TestPublishedDeletionCancellationAndAdmissionRejection(t *testing.T) {
 dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first");putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 result:=deletePublishedRecordingsWithRemove(ctx,dir,func(path string) error {err:=os.Remove(path);cancel();return err})
 if result.Count!=1 || result.Failures!=1 {t.Fatal("cancellation not reported",result)}
 if raw,err:=os.ReadFile(filepath.Join(dir,"b.mp4"));err!=nil || string(raw)!="second" {t.Fatal("deletion continued after cancellation")}
 n,_:=lifecycleFixture(ModeContinuous)
 n.shuttingDown.Store(true)
 w:=httptest.NewRecorder();serveDeletePublishedRecordings(w,httptest.NewRequest("POST","/api/recordings/delete-all",nil),n,nil)
 if w.Code!=503 {t.Fatal("late deletion was admitted",w.Code)}
 w=httptest.NewRecorder();serveDeletePublishedRecordings(w,httptest.NewRequest("GET","/api/recordings/delete-all",nil),n,nil)
 if w.Code!=405 || w.Header().Get("Allow")!="POST" {t.Fatal("method boundary changed")}
}
