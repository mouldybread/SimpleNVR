package main

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func TestOwnedCheckpointCreatesReplacesAndRetainsCreatorDescriptor(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 for _,data:=range []string{"first checkpoint","second checkpoint"} {
  previous:=inventory.entries[ownedCheckpointName].file
  if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte(data));err!=nil {t.Fatal(err)}
  assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),data)
  if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
  if previous!=nil {if _,err:=previous.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("superseded pin not closed",err)}}
  pinned:=inventory.entries[ownedCheckpointName].file
  flags,err:=unix.FcntlInt(pinned.Fd(),unix.F_GETFD,0);if err!=nil || flags&unix.FD_CLOEXEC==0 {t.Fatal(flags,err)}
 }
 if !lease.held() || len(inventory.entries)!=1 {t.Fatal("borrowed ownership or inventory cardinality changed")}
}

func TestOwnedCheckpointRefusesUnknownAndChangedCheckpoint(t *testing.T) {
 for _,change:=range []string{"unrecorded","replacement","modified","root","parent"} {t.Run(change,func(t *testing.T){
  inventory,lease,dir:=stagingInventoryFixture(t)
  path:=filepath.Join(dir,ownedCheckpointName)
  if change=="unrecorded" {if err:=os.WriteFile(path,[]byte("foreign"),0600);err!=nil {t.Fatal(err)}} else {
   if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("original"));err!=nil {t.Fatal(err)}
   switch change {
   case "replacement":if err:=os.Rename(path,path+".saved");err!=nil {t.Fatal(err)};if err:=os.WriteFile(path,[]byte("foreign"),0600);err!=nil {t.Fatal(err)}
   case "modified":if err:=os.WriteFile(path,[]byte("foreign changed length"),0600);err!=nil {t.Fatal(err)}
   case "root":if err:=os.Rename(dir,dir+"-saved");err!=nil {t.Fatal(err)};if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
   case "parent":parent:=filepath.Dir(dir);if err:=os.Rename(parent,parent+"-saved");err!=nil {t.Fatal(err)};if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
   }
  }
  if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("must not write"));err==nil {t.Fatal("changed checkpoint accepted",change)}
  if change=="root" || change=="parent" {entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=0 {t.Fatal("replacement directory touched",entries,err)}} else {
   want:="foreign";if change=="modified" {want="foreign changed length"};assertPublicationBytes(t,path,want)
  }
 })}
}

func TestOwnedCheckpointFailuresRetainEvidenceAndRefusePreflight(t *testing.T) {
 for _,phase:=range []string{"write","short-write","file-sync","file-close","rename","directory-sync","late-cancel"} {t.Run(phase,func(t *testing.T){
  inventory,lease,dir:=stagingInventoryFixture(t)
  if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("old checkpoint"));err!=nil {t.Fatal(err)}
  old:=inventory.entries[ownedCheckpointName].file
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  ops:=realOwnedCheckpointWriteOps();syncedDirectory:=false
  switch phase {
  case "write":ops.write=func(f *os.File,_ []byte)(int,error){n,_:=f.WriteString("part");return n,os.ErrPermission}
  case "short-write":ops.write=func(f *os.File,_ []byte)(int,error){return f.WriteString("part")}
  case "file-sync":ops.syncFile=func(*os.File)error{return os.ErrPermission}
  case "file-close":ops.closeFile=func(f *os.File)error{return errors.Join(f.Close(),os.ErrPermission)}
  case "rename":ops.rename=func(int,string,string,bool)error{return os.ErrPermission}
  case "directory-sync":ops.syncDirectory=func(*os.File)error{return os.ErrPermission}
  case "late-cancel":ops.rename=func(fd int,from,to string,replace bool)error{err:=realOwnedCheckpointWriteOps().rename(fd,from,to,replace);cancel();return err};ops.syncDirectory=func(f *os.File)error{syncedDirectory=true;return f.Sync()}
  }
  err:=writeOwnedEventRecoveryWithOps(ctx,lease,inventory,[]byte("new checkpoint"),ops)
  wantErr:=os.ErrPermission;if phase=="short-write" {wantErr=io.ErrShortWrite};if phase=="late-cancel" {wantErr=context.Canceled}
  if !errors.Is(err,wantErr) {t.Fatal(phase,err)}
  if phase=="late-cancel" && !syncedDirectory {t.Fatal("late cancellation skipped directory sync")}
  want:="old checkpoint";if phase=="directory-sync" || phase=="late-cancel" {want="new checkpoint"}
  assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),want)
  if inventory.entries[ownedCheckpointName].file!=old {t.Fatal("failed write silently adopted replacement")}
  if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("failed checkpoint gave clean inventory")}
  entries,err:=os.ReadDir(dir);if err!=nil {t.Fatal(err)}
  if phase!="directory-sync" && phase!="late-cancel" {
   if len(entries)!=2 {t.Fatal("temporary evidence not retained",entries)}
   for _,entry:=range entries {if entry.Name()!=ownedCheckpointName {data,err:=os.ReadFile(filepath.Join(dir,entry.Name()));if err!=nil || len(data)==0 {t.Fatal("failed write bytes lost",err)}}}
  }
  if !lease.held() {t.Fatal("writer released caller lease")}
 })}
}

func TestOwnedCheckpointDoesNotAdoptOtherEntries(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 foreign:=filepath.Join(dir,"foreign.tmp");if err:=os.WriteFile(foreign,[]byte("retain"),0600);err!=nil {t.Fatal(err)}
 if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("checkpoint"));err!=nil {t.Fatal(err)}
 if _,exists:=inventory.entries["foreign.tmp"];exists {t.Fatal("writer adopted foreign entry")}
 if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("foreign entry accepted for release")}
 assertPublicationBytes(t,foreign,"retain")
}

func TestOwnedCheckpointAllowsReplacementAtPersistentEntryLimit(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("old"));err!=nil {t.Fatal(err)}
 for i:=1;i<ownedStagingEntryLimit;i++ {
  name:=fmt.Sprintf("input-%03d",i)
  f:=stagingInventoryCreated(t,dir,name,"input")
  if err:=inventory.Remember(context.Background(),lease,name,f);err!=nil {t.Fatal(err)}
  if err:=f.Close();err!=nil {t.Fatal(err)}
 }
 if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,[]byte("new"));err!=nil {t.Fatal(err)}
 if len(inventory.entries)!=ownedStagingEntryLimit {t.Fatal("persistent budget changed")}
 if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
}

func TestOwnedCheckpointInvalidAdmissionDoesNotCreateFiles(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=writeOwnedEventRecovery(ctx,lease,inventory,[]byte("no"));!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if err:=writeOwnedEventRecovery(context.Background(),lease,inventory,make([]byte,maxRecoveryManifestBytes+1));err==nil {t.Fatal("oversized manifest accepted")}
 if err:=writeOwnedEventRecoveryWithOps(context.Background(),lease,inventory,nil,ownedCheckpointWriteOps{});err==nil {t.Fatal("missing ops accepted")}
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 lease.release()
 for _,invalid:=range []*mediaMutationLease{nil,lease,foreign} {if err:=writeOwnedEventRecovery(context.Background(),invalid,inventory,[]byte("no"));err==nil {t.Fatal("invalid lease accepted")}}
 entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=0 {t.Fatal("refused admission touched directory",entries,err)}
 if !foreign.held() {t.Fatal("writer disturbed foreign owner")}
}

func TestOwnedCheckpointFirstRenameCannotOverwriteLateCheckpoint(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 ops:=realOwnedCheckpointWriteOps()
 ops.rename=func(fd int,from,to string,replace bool)error {
  if replace {t.Error("first checkpoint requested replacement")}
  if err:=os.WriteFile(filepath.Join(dir,to),[]byte("late foreign checkpoint"),0600);err!=nil {return err}
  return realOwnedCheckpointWriteOps().rename(fd,from,to,replace)
 }
 if err:=writeOwnedEventRecoveryWithOps(context.Background(),lease,inventory,[]byte("new"),ops);!errors.Is(err,unix.EEXIST) {t.Fatal(err)}
 assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),"late foreign checkpoint")
 if len(inventory.entries)!=0 {t.Fatal("late checkpoint adopted")}
 if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("unrecorded checkpoint accepted")}
}
