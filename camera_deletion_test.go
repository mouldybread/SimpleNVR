package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func deletionGuard() error {return nil}
func deletionDeps(n *NVR,persist func() error,remove func(context.Context,string,func() error) error) cameraDeletionDeps {
 return cameraDeletionDeps{validate:func(string,string) error {return nil},persist:persist,remove:remove}
}
func deletionSnapshot(n *NVR) persistedState {
 n.mu.RLock();raw,err:=json.Marshal(persistedState{n.cameras,n.cameraOrder,n.events});n.mu.RUnlock()
 if err!=nil {panic(err)}
 var state persistedState;if err:=json.Unmarshal(raw,&state);err!=nil {panic(err)}
 return state
}

func TestCameraDeletionPreservesEvidenceBeforeAnyUnlink(t *testing.T) {
 for _,name:=range []string{"buffer/pre.mp4","temp/event/input.mp4","active.mp4.tmp","checkpoint.json","unknown/keep","directory.mp4/keep"} {t.Run(name,func(t *testing.T){
  dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"published.mp4"),"published")
  evidence:=filepath.Join(dir,name);putCleanupFixture(t,evidence,"retained")
  calls:=0
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int) error {calls++;return nil})
  if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal("evidence did not block all unlinking",err,calls)}
  for path,want:=range map[string]string{evidence:"retained",filepath.Join(dir,"published.mp4"):"published"} {
   raw,err:=os.ReadFile(path);if err!=nil || string(raw)!=want {t.Fatal("bytes changed",path,err)}
  }
 })}
}

func TestCameraDeletionAllowsOnlyPublishedFilesAndEmptyKnownDirectories(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"camera");if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 for _,name:=range []string{"a.mp4","partial_fixture.mp4"} {putCleanupFixture(t,filepath.Join(dir,name),"published")}
 for _,name:=range []string{"buffer","temp"} {if err:=os.Mkdir(filepath.Join(dir,name),0700);err!=nil {t.Fatal(err)}}
 if err:=removeCameraDirectory(context.Background(),dir,deletionGuard);err!=nil {t.Fatal(err)}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("directory not removed",err)}
 if err:=removeCameraDirectory(context.Background(),dir,deletionGuard);err==nil {t.Fatal("missing storage incorrectly treated as empty")}
}

func TestCameraDeletionRejectsSymlinksAndSpecialEntries(t *testing.T) {
 for _,kind:=range []string{"root","media","buffer","fifo"} {t.Run(kind,func(t *testing.T){
  parent,outside:=t.TempDir(),t.TempDir();dir:=filepath.Join(parent,"camera")
  target:=filepath.Join(outside,"keep.mp4");putCleanupFixture(t,target,"outside")
  if kind=="root" {if err:=os.Symlink(outside,dir);err!=nil {t.Fatal(err)}} else {
   if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
   switch kind {
   case "media":if err:=os.Symlink(target,filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
   case "buffer":if err:=os.Symlink(outside,filepath.Join(dir,"buffer"));err!=nil {t.Fatal(err)}
   case "fifo":if err:=unix.Mkfifo(filepath.Join(dir,"fifo.mp4"),0600);err!=nil {t.Fatal(err)}
   }
  }
  if err:=removeCameraDirectory(context.Background(),dir,deletionGuard);err==nil {t.Fatal("unsafe entry accepted")}
  if raw,err:=os.ReadFile(target);err!=nil || string(raw)!="outside" {t.Fatal("outside bytes changed",err)}
  if _,err:=os.Lstat(dir);err!=nil {t.Fatal("camera entry removed",err)}
 })}
}

func TestCameraDeletionBoundsInspection(t *testing.T) {
 dir:=t.TempDir()
 for i:=0;i<=cameraDeleteEntryLimit;i++ {f,err:=os.CreateTemp(dir,"entry-*.mp4");if err!=nil {t.Fatal(err)};if err:=f.Close();err!=nil {t.Fatal(err)}}
 calls:=0
 err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(int,string,int) error {calls++;return nil})
 if !errors.Is(err,errCameraDeleteBlocked) || calls!=0 {t.Fatal("limit did not fail before unlink",err,calls)}
}

func TestCameraDeletionStopsOnPartialUnlinkAndCancellation(t *testing.T) {
 for _,cancelAfterFirst:=range []bool{false,true} {t.Run(map[bool]string{false:"unlink-error",true:"cancellation"}[cancelAfterFirst],func(t *testing.T){
  dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first");putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();calls:=0
  err:=removeCameraDirectoryWithUnlink(ctx,dir,deletionGuard,func(fd int,name string,flags int) error {
   calls++;if calls==2 {return os.ErrPermission}
   err:=unix.Unlinkat(fd,name,flags);if cancelAfterFirst {cancel()};return err
  })
  if err==nil {t.Fatal("partial deletion reported success")}
  entries,readErr:=os.ReadDir(dir);if readErr!=nil || len(entries)!=1 {t.Fatal("remaining input lost",entries,readErr)}
  if cancelAfterFirst && (calls!=1 || !errors.Is(err,context.Canceled)) {t.Fatal("continued after cancellation",err,calls)}
 })}
}

func TestCameraDeletionLateDirectoryEntriesAreNeverRecursed(t *testing.T) {
 for _,where:=range []string{"buffer","root"} {t.Run(where,func(t *testing.T){
  dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"published")
  if where=="buffer" {if err:=os.Mkdir(filepath.Join(dir,"buffer"),0700);err!=nil {t.Fatal(err)}}
  injected:=false;var retained string
  err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(fd int,name string,flags int) error {
   if !injected && flags==unix.AT_REMOVEDIR {
    injected=true;retained=filepath.Join(dir,"late.tmp");if where=="buffer" {retained=filepath.Join(dir,"buffer","late.mp4")}
    putCleanupFixture(t,retained,"late evidence")
   }
   return unix.Unlinkat(fd,name,flags)
  })
  if err==nil || !injected {t.Fatal("late entry was ignored",err)}
  if raw,err:=os.ReadFile(retained);err!=nil || string(raw)!="late evidence" {t.Fatal("late evidence removed",err)}
 })}
}

func TestCameraDeletionPinsRootAndRejectsRegistryPathReplacement(t *testing.T) {
 parent:=t.TempDir();dir:=filepath.Join(parent,"camera");moved:=filepath.Join(parent,"moved")
 if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first");putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
 changed:=false
 err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(fd int,name string,flags int) error {
  if !changed {
   changed=true
   if err:=os.Rename(dir,moved);err!=nil {return err}
   if err:=os.Mkdir(dir,0700);err!=nil {return err}
   putCleanupFixture(t,filepath.Join(dir,"replacement.mp4"),"replacement")
  }
  return unix.Unlinkat(fd,name,flags)
 })
 if !errors.Is(err,errCameraDeleteBlocked) {t.Fatal("root replacement not rejected",err)}
 if raw,err:=os.ReadFile(filepath.Join(dir,"replacement.mp4"));err!=nil || string(raw)!="replacement" {t.Fatal("replacement directory touched",err)}
 entries,err:=os.ReadDir(moved);if err!=nil || len(entries)!=1 {t.Fatal("pinned directory not used",entries,err)}
}

func TestCameraDeletionPersistsStoppedIntentBeforeRemovalAndAfterFailure(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous);cam.RecordDir="/recordings/test"
 statePath:=filepath.Join(t.TempDir(),"cameras.json")
 started,joined:=make(chan struct{}),make(chan struct{})
 worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();close(started);<-ctx.Done();close(joined)}
 if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string) error {return nil});err!=nil {t.Fatal(err)}
 defer n.cancelAndWaitRecorders();waitHTTPJob(t,started,"recorder did not start")
 removes:=0
 deps:=deletionDeps(n,func() error {requireRegistryAvailable(t,n);return n.saveAt(statePath)},func(_ context.Context,_ string,guard func() error) error {
  removes++;select {case <-joined:default:t.Fatal("unlink before worker join")}
  restored:=&NVR{};if err:=restored.loadAt(statePath);err!=nil {t.Fatal(err)}
  if restored.cameras[cam.ID]==nil || restored.cameras[cam.ID].IsRecording {t.Fatal("stopped intent not durable before deletion")}
  if err:=guard();err!=nil {return err};return os.ErrPermission
 })
 err:=deleteManagedCameras(context.Background(),n,[]string{cam.ID},deps)
 if !errors.Is(err,os.ErrPermission) || removes!=1 {t.Fatal(err,removes)}
 if n.cameras[cam.ID]!=cam || cam.IsRecording || cam.recordCancel!=nil || len(n.cameraOrder)!=1 {t.Fatal("failed deletion lost camera or restored recording intent")}
}

func TestCameraDeletionPersistenceFailuresFailClosed(t *testing.T) {
 for _,failAt:=range []int{1,2} {t.Run(map[int]string{1:"before-unlink",2:"after-removal"}[failAt],func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);cam.IsRecording=true
  saves,removes:=0,0;var durable persistedState
  deps:=deletionDeps(n,func() error {saves++;if saves==failAt {return os.ErrPermission};durable=deletionSnapshot(n);return nil},func(context.Context,string,func() error) error {removes++;return nil})
  err:=deleteManagedCameras(context.Background(),n,[]string{cam.ID},deps)
  if !errors.Is(err,errCameraDeletePersistence) || !errors.Is(err,os.ErrPermission) {t.Fatal("save failure hidden",err)}
  if failAt==1 {if removes!=0 || n.cameras[cam.ID]!=cam || cam.IsRecording {t.Fatal("destruction after failed stop save")}} else {
   if removes!=1 || len(n.cameras)!=0 || len(n.cameraOrder)!=0 || durable.Cameras[cam.ID]==nil || durable.Cameras[cam.ID].IsRecording {t.Fatal("wrong partial state after final-save failure")}
  }
 })}
}

func TestCameraDeletionBulkPersistsEachOutcomeAndKeepsOrder(t *testing.T) {
 n,_:=lifecycleFixture(ModeContinuous);n.cameras=map[string]*Camera{}
 for _,id:=range []string{"a","b","c"} {n.cameras[id]=&Camera{ID:id,RecordDir:id,IsRecording:true}}
 n.cameraOrder=[]string{"c","a","b"};saves:=0;var durable persistedState;var removed []string
 deps:=deletionDeps(n,func() error {saves++;durable=deletionSnapshot(n);return nil},func(_ context.Context,dir string,_ func() error) error {removed=append(removed,dir);if dir=="b" {return errCameraDeleteBlocked};return nil})
 err:=deleteManagedCameras(context.Background(),n,nil,deps)
 if !errors.Is(err,errCameraDeleteBlocked) || saves!=5 || strings.Join(removed,",")!="a,b,c" {t.Fatal("bulk outcome hidden",err,saves,removed)}
 if len(n.cameras)!=1 || n.cameras["b"]==nil || n.cameras["b"].IsRecording || strings.Join(n.cameraOrder,",")!="b" {t.Fatal("registry/order mismatch")}
 if len(durable.Cameras)!=1 || durable.Cameras["b"].IsRecording || strings.Join(durable.CameraOrder,",")!="b" {t.Fatal("partial outcome not saved")}
}

func TestCameraDeletionJoinCancellationShutdownAndRegistryAvailability(t *testing.T) {
 for _,shutdown:=range []bool{false,true} {t.Run(map[bool]string{false:"cancel",true:"shutdown"}[shutdown],func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);started,canceled,release:=make(chan struct{}),make(chan struct{}),make(chan struct{})
  var once sync.Once;defer once.Do(func(){close(release)})
  worker:=func(ctx context.Context,cam *Camera,_ RecordingMode){defer cam.recordWG.Done();close(started);<-ctx.Done();close(canceled);<-release}
  if err:=n.transitionRecordingWithAdmission(cam.ID,"start","",worker,func(string) error {return nil});err!=nil {t.Fatal(err)}
  waitHTTPJob(t,started,"worker did not start")
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();saves,removes:=0,0
  deps:=deletionDeps(n,func() error {requireRegistryAvailable(t,n);saves++;return nil},func(context.Context,string,func() error) error {removes++;return nil})
  done:=make(chan error,1);go func(){done<-deleteManagedCameras(ctx,n,[]string{cam.ID},deps)}()
  waitHTTPJob(t,canceled,"worker did not receive cancellation");requireRegistryAvailable(t,n)
  if shutdown {n.shuttingDown.Store(true)} else {cancel()}
  once.Do(func(){close(release)})
  select {case err:=<-done:
   want:=context.Canceled;if shutdown {want=errRecorderShuttingDown};if !errors.Is(err,want) {t.Fatal("interrupt lost",err)}
  case <-time.After(2*time.Second):t.Fatal("join did not finish")}
  if saves!=1 || removes!=0 || n.cameras[cam.ID]!=cam || cam.IsRecording {t.Fatal("interrupted join bypassed stopped-intent save",saves,removes)}
 })}
}

func TestCameraDeletionRevalidatesIdentityAndStopsAtAdmissionBoundaries(t *testing.T) {
 for _,kind:=range []string{"shutdown","cancel","missing","identity","directory","invalid-path"} {t.Run(kind,func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  ids:=[]string{cam.ID};saves,removes:=0,0
  deps:=deletionDeps(n,func() error {saves++;return nil},func(context.Context,string,func() error) error {removes++;return nil})
  switch kind {
  case "shutdown":n.shuttingDown.Store(true)
  case "cancel":cancel()
  case "missing":ids=[]string{"absent"}
  case "identity":deps.validate=func(string,string) error {n.mu.Lock();n.cameras[cam.ID]=&Camera{ID:cam.ID};n.mu.Unlock();return nil}
  case "directory":deps.validate=func(string,string) error {n.mu.Lock();cam.RecordDir="changed";n.mu.Unlock();return nil}
  case "invalid-path":deps.validate=func(string,string) error {return os.ErrPermission}
  }
  if err:=deleteManagedCameras(ctx,n,ids,deps);err==nil {t.Fatal("admission unexpectedly passed")}
  if saves!=0 || removes!=0 {t.Fatal("mutation after failed admission",saves,removes)}
 })}
}

func TestCameraDeletionHTTPErrorPrecedence(t *testing.T) {
 for _,item:=range []struct{err error;status int}{
  {errCameraDeleteBlocked,409},
  {errors.Join(errCameraDeleteBlocked,errCameraDeletePersistence),500},
  {errors.Join(errCameraDeleteBlocked,errRecorderShuttingDown),503},
  {errCameraMissing,404},
 } {
  w:=httptest.NewRecorder();cameraDeletionError(w,item.err)
  if w.Code!=item.status {t.Fatal("wrong HTTP outcome",w.Code,item.status)}
 }
}
