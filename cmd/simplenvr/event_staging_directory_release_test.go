package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "golang.org/x/sys/unix"
)

func TestStagingDirectoryReleaseRemovesCheckpointLastAndOnlyEmptyRoot(t *testing.T) {
 inventory,lease,dir:=stagingInputReleaseFixture(t)
 checkpoint:=inventory.entries[ownedCheckpointName].file
 inputOps:=realOwnedStagingInputReleaseOps();ops:=realOwnedStagingDirectoryReleaseOps()
 inputs:=0;rootSync,parentSync:=0,0
 inputOps.unlink=func(fd int,name string,flags int)error{
  assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),"checkpoint evidence")
  inputs++;return unix.Unlinkat(fd,name,flags)
 }
 ops.unlink=func(fd int,name string,flags int)error{
  entries,err:=os.ReadDir(dir);if err!=nil {t.Fatal(err)}
  if flags==0 {
   if inputs!=2 || name!=ownedCheckpointName || len(entries)!=1 || entries[0].Name()!=ownedCheckpointName {t.Fatal("checkpoint not last",inputs,entries)}
  } else if flags!=unix.AT_REMOVEDIR || name!=filepath.Base(dir) || len(entries)!=0 {t.Fatal("rmdir not restricted to empty pinned root")}
  return unix.Unlinkat(fd,name,flags)
 }
 ops.syncDirectory=func(f *os.File)error{if f==inventory.root {rootSync++} else if f==inventory.parent {parentSync++} else {t.Fatal("unexpected sync descriptor")};return f.Sync()}
 result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,inputOps,ops)
 if err!=nil || !result.Complete || !result.Inputs.InputsReleased || len(result.Inputs.Removed)!=2 || !result.CheckpointUnlinked || !result.DirectoryRemoved || result.CheckpointRestored || len(result.CheckpointEvidence)!=0 {t.Fatal(result,err)}
 if rootSync!=1 || parentSync!=1 || !lease.held() || len(inventory.entries)!=0 {t.Fatal("durability/membership/borrow mismatch")}
 if _,err:=os.Lstat(dir);!os.IsNotExist(err) {t.Fatal("empty root not removed",err)}
 if _,err:=checkpoint.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("old checkpoint pin leaked",err)}
}

func TestStagingDirectoryReleaseForeignEntriesRefuseBeforeInputs(t *testing.T) {
 for _,kind:=range []string{"file","directory","symlink","fifo"} {t.Run(kind,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  outside:=filepath.Join(t.TempDir(),"outside");putStagingAdmissionFile(t,outside,"outside bytes")
  path:=filepath.Join(dir,"foreign");var err error
  switch kind {case "file":err=os.WriteFile(path,[]byte("foreign bytes"),0600);case "directory":err=os.Mkdir(path,0700);case "symlink":err=os.Symlink(outside,path);case "fifo":err=unix.Mkfifo(path,0600)}
  if err!=nil {t.Fatal(err)}
  calls:=0;inputOps:=realOwnedStagingInputReleaseOps();ops:=realOwnedStagingDirectoryReleaseOps()
  deny:=func(int,string,int)error{calls++;return unix.EPERM};inputOps.unlink=deny;ops.unlink=deny
  result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,inputOps,ops)
  if err==nil || result.Complete || calls!=0 || result.CheckpointUnlinked {t.Fatal(result,err,calls)}
  assertStagingReleaseCheckpoint(t,inventory,dir)
  assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
  assertPublicationBytes(t,outside,"outside bytes")
  if _,err:=os.Lstat(path);err!=nil {t.Fatal(err)}
 })}
}

func TestStagingDirectoryReleaseStopsBeforeCheckpointOnInputFailure(t *testing.T) {
 inventory,lease,dir:=stagingInputReleaseFixture(t)
 inputOps:=realOwnedStagingInputReleaseOps();ops:=realOwnedStagingDirectoryReleaseOps();checkpointCalls:=0
 inputOps.unlink=func(fd int,name string,flags int)error{if name=="event.mp4" {return unix.EIO};return unix.Unlinkat(fd,name,flags)}
 ops.unlink=func(int,string,int)error{checkpointCalls++;return unix.EPERM}
 result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,inputOps,ops)
 if !errors.Is(err,unix.EIO) || result.Complete || len(result.Inputs.Removed)!=1 || checkpointCalls!=0 || result.CheckpointUnlinked {t.Fatal(result,err)}
 assertStagingReleaseCheckpoint(t,inventory,dir)
 assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
}

func TestStagingDirectoryReleaseRestoresCheckpointOnFinalPhaseFailure(t *testing.T) {
 for _,stage:=range []string{"checkpoint unlink","late before scan","late at rmdir","rmdir error","root sync","cancel","guard revoked"} {t.Run(stage,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t);oldPin:=inventory.entries[ownedCheckpointName].file
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  ops:=realOwnedStagingDirectoryReleaseOps();revoked:=false;syncs:=0
  guard:=func()error{if revoked {return unix.EPERM};return nil}
  ops.unlink=func(fd int,name string,flags int)error{
   if flags==0 {
    if stage=="checkpoint unlink" {return unix.EIO}
    if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
    switch stage {case "late before scan":putStagingAdmissionFile(t,filepath.Join(dir,"late"),"late bytes");case "cancel":cancel();case "guard revoked":revoked=true}
    return nil
   }
   if stage=="late at rmdir" {putStagingAdmissionFile(t,filepath.Join(dir,"late"),"late bytes")}
   if stage=="rmdir error" {return unix.EPERM}
   return unix.Unlinkat(fd,name,flags)
  }
  ops.syncDirectory=func(f *os.File)error{syncs++;if stage=="root sync" {return unix.EIO};return f.Sync()}
  result,err:=releaseOwnedStagingWithOps(ctx,lease,inventory,guard,realOwnedStagingInputReleaseOps(),ops)
  if err==nil || result.Complete || result.DirectoryRemoved || !result.Inputs.InputsReleased || len(result.Inputs.Removed)!=2 || !lease.held() {t.Fatal(result,err)}
  if stage=="checkpoint unlink" {
   if result.CheckpointUnlinked || result.CheckpointRestored || syncs!=0 {t.Fatal(result,syncs)}
  } else {
   if !result.CheckpointUnlinked || !result.CheckpointRestored || syncs!=1 {t.Fatal(result,syncs)}
   if _,err:=oldPin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("old checkpoint descriptor leaked",err)}
  }
  if stage=="cancel" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
  assertStagingReleaseCheckpoint(t,inventory,dir)
  if strings.HasPrefix(stage,"late") {assertPublicationBytes(t,filepath.Join(dir,"late"),"late bytes")}
 })}
}

func TestStagingDirectoryReleaseRestorationFailureReportsOriginalBytes(t *testing.T) {
 for _,kind:=range []string{"foreign checkpoint","restore write failure","error after owned restore"} {t.Run(kind,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t);ops:=realOwnedStagingDirectoryReleaseOps()
  ops.unlink=func(fd int,name string,flags int)error{
   if flags==unix.AT_REMOVEDIR {return unix.EPERM}
   if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
   if kind=="foreign checkpoint" {putStagingAdmissionFile(t,filepath.Join(dir,ownedCheckpointName),"foreign checkpoint")}
   return nil
  }
  if kind=="restore write failure" {ops.restore=func(context.Context,*mediaMutationLease,*ownedStagingInventory,[]byte)error{return unix.ENOSPC}}
  if kind=="error after owned restore" {ops.restore=func(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,data []byte)error{return errors.Join(writeOwnedEventRecovery(ctx,lease,inventory,data),unix.EIO)}}
  result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,realOwnedStagingInputReleaseOps(),ops)
  if err==nil || result.Complete || result.DirectoryRemoved || !result.CheckpointUnlinked || string(result.CheckpointEvidence)!="checkpoint evidence" {t.Fatal(result,err)}
  if result.CheckpointRestored!=(kind=="error after owned restore") {t.Fatal("restoration claim does not match owned evidence",result)}
  if kind=="foreign checkpoint" {
   assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),"foreign checkpoint")
   if _,ok:=inventory.entries[ownedCheckpointName];ok {t.Fatal("foreign checkpoint adopted")}
  }
  if kind=="restore write failure" {if _,err:=os.Stat(filepath.Join(dir,ownedCheckpointName));!os.IsNotExist(err) {t.Fatal("failed restoration invented evidence",err)}}
  if kind=="error after owned restore" {assertStagingReleaseCheckpoint(t,inventory,dir)}
 })}
}

func TestStagingDirectoryReleaseRootReplacementNeverTouchesReplacement(t *testing.T) {
 for _,parent:=range []bool{false,true} {t.Run(map[bool]string{false:"root",true:"parent"}[parent],func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t);ops:=realOwnedStagingDirectoryReleaseOps();rmdirs:=0
  ops.unlink=func(fd int,name string,flags int)error{
   if flags!=0 {rmdirs++;return unix.EPERM}
   if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
   target:=dir;if parent {target=filepath.Dir(dir)}
   if err:=os.Rename(target,target+"-saved");err!=nil {t.Fatal(err)}
   if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
   putStagingAdmissionFile(t,filepath.Join(dir,"replacement"),"replacement bytes")
   return nil
  }
  result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,realOwnedStagingInputReleaseOps(),ops)
  if err==nil || result.Complete || result.DirectoryRemoved || result.CheckpointRestored || rmdirs!=0 || string(result.CheckpointEvidence)!="checkpoint evidence" {t.Fatal(result,err)}
  assertPublicationBytes(t,filepath.Join(dir,"replacement"),"replacement bytes")
 })}
}

func TestStagingDirectoryReleasePostRmdirErrorIsNotRollback(t *testing.T) {
 for _,cancelLate:=range []bool{false,true} {t.Run(map[bool]string{false:"parent sync error",true:"late cancellation"}[cancelLate],func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t);ops:=realOwnedStagingDirectoryReleaseOps()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();parentSyncs:=0
  ops.syncDirectory=func(f *os.File)error{
   if f==inventory.parent {parentSyncs++;if cancelLate {cancel();return f.Sync()};return unix.EIO};return f.Sync()
  }
  ops.restore=func(context.Context,*mediaMutationLease,*ownedStagingInventory,[]byte)error{t.Fatal("removed directory restoration attempted");return unix.EIO}
  result,err:=releaseOwnedStagingWithOps(ctx,lease,inventory,stagingReleaseAllowed,realOwnedStagingInputReleaseOps(),ops)
  if err==nil || result.Complete || !result.DirectoryRemoved || !result.CheckpointUnlinked || result.CheckpointRestored || parentSyncs!=1 || string(result.CheckpointEvidence)!="checkpoint evidence" {t.Fatal(result,err)}
  if _,err:=os.Stat(dir);!os.IsNotExist(err) {t.Fatal("post-rmdir error falsely restored directory",err)}
  if cancelLate && !errors.Is(err,context.Canceled) {t.Fatal(err)}
  if !lease.held() {t.Fatal("borrowed ownership released")}
 })}
}

func TestStagingDirectoryReleaseBoundsCheckpointBeforeAnyUnlink(t *testing.T) {
 for _,size:=range []int{0,maxRecoveryManifestBytes+1} {t.Run(map[bool]string{false:"empty",true:"oversized"}[size>0],func(t *testing.T){
  inventory,lease,dir:=stagingInventoryFixture(t)
  checkpoint:=stagingInventoryCreated(t,dir,ownedCheckpointName,strings.Repeat("x",size))
  if err:=inventory.Remember(context.Background(),lease,ownedCheckpointName,checkpoint);err!=nil {t.Fatal(err)}
  input:=stagingInventoryCreated(t,dir,"event.mp4","capture bytes")
  if err:=inventory.Remember(context.Background(),lease,"event.mp4",input);err!=nil {t.Fatal(err)}
  calls:=0;inputOps:=realOwnedStagingInputReleaseOps();ops:=realOwnedStagingDirectoryReleaseOps()
  deny:=func(int,string,int)error{calls++;return unix.EPERM};inputOps.unlink=deny;ops.unlink=deny
  result,err:=releaseOwnedStagingWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,inputOps,ops)
  if err==nil || result.Complete || calls!=0 {t.Fatal(result,err,calls)}
  assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture bytes")
 })}
}

func TestStagingDirectoryReleaseCanceledOrRefusedAdmissionRetainsAll(t *testing.T) {
 for _,kind:=range []string{"cancel","guard","lease"} {t.Run(kind,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  guard:=stagingReleaseAllowed;useLease:=lease
  switch kind {case "cancel":cancel();case "guard":guard=func()error{return unix.EPERM};case "lease":useLease=nil}
  result,err:=releaseOwnedStagingLeased(ctx,useLease,inventory,guard)
  if err==nil || result.Complete || result.CheckpointUnlinked || len(result.Inputs.Removed)!=0 {t.Fatal(result,err)}
  assertStagingReleaseCheckpoint(t,inventory,dir)
  assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
 })}
}
