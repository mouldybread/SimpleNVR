package main

import (
 "context"
 "errors"
 "os"
 "sort"
 "strconv"
 "strings"

 "golang.org/x/sys/unix"
)

// This is an input-only partial outcome, never directory cleanup authority.
// Checkpoint and root survive even when InputsReleased is true.
type ownedStagingInputReleaseResult struct {
 Removed []string
 InputsReleased bool
}
type ownedStagingInputReleaseOps struct {
 unlink func(int,string,int) error
 syncDirectory func(*os.File) error
 closeFile func(*os.File) error
}
func realOwnedStagingInputReleaseOps() ownedStagingInputReleaseOps {
 return ownedStagingInputReleaseOps{unix.Unlinkat,func(f *os.File)error{return f.Sync()},func(f *os.File)error{return f.Close()}}
}
func ownedStagingInputName(name string) bool {
 if name=="event.mp4" || name=="concat.txt" {return true}
 if len(name)!=11 || !strings.HasPrefix(name,"pre_") || !strings.HasSuffix(name,".mp4") {return false}
 for _,r:=range name[4:7] {if r<'0' || r>'9' {return false}}
 index,err:=strconv.Atoi(name[4:7])
 return err==nil && index<maxRecoveryInputs
}

// Opt-in primitive only; NO production caller yet. Caller supplies independent
// positive eligibility/registry checks in guard and serializes the inventory.
// Borrow ownership throughout; guard must neither reacquire it nor wait for a
// recording worker. Inventory membership/preflight alone is NOT authorization.
func releaseOwnedStagingInputsLeased(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,guard func()error) (ownedStagingInputReleaseResult,error) {
 return releaseOwnedStagingInputsWithOps(ctx,lease,inventory,guard,realOwnedStagingInputReleaseOps())
}
func releaseOwnedStagingInputsWithOps(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,guard func()error,ops ownedStagingInputReleaseOps) (result ownedStagingInputReleaseResult,resultErr error) {
 result.Removed=[]string{}
 if guard==nil || ops.unlink==nil || ops.syncDirectory==nil || ops.closeFile==nil {return result,errStagingInventory}
 check:=func()error{
  if err:=inventory.check(ctx,lease);err!=nil {return err}
  return guard()
 }
 if err:=check();err!=nil {return result,err}
 if _,ok:=inventory.entries[ownedCheckpointName];!ok {return result,errStagingInventory}
 // Refuse every foreign/unrecorded entry BEFORE the first unlink. The bounded
 // plan contains only recognized, explicitly recorded regular staging inputs.
 if err:=inventory.Preflight(ctx,lease);err!=nil {return result,err}
 names:=make([]string,0,len(inventory.entries))
 for name:=range inventory.entries {
  if name==ownedCheckpointName {continue}
  if !ownedStagingInputName(name) {return result,errStagingInventory}
  names=append(names,name)
 }
 sort.Strings(names)
 finished:=false
 defer func(){
  // An unlink is nonrollback. Attempt directory durability even after a later
  // cancellation/refusal/close failure, without restarting canceled admission.
  if len(result.Removed)>0 {resultErr=errors.Join(resultErr,ops.syncDirectory(inventory.root))}
  if finished && resultErr==nil {
   resultErr=check()
   if resultErr==nil {resultErr=inventory.Preflight(ctx,lease)}
  }
  result.InputsReleased=finished && resultErr==nil
 }()
 for _,name:=range names {
  if err:=check();err!=nil {return result,err}
  if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return result,err}
  entry:=inventory.entries[name]
  if entry.file==nil {return result,errStagingInventory}
  var held,named unix.Stat_t
  if err:=unix.Fstat(int(entry.file.Fd()),&held);err!=nil {return result,err}
  if err:=unix.Fstatat(int(inventory.root.Fd()),name,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return result,err}
  if !stagingInventoryIdentity(entry.identity,held) || !stagingInventoryIdentity(entry.identity,named) {return result,errStagingInventory}
  if err:=check();err!=nil {return result,err}
  // Trusted directory ownership is still required: stat + unlink is not an
  // atomic compare-and-unlink against hostile external mutation.
  if err:=ops.unlink(int(inventory.root.Fd()),name,0);err!=nil {return result,err}
  result.Removed=append(result.Removed,name)
  delete(inventory.entries,name)
  if err:=ops.closeFile(entry.file);err!=nil {
   // Also close when an injected close operation failed before doing so.
   closeErr:=entry.file.Close();if errors.Is(closeErr,os.ErrClosed) {closeErr=nil}
   return result,errors.Join(err,closeErr)
  }
 }
 if err:=check();err!=nil {return result,err}
 // Fresh enumeration detects late arrivals, including when all selected inputs
 // were already removed. Never sweep them, remove the checkpoint or rmdir.
 if err:=inventory.Preflight(ctx,lease);err!=nil {return result,err}
 finished=true
 return result,nil
}
