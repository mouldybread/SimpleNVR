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

func stagingInventoryFixture(t *testing.T) (*ownedStagingInventory,*mediaMutationLease,string) {
 t.Helper();lease:=publishedEngineLease(t)
 dir:=filepath.Join(t.TempDir(),"parent","event");if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
 inventory,err:=openOwnedStagingInventory(context.Background(),lease,dir);if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){if err:=inventory.Close();err!=nil {t.Error(err)}})
 return inventory,lease,dir
}
func stagingInventoryCreated(t *testing.T,dir,name,data string) *os.File {
 t.Helper();file,err:=os.OpenFile(filepath.Join(dir,name),os.O_CREATE|os.O_EXCL|os.O_RDWR,0600);if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){file.Close()})
 if _,err:=file.WriteString(data);err!=nil {t.Fatal(err)}
 return file
}

func TestStagingInventoryPinsDescriptorsAndRepeatsPreflight(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 file:=stagingInventoryCreated(t,dir,"owned","evidence")
 if err:=inventory.Remember(context.Background(),lease,"owned",file);err!=nil {t.Fatal(err)}
 if err:=file.Close();err!=nil {t.Fatal(err)}
 for i:=0;i<2;i++ {if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}}
 for _,pinned:=range []*os.File{inventory.parent,inventory.root,inventory.entries["owned"].file} {
  flags,err:=unix.FcntlInt(pinned.Fd(),unix.F_GETFD,0);if err!=nil || flags&unix.FD_CLOEXEC==0 {t.Fatal("descriptor can leak into children",flags,err)}
 }
 assertPublicationBytes(t,filepath.Join(dir,"owned"),"evidence")
 if !lease.held() {t.Fatal("inventory released borrowed ownership")}
}

func TestStagingInventoryRefusesAllUnrecordedEntryKinds(t *testing.T) {
 for _,kind:=range []string{"regular","nested","symlink","fifo"} {t.Run(kind,func(t *testing.T){
  inventory,lease,dir:=stagingInventoryFixture(t)
  checkpoint:=stagingInventoryCreated(t,dir,"recovery.json","checkpoint")
  if err:=inventory.Remember(context.Background(),lease,"recovery.json",checkpoint);err!=nil {t.Fatal(err)}
  outside:=filepath.Join(t.TempDir(),"outside");if err:=os.WriteFile(outside,[]byte("outside evidence"),0600);err!=nil {t.Fatal(err)}
  foreign:=filepath.Join(dir,"late.tmp")
  var err error
  switch kind {case "regular":err=os.WriteFile(foreign,[]byte("foreign evidence"),0600);case "nested":err=os.Mkdir(foreign,0700);case "symlink":err=os.Symlink(outside,foreign);case "fifo":err=unix.Mkfifo(foreign,0600)}
  if err!=nil {t.Fatal(err)}
  if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("foreign entry accepted",kind)}
  assertPublicationBytes(t,filepath.Join(dir,"recovery.json"),"checkpoint")
  assertPublicationBytes(t,outside,"outside evidence")
  if _,err:=os.Lstat(foreign);err!=nil {t.Fatal("foreign entry changed",err)}
 })}
}

func TestStagingInventoryDetectsEntryAndDirectoryChanges(t *testing.T) {
 for _,change:=range []string{"replacement","symlink","modified","missing","root","parent"} {t.Run(change,func(t *testing.T){
  inventory,lease,dir:=stagingInventoryFixture(t)
  file:=stagingInventoryCreated(t,dir,"owned","original evidence")
  if err:=inventory.Remember(context.Background(),lease,"owned",file);err!=nil {t.Fatal(err)}
  path:=filepath.Join(dir,"owned");want:="original evidence"
  switch change {
  case "replacement","symlink","missing":
   saved:=filepath.Join(t.TempDir(),"saved");if err:=os.Rename(path,saved);err!=nil {t.Fatal(err)}
   if change=="replacement" {if err:=os.WriteFile(path,[]byte("replacement evidence"),0600);err!=nil {t.Fatal(err)}}
   if change=="symlink" {if err:=os.Symlink(saved,path);err!=nil {t.Fatal(err)}}
  case "modified":want="changed content with different length";if err:=os.WriteFile(path,[]byte(want),0600);err!=nil {t.Fatal(err)}
  case "root":if err:=os.Rename(dir,dir+"-retained");err!=nil {t.Fatal(err)};if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
  case "parent":parent:=filepath.Dir(dir);if err:=os.Rename(parent,parent+"-retained");err!=nil {t.Fatal(err)};if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
  }
  if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("changed ownership evidence accepted",change)}
  if _,err:=file.Seek(0,0);err!=nil {t.Fatal(err)}
  got,err:=io.ReadAll(file);if err!=nil || string(got)!=want {t.Fatal("inspection changed original inode",string(got),err)}
 })}
}

func TestStagingInventoryRequiresExplicitMatchingOwnerDescriptor(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 file:=stagingInventoryCreated(t,dir,"owned","evidence")
 for _,name:=range []string{"",".","..","../owned","nested/owned","back\\slash","nul\x00name"} {
  if err:=inventory.Remember(context.Background(),lease,name,file);err==nil {t.Fatal("invalid basename accepted",name)}
 }
 foreign:=stagingInventoryCreated(t,t.TempDir(),"elsewhere","foreign")
 if err:=inventory.Remember(context.Background(),lease,"owned",foreign);err==nil {t.Fatal("pathname substituted for owner descriptor")}
 if err:=inventory.Remember(context.Background(),lease,"owned",nil);err==nil {t.Fatal("nil owner descriptor accepted")}
 if len(inventory.entries)!=0 {t.Fatal("refusal changed inventory")}
 if err:=inventory.Remember(context.Background(),lease,"owned",file);err!=nil {t.Fatal(err)}
 if err:=inventory.Remember(context.Background(),lease,"owned",file);err==nil {t.Fatal("duplicate silently readopted")}
 if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
}

func TestStagingInventoryBoundsOwnedEntries(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 for i:=0;i<ownedStagingEntryLimit;i++ {
  name:=fmt.Sprintf("owned-%03d",i);file:=stagingInventoryCreated(t,dir,name,"evidence")
  if err:=inventory.Remember(context.Background(),lease,name,file);err!=nil {t.Fatal(i,err)}
  if err:=file.Close();err!=nil {t.Fatal(err)}
 }
 if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
 extra:=stagingInventoryCreated(t,dir,"extra","retain")
 if err:=inventory.Remember(context.Background(),lease,"extra",extra);err==nil {t.Fatal("owned-entry bound ignored")}
 if err:=inventory.Preflight(context.Background(),lease);err==nil {t.Fatal("oversized directory accepted")}
 assertPublicationBytes(t,filepath.Join(dir,"extra"),"retain")
}

func TestStagingInventoryLeaseCancellationAndCloseBoundaries(t *testing.T) {
 inventory,lease,dir:=stagingInventoryFixture(t)
 file:=stagingInventoryCreated(t,dir,"owned","evidence")
 if err:=inventory.Remember(context.Background(),lease,"owned",file);err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=inventory.Preflight(ctx,lease);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if !lease.held() {t.Fatal("cancellation released borrowed lease")}
 lease.release()
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 for _,invalid:=range []*mediaMutationLease{nil,lease,foreign} {
  if err:=inventory.Preflight(context.Background(),invalid);err==nil {t.Fatal("invalid lease accepted")}
  if err:=inventory.Remember(context.Background(),invalid,"other",file);err==nil {t.Fatal("invalid lease recorded ownership")}
  if opened,err:=openOwnedStagingInventory(context.Background(),invalid,dir);err==nil {opened.Close();t.Fatal("invalid lease opened inventory")}
 }
 if err:=inventory.Close();err!=nil {t.Fatal(err)};if err:=inventory.Close();err!=nil {t.Fatal("close not idempotent",err)}
 current:=publishedEngineLease(t)
 if err:=inventory.Preflight(context.Background(),current);err==nil {t.Fatal("closed inventory accepted")}
 if !foreign.held() || !current.held() {t.Fatal("inventory disturbed lease owner")}
 assertPublicationBytes(t,filepath.Join(dir,"owned"),"evidence")
}
