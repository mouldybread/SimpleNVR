package main

import (
 "context"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

func retentionSnapshotFixture(t *testing.T) (*NVR,*Camera,string,retentionCleanupSnapshot) {
 t.Helper()
 dir:=t.TempDir()
 name:=time.Now().UTC().Add(-10*24*time.Hour).Format("20060102_150405")+"_saved.mp4"
 file:=filepath.Join(dir,name);putCleanupFixture(t,file,"published")
 cam:=&Camera{ID:"a",Name:"A",RecordDir:dir,RetentionDays:1}
 n:=&NVR{cameras:map[string]*Camera{"a":cam}}
 targets:=snapshotRetentionCleanup(n);if len(targets)!=1 {t.Fatal("snapshot missing")}
 return n,cam,file,targets[0]
}

func TestRetentionIncreaseInvalidatesSingleAndBulkCleanupSnapshots(t *testing.T) {
 for _,bulk:=range []bool{false,true} {
  n,_,file,target:=retentionSnapshotFixture(t)
  var err error;if bulk {err=setAllCameraRetention(n,30)} else {err=setCameraRetention(n,"a",30)}
  if err!=nil {t.Fatal(err)}
  if cleanupRetentionSnapshot(context.Background(),n,target) {t.Fatal("stale shorter retention invoked deletion")}
  if _,err:=os.Stat(file);err!=nil {t.Fatal("retention increase lost protected recording")}
  fresh:=snapshotRetentionCleanup(n)
  if !cleanupRetentionSnapshot(context.Background(),n,fresh[0]) {t.Fatal("fresh valid snapshot rejected")}
  if _,err:=os.Stat(file);err!=nil {t.Fatal("fresh longer retention removed ten-day-old recording")}
 }
}

func TestChangedCameraIdentityPathOrPolicyRejectsSnapshot(t *testing.T) {
 for _,change:=range []string{"remove","replace","path","invalid"} {t.Run(change,func(t *testing.T){
  n,cam,file,target:=retentionSnapshotFixture(t)
  n.mu.Lock()
  switch change {case "remove":delete(n.cameras,"a");case "replace":copy:=*cam;n.cameras["a"]=&copy;case "path":cam.RecordDir=filepath.Join(cam.RecordDir,"other");case "invalid":cam.RetentionDays=-1}
  n.mu.Unlock()
  if cleanupRetentionSnapshot(context.Background(),n,target) {t.Fatal("changed snapshot was used")}
  if _,err:=os.Stat(file);err!=nil {t.Fatal("changed snapshot deleted published media")}
 })}
}

func TestRetentionDeletionPhaseExcludesPolicyWritersWithoutRegistryLock(t *testing.T) {
 n,_,_,target:=retentionSnapshotFixture(t)
 entered,release,finished:=make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 go func(){defer close(finished);cleanupRetentionSnapshotWith(context.Background(),n,target,func(context.Context,string,time.Duration){close(entered);<-release})}()
 awaitShutdownStep(t,entered,"deletion phase not entered")
 if retentionPolicyMu.TryLock() {retentionPolicyMu.Unlock();t.Fatal("retention writer could enter during deletion phase")}
 if !n.mu.TryLock() {t.Fatal("filesystem phase held registry lock")}
 n.mu.Unlock()
 updated:=make(chan error,1)
 go func(){updated<-setCameraRetention(n,"a",30)}()
 once.Do(func(){close(release)})
 awaitShutdownStep(t,finished,"deletion phase did not finish")
 select {case err:=<-updated:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("retention update did not finish after deletion joined")}
 n.mu.RLock();days:=n.cameras["a"].RetentionDays;n.mu.RUnlock()
 if days!=30 {t.Fatal("retention update lost")}
}

func TestCanceledRetentionSnapshotDoesNotInvokeDeletion(t *testing.T) {
 n,_,file,target:=retentionSnapshotFixture(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 invoked:=false
 if cleanupRetentionSnapshotWith(ctx,n,target,func(context.Context,string,time.Duration){invoked=true}) || invoked {t.Fatal("canceled snapshot invoked deletion")}
 if _,err:=os.Stat(file);err!=nil {t.Fatal(err)}
}

func TestFreshSnapshotStillDeletesExpiredPublishedMedia(t *testing.T) {
 n,_,file,target:=retentionSnapshotFixture(t)
 if !cleanupRetentionSnapshot(context.Background(),n,target) {t.Fatal("fresh cleanup rejected")}
 if _,err:=os.Stat(file);!os.IsNotExist(err) {t.Fatalf("expired media retained: %v",err)}
}
