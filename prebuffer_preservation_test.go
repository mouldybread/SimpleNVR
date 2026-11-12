package main

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func requirePreservedBytes(t *testing.T,path,want string) {
 t.Helper()
 got,err:=os.ReadFile(path)
 if err!=nil || string(got)!=want {t.Fatalf("evidence changed at %s: %q %v",path,got,err)}
}

func TestStartupRecoveryPreservesPrebufferAndEventEvidence(t *testing.T) {
 for _,legacy:=range []bool{false,true} {
  name:="context-aware";if legacy {name="legacy"}
  t.Run(name,func(t *testing.T){
   cam:=&Camera{ID:"preserved",RecordDir:t.TempDir()}
   fixtures:=map[string]string{
    "buffer/closed.mp4":"closed prior-process bytes",
    "buffer/unreported.mp4":"incomplete capture bytes",
    "buffer/nested/unknown":"unexpected evidence",
    "temp/event-old/recovery.json":"checkpoint evidence",
    "temp/event-old/pre_000.mp4":"pinned evidence",
   }
   for name,data:=range fixtures {putCleanupFixture(t,filepath.Join(cam.RecordDir,name),data)}
   n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}}
   if legacy {
    old:=nvr;nvr=n;defer func(){nvr=old}()
    recoverPartialRecordings()
   } else {recoverBeforeStartup(context.Background(),n)}
   for name,data:=range fixtures {requirePreservedBytes(t,filepath.Join(cam.RecordDir,name),data)}
  })
 }
}

func TestStartupRecoveryLeavesStableBufferSymlinkUntouched(t *testing.T) {
 for _,legacy:=range []bool{false,true} {
  name:="context-aware";if legacy {name="legacy"}
  t.Run(name,func(t *testing.T){
   cam:=&Camera{ID:"linked",RecordDir:t.TempDir()}
   outside:=t.TempDir();target:=filepath.Join(outside,"evidence.mp4")
   putCleanupFixture(t,target,"outside evidence")
   link:=filepath.Join(cam.RecordDir,"buffer")
   if err:=os.Symlink(outside,link);err!=nil {t.Fatal(err)}
   n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}}
   if legacy {old:=nvr;nvr=n;defer func(){nvr=old}();recoverPartialRecordings()} else {recoverBeforeStartup(context.Background(),n)}
   info,err:=os.Lstat(link);if err!=nil || info.Mode()&os.ModeSymlink==0 {t.Fatal("staging symlink removed",err)}
   requirePreservedBytes(t,target,"outside evidence")
  })
 }
}

func TestStoppedBufferPreservesCurrentAndUnownedFiles(t *testing.T) {
 bm:=bufferFixture(t)
 closed:=filepath.Join(bm.camera.RecordDir,"buffer","closed.mp4")
 content:=strings.Repeat("p",2048)
 putCleanupFixture(t,closed,content)
 unknown:=filepath.Join(bm.camera.RecordDir,"buffer","old-process.mp4")
 putCleanupFixture(t,unknown,"unowned evidence")
 pinned:=filepath.Join(bm.camera.RecordDir,"temp","event-old","pre_000.mp4")
 if err:=os.MkdirAll(filepath.Dir(pinned),0700);err!=nil {t.Fatal(err)}
 if err:=os.Link(closed,pinned);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(closed,now.Add(-time.Second),now)
 bm.cleanupBuffer()
 if len(bm.buffer)!=0 {t.Fatal("stopped manager retained its active index")}
 requirePreservedBytes(t,closed,content)
 requirePreservedBytes(t,unknown,"unowned evidence")
 requirePreservedBytes(t,pinned,content)
 fresh:=newBufferManager(context.Background(),bm.camera);defer fresh.cancel()
 fresh.pruneBefore(now.Add(time.Hour))
 if len(fresh.buffer)!=0 {t.Fatal("new manager adopted prior-process files")}
 requirePreservedBytes(t,closed,content)
 requirePreservedBytes(t,unknown,"unowned evidence")
}

func TestUnclosedBufferCleanupPreservesNonemptyAndUnexpectedEntries(t *testing.T) {
 bm:=bufferFixture(t)
 dir:=filepath.Join(bm.camera.RecordDir,"buffer")
 content:=strings.Repeat("c",2048)
 closed:=filepath.Join(dir,"closed_nonce.mp4");putCleanupFixture(t,closed,content)
 partial:=filepath.Join(dir,"partial_nonce.mp4");putCleanupFixture(t,partial,"unreported capture")
 empty:=filepath.Join(dir,"empty_nonce.mp4");putCleanupFixture(t,empty,"")
 other:=filepath.Join(dir,"empty_other.mp4");putCleanupFixture(t,other,"")
 nested:=filepath.Join(dir,"directory_nonce.mp4","evidence");putCleanupFixture(t,nested,"nested evidence")
 target:=filepath.Join(t.TempDir(),"outside.mp4");putCleanupFixture(t,target,"outside evidence")
 link:=filepath.Join(dir,"link_nonce.mp4");if err:=os.Symlink(target,link);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC();bm.registerCompleted(closed,now.Add(-time.Second),now)
 bm.discardUnclosed(filepath.Join(dir,"*_nonce.mp4"))
 requirePreservedBytes(t,closed,content)
 requirePreservedBytes(t,partial,"unreported capture")
 requirePreservedBytes(t,other,"")
 requirePreservedBytes(t,nested,"nested evidence")
 requirePreservedBytes(t,target,"outside evidence")
 if info,err:=os.Lstat(link);err!=nil || info.Mode()&os.ModeSymlink==0 {t.Fatal("stable link removed",err)}
 if _,err:=os.Lstat(empty);!os.IsNotExist(err) {t.Fatal("empty nonce-scoped output retained",err)}
 bm.discardUnclosed("[")
 requirePreservedBytes(t,partial,"unreported capture")
}

func TestEventStorageInitializationRejectsStableRedirects(t *testing.T) {
 for _,kind:=range []string{"missing","directory","file","symlink"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"buffer")
  target:=t.TempDir();evidence:=filepath.Join(target,"evidence");putCleanupFixture(t,evidence,"outside evidence")
  switch kind {
  case "directory":putCleanupFixture(t,filepath.Join(dir,"old.mp4"),"retained")
  case "file":putCleanupFixture(t,dir,"unexpected file")
  case "symlink":if err:=os.Symlink(target,dir);err!=nil {t.Fatal(err)}
  }
  err:=ensureEventStorageDirectory(dir)
  allowed:=kind=="missing" || kind=="directory"
  if (err==nil)!=allowed {t.Fatal("incorrect staging initialization result",kind,err)}
  requirePreservedBytes(t,evidence,"outside evidence")
  if kind=="directory" {requirePreservedBytes(t,filepath.Join(dir,"old.mp4"),"retained")}
  if kind=="file" {requirePreservedBytes(t,dir,"unexpected file")}
  if kind=="symlink" {if info,err:=os.Lstat(dir);err!=nil || info.Mode()&os.ModeSymlink==0 {t.Fatal("redirect entry changed",err)}}
 })}
}
