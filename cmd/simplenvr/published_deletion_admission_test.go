package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

type publishedAdmissionWaitContext struct {context.Context;waiting chan struct{};once sync.Once}
func (ctx *publishedAdmissionWaitContext) Done() <-chan struct{} {ctx.once.Do(func(){close(ctx.waiting)});return ctx.Context.Done()}
type publishedAdmissionOutcome struct {result publishedDeleteResult;err error}
func publishedAdmissionResult(t *testing.T,ch <-chan publishedAdmissionOutcome) publishedAdmissionOutcome {
 t.Helper();select {case value:=<-ch:return value;case <-time.After(3*time.Second):t.Fatal("published deletion admission did not join");return publishedAdmissionOutcome{}}
}
func publishedAdmissionFixture(t *testing.T) (*NVR,*Camera,publishedDeletionTarget) {
 t.Helper();n,cam:=lifecycleFixture(ModeContinuous);cam.RecordDir=t.TempDir()
 putCleanupFixture(t,filepath.Join(cam.RecordDir,"a.mp4"),"bytes")
 return n,cam,publishedDeletionTarget{cam.ID,cam.RecordDir,cam}
}

func TestPublishedAdmissionRevalidatesWaitedTargets(t *testing.T) {
 for _,change:=range []string{"replace","id","path","remove","shutdown","cancel"} {t.Run(change,func(t *testing.T){
  n,cam,target:=publishedAdmissionFixture(t)
  owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  ctx:=&publishedAdmissionWaitContext{Context:parent,waiting:make(chan struct{})}
  called:=false;done:=make(chan publishedAdmissionOutcome,1)
  go func(){result,err:=deleteRegisteredPublished(ctx,n,target,func(ctx context.Context,lease *mediaMutationLease,dir string,guard func() error)(publishedDeleteResult,error){called=true;return deleteAllPublishedLeased(ctx,lease,dir,guard)});done<-publishedAdmissionOutcome{result,err}}()
  awaitShutdownStep(t,ctx.waiting,"explicit deletion did not reach media wait")
  requireRegistryAvailable(t,n)
  n.mu.Lock()
  switch change {case "replace":n.cameras[target.id]=&Camera{ID:target.id};case "id":cam.ID="changed";case "path":cam.RecordDir="changed";case "remove":delete(n.cameras,target.id);case "shutdown":n.shuttingDown.Store(true)}
  n.mu.Unlock()
  if change=="cancel" {cancel()} else {owner.release()}
  outcome:=publishedAdmissionResult(t,done)
  want:=errRecordingStateChanged;if change=="shutdown" {want=errRecorderShuttingDown};if change=="cancel" {want=context.Canceled}
  if !errors.Is(outcome.err,want) || called || outcome.result.Count!=0 || outcome.result.Failures!=1 {t.Fatal(outcome,called)}
  if got,err:=os.ReadFile(filepath.Join(target.dir,"a.mp4"));err!=nil || string(got)!="bytes" {t.Fatal("refused target bytes changed",err)}
  if change=="cancel" && !owner.held() {t.Fatal("canceled waiter disturbed owner")}
 })}
}

func TestPublishedAdmissionGuardReachesFilesystemEngine(t *testing.T) {
 n,cam,target:=publishedAdmissionFixture(t)
 result,err:=deleteRegisteredPublished(context.Background(),n,target,func(ctx context.Context,lease *mediaMutationLease,dir string,guard func() error)(publishedDeleteResult,error){
  n.mu.Lock();cam.RecordDir="changed";n.mu.Unlock()
  return deleteAllPublishedLeased(ctx,lease,dir,guard)
 })
 if !errors.Is(err,errRecordingStateChanged) || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err)}
 if _,err:=os.Stat(filepath.Join(target.dir,"a.mp4"));err!=nil {t.Fatal(err)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("refusal leaked lease")}
}

func TestManagedPublishedDeletionReportsPartialResultsWithoutRegistryLock(t *testing.T) {
 n,_,_:=publishedAdmissionFixture(t)
 entered,release,finished:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 w:=httptest.NewRecorder()
 go func(){defer close(finished);serveManagedPublishedDeletion(w,httptest.NewRequest("POST","/api/recordings/delete-all",nil),n,func(_ context.Context,lease *mediaMutationLease,_ string,guard func() error)(publishedDeleteResult,error){
  if !lease.held() {t.Error("callback has no lease")};if err:=guard();err!=nil {t.Error(err)}
  close(entered);<-release
  return publishedDeleteResult{Count:1,Bytes:1048576,Failures:2},errors.New("injected partial failure")
 })}()
 awaitShutdownStep(t,entered,"filesystem callback not entered")
 requireRegistryAvailable(t,n)
 if len(recordingMediaOwnership.token)!=1 {t.Fatal("filesystem phase lost ownership")}
 once.Do(func(){close(release)});awaitShutdownStep(t,finished,"handler did not join")
 var body struct {Success bool `json:"success"`;Deleted int `json:"deleted"`;Size float64 `json:"size_mb"`;Failures int `json:"failures"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&body);err!=nil {t.Fatal(err)}
 if w.Code!=200 || body.Success || body.Deleted!=1 || body.Size!=1 || body.Failures!=2 {t.Fatal(w.Code,body)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("partial result leaked lease")}
}

func TestPublishedDeletionProductionEntryUsesLeasedEngine(t *testing.T) {
 n,cam,_:=publishedAdmissionFixture(t)
 for name,data:=range map[string]string{"partial_partial.mp4":"partial","active.mp4.tmp":"tmp","temp/event/input.mp4":"input","buffer/pre.mp4":"pre"} {putCleanupFixture(t,filepath.Join(cam.RecordDir,name),data)}
 old:=nvr;nvr=n;defer func(){nvr=old}()
 w:=httptest.NewRecorder();deleteAllRecordingsHandler(w,httptest.NewRequest("POST","/api/recordings/delete-all",nil))
 var body struct {Success bool `json:"success"`;Deleted int `json:"deleted"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&body);err!=nil {t.Fatal(err)}
 if w.Code!=200 || !body.Success || body.Deleted!=2 {t.Fatal(w.Code,body)}
 for _,name:=range []string{"a.mp4","partial_partial.mp4"} {if _,err:=os.Stat(filepath.Join(cam.RecordDir,name));!os.IsNotExist(err) {t.Fatal("published output not removed",name,err)}}
 for _,name:=range []string{"active.mp4.tmp","temp/event/input.mp4","buffer/pre.mp4"} {if _,err:=os.Stat(filepath.Join(cam.RecordDir,name));err!=nil {t.Fatal("evidence removed",name,err)}}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("production handler leaked lease")}
}

func TestManagedPublishedDeletionPreservesEarlierBulkSuccess(t *testing.T) {
 dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"bytes")
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:dir},"b":{ID:"b",RecordDir:filepath.Join(t.TempDir(),"missing")}}}
 w:=httptest.NewRecorder();serveManagedPublishedDeletion(w,httptest.NewRequest("POST","/api/recordings/delete-all",nil),n,deleteAllPublishedLeased)
 var body struct {Success bool `json:"success"`;Deleted int `json:"deleted"`;Failures int `json:"failures"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&body);err!=nil {t.Fatal(err)}
 if body.Success || body.Deleted!=1 || body.Failures!=1 {t.Fatal(body)}
 if len(n.cameras)!=2 {t.Fatal("recording deletion changed registrations")}
 if _,err:=os.Stat(filepath.Join(dir,"a.mp4"));!os.IsNotExist(err) {t.Fatal("pretend rollback",err)}
}

func TestManagedPublishedDeletionMethodShutdownAndNilOperation(t *testing.T) {
 n,_,target:=publishedAdmissionFixture(t)
 n.shuttingDown.Store(true)
 w:=httptest.NewRecorder();serveManagedPublishedDeletion(w,httptest.NewRequest("GET","/",nil),n,nil)
 if w.Code!=405 || w.Header().Get("Allow")!="POST" {t.Fatal(w.Code)}
 w=httptest.NewRecorder();serveManagedPublishedDeletion(w,httptest.NewRequest("POST","/",nil),n,nil)
 if w.Code!=503 {t.Fatal(w.Code)}
 n.shuttingDown.Store(false)
 result,err:=deleteRegisteredPublished(context.Background(),n,target,nil)
 if err==nil || result.Failures!=1 || result.Count!=0 {t.Fatal(result,err)}
 result,err=deleteRegisteredPublished(context.Background(),n,target,func(context.Context,*mediaMutationLease,string,func() error)(publishedDeleteResult,error){return publishedDeleteResult{},errors.New("injected error without count")})
 if err==nil || result.Failures!=1 {t.Fatal(result,err)}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("callback error leaked lease")}
}
