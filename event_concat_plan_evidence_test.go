package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "testing"
)

func concatPlanJob(t *testing.T) (*BufferManager,*bufferEvent,string) {
 t.Helper();installEventFailureTools(t,0,false)
 bm,job:=selectionTimingJob(t)
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
 job.execution.beginAssembly(bm,job,input)
 if job.execution.stage!=eventExecutionAssembling {t.Fatal("assembly did not enter")}
 return bm,job,input
}
func TestConcatPlanProductionWriterSealsExactFrozenOrder(t *testing.T) {
 bm,job,input:=concatPlanJob(t);p:=job.execution
 if err:=job.writeEventConcat(bm.ctx,input);err!=nil {t.Fatal(err)}
 c:=p.concat
 if !c.ready(p,bm,job) || p.qualifies(bm,job) {t.Fatal("writer alone qualified assembly or failed to seal")}
 want:="file 'pre_000.mp4'\nfile 'pre_001.mp4'\nfile 'event.mp4'\n"
 got,err:=os.ReadFile(filepath.Join(job.dir,"concat.txt"));if err!=nil || string(got)!=want {t.Fatal(string(got),err)}
 lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer lease.release()
 offset,err:=c.file.Seek(0,io.SeekCurrent);if err!=nil {t.Fatal(err)}
 if err:=c.check(context.Background(),lease,p,bm,job);err!=nil {t.Fatal(err)}
 after,err:=c.file.Seek(0,io.SeekCurrent);if err!=nil || after!=offset {t.Fatal("ReadAt changed descriptor offset",offset,after,err)}
 if !lease.held() {t.Fatal("borrowed lease released")}
}
func TestConcatRegistrationRequiresRealExactBytesAndWriterCompletion(t *testing.T) {
 for _,kind:=range []string{"registered but not sealed","reordered","missing capture","duplicate input","extra bytes","callback only","synthetic close error"} {t.Run(kind,func(t *testing.T){
  bm,job,_:=concatPlanJob(t);p:=job.execution
  data,err:=expectedEventConcatPlan(p,bm,job);if err!=nil {t.Fatal(err)}
  switch kind {
  case "reordered":data=[]byte("file 'pre_001.mp4'\nfile 'pre_000.mp4'\nfile 'event.mp4'\n")
  case "missing capture":data=[]byte("file 'pre_000.mp4'\nfile 'pre_001.mp4'\n")
  case "duplicate input":data=[]byte("file 'pre_000.mp4'\nfile 'pre_000.mp4'\nfile 'event.mp4'\n")
  case "extra bytes":data=append(data,'x')
  case "callback only":data=nil
  }
  err=job.withOwnedStagingCreator(bm.ctx,"concat.txt",func(f *os.File)error{if kind=="callback only" {return nil};_,err:=f.Write(data);return err})
  if err!=nil {t.Fatal("observational refusal changed writer return",err)}
  if kind=="registered but not sealed" || kind=="synthetic close error" {
   if p.concat==nil || p.concat.stage!=eventConcatPlanRegistered || p.concat.ready(p,bm,job) {t.Fatal("registration alone sealed the writer")}
   if kind=="synthetic close error" {p.finishConcatWrite(job,os.ErrInvalid)} else {p.finishConcatWrite(job,nil);if !p.concat.ready(p,bm,job) {t.Fatal("successful close result not accepted")};return}
  } else {p.finishConcatWrite(job,nil)}
  if p.stage!=eventExecutionRefused || p.concat.ready(p,bm,job) {t.Fatal("invalid plan/close result qualified",kind)}
  if _,err:=os.Stat(job.dir);err!=nil {t.Fatal("evidence removed",err)}
 })}
}
func TestConcatPlanReceiptPinnedBeforeAndAfterRevalidationIO(t *testing.T) {
 for _,kind:=range []string{"missing","copied","digest","wrong execution","removed during IO"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t);p:=job.execution;c:=p.concat
  if c==nil || check.concat!=c || !c.ready(p,bm,job) {t.Fatal("production receipt missing")}
  saved:=*c;copy:=*c
  switch kind {
  case "missing":p.concat=nil
  case "copied":p.concat=&copy
  case "digest":c.digest[0]^=1
  case "wrong execution":other:=*p;c.execution=&other
  }
  calls:=0
  err:=check.checkWithOutput(context.Background(),lease,func(ctx context.Context,lease *mediaMutationLease)error{
   calls++;if kind=="removed during IO" {p.concat=nil};return check.output.check(ctx,lease)
  })
  wantCalls:=0;if kind=="removed during IO" {wantCalls=1}
  if err==nil || calls!=wantCalls {t.Fatal(kind,err,calls)}
  *c=saved;p.concat=c
  if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("genuine receipt not reusable during its lifetime",err)}
  bm.releaseJob(job)
  if c.ready(p,bm,job) || !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("receipt lifetime or terminal ownership changed")}
 })}
}
func TestConcatPlanBytesMustRemainExactAfterPublication(t *testing.T) {
 for _,kind:=range []string{"same size wrong bytes","matching prefix","replacement","truncated"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  path:=filepath.Join(job.dir,"concat.txt")
  original,err:=os.ReadFile(path);if err!=nil {t.Fatal(err)}
  data:=append([]byte(nil),original...)
  switch kind {
  case "same size wrong bytes":data[0]='x'
  case "matching prefix":data=append(data,'x')
  case "replacement":if err:=os.Rename(path,path+".saved");err!=nil {t.Fatal(err)}
  case "truncated":data=data[:len(data)-1]
  }
  if err:=os.WriteFile(path,data,0600);err!=nil {t.Fatal(err)}
  if err:=check.check(context.Background(),lease);err==nil {t.Fatal("changed concat plan accepted",kind)}
  bm.releaseJob(job)
  if !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("refusal prevented retention/teardown")}
  got,err:=os.ReadFile(path);if err!=nil || string(got)!=string(data) {t.Fatal("check changed evidence",err)}
 })}
}
func TestConcatPlanCheckCancellationAndBorrowedLease(t *testing.T) {
 _,bm,job,check,lease:=releaseRevalidationFixture(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=check.concat.check(ctx,lease,job.execution,bm,job);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 if err:=check.concat.check(context.Background(),nil,job.execution,bm,job);err==nil {t.Fatal("nil lease admitted")}
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 if err:=check.concat.check(context.Background(),foreign,job.execution,bm,job);err==nil {t.Fatal("foreign lease admitted")}
 if !lease.held() || !foreign.held() {t.Fatal("borrowed ownership changed")}
}
