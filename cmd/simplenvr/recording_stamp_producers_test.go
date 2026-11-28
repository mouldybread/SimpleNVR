package main

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestContinuousProducerPublishesStampedDayFile(t *testing.T) {
 installRecoveryContractTools(t,0,0,false)
 camera:=&Camera{ID:"a",Name:"Front",RecordDir:t.TempDir(),RTSPUrl:"rtsp://fixture"}
 before:=time.Now().UTC().Truncate(time.Second)
 if err:=recordSegment(context.Background(),camera,600);err!=nil {t.Fatal(err)}
 files,err:=filepath.Glob(filepath.Join(camera.RecordDir,"*","*.mp4"));if err!=nil || len(files)!=1 {t.Fatal(files,err)}
 stamp,err:=parseStampedName(filepath.Base(files[0]));if err!=nil {t.Fatal(err)}
 if stamp.Start.Before(before) || stamp.End.After(stampedCeilSecond(time.Now())) || filepath.Base(filepath.Dir(files[0]))!=stampedDayName(stamp.Start) {t.Fatal("wrong producer timestamps or day",stamp,files[0])}
 assertPublicationBytes(t,files[0],"synthetic assembly evidence\n")
 loose,err:=filepath.Glob(filepath.Join(camera.RecordDir,"*.mp4*"));if err!=nil || len(loose)!=0 {t.Fatal("loose published or temporary output",loose,err)}
}

func TestEventProducerPublishesStampedDayAndRootTemporaryIntent(t *testing.T) {
 for _,partial:=range []bool{false,true} {t.Run(map[bool]string{false:"complete",true:"partial"}[partial],func(t *testing.T){
  exit:=0;if partial {exit=1}
  installRecoveryContractTools(t,0,exit,false)
  bm:=bufferFixture(t)
  event:=bufferTestEvent("stamped-producer")
  if err:=bm.TriggerEvent(event);err!=nil {t.Fatal(err)}
  job:=<-bm.eventChan
  defer bm.releaseJob(job)
  input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
  final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("assembly failed")}
  stamp,err:=parseStampedName(filepath.Base(final));if err!=nil {t.Fatal(err)}
  if !stamp.Start.Equal(job.preStart.UTC().Truncate(time.Second)) || !stamp.End.Equal(stampedCeilSecond(event.StartTime.Add(time.Duration(event.Duration)*time.Second))) {t.Fatal("wrong event stamp",stamp)}
  if filepath.Dir(final)!=filepath.Join(bm.camera.RecordDir,stampedDayName(stamp.Start)) || strings.HasSuffix(final,"_partial.mp4")!=partial {t.Fatal("wrong publication path",final)}
  manifest:=readRecoveryTest(t,job.dir)
  if manifest.OutputPublished!=filepath.Base(final) || manifest.Partial!=partial {t.Fatal("manifest disagrees",manifest)}
  expected:=strings.TrimSuffix(manifest.OutputTemporary,".tmp");if partial {expected=partialRecordingName(expected)}
  if expected!=manifest.OutputPublished {t.Fatal("temporary/publication name contract changed")}
  if _,err:=os.Lstat(filepath.Join(bm.camera.RecordDir,manifest.OutputTemporary));!os.IsNotExist(err) {t.Fatal("temporary not published",err)}
  assertPublicationBytes(t,final,"synthetic assembly evidence\n")
 })}
}
