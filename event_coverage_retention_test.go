package main

import (
 "bytes"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
)

func TestCoverageRetentionPreservesProcessingEvidence(t *testing.T) {
 for _,processed:=range []bool{false,true} {t.Run(map[bool]string{false:"unprocessed_nil_assessment",true:"processing_snapshot_after_cancel"}[processed],func(t *testing.T){
  bm,job:=selectionTimingJob(t)
  if job.recovery.Coverage!=nil {t.Fatal("setup checkpoint invented assessment")}
  if processed {
   if err:=job.recoveryCheckpoint("capturing","");err!=nil {t.Fatal(err)}
   if job.recovery.Coverage==nil || !job.recovery.Coverage.SelectionBound {t.Fatal("processing assessment missing")}
  }
  original:=job.recovery.Coverage
  before,err:=json.Marshal(job.recovery);if err!=nil {t.Fatal(err)}
  var want eventRecoveryManifest
  if err:=json.Unmarshal(before,&want);err!=nil {t.Fatal(err)}
  bm.cancel()
  if job.execution.bound(bm,job) {t.Fatal("canceled manager still qualifies live binding")}
  if err:=job.recoveryCheckpoint("retained","");err!=nil {t.Fatal(err)}
  if job.recovery.Coverage!=original {t.Fatal("retention recomputed processing assessment")}
  want.Revision++
  want.ObservedAt=job.recovery.ObservedAt
  expected,err:=json.Marshal(&want);if err!=nil {t.Fatal(err)}
  retained,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
  if !bytes.Equal(expected,retained) {t.Fatal("retention changed processing evidence",string(retained))}
  if _,err:=parseRecoveryManifest(retained,bm.camera.ID);err!=nil {t.Fatal("preserved snapshot became unreadable",err)}
  var coverageBefore []byte
  if original!=nil {coverageBefore,err=json.Marshal(original);if err!=nil {t.Fatal(err)}}
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("retention broke teardown/capacity")}
  final:=readRecoveryTest(t,job.dir)
  if processed {
   coverageAfter,err:=json.Marshal(final.Coverage);if err!=nil {t.Fatal(err)}
   if !bytes.Equal(coverageBefore,coverageAfter) {t.Fatal("terminal release changed assessment")}
  } else if final.Coverage!=nil {t.Fatal("terminal release invented assessment")}
  if _,err:=os.Stat(job.dir);err!=nil {t.Fatal("staging evidence removed",err)}
 })}
}
