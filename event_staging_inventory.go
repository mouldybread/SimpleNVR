package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "runtime"
 "strings"

 "golang.org/x/sys/unix"
)

const ownedStagingEntryLimit=maxRecoveryInputs+3
var errStagingInventory=errors.New("staging inventory refused: invalid ownership, entry, identity or limit")
type ownedStagingEntry struct {file *os.File;identity unix.Stat_t}
// Read-only identity evidence, NOT publication proof or permission to unlink.
// Methods are serialized by the owning caller; never copy or close while borrowed.
type ownedStagingInventory struct {
 parent,root *os.File
 path string
 parentID,rootID unix.Stat_t
 entries map[string]ownedStagingEntry
}

func stagingInventoryDirectory(fd int,name string) (*os.File,error) {
 opened,err:=unix.Openat(fd,name,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return nil,err}
 return os.NewFile(uintptr(opened),name),nil
}
func stagingInventoryIdentity(a,b unix.Stat_t) bool {
 return a.Dev==b.Dev && a.Ino==b.Ino && a.Mode==b.Mode && a.Uid==b.Uid && a.Gid==b.Gid && a.Size==b.Size && a.Mtim==b.Mtim && a.Ctim==b.Ctim
}
func (inventory *ownedStagingInventory) check(ctx context.Context,lease *mediaMutationLease) error {
 defer runtime.KeepAlive(inventory)
 if err:=ctx.Err();err!=nil {return err}
 if !lease.held() || lease.gate!=recordingMediaOwnership || inventory==nil || inventory.parent==nil || inventory.root==nil {return errStagingInventory}
 for _,entry:=range []struct{fd int;name string;want unix.Stat_t}{
  {unix.AT_FDCWD,filepath.Dir(inventory.path),inventory.parentID},
  {int(inventory.parent.Fd()),filepath.Base(inventory.path),inventory.rootID},
 } {
  var current unix.Stat_t
  if err:=unix.Fstatat(entry.fd,entry.name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if current.Dev!=entry.want.Dev || current.Ino!=entry.want.Ino || current.Mode&unix.S_IFMT!=unix.S_IFDIR || current.Mode!=entry.want.Mode || current.Uid!=entry.want.Uid || current.Gid!=entry.want.Gid {return errStagingInventory}
 }
 return nil
}

// Call immediately while the new job directory is owned, not during cleanup.
// No media acquisition here: the caller borrows an already held shared lease.
func openOwnedStagingInventory(ctx context.Context,lease *mediaMutationLease,dir string) (*ownedStagingInventory,error) {
 if err:=ctx.Err();err!=nil {return nil,err}
 if !lease.held() || lease.gate!=recordingMediaOwnership {return nil,errStagingInventory}
 dir=filepath.Clean(dir)
 if !filepath.IsAbs(dir) || dir==string(filepath.Separator) {return nil,errStagingInventory}
 inventory:=&ownedStagingInventory{path:dir,entries:make(map[string]ownedStagingEntry)}
 fail:=func(err error)(*ownedStagingInventory,error){return nil,errors.Join(err,inventory.Close())}
 var err error
 inventory.parent,err=stagingInventoryDirectory(unix.AT_FDCWD,filepath.Dir(dir));if err!=nil {return fail(err)}
 inventory.root,err=stagingInventoryDirectory(int(inventory.parent.Fd()),filepath.Base(dir));if err!=nil {return fail(err)}
 if err:=unix.Fstat(int(inventory.parent.Fd()),&inventory.parentID);err!=nil {return fail(err)}
 if err:=unix.Fstat(int(inventory.root.Fd()),&inventory.rootID);err!=nil {return fail(err)}
 if err:=inventory.check(ctx,lease);err!=nil {return fail(err)}
 return inventory,nil
}

// The descriptor MUST originate from the caller's creation/pinning operation.
// Opening a filename found at cleanup time does not establish ownership.
// Pin a CLOEXEC duplicate so closing the original cannot permit inode reuse.
// Duplicate names are refused: expected checkpoint replacement needs a later,
// explicit owner-writer integration, never implicit re-adoption by filename.
func (inventory *ownedStagingInventory) Remember(ctx context.Context,lease *mediaMutationLease,name string,owned *os.File) error {
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 if owned==nil || name=="" || name=="." || name==".." || strings.ContainsAny(name,"/\\\x00") || len(inventory.entries)>=ownedStagingEntryLimit {return errStagingInventory}
 if _,exists:=inventory.entries[name];exists {return errStagingInventory}
 duplicate,err:=unix.FcntlInt(owned.Fd(),unix.F_DUPFD_CLOEXEC,0)
 runtime.KeepAlive(owned)
 if err!=nil {return err}
 pinned:=os.NewFile(uintptr(duplicate),name)
 fail:=func(err error) error {return errors.Join(err,pinned.Close())}
 var held,named unix.Stat_t
 if err:=unix.Fstat(duplicate,&held);err!=nil {return fail(err)}
 if held.Mode&unix.S_IFMT!=unix.S_IFREG {return fail(errStagingInventory)}
 if err:=unix.Fstatat(int(inventory.root.Fd()),name,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return fail(err)}
 if !stagingInventoryIdentity(held,named) {return fail(errStagingInventory)}
 if err:=inventory.check(ctx,lease);err!=nil {return fail(err)}
 inventory.entries[name]=ownedStagingEntry{pinned,held}
 return nil
}

// Inspect every entry before a future cleanup implementation may even consider
// unlinking. A fresh directory description makes repeated preflights independent.
// This function never removes, rewrites, restores or authorizes any file.
func (inventory *ownedStagingInventory) Preflight(ctx context.Context,lease *mediaMutationLease) (result error) {
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 scan,err:=stagingInventoryDirectory(int(inventory.root.Fd()),".");if err!=nil {return err}
 defer func(){result=errors.Join(result,scan.Close())}()
 seen:=make(map[string]bool)
 for {
  if err:=inventory.check(ctx,lease);err!=nil {return err}
  names,readErr:=scan.Readdirnames(32)
  for _,name:=range names {
   if len(seen)>=ownedStagingEntryLimit || seen[name] {return errStagingInventory}
   seen[name]=true
   entry,exists:=inventory.entries[name];if !exists {return errStagingInventory}
   if err:=inventory.check(ctx,lease);err!=nil {return err}
   var held,named unix.Stat_t
   if err:=unix.Fstat(int(entry.file.Fd()),&held);err!=nil {return err}
   if err:=unix.Fstatat(int(inventory.root.Fd()),name,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
   if !stagingInventoryIdentity(entry.identity,held) || !stagingInventoryIdentity(entry.identity,named) {return errStagingInventory}
  }
  if readErr==io.EOF {break};if readErr!=nil {return readErr}
 }
 if len(seen)!=len(inventory.entries) {return errStagingInventory}
 return inventory.check(ctx,lease)
}

func (inventory *ownedStagingInventory) Close() (result error) {
 if inventory==nil {return nil}
 for name,entry:=range inventory.entries {result=errors.Join(result,entry.file.Close());delete(inventory.entries,name)}
 for _,file:=range []*os.File{inventory.root,inventory.parent} {if file!=nil {result=errors.Join(result,file.Close())}}
 inventory.root=nil;inventory.parent=nil
 return result
}
