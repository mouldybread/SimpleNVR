package main

import (
 "bytes"
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func recoveryDayInspectionFixture(t *testing.T,state string,partial bool) (string,string,string,[]byte,eventRecoveryManifest) {
 t.Helper()
 root,id,data:=recoveryInspectionFixture(t)
 var m eventRecoveryManifest
 if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
 start:=time.Date(2026,11,23,23,59,55,0,time.UTC)
 m.RequestedStart=start.Add(10*time.Second);m.RequestedEnd=m.RequestedStart.Add(30*time.Second)
 m.State=state;m.Partial=partial
 if state=="publication_failed" {m.FailureCode="validation_or_publication_failed"}
 normal,err:=stampedFinalPath(root,start,m.RequestedEnd,"inspection",false);if err!=nil {t.Fatal(err)}
 final:=normal;if partial {final=partialRecordingName(final)}
 m.OutputTemporary=filepath.Base(normal)+".tmp";m.OutputPublished=filepath.Base(final)
 data,err=json.Marshal(m);if err!=nil {t.Fatal(err)}
 if _,err:=parseRecoveryManifest(data,"a");err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(filepath.Join(root,"temp",id),data);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,m.OutputTemporary),[]byte("temporary evidence"),0600);err!=nil {t.Fatal(err)}
 return root,id,final,data,m
}

func TestRecoveryInspectionFindsStampedPublishedOutputs(t *testing.T) {
 for _,state:=range []string{"publishing","published","publication_failed"} {
  for _,partial:=range []bool{false,true} {
   label:="complete";if partial {label="partial"}
   t.Run(state+"/"+label,func(t *testing.T){
    root,id,final,before,_:=recoveryDayInspectionFixture(t,state,partial)
    if err:=os.Mkdir(filepath.Dir(final),0700);err!=nil {t.Fatal(err)}
    evidence:=[]byte("not certified media")
    if err:=os.WriteFile(final,evidence,0600);err!=nil {t.Fatal(err)}
    report:=inspectEventRecovery(context.Background(),root,"a",id)
    expected:="publication_outcome_ambiguous"
    if state=="published" {expected="published_checkpoint_unverified"}
    if state=="publication_failed" {expected="observed_processing_failure"}
    if !report.Complete || report.Classification!=expected || len(report.Artifacts)!=4 {t.Fatalf("bad report: %+v",report)}
    temporary,published:=report.Artifacts[2],report.Artifacts[3]
    if temporary.Role!="temporary_output" || !temporary.Present || published.Role!="published_output" || !published.Present || published.LogicalBytes==nil || *published.LogicalBytes!=int64(len(evidence)) {t.Fatalf("bad observations: %+v",report.Artifacts)}
    after,err:=os.ReadFile(filepath.Join(root,"temp",id,"recovery.json"));if err!=nil || !bytes.Equal(before,after) {t.Fatal("manifest changed",err)}
    after,err=os.ReadFile(final);if err!=nil || !bytes.Equal(evidence,after) {t.Fatal("output changed",err)}
   })
  }
 }
}

func TestRecoveryInspectionDoesNotAdoptMisplacedStampedOutput(t *testing.T) {
 for _,kind:=range []string{"missing_day","empty_day","root_file","wrong_day"} {t.Run(kind,func(t *testing.T){
  root,id,final,_,_:=recoveryDayInspectionFixture(t,"published",false)
  if kind=="empty_day" {if err:=os.Mkdir(filepath.Dir(final),0700);err!=nil {t.Fatal(err)}}
  if kind=="root_file" {if err:=os.WriteFile(filepath.Join(root,filepath.Base(final)),[]byte("misplaced"),0600);err!=nil {t.Fatal(err)}}
  if kind=="wrong_day" {
   wrong:=filepath.Join(root,"20261124");if err:=os.Mkdir(wrong,0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(filepath.Join(wrong,filepath.Base(final)),[]byte("wrong day"),0600);err!=nil {t.Fatal(err)}
  }
  report:=inspectEventRecovery(context.Background(),root,"a",id)
  if !report.Complete || len(report.Artifacts)!=4 || report.Artifacts[3].Present {t.Fatalf("misplaced file adopted: %+v",report)}
  if kind!="empty_day" {if _,err:=os.Lstat(filepath.Dir(final));!os.IsNotExist(err) {t.Fatal("inspection created day",err)}}
 })}
}

func TestRecoveryInspectionRejectsUnsafeDayAndPublishedObjects(t *testing.T) {
 for _,kind:=range []string{"day_symlink","day_file","day_fifo","output_symlink","output_fifo","output_directory"} {t.Run(kind,func(t *testing.T){
  root,id,final,_,_:=recoveryDayInspectionFixture(t,"published",false)
  day:=filepath.Dir(final)
  var err error
  switch kind {
  case "day_symlink":err=os.Symlink(t.TempDir(),day)
  case "day_file":err=os.WriteFile(day,[]byte("not a directory"),0600)
  case "day_fifo":err=unix.Mkfifo(day,0600)
  default:
   if err=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
   switch kind {
   case "output_symlink":err=os.Symlink(filepath.Join(t.TempDir(),"outside"),final)
   case "output_fifo":err=unix.Mkfifo(final,0600)
   case "output_directory":err=os.Mkdir(final,0700)
   }
  }
  if err!=nil {t.Fatal(err)}
  report:=inspectEventRecovery(context.Background(),root,"a",id)
  if report.Complete || len(report.Errors)==0 {t.Fatalf("unsafe object accepted: %+v",report)}
 })}
}

func TestRecoveryPublishedLocationRechecksDayIdentityAndAbsence(t *testing.T) {
 for _,exists:=range []bool{false,true} {
  label:="missing";if exists {label="opened"}
  t.Run(label,func(t *testing.T){
   root,_,final,_,_:=recoveryDayInspectionFixture(t,"published",false)
   day:=filepath.Dir(final)
   if exists {if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}}
   directory,err:=os.Open(root);if err!=nil {t.Fatal(err)};defer directory.Close()
   location,err:=openRecoveryPublishedLocation(directory,filepath.Base(final));if err!=nil {t.Fatal(err)};defer location.close()
   if err:=location.current();err!=nil {t.Fatal(err)}
   if exists {if err:=os.Rename(day,day+"-old");err!=nil {t.Fatal(err)}}
   if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
   if err:=location.current();err==nil {t.Fatal("changed day accepted")}
  })
 }
}

func TestRecoveryInspectionRetainsLegacyRootOutputLookup(t *testing.T) {
 root,id,data:=recoveryInspectionFixture(t)
 var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {t.Fatal(err)}
 m.State="published";m.OutputTemporary="legacy.mp4.tmp";m.OutputPublished="legacy.mp4"
 data,err:=json.Marshal(m);if err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(filepath.Join(root,"temp",id),data);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,m.OutputPublished),[]byte("legacy evidence"),0600);err!=nil {t.Fatal(err)}
 report:=inspectEventRecovery(context.Background(),root,"a",id)
 if !report.Complete || len(report.Artifacts)!=4 || !report.Artifacts[3].Present {t.Fatalf("legacy lookup regressed: %+v",report)}
}
