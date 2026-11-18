package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
 "testing"

 "golang.org/x/sys/unix"
)

// Test-only scheduling at the existing post-O_PATH/pre-reopen context check.
// Real fixture open/stat/read/rename operations are not replaced.
type recoveryContractBoundaryContext struct {context.Context;calls int;action func()}
func (ctx *recoveryContractBoundaryContext) Err()error {
 ctx.calls++;if ctx.calls==2 {ctx.action()};return ctx.Context.Err()
}
func recoveryContractFDCount(t *testing.T,identity os.FileInfo)int {
 t.Helper();entries,err:=os.ReadDir("/proc/self/fd");if err!=nil {t.Fatal(err)};count:=0
 for _,entry:=range entries {
  info,err:=os.Stat(filepath.Join("/proc/self/fd",entry.Name()));if os.IsNotExist(err) {continue};if err!=nil {t.Fatal(err)}
  if os.SameFile(info,identity) {count++}
 }
 return count
}
func TestRecoveryContractSnapshotMutationAtVerifiedReopenBoundary(t *testing.T) {
 for _,kind:=range []string{"stable","same_bytes_new_inode","inplace_growth","fifo_replacement","symlink_replacement","unlink","cancel"} {t.Run(kind,func(t *testing.T){
  root,id,data:=recoveryInspectionFixture(t);dir:=filepath.Join(root,"temp",id);path:=filepath.Join(dir,"recovery.json")
  parent,err:=os.Open(dir);if err!=nil {t.Fatal(err)};defer parent.Close()
  identity,err:=os.Stat(path);if err!=nil {t.Fatal(err)};baseline:=recoveryContractFDCount(t,identity)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();reached:=false;var mutationErr error
  mutate:=func()error{
   reached=true
   if recoveryContractFDCount(t,identity)<=baseline {return errors.New("identity descriptor not retained before mutation")}
   switch kind {
   case "stable":return nil
   case "same_bytes_new_inode","fifo_replacement","symlink_replacement":
    if err:=os.Rename(path,path+".saved");err!=nil {return err}
    switch kind {
    case "same_bytes_new_inode":return os.WriteFile(path,data,0600)
    case "fifo_replacement":return unix.Mkfifo(path,0600)
    case "symlink_replacement":return os.Symlink(path+".saved",path)
    }
   case "inplace_growth":return os.WriteFile(path,append(append([]byte(nil),data...),byte(' ')),0600)
   case "unlink":return os.Rename(path,path+".saved")
   case "cancel":cancel();return nil
   }
   return errors.New("unknown fixture mutation")
  }
  boundary:=&recoveryContractBoundaryContext{Context:ctx,action:func(){mutationErr=mutate()}}
  got,missing,readErr:=readRecoverySnapshot(boundary,parent)
  if mutationErr!=nil {t.Fatal(mutationErr)};if !reached {t.Fatal("verified reopen boundary not reached")}
  if kind=="stable" {
   if readErr!=nil || missing || !bytes.Equal(got,data) {t.Fatal("stable control failed",readErr)}
  } else if !errors.Is(readErr,errRecoveryInspection) || got!=nil || missing {t.Fatal("changed/canceled snapshot accepted",kind,readErr)}
  if recoveryContractFDCount(t,identity)!=baseline {t.Fatal("manifest identity/read descriptor leaked")}
  if kind!="inplace_growth" && kind!="stable" && kind!="cancel" {assertPublicationBytes(t,path+".saved",string(data))}
 })}
}
func TestRecoveryContractAtomicReplacementHasNoTornReaderSuccess(t *testing.T) {
 root,id,data:=recoveryInspectionFixture(t);dir:=filepath.Join(root,"temp",id)
 parent,err:=os.Open(dir);if err!=nil {t.Fatal(err)};defer parent.Close()
 var writerErr error;start:=make(chan struct{});var joined sync.WaitGroup;joined.Add(1)
 go func(){
  defer joined.Done();<-start
  for i:=0;i<12;i++ {
   payload:=bytes.Replace(data,[]byte(`"revision":2`),[]byte(fmt.Sprintf(`"revision":%d`,i+3)),1)
   if err:=writeEventRecovery(dir,payload);err!=nil {writerErr=err;return}
  }
 }()
 defer joined.Wait();close(start)
 for i:=0;i<24;i++ {
  payload,missing,err:=readRecoverySnapshot(context.Background(),parent)
  if err!=nil {if !errors.Is(err,errRecoveryInspection) {t.Fatal(err)};continue}
  if missing {t.Fatal("atomic replacement exposed missing checkpoint")}
  m,err:=parseRecoveryManifest(payload,"a");if err!=nil || m.Revision<2 || m.Revision>14 {t.Fatal("successful read torn or outside committed revisions",err)}
  expected:=bytes.Replace(data,[]byte(`"revision":2`),[]byte(fmt.Sprintf(`"revision":%d`,m.Revision)),1)
  if !bytes.Equal(payload,expected) {t.Fatal("accepted snapshot differs from producer payload")}
 }
 joined.Wait();if writerErr!=nil {t.Fatal(writerErr)}
 payload,missing,err:=readRecoverySnapshot(context.Background(),parent);if err!=nil || missing {t.Fatal("stable final snapshot failed",err)}
 m,err:=parseRecoveryManifest(payload,"a");if err!=nil || m.Revision!=14 {t.Fatal("final producer revision lost",err)}
}
