package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestPublicationPostRenameParentReplacementRefusesObserver(t *testing.T) {
 for _,scope:=range []string{"same-parent","source-parent","destination-parent"} {
  for _,phase:=range []string{"rename","directory-sync"} {
   for _,syncFailure:=range []bool{false,true} {
    failureName:="sync-ok";if syncFailure {failureName="sync-error"}
    t.Run(scope+"/"+phase+"/"+failureName,func(t *testing.T){
     base:=t.TempDir()
     sourceDir:=filepath.Join(base,"source");finalDir:=filepath.Join(base,"destination")
     if err:=os.Mkdir(sourceDir,0700);err!=nil {t.Fatal(err)}
     if scope=="same-parent" {finalDir=sourceDir} else if err:=os.Mkdir(finalDir,0700);err!=nil {t.Fatal(err)}
     source:=publicationSource(t,sourceDir,"capture.tmp","published evidence")
     final:=filepath.Join(finalDir,"final.mp4")
     replaced:=sourceDir;if scope=="destination-parent" {replaced=finalDir}
     moved:=filepath.Join(base,"retained-parent")
     swaps,syncs,observations,renames:=0,0,0,0
     replace:=func()error{
      swaps++
      if err:=os.Rename(replaced,moved);err!=nil {return err}
      if err:=os.Mkdir(replaced,0700);err!=nil {return err}
      // Preserve the final inode identity at its original pathname. A final
      // file-only check cannot distinguish this replacement parent.
      if replaced==finalDir {return os.Link(filepath.Join(moved,"final.mp4"),final)}
      return nil
     }
     injected:=errors.New("injected directory sync failure")
     ops:=realPublicationDurabilityOps()
     ops.rename=func(a,b string)error{
      renames++
      if err:=renameRecordingNoReplace(a,b);err!=nil {return err}
      if phase=="rename" {return replace()};return nil
     }
     ops.syncDirectory=func(f *os.File)error{
      syncs++
      if phase=="directory-sync" && syncs==1 {if err:=replace();err!=nil {return err}}
      err:=f.Sync()
      if syncFailure && syncs==1 {return errors.Join(err,injected)}
      return err
     }
     lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)}
     defer lease.release()
     err=publishRecordingDurablyObservedLeased(context.Background(),lease,source,final,acceptPublicationFixture,ops,func(*os.File,*os.File)error{observations++;return nil})
     expectedSyncs:=2;if scope=="same-parent" {expectedSyncs=1}
     if swaps!=1 || renames!=1 || syncs!=expectedSyncs {t.Fatalf("swap/rename/sync mismatch: %d/%d/%d",swaps,renames,syncs)}
     if err==nil || observations!=0 {t.Fatalf("replaced parent accepted: err=%v observations=%d",err,observations)}
     if syncFailure && !errors.Is(err,injected) {t.Fatalf("sync error lost: %v",err)}
     if !lease.held() {t.Fatal("borrowed publication released caller ownership")}
     assertPublicationBytes(t,final,"published evidence")
     if replaced==finalDir {assertPublicationBytes(t,filepath.Join(moved,"final.mp4"),"published evidence")}
     if _,statErr:=os.Lstat(source);!os.IsNotExist(statErr) {t.Fatalf("source unexpectedly restored: %v",statErr)}
    })
   }
  }
 }
}

func TestPublicationStableParentsStillReachObserver(t *testing.T) {
 for _,sameParent:=range []bool{false,true} {
  name:="different-parents";if sameParent {name="same-parent"}
  t.Run(name,func(t *testing.T){
   sourceDir,finalDir:=t.TempDir(),t.TempDir();if sameParent {finalDir=sourceDir}
   source:=publicationSource(t,sourceDir,"capture.tmp","published evidence");final:=filepath.Join(finalDir,"final.mp4")
   lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer lease.release()
   observations:=0
   err=publishRecordingDurablyObservedLeased(context.Background(),lease,source,final,acceptPublicationFixture,realPublicationDurabilityOps(),func(file,directory *os.File)error{
    observations++
    held,err:=directory.Stat();if err!=nil {return err}
    named,err:=os.Stat(finalDir);if err!=nil {return err}
    if !os.SameFile(held,named) {t.Error("observer received wrong directory")}
    heldFile,err:=file.Stat();if err!=nil {return err}
    namedFile,err:=os.Stat(final);if err!=nil {return err}
    if !os.SameFile(heldFile,namedFile) {t.Error("observer received wrong output")}
    return nil
   })
   if err!=nil || observations!=1 || !lease.held() {t.Fatalf("stable publication failed: %v observations=%d",err,observations)}
  })
 }
}
