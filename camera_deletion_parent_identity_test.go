package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func TestCameraDeletionRefusesChangedParentPath(t *testing.T) {
 for _,kind:=range []string{"missing","replacement-directory","symlink-to-original"} {
  for _,phase:=range []string{"inspection","after-first-unlink"} {
   t.Run(kind+"/"+phase,func(t *testing.T){
    base:=t.TempDir();parent:=filepath.Join(base,"recordings");dir:=filepath.Join(parent,"camera");moved:=filepath.Join(base,"retained-recordings")
    if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
    for _,name:=range []string{"a.mp4","b.mp4"} {if err:=os.WriteFile(filepath.Join(dir,name),[]byte("retained evidence"),0600);err!=nil {t.Fatal(err)}}
    swaps:=0
    swap:=func()error{
     swaps++
     if err:=os.Rename(parent,moved);err!=nil {return err}
     switch kind {
     case "replacement-directory":
      if err:=os.MkdirAll(dir,0700);err!=nil {return err}
      return os.WriteFile(filepath.Join(dir,"replacement.mp4"),[]byte("replacement evidence"),0600)
     case "symlink-to-original":return os.Symlink(moved,parent)
     }
     return nil
    }
    checks,unlinks:=0,0
    guard:=func()error{
     checks++
     // The second guard runs in the first rootCheck, after both directory
     // descriptors are open, before inspection authorizes any unlink.
     if phase=="inspection" && checks==2 {return swap()}
     return nil
    }
    err:=removeCameraDirectoryWithUnlink(context.Background(),dir,guard,func(fd int,name string,flags int)error{
     unlinks++
     if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
     if phase=="after-first-unlink" && unlinks==1 {return swap()}
     return nil
    })
    if err==nil || swaps!=1 {t.Fatalf("changed parent accepted: err=%v swaps=%d",err,swaps)}
    if kind!="missing" && !errors.Is(err,errCameraDeleteBlocked) {t.Fatalf("wrong refusal: %v",err)}
    wantUnlinks:=0;if phase=="after-first-unlink" {wantUnlinks=1}
    if unlinks!=wantUnlinks {t.Fatalf("continued through detached parent: unlinks=%d want=%d",unlinks,wantUnlinks)}
    retainedDir:=filepath.Join(moved,"camera")
    entries,err:=os.ReadDir(retainedDir);if err!=nil {t.Fatal(err)}
    if len(entries)!=2-wantUnlinks {t.Fatalf("remaining originals lost: %v",entries)}
    for _,entry:=range entries {raw,err:=os.ReadFile(filepath.Join(retainedDir,entry.Name()));if err!=nil || string(raw)!="retained evidence" {t.Fatal("remaining bytes changed",err)}}
    if kind=="replacement-directory" {raw,err:=os.ReadFile(filepath.Join(dir,"replacement.mp4"));if err!=nil || string(raw)!="replacement evidence" {t.Fatal("replacement touched",err)}}
    if kind=="symlink-to-original" {target,err:=os.Readlink(parent);if err!=nil || target!=moved {t.Fatal("replacement symlink changed",err)}}
    if kind=="missing" {if _,err:=os.Lstat(parent);!os.IsNotExist(err) {t.Fatal("missing parent recreated",err)}}
   })
  }
 }
}

func TestCameraDeletionUnchangedParentStillCompletes(t *testing.T) {
 parent:=filepath.Join(t.TempDir(),"recordings");dir:=filepath.Join(parent,"camera")
 if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"clip.mp4"),[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
 if err:=removeCameraDirectory(context.Background(),dir,func()error{return nil});err!=nil {t.Fatal(err)}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("camera directory not removed",err)}
 info,err:=os.Lstat(parent);if err!=nil || !info.IsDir() {t.Fatal("parent directory removed",err)}
}
