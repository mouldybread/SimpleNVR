package main

import (
 "context"
 "encoding/json"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
)

func TestOrphanCleanupPreservesPrebufferAndPrivateDirectories(t *testing.T) {
 for _,kind:=range []string{"contents","file","symlink","empty"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  putCleanupFixture(t,filepath.Join(dir,"saved.mp4"),"published")
  putCleanupFixture(t,filepath.Join(root,".auth","private.mp4"),"private")
  buffer:=filepath.Join(dir,"buffer")
  switch kind {
  case "contents":putCleanupFixture(t,filepath.Join(buffer,"pre.mp4"),"capture input")
  case "file":putCleanupFixture(t,buffer,"unexpected")
  case "symlink":if err:=os.Symlink(t.TempDir(),buffer);err!=nil {t.Fatal(err)}
  case "empty":if err:=os.Mkdir(buffer,0700);err!=nil {t.Fatal(err)}
  }
  result,err:=cleanupOrphanedAt(root,nil)
  want:=1;if kind=="empty" {want=0}
  if err!=nil || result.SkippedRecoveryDirs!=want || result.Count!=1-want || result.Failures!=0 {t.Fatal(result,err)}
  if raw,err:=os.ReadFile(filepath.Join(root,".auth","private.mp4"));err!=nil || string(raw)!="private" {t.Fatal("private directory changed",err)}
  if kind!="empty" {if raw,err:=os.ReadFile(filepath.Join(dir,"saved.mp4"));err!=nil || string(raw)!="published" {t.Fatal("associated media removed",err)}}
  if _,err:=os.Lstat(buffer);err!=nil {t.Fatal("buffer entry removed",err)}
 })}
}

func TestOrphanCleanupRechecksNewEvidenceBetweenUnlinks(t *testing.T) {
 root:=t.TempDir();dir:=filepath.Join(root,"orphan")
 putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first")
 putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
 ops:=orphanCleanupIOFor(context.Background());calls:=0
 ops.remove=func(path string) error {
  calls++;if err:=os.Remove(path);err!=nil {return err}
  putCleanupFixture(t,filepath.Join(dir,"new.mp4.tmp"),"")
  return nil
 }
 result,err:=cleanupOrphanedWithIO(root,nil,ops)
 if err!=nil || result.Count!=1 || result.Bytes!=5 || result.SkippedRecoveryDirs!=1 || result.Failures!=0 || calls!=1 {t.Fatal(result,err,calls)}
 for name,want:=range map[string]string{"b.mp4":"second","new.mp4.tmp":""} {
  raw,err:=os.ReadFile(filepath.Join(dir,name));if err!=nil || string(raw)!=want {t.Fatal("new evidence or associated media removed",name,err)}
 }
}

func TestOrphanCleanupReportsEnumerationInspectionAndUnlinkFailures(t *testing.T) {
 for _,phase:=range []string{"root","directory","inspection","unlink","empty-directory"} {t.Run(phase,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
  if phase!="empty-directory" {putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"keep")}
  ops:=orphanCleanupIOFor(context.Background());removeCalls:=0
  ops.readDir=func(path string)([]os.DirEntry,error) {if (phase=="root" && path==root) || (phase=="directory" && path==dir) {return nil,os.ErrPermission};return os.ReadDir(path)}
  ops.inspect=func(path string)(bool,error) {if phase=="inspection" {return true,os.ErrPermission};return recoveryArtifactsPresent(path)}
  ops.remove=func(string) error {removeCalls++;return os.ErrPermission}
  result,err:=cleanupOrphanedWithIO(root,nil,ops)
  if phase=="root" {if !errors.Is(err,os.ErrPermission) {t.Fatal("root error hidden",err)}} else if err!=nil || result.Failures!=1 {t.Fatal("failure hidden",result,err)}
  if result.Count!=0 || result.Bytes!=0 {t.Fatal("failed deletion counted",result)}
  wantCalls:=0;if phase=="unlink" || phase=="empty-directory" {wantCalls=1}
  if removeCalls!=wantCalls {t.Fatal("unexpected removal after failed inspection",removeCalls)}
 })}
}

func TestOrphanCleanupCancellationRetainsRemainingFiles(t *testing.T) {
 for _,when:=range []string{"before","after-first"} {t.Run(when,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first")
  putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  if when=="before" {cancel()}
  ops:=orphanCleanupIOFor(ctx);calls:=0
  ops.remove=func(path string) error {calls++;err:=os.Remove(path);cancel();return err}
  result,err:=cleanupOrphanedWithIO(root,nil,ops)
  want:=1;if when=="before" {want=0}
  if err!=nil || !result.Interrupted || result.Failures!=1 || result.Count!=want || calls!=want {t.Fatal(result,err,calls)}
  if raw,err:=os.ReadFile(filepath.Join(dir,"b.mp4"));err!=nil || string(raw)!="second" {t.Fatal("continued after cancellation",err)}
 })}
}

func TestOrphanCleanupRejectsSymlinkRootAndRetainsNonregularEntries(t *testing.T) {
 outside:=t.TempDir();putCleanupFixture(t,filepath.Join(outside,"orphan","a.mp4"),"outside")
 root:=filepath.Join(t.TempDir(),"link");if err:=os.Symlink(outside,root);err!=nil {t.Fatal(err)}
 if _,err:=cleanupOrphanedAt(root,nil);err==nil {t.Fatal("symlink root accepted")}
 if raw,err:=os.ReadFile(filepath.Join(outside,"orphan","a.mp4"));err!=nil || string(raw)!="outside" {t.Fatal("outside media changed",err)}
 root=t.TempDir();dir:=filepath.Join(root,"orphan");if err:=os.MkdirAll(filepath.Join(dir,"directory.mp4"),0700);err!=nil {t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
 result,err:=cleanupOrphanedAt(root,nil)
 if err!=nil || result.Count!=0 || result.Failures!=0 {t.Fatal(result,err)}
 for _,name:=range []string{"directory.mp4","link.mp4"} {if _,err:=os.Lstat(filepath.Join(dir,name));err!=nil {t.Fatal("nonregular entry removed",name,err)}}
}

func TestOrphanCleanupHandlerShutdownPartialJSONAndRegistryBoundary(t *testing.T) {
 root:=t.TempDir();dir:=filepath.Join(root,"orphan")
 putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first")
 putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
 n:=&NVR{cameras:make(map[string]*Camera)}
 ops:=orphanCleanupIOFor(context.Background())
 ops.remove=func(path string) error {requireRegistryAvailable(t,n);err:=os.Remove(path);n.shuttingDown.Store(true);return err}
 w:=httptest.NewRecorder()
 serveOrphanCleanup(w,httptest.NewRequest("POST","/api/recordings/cleanup-orphaned",nil),n,root,ops)
 var result struct {Success bool `json:"success"`;Count int `json:"count"`;Failures int `json:"failures"`;Interrupted bool `json:"interrupted"`}
 if err:=json.Unmarshal(w.Body.Bytes(),&result);err!=nil {t.Fatal(err)}
 if w.Code!=200 || result.Success || result.Count!=1 || result.Failures!=1 || !result.Interrupted {t.Fatal("partial shutdown response lost",w.Code,result)}
 if raw,err:=os.ReadFile(filepath.Join(dir,"b.mp4"));err!=nil || string(raw)!="second" {t.Fatal("continued after shutdown",err)}
 w=httptest.NewRecorder();serveOrphanCleanup(w,httptest.NewRequest("POST","/api/recordings/cleanup-orphaned",nil),n,root,ops)
 if w.Code!=503 {t.Fatal("late cleanup admitted",w.Code)}
 w=httptest.NewRecorder();serveOrphanCleanup(w,httptest.NewRequest("GET","/api/recordings/cleanup-orphaned",nil),n,root,ops)
 if w.Code!=405 || w.Header().Get("Allow")!="POST" {t.Fatal("method boundary changed")}
}
