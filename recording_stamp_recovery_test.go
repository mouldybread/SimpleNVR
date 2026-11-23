package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestStampedRecoveryPublishesMtimeEndAndPreservesFailures(t *testing.T) {
 start:=time.Date(2026,11,10,23,55,0,0,time.UTC)
 for _,kind:=range []string{"normal","stamped","old mtime","invalid name","overspan","validation","symlink","collision","canceled"} {t.Run(kind,func(t *testing.T){
  camera:=t.TempDir();name:="20261110_235500_cam_id_x.mp4.tmp"
  if kind=="stamped" {name="20261110_235500_E20261111_001000_cam_id_x.mp4.tmp"}
  if kind=="invalid name" {name="unknown.mp4.tmp"}
  source:=filepath.Join(camera,name);end:=start.Add(10*time.Minute)
  if kind=="old mtime" {end=start.Add(-time.Hour)}
  if kind=="overspan" {end=start.Add(25*time.Hour)}
  if err:=os.WriteFile(source,[]byte("candidate"),0600);err!=nil {t.Fatal(err)}
  if err:=os.Chtimes(source,end,end);err!=nil {t.Fatal(err)}
  outside:=filepath.Join(t.TempDir(),"outside.mp4.tmp")
  if kind=="symlink" {
   if err:=os.Rename(source,outside);err!=nil {t.Fatal(err)}
   if err:=os.Symlink(outside,source);err!=nil {t.Fatal(err)}
  }
  effective:=end;if !effective.After(start) {effective=start.Add(time.Second)}
  expected,buildErr:=stampedFinalPath(camera,start,effective,"cam_id_x",true)
  if kind=="collision" {
   if buildErr!=nil {t.Fatal(buildErr)}
   if err:=os.Mkdir(filepath.Dir(expected),0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(expected,[]byte("existing"),0600);err!=nil {t.Fatal(err)}
  }
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  if kind=="canceled" {cancel()}
  validate:=acceptPublicationFixture
  if kind=="validation" {validate=func(context.Context,string) error{return os.ErrInvalid}}
  final,err:=recoverStampedTemporary(ctx,camera,source,validate)
  success:=kind=="normal" || kind=="stamped" || kind=="old mtime"
  if success {
   if err!=nil || final!=expected {t.Fatal(final,expected,err)}
   assertPublicationBytes(t,final,"candidate")
   if _,err:=os.Lstat(source);!os.IsNotExist(err) {t.Fatal("source not renamed",err)}
  } else {
   if err==nil {t.Fatal("invalid recovery succeeded")}
   assertPublicationBytes(t,source,"candidate")
   if kind=="collision" {assertPublicationBytes(t,expected,"existing")}
   if kind=="symlink" {assertPublicationBytes(t,outside,"candidate")}
   if kind=="canceled" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
  }
 })}
}

func TestStartupUsesStampedRecoveryAndDoesNotTraverseStaging(t *testing.T) {
 installEventFailureTools(t,0,false)
 camera:=t.TempDir();source:=filepath.Join(camera,"20261110_235500_cam_id_x.mp4.tmp")
 if err:=os.WriteFile(source,[]byte("candidate"),0600);err!=nil {t.Fatal(err)}
 end:=time.Date(2026,11,11,0,5,0,0,time.UTC)
 if err:=os.Chtimes(source,end,end);err!=nil {t.Fatal(err)}
 for _,relative:=range []string{"unknown.mp4.tmp","temp/event-evidence/20261110_235500_cam_id_x.mp4.tmp","buffer/20261110_235500_cam_id_x.mp4.tmp"} {
  path:=filepath.Join(camera,relative)
  if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil {t.Fatal(err)}
  if err:=os.WriteFile(path,[]byte("retained"),0600);err!=nil {t.Fatal(err)}
 }
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:camera}}}
 recoverBeforeStartup(context.Background(),n)
 assertPublicationBytes(t,filepath.Join(camera,"20261110","20261110_235500_E20261111_000500_cam_id_x_partial.mp4"),"candidate")
 for _,relative:=range []string{"unknown.mp4.tmp","temp/event-evidence/20261110_235500_cam_id_x.mp4.tmp","buffer/20261110_235500_cam_id_x.mp4.tmp"} {assertPublicationBytes(t,filepath.Join(camera,relative),"retained")}
}
