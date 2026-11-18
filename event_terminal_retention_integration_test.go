package main

import (
 "context"
 "os"
 "path/filepath"
 "sync/atomic"
 "testing"
 "time"
)

func TestEventTerminalOwnershipSerializesRealRetentionRevalidation(t *testing.T) {
 for _,change:=range []string{"unchanged","policy","replacement","directory","shutdown","cancel"} {t.Run(change,func(t *testing.T){
  n,bm,job,releaseJob:=terminalInteractionJob(t)
  cam:=bm.camera
  root:=cam.RecordDir
  oldName:="20000101_000000_expired.mp4"
  oldPath:=filepath.Join(root,oldName)
  if err:=os.WriteFile(oldPath,[]byte("disposable published retention fixture"),0600);err!=nil {t.Fatal(err)}
  before:=recoveryContractTree(t,root)
  checkpoint:=filepath.Join("temp",filepath.Base(job.dir),ownedCheckpointName)
  inventory:=job.inventory
  owner:=job.recoveryBorrow.lease
  targets:=snapshotRetentionCleanup(n)
  if len(targets)!=1 {t.Fatal("retention fixture not selected")}
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  ctx:=&cameraOwnershipWaitContext{Context:parent,waiting:make(chan struct{})}
  var calls atomic.Int32
  var crossed atomic.Bool
  done:=make(chan bool,1)
  finished:=make(chan struct{})
  operation:=func(ctx context.Context,lease *mediaMutationLease,dir string,duration time.Duration,guard func()error)error{
   calls.Add(1)
   if owner.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 || !lease.held() {crossed.Store(true)}
   return cleanupExpiredPublishedLeased(ctx,lease,dir,duration,guard)
  }
  go func(){defer close(finished);done<-admitRetentionDeletion(ctx,n,targets[0],recordingMediaOwnership,operation)}()
  t.Cleanup(func(){cancel();releaseJob();awaitShutdownStep(t,finished,"retention cleanup did not join")})
  awaitShutdownStep(t,ctx.waiting,"retention did not wait on real published-job owner")
  requireRegistryAvailable(t,n)
  if calls.Load()!=0 || !owner.held() {t.Fatal("retention crossed terminal ownership")}
  select {case <-done:t.Fatal("retention returned before terminal ownership ended");default:}
  switch change {
  case "policy":if err:=setCameraRetention(n,cam.ID,2);err!=nil {t.Fatal(err)}
  case "replacement":n.mu.Lock();n.cameras[cam.ID]=&Camera{ID:cam.ID,RecordDir:root,RetentionDays:1};n.mu.Unlock()
  case "directory":n.mu.Lock();cam.RecordDir=root+"-changed";n.mu.Unlock()
  case "shutdown":n.shuttingDown.Store(true)
  case "cancel":cancel()
  }
  releaseJob()
  var invoked bool
  select {case invoked=<-done:case <-time.After(3*time.Second):t.Fatal("retention contender did not join")}
  want:=change=="unchanged"
  if invoked!=want || (calls.Load()==1)!=want || crossed.Load() {t.Fatal("stale retention admission or early destructive entry",invoked,calls.Load())}
  if want {
   if _,err:=os.Lstat(oldPath);!os.IsNotExist(err) {t.Fatal("positive control did not use real published deletion engine",err)}
   delete(before,oldName)
  } else {assertPublicationBytes(t,oldPath,"disposable published retention fixture")}
  assertJobInventoryClosed(t,inventory)
  if job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 || len(recordingMediaOwnership.token)!=0 {t.Fatal("retention/control handoff leaked resources")}
  terminalInteractionTreeEqual(t,before,recoveryContractTree(t,root),checkpoint)
  if m:=readRecoveryTest(t,job.dir);m.State!="published" || m.Coverage==nil || m.Coverage.Complete || m.Coverage.Status!="unknown" {t.Fatal("retention gained complete-capture authority")}
 })}
}
