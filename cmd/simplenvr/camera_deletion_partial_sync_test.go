package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func TestCameraDeletionPartialProgressSync(t *testing.T) {
 for _,kind:=range []string{"first-unlink-fails","second-unlink-fails","guard-after-unlink","cancel-after-unlink","partial-sync-fails","success"} {
  t.Run(kind,func(t *testing.T){
   parent:=t.TempDir();dir:=filepath.Join(parent,"camera")
   if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
   for _,name:=range []string{"a.mp4","b.mp4"} {if err:=os.WriteFile(filepath.Join(dir,name),[]byte("fixture evidence"),0600);err!=nil {t.Fatal(err)}}
   ctx,cancel:=context.WithCancel(context.Background());defer cancel()
   unlinkErr:=errors.New("injected unlink failure");guardErr:=errors.New("injected guard failure");syncErr:=errors.New("injected sync failure")
   unlinked,calls,syncs:=0,0,0
   guard:=func()error{if kind=="guard-after-unlink" && unlinked>0 {return guardErr};return nil}
   err:=removeCameraDirectoryWithIO(ctx,dir,guard,func(fd int,name string,flags int)error{
    calls++
    if kind=="first-unlink-fails" || ((kind=="second-unlink-fails" || kind=="partial-sync-fails") && unlinked==1) {return unlinkErr}
    if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
    if flags==0 {unlinked++;if kind=="cancel-after-unlink" {cancel()}}
    return nil
   },func(root *os.File)error{
    syncs++
    if _,err:=root.Stat();err!=nil {t.Error("sync attempted after descriptor close",err)}
    if kind=="partial-sync-fails" {return syncErr}
    return root.Sync()
   })
   wantSyncs,wantRemaining:=1,1
   switch kind {
   case "first-unlink-fails":wantSyncs=0;wantRemaining=2;if !errors.Is(err,unlinkErr) {t.Fatal(err)}
   case "second-unlink-fails":if !errors.Is(err,unlinkErr) {t.Fatal(err)}
   case "guard-after-unlink":if !errors.Is(err,guardErr) {t.Fatal(err)}
   case "cancel-after-unlink":if !errors.Is(err,context.Canceled) {t.Fatal(err)}
   case "partial-sync-fails":if !errors.Is(err,unlinkErr) || !errors.Is(err,syncErr) {t.Fatal("lost primary or sync error",err)}
   case "success":if err!=nil {t.Fatal(err)};wantRemaining=0
   }
   if syncs!=wantSyncs {t.Fatalf("sync attempts=%d want=%d (unlink calls=%d)",syncs,wantSyncs,calls)}
   if kind=="success" {if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("camera directory remains",err)};return}
   entries,err:=os.ReadDir(dir);if err!=nil {t.Fatal(err)}
   if len(entries)!=wantRemaining {t.Fatalf("remaining entries=%d want=%d",len(entries),wantRemaining)}
   for _,entry:=range entries {raw,err:=os.ReadFile(filepath.Join(dir,entry.Name()));if err!=nil || string(raw)!="fixture evidence" {t.Fatal("remaining evidence changed",err)}}
  })
 }
}
