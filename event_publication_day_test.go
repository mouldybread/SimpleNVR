package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestEventPublisherCreatesStampedDayAndRetainsObservedOutput(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 released:=false
 defer func(){if !released {bm.releaseJob(job)}}()
 name:="20261110_235500_E20261111_000500_event_id_x.mp4"
 temporary:=filepath.Join(bm.camera.RecordDir,name+".tmp")
 final:=filepath.Join(bm.camera.RecordDir,"20261110",name)
 if err:=os.WriteFile(temporary,[]byte("published bytes"),0600);err!=nil {t.Fatal(err)}
 job.recovery.OutputTemporary=filepath.Base(temporary);job.recovery.OutputPublished=name
 if err:=bm.publishEventRecording(context.Background(),bm.ctx,job,temporary,final,false);err!=nil {t.Fatal(err)}
 borrowed:=job.recoveryBorrow
 if borrowed==nil || borrowed.output==nil || !borrowed.lease.held() {t.Fatal("publication handoff missing")}
 output:=borrowed.output
 if output.path!=final {t.Fatal("wrong published path",output.path)}
 if err:=output.check(context.Background(),borrowed.lease);err!=nil {t.Fatal(err)}
 held,err:=output.directory.Stat();if err!=nil {t.Fatal(err)}
 named,err:=os.Stat(filepath.Dir(final));if err!=nil || !os.SameFile(held,named) {t.Fatal("observer did not retain day directory",err)}
 assertPublicationBytes(t,final,"published bytes")
 if _,err:=os.Lstat(temporary);!os.IsNotExist(err) {t.Fatal("temporary not renamed",err)}
 file,directory:=output.file,output.directory
 bm.releaseJob(job);released=true
 if borrowed.lease.held() || output.file!=nil || output.directory!=nil || job.recoveryBorrow!=nil {t.Fatal("terminal publication ownership leaked")}
 for _,pin:=range []*os.File{file,directory} {if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("publication pin leaked",err)}}
}

func TestEventPublisherRefusesUnsafeDayDestinationsAndPreservesCandidate(t *testing.T) {
 for _,kind:=range []string{"wrong day","other camera","symlink","collision"} {t.Run(kind,func(t *testing.T){
  installEventFailureTools(t,0,false)
  bm,job:=publicationCheckpointJob(t)
  defer bm.releaseJob(job)
  name:="20261110_235500_E20261111_000500_event_id_x.mp4"
  temporary:=filepath.Join(bm.camera.RecordDir,name+".tmp")
  final:=filepath.Join(bm.camera.RecordDir,"20261110",name)
  if err:=os.WriteFile(temporary,[]byte("candidate"),0600);err!=nil {t.Fatal(err)}
  outside:=t.TempDir()
  switch kind {
  case "wrong day":final=filepath.Join(bm.camera.RecordDir,"20261111",name)
  case "other camera":final=filepath.Join(outside,"20261110",name)
  case "symlink":if err:=os.Symlink(outside,filepath.Dir(final));err!=nil {t.Fatal(err)}
  case "collision":
   if err:=os.Mkdir(filepath.Dir(final),0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(final,[]byte("existing"),0600);err!=nil {t.Fatal(err)}
  }
  err:=bm.publishEventRecording(context.Background(),bm.ctx,job,temporary,final,false)
  if err==nil || job.recoveryBorrow!=nil {t.Fatal("unsafe publication accepted or borrow leaked",err)}
  if kind=="collision" && !os.IsExist(err) {t.Fatal("collision error lost",err)}
  assertPublicationBytes(t,temporary,"candidate")
  if kind=="collision" {assertPublicationBytes(t,final,"existing")}
  entries,err:=os.ReadDir(outside);if err!=nil || len(entries)!=0 {t.Fatal("outside directory changed",err)}
 })}
}
