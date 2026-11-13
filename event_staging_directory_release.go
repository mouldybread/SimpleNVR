package main

import (
 "bytes"
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"

 "golang.org/x/sys/unix"
)

// Explicit partial outcomes, not rollback or cleanup authorization. Evidence is
// at most maxRecoveryManifestBytes, returned only after checkpoint unlink/error;
// it is an in-memory fallback, NOT a claim of durable recovery preservation.
type ownedStagingReleaseResult struct {
 Inputs ownedStagingInputReleaseResult
 CheckpointUnlinked bool
 DirectoryRemoved bool
 CheckpointRestored bool
 Complete bool
 CheckpointEvidence []byte
}
type ownedStagingDirectoryReleaseOps struct {
 unlink func(int,string,int) error
 syncDirectory func(*os.File) error
 restore func(context.Context,*mediaMutationLease,*ownedStagingInventory,[]byte) error
}
func realOwnedStagingDirectoryReleaseOps() ownedStagingDirectoryReleaseOps {
 return ownedStagingDirectoryReleaseOps{unix.Unlinkat,func(f *os.File)error{return f.Sync()},writeOwnedEventRecovery}
}

// Read ONLY the previously pinned checkpoint; never discover/re-adopt by name.
// Slot validation before/after bounded ReadAt preserves the original descriptor
// offset and refuses replacement, mutation, missing or oversized evidence.
func readOwnedStagingReleaseCheckpoint(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory) (*os.File,[]byte,error) {
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return nil,nil,err}
 entry,ok:=inventory.entries[ownedCheckpointName]
 if !ok || entry.file==nil || entry.identity.Size<=0 || entry.identity.Size>maxRecoveryManifestBytes {return nil,nil,errStagingInventory}
 data:=make([]byte,int(entry.identity.Size))
 n,err:=entry.file.ReadAt(data,0)
 if err!=nil && !errors.Is(err,io.EOF) {return nil,nil,err}
 if n!=len(data) {return nil,nil,io.ErrUnexpectedEOF}
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return nil,nil,err}
 return entry.file,data,nil
}

// Opt-in complete engine; NO production caller. Caller must supply independent
// positive eligibility/registry checks, serialize inventory use and own the lease.
// A successful input-only result is NOT authorization to call this function.
func releaseOwnedStagingLeased(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,guard func()error) (ownedStagingReleaseResult,error) {
 return releaseOwnedStagingWithOps(ctx,lease,inventory,guard,realOwnedStagingInputReleaseOps(),realOwnedStagingDirectoryReleaseOps())
}
func releaseOwnedStagingWithOps(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,guard func()error,inputOps ownedStagingInputReleaseOps,ops ownedStagingDirectoryReleaseOps) (result ownedStagingReleaseResult,resultErr error) {
 if guard==nil || ops.unlink==nil || ops.syncDirectory==nil || ops.restore==nil {return result,errStagingInventory}
 if err:=inventory.check(ctx,lease);err!=nil {return result,err}
 if err:=guard();err!=nil {return result,err}
 // Validate/bound recovery bytes BEFORE the input engine may unlink anything.
 if _,_,err:=readOwnedStagingReleaseCheckpoint(ctx,lease,inventory);err!=nil {return result,err}
 result.Inputs,resultErr=releaseOwnedStagingInputsWithOps(ctx,lease,inventory,guard,inputOps)
 if resultErr!=nil {return result,resultErr}
 check:=func()error{
  if err:=inventory.check(ctx,lease);err!=nil {return err}
  return guard()
 }
 if err:=check();err!=nil {return result,err}
 if len(inventory.entries)!=1 {return result,errStagingInventory}
 if err:=inventory.Preflight(ctx,lease);err!=nil {return result,err}
 checkpoint,data,err:=readOwnedStagingReleaseCheckpoint(ctx,lease,inventory)
 if err!=nil {return result,err}
 defer func(){
  if result.CheckpointUnlinked {
   if !result.DirectoryRemoved {
    // Restoration is evidence repair, not renewed deletion permission. Use a
    // separate bounded metadata context, borrowing the SAME held lease. The
    // creator-owned writer refuses foreign checkpoint names and changed roots.
    restoreCtx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace)
    restoreErr:=ops.restore(restoreCtx,lease,inventory,data)
    _,restored,verifyErr:=readOwnedStagingReleaseCheckpoint(restoreCtx,lease,inventory)
    result.CheckpointRestored=verifyErr==nil && bytes.Equal(restored,data)
    if verifyErr==nil && !result.CheckpointRestored {verifyErr=errStagingInventory}
    cancel()
    resultErr=errors.Join(resultErr,restoreErr,verifyErr)
   }
   // Membership was removed only after successful unlink. Old evidence pin
   // remains live through any restoration, then closes on every return path.
   resultErr=errors.Join(resultErr,checkpoint.Close())
   if resultErr!=nil {result.CheckpointEvidence=data}
  }
  result.Complete=result.DirectoryRemoved && resultErr==nil
 }()
 if err:=check();err!=nil {return result,err}
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return result,err}
 if err:=ops.unlink(int(inventory.root.Fd()),ownedCheckpointName,0);err!=nil {return result,err}
 result.CheckpointUnlinked=true
 delete(inventory.entries,ownedCheckpointName)
 // Finish checkpoint-unlink durability even if cancellation has arrived. A
 // failure prevents rmdir and triggers best-effort owned checkpoint restoration.
 if err:=ops.syncDirectory(inventory.root);err!=nil {return result,err}
 if err:=check();err!=nil {return result,err}
 // Exact fresh enumeration must now find no entries. Unlinkat(AT_REMOVEDIR)
 // supplies an additional empty-directory check for arrivals after enumeration.
 if err:=inventory.Preflight(ctx,lease);err!=nil {return result,err}
 if err:=check();err!=nil {return result,err}
 if err:=ops.unlink(int(inventory.parent.Fd()),filepath.Base(inventory.path),unix.AT_REMOVEDIR);err!=nil {return result,err}
 result.DirectoryRemoved=true
 // Parent durability is attempted despite late cancellation. A post-rmdir
 // error cannot restore the removed directory or be described as rollback.
 syncErr:=ops.syncDirectory(inventory.parent)
 return result,errors.Join(syncErr,ctx.Err())
}
