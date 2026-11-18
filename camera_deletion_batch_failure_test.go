package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestCameraDeletionBatchStopsAfterRemovalError(t *testing.T) {
 for _,partial:=range []bool{false,true} {
  name:="before-unlink";if partial {name="after-unlink"}
  t.Run(name,func(t *testing.T){
   n,first:=lifecycleFixture(ModeContinuous)
   second:=&Camera{ID:first.ID+"-later",RecordDir:filepath.Join(t.TempDir(),"later")}
   n.cameras[second.ID]=second
   n.cameraOrder=append(n.cameraOrder,second.ID)
   evidence:=filepath.Join(t.TempDir(),"recording.mp4")
   if err:=os.WriteFile(evidence,[]byte("fixture"),0600);err!=nil {t.Fatal(err)}
   injected:=errors.New("removal failed")
   calls,saves:=0,0
   err:=deleteManagedCameras(context.Background(),n,[]string{first.ID,second.ID},cameraOwnershipDeps(
    func(_ context.Context,_ string,guard func()error)error {
     calls++
     if calls!=1 {t.Error("later camera removal entered");return injected}
     if err:=guard();err!=nil {return err}
     if partial {if err:=os.Remove(evidence);err!=nil {return err}}
     return injected
    },func()error{saves++;return nil}))
   if !errors.Is(err,injected) || calls!=1 || saves!=1 {t.Fatalf("err=%v calls=%d saves=%d",err,calls,saves)}
   n.mu.RLock()
   registered:=n.cameras[first.ID]==first && n.cameras[second.ID]==second
   n.mu.RUnlock()
   if !registered {t.Fatal("failed or unvisited camera unregistered")}
   _,statErr:=os.Stat(evidence)
   if partial && !os.IsNotExist(statErr) {t.Fatalf("expected partial unlink, got %v",statErr)}
   if !partial && statErr!=nil {t.Fatal(statErr)}
   if len(recordingMediaOwnership.token)!=0 {t.Fatal("media lease leaked")}
  })
 }
}

func TestCameraDeletionBatchContinuesAfterValidationRefusal(t *testing.T) {
 n,first:=lifecycleFixture(ModeContinuous)
 second:=&Camera{ID:first.ID+"-later",RecordDir:filepath.Join(t.TempDir(),"later")}
 n.cameras[second.ID]=second
 n.cameraOrder=append(n.cameraOrder,second.ID)
 refused:=errors.New("validation refused")
 calls,saves:=0,0
 deps:=cameraOwnershipDeps(func(_ context.Context,dir string,guard func()error)error{
  calls++;if dir!=second.RecordDir {t.Error("validation-refused camera removed")};return guard()
 },func()error{saves++;return nil})
 deps.validate=func(id,dir string)error{if id==first.ID{return refused};return nil}
 err:=deleteManagedCameras(context.Background(),n,[]string{first.ID,second.ID},deps)
 if !errors.Is(err,refused) || calls!=1 || saves!=2 {t.Fatalf("err=%v calls=%d saves=%d",err,calls,saves)}
 n.mu.RLock()
 correct:=n.cameras[first.ID]==first && n.cameras[second.ID]==nil
 n.mu.RUnlock()
 if !correct {t.Fatal("preflight refusal changed batch semantics")}
 if len(recordingMediaOwnership.token)!=0 {t.Fatal("media lease leaked")}
}
