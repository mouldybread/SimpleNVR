package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestPreparedExportRejectsFlatAndInvalidNamesBeforeMediaInspection(t *testing.T) {
 dir:=t.TempDir()
 base:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 intervalFixture(t,dir,"20261110_000000_flat.mp4")
 path:=scanFixture(t,dir,base,base.Add(time.Second),"valid")
 intervalFixture(t,filepath.Dir(path),"20261110_000000_invalid.mp4")
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 // This request touches the valid clip's end, so no clip covers it. Neither
 // malformed fixture may reach profile/keyframe inspection or satisfy coverage.
 req:=exportCreateRequest{CameraID:"front",Start:base.Add(time.Second).Format(time.RFC3339),End:base.Add(2*time.Second).Format(time.RFC3339),Mode:"copy"}
 if _,err:=planExistingExportJob(context.Background(),n,"",req);!errors.Is(err,errExportPlanGap) {t.Fatal("invalid name supplied export coverage",err)}
}

func TestPreparedExportUsesStampedDayFolder(t *testing.T) {
 requireFFmpeg(t)
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 source:=makePreparedMediaSource(t,ctx,false)
 dir:=t.TempDir();base:=time.Date(2026,11,21,0,0,0,0,time.UTC)
 target:=scanFixture(t,dir,base,base.Add(2*time.Second),"fixture")
 // Copy only this test's synthetic source, not application media.
 raw,err:=os.ReadFile(source);if err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(target,raw,0600);err!=nil {t.Fatal(err)}
 before:=preparedMediaDigest(t,target)
 n:=&NVR{cameras:map[string]*Camera{"front":{ID:"front",Name:"Front",RecordDir:dir}}}
 var saved exportSavedRange
 req:=exportCreateRequest{CameraID:"front",Start:base.Format(time.RFC3339),End:base.Add(2*time.Second).Format(time.RFC3339),Mode:"copy"}
 render,err:=planExistingExportJob(context.WithValue(ctx,exportSavedRangeKey{},&saved),n,"",req)
 if err!=nil || render==nil || !saved.Start.Equal(base) || !saved.End.Equal(base.Add(2*time.Second)) {t.Fatal("stamped prepared plan failed",saved,err)}
 if preparedMediaDigest(t,target)!=before {t.Fatal("planning changed source")}
}
