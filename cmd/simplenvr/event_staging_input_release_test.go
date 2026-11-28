package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "testing"

 "golang.org/x/sys/unix"
)

func stagingInputReleaseFixture(t *testing.T) (*ownedStagingInventory,*mediaMutationLease,string) {
 t.Helper();inventory,lease,dir:=stagingInventoryFixture(t)
 for name,data:=range map[string]string{ownedCheckpointName:"checkpoint evidence","event.mp4":"capture evidence","concat.txt":"concat evidence"} {
  file:=stagingInventoryCreated(t,dir,name,data)
  if err:=inventory.Remember(context.Background(),lease,name,file);err!=nil {t.Fatal(err)}
  if err:=file.Close();err!=nil {t.Fatal(err)}
 }
 return inventory,lease,dir
}
func stagingReleaseAllowed() error {return nil}
func assertStagingReleaseCheckpoint(t *testing.T,inventory *ownedStagingInventory,dir string) {
 t.Helper();assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),"checkpoint evidence")
 entry,ok:=inventory.entries[ownedCheckpointName];if !ok {t.Fatal("checkpoint membership removed")}
 if _,err:=entry.file.Stat();err!=nil {t.Fatal("checkpoint descriptor closed",err)}
 if _,err:=os.Stat(dir);err!=nil {t.Fatal("job root removed",err)}
}
func TestStagingInputReleaseSuccessPreservesCheckpointAndBorrow(t *testing.T) {
 inventory,lease,dir:=stagingInputReleaseFixture(t)
 pins:=[]*os.File{inventory.entries["concat.txt"].file,inventory.entries["event.mp4"].file}
 result,err:=releaseOwnedStagingInputsLeased(context.Background(),lease,inventory,stagingReleaseAllowed)
 if err!=nil || !result.InputsReleased || !reflect.DeepEqual(result.Removed,[]string{"concat.txt","event.mp4"}) {t.Fatal(result,err)}
 for _,pin:=range pins {if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("removed input pin leaked",err)}}
 assertStagingReleaseCheckpoint(t,inventory,dir)
 if len(inventory.entries)!=1 || !lease.held() {t.Fatal("membership or borrowed lease changed")}
 if err:=inventory.Preflight(context.Background(),lease);err!=nil {t.Fatal(err)}
 again,err:=releaseOwnedStagingInputsLeased(context.Background(),lease,inventory,stagingReleaseAllowed)
 if err!=nil || !again.InputsReleased || len(again.Removed)!=0 {t.Fatal(again,err)}
}
func TestStagingInputReleaseForeignEntriesRefuseBeforeFirstUnlink(t *testing.T) {
 for _,kind:=range []string{"file","directory","symlink","fifo"} {t.Run(kind,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  outside:=filepath.Join(t.TempDir(),"outside");if err:=os.WriteFile(outside,[]byte("outside evidence"),0600);err!=nil {t.Fatal(err)}
  foreign:=filepath.Join(dir,"foreign")
  var err error
  switch kind {case "file":err=os.WriteFile(foreign,[]byte("foreign evidence"),0600);case "directory":err=os.Mkdir(foreign,0700);case "symlink":err=os.Symlink(outside,foreign);case "fifo":err=unix.Mkfifo(foreign,0600)}
  if err!=nil {t.Fatal(err)}
  calls:=0;ops:=realOwnedStagingInputReleaseOps();ops.unlink=func(int,string,int)error{calls++;return errors.New("unexpected unlink")}
  result,err:=releaseOwnedStagingInputsWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,ops)
  if err==nil || result.InputsReleased || len(result.Removed)!=0 || calls!=0 {t.Fatal(result,err,calls)}
  assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
  assertPublicationBytes(t,filepath.Join(dir,"concat.txt"),"concat evidence")
  assertPublicationBytes(t,outside,"outside evidence")
  if _,err:=os.Lstat(foreign);err!=nil {t.Fatal("foreign entry changed",err)}
  assertStagingReleaseCheckpoint(t,inventory,dir)
 })}
}
func TestStagingInputReleasePartialUnlinkFailure(t *testing.T) {
 inventory,lease,dir:=stagingInputReleaseFixture(t)
 injected:=errors.New("unlink failure");ops:=realOwnedStagingInputReleaseOps();syncs:=0
 ops.unlink=func(fd int,name string,flags int)error{if name=="event.mp4" {return injected};return unix.Unlinkat(fd,name,flags)}
 ops.syncDirectory=func(f *os.File)error{syncs++;return f.Sync()}
 result,err:=releaseOwnedStagingInputsWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,ops)
 if !errors.Is(err,injected) || result.InputsReleased || !reflect.DeepEqual(result.Removed,[]string{"concat.txt"}) || syncs!=1 {t.Fatal(result,err,syncs)}
 assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
 assertStagingReleaseCheckpoint(t,inventory,dir)
 if _,err:=os.Stat(filepath.Join(dir,"concat.txt"));!os.IsNotExist(err) {t.Fatal("successful unlink not reported accurately",err)}
}
func TestStagingInputReleaseRechecksNextInputAndRoot(t *testing.T) {
 for _,change:=range []string{"replacement","modified","missing","root","parent"} {t.Run(change,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  ops:=realOwnedStagingInputReleaseOps()
  saved:=filepath.Join(t.TempDir(),"saved")
  ops.unlink=func(fd int,name string,flags int)error{
   if name!="concat.txt" {return errors.New("changed entry reached unlink")}
   if err:=unix.Unlinkat(fd,name,flags);err!=nil {return err}
   path:=filepath.Join(dir,"event.mp4")
   switch change {
   case "replacement","missing":
    if err:=os.Rename(path,saved);err!=nil {t.Fatal(err)}
    if change=="replacement" {if err:=os.WriteFile(path,[]byte("replacement evidence"),0600);err!=nil {t.Fatal(err)}}
   case "modified":if err:=os.WriteFile(path,[]byte("modified evidence"),0600);err!=nil {t.Fatal(err)}
   case "root":if err:=os.Rename(dir,dir+"-saved");err!=nil {t.Fatal(err)};if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
   case "parent":parent:=filepath.Dir(dir);if err:=os.Rename(parent,parent+"-saved");err!=nil {t.Fatal(err)};if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
   }
   return nil
  }
  result,err:=releaseOwnedStagingInputsWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,ops)
  if err==nil || result.InputsReleased || !reflect.DeepEqual(result.Removed,[]string{"concat.txt"}) {t.Fatal(result,err)}
  switch change {
  case "replacement":assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"replacement evidence");assertPublicationBytes(t,saved,"capture evidence")
  case "missing":assertPublicationBytes(t,saved,"capture evidence")
  case "modified":assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"modified evidence")
  case "root":assertPublicationBytes(t,filepath.Join(dir+"-saved","event.mp4"),"capture evidence")
  case "parent":assertPublicationBytes(t,filepath.Join(filepath.Dir(dir)+"-saved",filepath.Base(dir),"event.mp4"),"capture evidence")
  }
  checkpointDir:=dir
  if change=="root" {checkpointDir=dir+"-saved"};if change=="parent" {checkpointDir=filepath.Join(filepath.Dir(dir)+"-saved",filepath.Base(dir))}
  assertStagingReleaseCheckpoint(t,inventory,checkpointDir)
 })}
}
func TestStagingInputReleaseLateEntryNeverSwept(t *testing.T) {
 for _,duringSync:=range []bool{false,true} {t.Run(map[bool]string{false:"during unlink",true:"during sync"}[duringSync],func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  ops:=realOwnedStagingInputReleaseOps()
  late:=filepath.Join(dir,"late")
  add:=func(){if err:=os.WriteFile(late,[]byte("late evidence"),0600);err!=nil {t.Fatal(err)}}
  if duringSync {ops.syncDirectory=func(f *os.File)error{add();return f.Sync()}} else {
   ops.unlink=func(fd int,name string,flags int)error{if name=="event.mp4" {add()};return unix.Unlinkat(fd,name,flags)}
  }
  result,err:=releaseOwnedStagingInputsWithOps(context.Background(),lease,inventory,stagingReleaseAllowed,ops)
  if err==nil || result.InputsReleased || len(result.Removed)!=2 {t.Fatal(result,err)}
  assertPublicationBytes(t,late,"late evidence");assertStagingReleaseCheckpoint(t,inventory,dir)
 })}
}
func TestStagingInputReleaseCancellationAndSyncFailure(t *testing.T) {
 for _,stage:=range []string{"before","after unlink","during sync","sync error","close error"} {t.Run(stage,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  injected:=errors.New("injected I/O failure");ops:=realOwnedStagingInputReleaseOps();syncs:=0
  pin:=inventory.entries["concat.txt"].file
  ops.unlink=func(fd int,name string,flags int)error{err:=unix.Unlinkat(fd,name,flags);if err==nil && stage=="after unlink" {cancel()};return err}
  ops.syncDirectory=func(f *os.File)error{syncs++;if stage=="during sync" {cancel()};if stage=="sync error" {return injected};return f.Sync()}
  if stage=="close error" {ops.closeFile=func(*os.File)error{return injected}}
  if stage=="before" {cancel()}
  result,err:=releaseOwnedStagingInputsWithOps(ctx,lease,inventory,stagingReleaseAllowed,ops)
  if err==nil || result.InputsReleased || !lease.held() {t.Fatal(result,err)}
  if stage=="sync error" || stage=="close error" {if !errors.Is(err,injected) {t.Fatal(err)}} else if !errors.Is(err,context.Canceled) {t.Fatal(err)}
  if stage=="before" {if len(result.Removed)!=0 || syncs!=0 {t.Fatal(result,syncs)}} else {
   if syncs!=1 || len(result.Removed)==0 {t.Fatal(result,syncs)}
   if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("removed input pin leaked",err)}
  }
  assertStagingReleaseCheckpoint(t,inventory,dir)
 })}
}
func TestStagingInputReleaseRequiresGuardCheckpointAndRecognizedNames(t *testing.T) {
 for _,refusal:=range []string{"nil guard","guard refusal","guard revoked","missing checkpoint","recorded foreign","nil lease"} {t.Run(refusal,func(t *testing.T){
  inventory,lease,dir:=stagingInputReleaseFixture(t)
  ops:=realOwnedStagingInputReleaseOps();calls:=0
  ops.unlink=func(int,string,int)error{calls++;return errors.New("unexpected unlink")}
  guard:=stagingReleaseAllowed
  useLease:=lease
  switch refusal {
  case "nil guard":guard=nil
  case "guard refusal":guard=func()error{return os.ErrPermission}
  case "guard revoked":checks:=0;guard=func()error{checks++;if checks>1 {return os.ErrPermission};return nil}
  case "missing checkpoint":entry:=inventory.entries[ownedCheckpointName];delete(inventory.entries,ownedCheckpointName);if err:=entry.file.Close();err!=nil {t.Fatal(err)}
  case "recorded foreign":file:=stagingInventoryCreated(t,dir,"owned-but-not-input","retain");if err:=inventory.Remember(context.Background(),lease,"owned-but-not-input",file);err!=nil {t.Fatal(err)};file.Close()
  case "nil lease":useLease=nil
  }
  result,err:=releaseOwnedStagingInputsWithOps(context.Background(),useLease,inventory,guard,ops)
  if err==nil || result.InputsReleased || len(result.Removed)!=0 || calls!=0 {t.Fatal(result,err,calls)}
  assertPublicationBytes(t,filepath.Join(dir,"event.mp4"),"capture evidence")
  assertPublicationBytes(t,filepath.Join(dir,ownedCheckpointName),"checkpoint evidence")
 })}
}
