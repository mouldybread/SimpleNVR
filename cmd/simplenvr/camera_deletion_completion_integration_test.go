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

func TestManagedCameraDeletionStopsBatchAfterCompletionFailure(t *testing.T) {
 for _,kind:=range []string{"sync-retry-succeeds","sync-retry-fails","late-entry"} {
  t.Run(kind,func(t *testing.T){
   n,first:=lifecycleFixture(ModeContinuous)
   parent:=t.TempDir();first.RecordDir=filepath.Join(parent,"first");first.IsRecording=true
   second:=&Camera{ID:first.ID+"-later",RecordDir:filepath.Join(parent,"second"),RecordingMode:ModeContinuous,IsRecording:true}
   n.cameras[second.ID]=second;n.cameraOrder=append(n.cameraOrder,second.ID)
   initialOrder:=append([]string(nil),n.cameraOrder...)
   for _,dir:=range []string{first.RecordDir,second.RecordDir} {
    if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(filepath.Join(dir,"clip.mp4"),[]byte("fixture evidence"),0600);err!=nil {t.Fatal(err)}
   }
   initialSyncErr:=errors.New("injected completion sync failure")
   retrySyncErr:=errors.New("injected deferred sync failure")
   saves,removes,syncs:=0,0,0
   var durable persistedState
   deps:=deletionDeps(n,func()error{saves++;durable=deletionSnapshot(n);return nil},func(ctx context.Context,dir string,guard func()error)error{
    removes++
    if dir!=first.RecordDir {t.Error("later camera removal reached");return errors.New("unexpected later camera removal")}
    if !recordingMediaOwnershipIsHeldForCompletionTest() {t.Error("filesystem removal entered without media ownership")}
    return removeCameraDirectoryWithIO(ctx,dir,guard,unix.Unlinkat,func(root *os.File)error{
     syncs++
     if kind!="late-entry" {
      if syncs==1 {return initialSyncErr}
      if kind=="sync-retry-fails" {return retrySyncErr}
     } else if syncs==1 {
      if err:=os.WriteFile(filepath.Join(dir,"late.mp4"),[]byte("late evidence"),0600);err!=nil {return err}
     }
     return root.Sync()
    })
   })
   err:=deleteManagedCameras(context.Background(),n,[]string{first.ID,second.ID},deps)
   if err==nil {t.Fatal("completion failure reported success")}
   wantSyncs:=2
   if kind=="late-entry" {
    wantSyncs=1
    if !errors.Is(err,unix.ENOTEMPTY) && !errors.Is(err,unix.EEXIST) {t.Fatal("unexpected late-entry error",err)}
   } else {
    if !errors.Is(err,initialSyncErr) {t.Fatal("initial sync error lost",err)}
    if kind=="sync-retry-fails" && !errors.Is(err,retrySyncErr) {t.Fatal("deferred sync error lost",err)}
   }
   if saves!=1 || removes!=1 || syncs!=wantSyncs {t.Fatalf("saves=%d removes=%d syncs=%d want=%d",saves,removes,syncs,wantSyncs)}
   n.mu.RLock()
   stateOK:=n.cameras[first.ID]==first && !first.IsRecording && first.recordCancel==nil && n.cameras[second.ID]==second && second.IsRecording && reflect.DeepEqual(n.cameraOrder,initialOrder)
   n.mu.RUnlock()
   if !stateOK {t.Fatal("failed or unvisited camera state changed incorrectly")}
   if durable.Cameras[first.ID]==nil || durable.Cameras[first.ID].IsRecording || durable.Cameras[second.ID]==nil || !durable.Cameras[second.ID].IsRecording || !reflect.DeepEqual(durable.CameraOrder,initialOrder) {t.Fatal("stopped-intent persistence snapshot incorrect")}
   if len(recordingMediaOwnership.token)!=0 {t.Fatal("completion failure leaked media ownership")}
   raw,err:=os.ReadFile(filepath.Join(second.RecordDir,"clip.mp4"));if err!=nil || string(raw)!="fixture evidence" {t.Fatal("later camera evidence changed",err)}
   entries,err:=os.ReadDir(first.RecordDir);if err!=nil {t.Fatal("failed camera directory not retained",err)}
   wantEntries:=0;if kind=="late-entry" {wantEntries=1;raw,err:=os.ReadFile(filepath.Join(first.RecordDir,"late.mp4"));if err!=nil || string(raw)!="late evidence" {t.Fatal("late evidence changed",err)}}
   if len(entries)!=wantEntries {t.Fatalf("unexpected remaining entries: %v",entries)}
  })
 }
}

func recordingMediaOwnershipIsHeldForCompletionTest() bool {
 return len(recordingMediaOwnership.token)==1
}
