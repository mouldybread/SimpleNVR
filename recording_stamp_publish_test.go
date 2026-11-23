package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func stampLease(t *testing.T) *mediaMutationLease {
 t.Helper();lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)}
 t.Cleanup(lease.release);return lease
}

func TestStampedFinalPathBuildsDayFolderAndRejectsBadInput(t *testing.T) {
 camera:="/recordings/cam_a";hkt:=time.FixedZone("HKT",8*3600)
 start:=time.Date(2026,11,10,23,55,0,0,time.UTC)
 got,err:=stampedFinalPath(camera,start,start.Add(10*time.Minute),"cam_a_id_x",false)
 if want:="/recordings/cam_a/20261110/20261110_235500_E20261111_000500_cam_a_id_x.mp4";err!=nil || got!=want {t.Fatal(got,err)}
 got,err=stampedFinalPath(camera,start.In(hkt),start.Add(10*time.Minute).In(hkt),"cam_a_id_x",true)
 if want:="/recordings/cam_a/20261110/20261110_235500_E20261111_000500_cam_a_id_x_partial.mp4";err!=nil || got!=want {t.Fatal(got,err)}
 fraction:=time.Date(2026,11,10,12,0,0,900000000,time.UTC)
 got,err=stampedFinalPath(camera,fraction,fraction.Add(10*time.Minute),"x",false)
 if want:="/recordings/cam_a/20261110/20261110_120000_E20261110_121001_x.mp4";err!=nil || got!=want {t.Fatal("rounding",got,err)}
 if _,err:=stampedFinalPath(camera,start,start.Add(maxStampedSpan),"x",false);err!=nil {t.Fatal("exactly 24h rejected",err)}
 bad:=[]struct{name,dir string;start,end time.Time;label string}{
  {"relative dir","cam",start,start.Add(time.Minute),"x"},
  {"unclean dir","/recordings/../x",start,start.Add(time.Minute),"x"},
  {"root dir","/",start,start.Add(time.Minute),"x"},
  {"empty label",camera,start,start.Add(time.Minute),""},
  {"slash label",camera,start,start.Add(time.Minute),"a/b"},
  {"dot label",camera,start,start.Add(time.Minute),"a.b"},
  {"non-ascii label",camera,start,start.Add(time.Minute),"caf\u00e9"},
  {"long label",camera,start,start.Add(time.Minute),strings.Repeat("a",161)},
  {"zero span",camera,start,start,"x"},
  {"end before start",camera,start,start.Add(-time.Second),"x"},
  {"sub-second span",camera,start,start.Add(time.Nanosecond),"x"},
  {"over 24h",camera,start,start.Add(maxStampedSpan+time.Second),"x"},
 }
 for _,c:=range bad {
  if path,err:=stampedFinalPath(c.dir,c.start,c.end,c.label,false);!errors.Is(err,errStampedName) || path!="" {t.Fatal(c.name,path,err)}
 }
}

func TestEnsureStampedDayDirCreatesOnceAndRefusesUnsafeEntries(t *testing.T) {
 lease:=stampLease(t);camera:=t.TempDir();outside:=t.TempDir();ctx:=context.Background()
 if err:=ensureStampedDayDirLeased(ctx,lease,camera,"20261110");err!=nil {t.Fatal(err)}
 if info,err:=os.Lstat(filepath.Join(camera,"20261110"));err!=nil || !info.IsDir() {t.Fatal(info,err)}
 if err:=ensureStampedDayDirLeased(ctx,lease,camera,"20261110");err!=nil {t.Fatal("not idempotent",err)}
 if err:=os.Symlink(outside,filepath.Join(camera,"20261111"));err!=nil {t.Fatal(err)}
 if err:=ensureStampedDayDirLeased(ctx,lease,camera,"20261111");!errors.Is(err,errStampedDayDir) {t.Fatal("symlink accepted",err)}
 if err:=os.WriteFile(filepath.Join(camera,"20261112"),[]byte("x"),0600);err!=nil {t.Fatal(err)}
 if err:=ensureStampedDayDirLeased(ctx,lease,camera,"20261112");!errors.Is(err,errStampedDayDir) {t.Fatal("regular file accepted",err)}
 for _,day:=range []string{"","2026111","202611100","20261310","../x","2026-11-1"} {
  if err:=ensureStampedDayDirLeased(ctx,lease,camera,day);!errors.Is(err,errStampedDayDir) {t.Fatal("bad day accepted",day,err)}
 }
 if entries,err:=os.ReadDir(outside);err!=nil || len(entries)!=0 {t.Fatal("wrote through symlink",err)}
 if err:=ensureStampedDayDirLeased(ctx,nil,camera,"20261113");!errors.Is(err,errPublicationOwnership) {t.Fatal("nil lease accepted",err)}
 if _,err:=os.Lstat(filepath.Join(camera,"20261113"));!os.IsNotExist(err) {t.Fatal("created without a lease",err)}
 if err:=ensureStampedDayDirLeased(ctx,lease,"relative","20261113");!errors.Is(err,errStampedDayDir) {t.Fatal("relative camera dir accepted",err)}
 canceled,cancel:=context.WithCancel(context.Background());cancel()
 if err:=ensureStampedDayDirLeased(canceled,lease,camera,"20261114");!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if _,err:=os.Lstat(filepath.Join(camera,"20261114"));!os.IsNotExist(err) {t.Fatal("created after cancellation",err)}
}

func TestPublishStampedMovesTempIntoNewDayFolderWithoutOverwrite(t *testing.T) {
 lease:=stampLease(t);camera:=t.TempDir();ctx:=context.Background()
 start:=time.Date(2026,11,10,12,0,0,0,time.UTC);end:=start.Add(10*time.Minute)
 temporary:=publicationSource(t,camera,"20261110_120000_cam.mp4.tmp","candidate")
 final,err:=publishStampedLeasedWith(ctx,lease,temporary,camera,start,end,"cam_x",false,acceptPublicationFixture)
 if want:=filepath.Join(camera,"20261110","20261110_120000_E20261110_121000_cam_x.mp4");err!=nil || final!=want {t.Fatal(final,err)}
 assertPublicationBytes(t,final,"candidate")
 if _,err:=os.Lstat(temporary);!os.IsNotExist(err) {t.Fatal("temporary retained after publish",err)}
 if _,err:=parseStampedName(filepath.Base(final));err!=nil {t.Fatal("published name does not parse",err)}
 second:=publicationSource(t,camera,"second.mp4.tmp","second candidate")
 if _,err:=publishStampedLeasedWith(ctx,lease,second,camera,start,end,"cam_x",false,acceptPublicationFixture);!os.IsExist(err) {t.Fatal("collision not rejected",err)}
 assertPublicationBytes(t,second,"second candidate");assertPublicationBytes(t,final,"candidate")
 partialSource:=publicationSource(t,camera,"third.mp4.tmp","partial candidate")
 partial,err:=publishStampedLeasedWith(ctx,lease,partialSource,camera,start.Add(time.Hour),end.Add(time.Hour),"cam_x",true,acceptPublicationFixture)
 if want:=filepath.Join(camera,"20261110","20261110_130000_E20261110_131000_cam_x_partial.mp4");err!=nil || partial!=want {t.Fatal(partial,err)}
 assertPublicationBytes(t,partial,"partial candidate")
 if _,err:=parseStampedName(filepath.Base(partial));err!=nil {t.Fatal("partial name does not parse",err)}
}

func TestPublishStampedFailuresRetainCandidateAndPublishNothing(t *testing.T) {
 lease:=stampLease(t);camera:=t.TempDir();ctx:=context.Background()
 start:=time.Date(2026,11,10,12,0,0,0,time.UTC);end:=start.Add(10*time.Minute)
 invalid:=publicationSource(t,camera,"invalid.mp4.tmp","invalid media")
 reject:=func(context.Context,string) error {return errors.New("invalid media")}
 final,err:=publishStampedLeasedWith(ctx,lease,invalid,camera,start,end,"cam_x",false,reject)
 if err==nil {t.Fatal("validation failure published")}
 if _,statErr:=os.Lstat(final);!os.IsNotExist(statErr) {t.Fatal("failed validation left a published file",statErr)}
 assertPublicationBytes(t,invalid,"invalid media")
 unowned:=publicationSource(t,camera,"unowned.mp4.tmp","unowned")
 next:=start.Add(24*time.Hour)
 if _,err:=publishStampedLeasedWith(ctx,nil,unowned,camera,next,next.Add(time.Minute),"cam_x",false,acceptPublicationFixture);!errors.Is(err,errPublicationOwnership) {t.Fatal("published without a lease",err)}
 assertPublicationBytes(t,unowned,"unowned")
 if _,err:=os.Lstat(filepath.Join(camera,"20261111"));!os.IsNotExist(err) {t.Fatal("day folder created without a lease",err)}
 if _,err:=publishStampedLeasedWith(ctx,lease,unowned,camera,start,start,"cam_x",false,acceptPublicationFixture);!errors.Is(err,errStampedName) {t.Fatal("empty span accepted",err)}
 assertPublicationBytes(t,unowned,"unowned")
}
