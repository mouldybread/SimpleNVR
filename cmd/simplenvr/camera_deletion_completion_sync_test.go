package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func TestCameraDeletionCompletionBoundaryFailures(t *testing.T) {
 for _,kind:=range []string{"sync-retry-succeeds","sync-retry-fails","guard-after-sync","rmdir-fails","late-entry"} {
  t.Run(kind,func(t *testing.T){
   parent:=t.TempDir();dir:=filepath.Join(parent,"camera")
   if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(dir,"clip.mp4"),[]byte("fixture recording"),0600);err!=nil {t.Fatal(err)}
   firstSyncErr:=errors.New("injected first sync failure")
   retrySyncErr:=errors.New("injected retry sync failure")
   guardErr:=errors.New("injected post-sync guard refusal")
   rmdirErr:=errors.New("injected camera rmdir failure")
   syncs,childUnlinks,rmdirCalls:=0,0,0
   synced:=false
   guard:=func()error{if kind=="guard-after-sync" && synced {return guardErr};return nil}
   err:=removeCameraDirectoryWithIO(context.Background(),dir,guard,func(fd int,name string,flags int)error{
    if flags==unix.AT_REMOVEDIR {
     rmdirCalls++
     if kind=="rmdir-fails" {return rmdirErr}
    }
    if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
    if flags==0 {childUnlinks++}
    return nil
   },func(root *os.File)error{
    syncs++
    info,err:=root.Stat();if err!=nil {t.Fatal("sync descriptor unavailable",err)}
    named,err:=os.Stat(dir);if err!=nil {t.Fatal(err)}
    if !info.IsDir() || !os.SameFile(info,named) {t.Fatal("wrong directory synced")}
    if kind=="sync-retry-succeeds" || kind=="sync-retry-fails" {
     if syncs==1 {return firstSyncErr}
     if kind=="sync-retry-fails" {return retrySyncErr}
    }
    if kind=="late-entry" && syncs==1 {
     if err:=os.WriteFile(filepath.Join(dir,"late.mp4"),[]byte("retained late evidence"),0600);err!=nil {t.Fatal(err)}
    }
    if err:=root.Sync();err!=nil {return err}
    synced=true
    return nil
   })
   if err==nil {t.Fatal("completion failure reported success")}
   wantSyncs,wantRmdir:=1,0
   switch kind {
   case "sync-retry-succeeds":
    wantSyncs=2
    if !errors.Is(err,firstSyncErr) || errors.Is(err,retrySyncErr) {t.Fatal("first sync error lost or spurious retry error",err)}
   case "sync-retry-fails":
    wantSyncs=2
    if !errors.Is(err,firstSyncErr) || !errors.Is(err,retrySyncErr) {t.Fatal("sync errors not both preserved",err)}
   case "guard-after-sync":if !errors.Is(err,guardErr) {t.Fatal("guard refusal lost",err)}
   case "rmdir-fails":wantRmdir=1;if !errors.Is(err,rmdirErr) {t.Fatal("rmdir error lost",err)}
   case "late-entry":wantRmdir=1;if !errors.Is(err,unix.ENOTEMPTY) && !errors.Is(err,unix.EEXIST) {t.Fatal("unexpected late-entry refusal",err)}
   }
   if syncs!=wantSyncs || rmdirCalls!=wantRmdir || childUnlinks!=1 {t.Fatalf("syncs=%d want=%d rmdir=%d want=%d child unlinks=%d",syncs,wantSyncs,rmdirCalls,wantRmdir,childUnlinks)}
   entries,err:=os.ReadDir(dir);if err!=nil {t.Fatal("camera directory not retained",err)}
   wantEntries:=0;if kind=="late-entry" {wantEntries=1}
   if len(entries)!=wantEntries {t.Fatalf("unexpected retained entries: %v",entries)}
   if kind=="late-entry" {
    raw,err:=os.ReadFile(filepath.Join(dir,"late.mp4"));if err!=nil || string(raw)!="retained late evidence" {t.Fatal("late evidence changed",err)}
   }
   info,err:=os.Stat(parent);if err!=nil || !info.IsDir() {t.Fatal("parent directory not retained",err)}
  })
 }
}
