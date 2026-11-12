package main

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "runtime"

 "golang.org/x/sys/unix"
)

const ownedCheckpointName="recovery.json"
type ownedCheckpointWriteOps struct {
 write func(*os.File,[]byte) (int,error)
 syncFile func(*os.File) error
 closeFile func(*os.File) error
 rename func(int,string,string,bool) error
 syncDirectory func(*os.File) error
}
func realOwnedCheckpointWriteOps() ownedCheckpointWriteOps {
 return ownedCheckpointWriteOps{
  write:func(f *os.File,data []byte)(int,error){return f.Write(data)},
  syncFile:func(f *os.File)error{return f.Sync()},
  closeFile:func(f *os.File)error{return f.Close()},
  rename:func(fd int,from,to string,replace bool)error{
   if replace {return unix.Renameat(fd,from,fd,to)}
   return unix.Renameat2(fd,from,fd,to,unix.RENAME_NOREPLACE)
  },
  syncDirectory:func(f *os.File)error{return f.Sync()},
 }
}

// Check only the checkpoint slot: capture inputs may still be changing or not
// registered. Full inventory preflight is a separate prerequisite for release.
func (inventory *ownedStagingInventory) checkOwnedCheckpoint(ctx context.Context,lease *mediaMutationLease) error {
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 if inventory.entries==nil {return errStagingInventory}
 previous,exists:=inventory.entries[ownedCheckpointName]
 var named unix.Stat_t
 err:=unix.Fstatat(int(inventory.root.Fd()),ownedCheckpointName,&named,unix.AT_SYMLINK_NOFOLLOW)
 if !exists {
  if !errors.Is(err,unix.ENOENT) || len(inventory.entries)>=ownedStagingEntryLimit {return errStagingInventory}
  return nil
 }
 if err!=nil {return err}
 if previous.file==nil {return errStagingInventory}
 var held unix.Stat_t
 if err:=unix.Fstat(int(previous.file.Fd()),&held);err!=nil {return err}
 if !stagingInventoryIdentity(previous.identity,held) || !stagingInventoryIdentity(previous.identity,named) {return errStagingInventory}
 return nil
}

func writeOwnedEventRecovery(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,data []byte) error {
 return writeOwnedEventRecoveryWithOps(ctx,lease,inventory,data,realOwnedCheckpointWriteOps())
}

// Caller serializes inventory use and borrows one shared lease for this call.
// Never adopt a discovered checkpoint, reacquire ownership, remove evidence or
// infer cleanup eligibility from successful persistence or inventory membership.
func writeOwnedEventRecoveryWithOps(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,data []byte,ops ownedCheckpointWriteOps) (result error) {
 defer runtime.KeepAlive(inventory)
 if len(data)>maxRecoveryManifestBytes || ops.write==nil || ops.syncFile==nil || ops.closeFile==nil || ops.rename==nil || ops.syncDirectory==nil {return errStagingInventory}
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return err}
 var nonce [16]byte
 if _,err:=rand.Read(nonce[:]);err!=nil {return err}
 temporary:="recovery-owned-"+hex.EncodeToString(nonce[:])+".json.tmp"
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return err}
 fd:=int(inventory.root.Fd())
 created,err:=unix.Openat(fd,temporary,unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC,0600)
 if err!=nil {return err}
 file:=os.NewFile(uintptr(created),temporary)
 var pinned *os.File
 creatorClosed,transferred:=false,false
 defer func(){
  if pinned!=nil && !transferred {result=errors.Join(result,pinned.Close())}
  if !creatorClosed {if err:=file.Close();!errors.Is(err,os.ErrClosed) {result=errors.Join(result,err)}}
 }()
 n,err:=ops.write(file,data);if err!=nil {return err};if n!=len(data) {return io.ErrShortWrite}
 if err:=ctx.Err();err!=nil {return err}
 if err:=ops.syncFile(file);err!=nil {return err}
 // Retain creator provenance while preserving the legacy pre-rename close check.
 duplicate,err:=unix.FcntlInt(file.Fd(),unix.F_DUPFD_CLOEXEC,0);if err!=nil {return err}
 pinned=os.NewFile(uintptr(duplicate),temporary)
 if err:=ops.closeFile(file);err!=nil {return err}
 creatorClosed=true
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return err}
 var before,named unix.Stat_t
 if err:=unix.Fstat(duplicate,&before);err!=nil {return err}
 if before.Mode&unix.S_IFMT!=unix.S_IFREG || before.Size!=int64(len(data)) {return errStagingInventory}
 if err:=unix.Fstatat(fd,temporary,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
 if !stagingInventoryIdentity(before,named) {return errStagingInventory}
 if err:=inventory.checkOwnedCheckpoint(ctx,lease);err!=nil {return err}
 previous,replacing:=inventory.entries[ownedCheckpointName]
 if err:=ctx.Err();err!=nil {return err}
 if err:=ops.rename(fd,temporary,ownedCheckpointName,replacing);err!=nil {return err}
 // As in the legacy writer, finish directory durability after rename. A late
 // cancellation or error retains disk evidence and cannot authorize release.
 if err:=ops.syncDirectory(inventory.root);err!=nil {return err}
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 var held unix.Stat_t
 if err:=unix.Fstat(duplicate,&held);err!=nil {return err}
 if err:=unix.Fstatat(fd,ownedCheckpointName,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
 // Rename can change ctime; capture the post-rename full identity for preflight.
 if before.Dev!=held.Dev || before.Ino!=held.Ino || before.Mode!=held.Mode || before.Uid!=held.Uid || before.Gid!=held.Gid || before.Size!=held.Size || before.Mtim!=held.Mtim || !stagingInventoryIdentity(held,named) {return errStagingInventory}
 if err:=ctx.Err();err!=nil {return err}
 inventory.entries[ownedCheckpointName]=ownedStagingEntry{pinned,held}
 transferred=true
 if replacing {return previous.file.Close()}
 return nil
}
