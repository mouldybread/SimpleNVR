package main

import (
 "os"
 "path/filepath"
 "testing"
 "time"
)

func putCleanupFixture(t *testing.T,path,content string) {
 t.Helper()
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte(content),0600);err!=nil {t.Fatal(err)}
 old:=time.Now().Add(-48*time.Hour)
 if err:=os.Chtimes(path,old,old);err!=nil {t.Fatal(err)}
}

func TestPeriodicCleanupPreservesAllStaging(t *testing.T) {
 dir:=t.TempDir()
 fixtures:=map[string]string{
  "old.mp4.tmp":"recoverable media", "active.mp4.tmp":"", "newer.mp4.tmp":"other media",
  "temp/event-old/pre_000.mp4":"pinned prebuffer", "temp/event-old/event.mp4":"failed capture",
  "temp/event-old/concat.txt":"file 'pre_000.mp4'\n", "buffer/buffer.mp4":"active prebuffer",
 }
 for name,data:=range fixtures {putCleanupFixture(t,filepath.Join(dir,name),data)}
 expired:=filepath.Join(dir,"20000101_000000_expired.mp4")
 partial:=filepath.Join(dir,"20000101_000000_expired_partial.mp4")
 putCleanupFixture(t,expired,"published");putCleanupFixture(t,partial,"published partial")
 cleanupTempFiles(dir);cleanupOldRecordings(dir,24*time.Hour)
 for name,data:=range fixtures {
  raw,err:=os.ReadFile(filepath.Join(dir,name))
  if err!=nil || string(raw)!=data {t.Fatalf("staging changed: %s: %v",name,err)}
 }
 for _,path:=range []string{expired,partial} {if _,err:=os.Stat(path);!os.IsNotExist(err) {t.Fatalf("published retention changed: %s: %v",path,err)}}
}

func TestOrphanCleanupPreservesRegisteredAndRecoveryDirectories(t *testing.T) {
 root:=t.TempDir()
 fixtures:=map[string]string{
  "registered/old.mp4.tmp":"retained", "registered/new.mp4.tmp":"newer", "registered/saved.mp4":"registered",
  "partial-orphan/broken.mp4.tmp":"failed finalization", "partial-orphan/saved.mp4":"preserve entire recovery directory",
  "empty-orphan/active.mp4.tmp":"", "empty-orphan/saved.mp4":"possibly active",
  "event-orphan/temp/event-1/pre_000.mp4":"pinned", "event-orphan/saved.mp4":"preserve with event staging",
  "ordinary-orphan/old.mp4":"delete me", ".auth/credentials.json":"private",
 }
 for name,data:=range fixtures {putCleanupFixture(t,filepath.Join(root,name),data)}
 result,err:=cleanupOrphanedAt(root,map[string]bool{"registered":true})
 if err!=nil || result.Count!=1 || result.Bytes!=int64(len("delete me")) || result.SkippedRecoveryDirs!=3 || result.Failures!=0 {t.Fatalf("unexpected result: %+v, %v",result,err)}
 for name,data:=range fixtures {
  if name=="ordinary-orphan/old.mp4" {continue}
  raw,err:=os.ReadFile(filepath.Join(root,name))
  if err!=nil || string(raw)!=data {t.Fatalf("preserved file changed: %s: %v",name,err)}
 }
 if _,err:=os.Stat(filepath.Join(root,"ordinary-orphan"));!os.IsNotExist(err) {t.Fatal("ordinary orphan not removed")}
}

func TestCleanupDoesNotFollowSymlinkedStagingOrMedia(t *testing.T) {
 root:=t.TempDir();outside:=t.TempDir()
 target:=filepath.Join(outside,"important.mp4")
 putCleanupFixture(t,target,"outside media")
 staged:=filepath.Join(root,"staged");plain:=filepath.Join(root,"plain")
 if err:=os.Mkdir(staged,0700);err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(plain,0700);err!=nil {t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(staged,"temp"));err!=nil {t.Fatal(err)}
 if err:=os.Symlink(target,filepath.Join(plain,"20000101_000000_link.mp4"));err!=nil {t.Fatal(err)}
 putCleanupFixture(t,filepath.Join(staged,"saved.mp4"),"preserve")
 result,err:=cleanupOrphanedAt(root,nil)
 if err!=nil || result.Count!=0 || result.SkippedRecoveryDirs!=1 {t.Fatalf("unexpected result: %+v %v",result,err)}
 cleanupOldRecordings(plain,time.Hour)
 if raw,err:=os.ReadFile(target);err!=nil || string(raw)!="outside media" {t.Fatal("outside file changed")}
 if _,err:=os.Lstat(filepath.Join(plain,"20000101_000000_link.mp4"));err!=nil {t.Fatal("media symlink removed")}
}

func TestRecoveryInspectionFailsClosed(t *testing.T) {
 retained,err:=recoveryArtifactsPresent(filepath.Join(t.TempDir(),"missing"))
 if !retained || !os.IsNotExist(err) {t.Fatalf("inspection failed open: %v %v",retained,err)}
 dir:=t.TempDir()
 if retained,err:=recoveryArtifactsPresent(dir);retained || err!=nil {t.Fatalf("empty directory treated as staging: %v %v",retained,err)}
 putCleanupFixture(t,filepath.Join(dir,"temp"),"unexpected regular file")
 if retained,err:=recoveryArtifactsPresent(dir);!retained || err!=nil {t.Fatalf("unexpected staging type not preserved: %v %v",retained,err)}
}
