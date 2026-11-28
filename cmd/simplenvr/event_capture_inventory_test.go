package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func captureInventoryJob(t *testing.T) (*BufferManager,*bufferEvent) {
 t.Helper()
 bm:=bufferFixture(t)
 job,err:=bm.pinBuffer(bufferTestEvent("creator"));if err!=nil {t.Fatal(err)}
 // Private pin callers own teardown even if the test exits before releaseJob.
 t.Cleanup(func(){if err:=job.closeOwnedInventory();err!=nil {t.Error(err)}})
 return bm,job
}
func assertCreatorClosed(t *testing.T,file *os.File) {
 t.Helper();if file==nil {t.Fatal("writer not called")}
 if _,err:=file.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("creator leaked",err)}
}

func TestOwnedCreatorRegistersAfterWriterAndCloses(t *testing.T) {
 for _,failed:=range []bool{false,true} {t.Run(map[bool]string{false:"success",true:"write failure"}[failed],func(t *testing.T){
  _,job:=captureInventoryJob(t)
  var creator *os.File
  injected:=errors.New("writer failed")
  err:=job.withOwnedStagingCreator(context.Background(),"event.mp4",func(file *os.File)error{
   creator=file
   if _,exists:=job.inventory.entries["event.mp4"];exists {return errors.New("registered before write finished")}
   if _,err:=file.WriteString("recovery bytes");err!=nil {return err}
   if failed {return injected};return nil
  })
  if failed {if !errors.Is(err,injected) {t.Fatal(err)}} else if err!=nil {t.Fatal(err)}
  assertCreatorClosed(t,creator)
  entry,ok:=job.inventory.entries["event.mp4"];if !ok {t.Fatal("final evidence not registered")}
  if _,err:=entry.file.Stat();err!=nil {t.Fatal("inventory duplicate closed too early",err)}
  lease:=publishedEngineLease(t)
  err=job.inventory.Preflight(context.Background(),lease);lease.release();if err!=nil {t.Fatal(err)}
  if err:=job.closeOwnedInventory();err!=nil {t.Fatal(err)}
  assertCreatorClosed(t,entry.file)
 })}
}

func TestOwnedCreatorReplacementRefusesWithoutWritingReplacement(t *testing.T) {
 for _,name:=range []string{"event.mp4","concat.txt"} {t.Run(name,func(t *testing.T){
  _,job:=captureInventoryJob(t)
  named:=filepath.Join(job.dir,name);moved:=named+".evidence"
  var creator *os.File
  err:=job.withOwnedStagingCreator(context.Background(),name,func(file *os.File)error{
   creator=file
   if err:=os.Rename(named,moved);err!=nil {return err}
   if err:=os.WriteFile(named,[]byte("foreign evidence"),0600);err!=nil {return err}
   _,err:=file.WriteString("creator bytes");return err
  })
  if err==nil || !job.keepStaging {t.Fatal("replacement accepted")}
  assertCreatorClosed(t,creator)
  if _,ok:=job.inventory.entries[name];ok {t.Fatal("replacement re-adopted")}
  assertPublicationBytes(t,named,"foreign evidence")
  assertPublicationBytes(t,moved,"creator bytes")
 })}
}

func TestOwnedCreatorRejectsExistingEntryAndCanceledAdmission(t *testing.T) {
 for _,name:=range []string{"event.mp4","concat.txt"} {t.Run(name,func(t *testing.T){
  _,job:=captureInventoryJob(t)
  named:=filepath.Join(job.dir,name)
  if err:=os.Symlink(filepath.Join(t.TempDir(),"outside"),named);err!=nil {t.Fatal(err)}
  called:=false
  if err:=job.withOwnedStagingCreator(context.Background(),name,func(*os.File)error{called=true;return nil});err==nil || called {t.Fatal("existing name accepted")}
  if err:=os.Remove(named);err!=nil {t.Fatal(err)}
  owner:=publishedEngineLease(t)
  parent,cancel:=context.WithCancel(context.Background())
  ctx:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
  result:=make(chan error,1)
  go func(){result<-job.withOwnedStagingCreator(ctx,name,func(*os.File)error{return errors.New("must not run")})}()
  awaitShutdownStep(t,ctx.waiting,"creator did not wait");cancel()
  var err error
  select {case err=<-result:case <-time.After(3*time.Second):owner.release();t.Fatal("creator wait did not join")}
  held:=owner.held();owner.release()
  if !errors.Is(err,context.Canceled) || !held {t.Fatal("canceled admission disturbed owner",err)}
  if _,err:=os.Lstat(named);!os.IsNotExist(err) {t.Fatal("canceled admission created entry",err)}
 })}
}

func TestOwnedCreatorCanceledWriterStillRecordsFinalEvidence(t *testing.T) {
 _,job:=captureInventoryJob(t)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 var creator *os.File
 err:=job.withOwnedStagingCreator(ctx,"event.mp4",func(file *os.File)error{
  creator=file
  if _,err:=file.WriteString("canceled bytes");err!=nil {return err}
  cancel();return ctx.Err()
 })
 if !errors.Is(err,context.Canceled) {t.Fatal(err)}
 assertCreatorClosed(t,creator)
 if _,ok:=job.inventory.entries["event.mp4"];!ok {t.Fatal("canceled writer lost final evidence")}
}

// A synthetic child, not FFmpeg or media validation. Pipe barriers prove that
// ownership can be acquired while a real child is running and awaiting input.
func TestOwnedCaptureDescriptorChild(t *testing.T) {
 if os.Getenv("SIMPLENVR_CREATOR_CHILD")!="1" {return}
 output:=os.NewFile(3,"creator");ready:=os.NewFile(4,"ready");resume:=os.NewFile(5,"resume")
 if _,err:=ready.Write([]byte{1});err!=nil {os.Exit(2)}
 ready.Close()
 var one [1]byte
 if _,err:=io.ReadFull(resume,one[:]);err!=nil {os.Exit(3)}
 if _,err:=output.WriteString("synthetic child bytes");err!=nil {os.Exit(4)}
 output.Close();resume.Close();os.Exit(0)
}
func TestOwnedCreatorNoMediaLeaseAcrossChildExecution(t *testing.T) {
 _,job:=captureInventoryJob(t)
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 readyR,readyW,err:=os.Pipe();if err!=nil {cancel();t.Fatal(err)}
 resumeR,resumeW,err:=os.Pipe();if err!=nil {cancel();readyR.Close();readyW.Close();t.Fatal(err)}
 result:=make(chan error,1);done:=make(chan struct{})
 defer func(){
  cancel();readyR.Close();readyW.Close();resumeR.Close();resumeW.Close()
  select {case <-done:case <-time.After(6*time.Second):t.Error("synthetic child worker did not join")}
 }()
 var creator *os.File
 go func(){defer close(done);result<-job.withOwnedStagingCreator(ctx,"event.mp4",func(file *os.File)error{
  creator=file
  cmd:=interruptibleCommand(ctx,os.Args[0],time.Second,"-test.run=^TestOwnedCaptureDescriptorChild$")
  cmd.Env=append(os.Environ(),"SIMPLENVR_CREATOR_CHILD=1")
  cmd.ExtraFiles=[]*os.File{file,readyW,resumeR}
  if err:=cmd.Start();err!=nil {readyW.Close();return err}
  readyW.Close();resumeR.Close()
  return cmd.Wait()
 })}()
 if err:=readyR.SetReadDeadline(time.Now().Add(3*time.Second));err!=nil {t.Fatal(err)}
 var one [1]byte
 if _,err:=io.ReadFull(readyR,one[:]);err!=nil {t.Fatal("child not ready",err)}
 gateCtx,gateCancel:=context.WithTimeout(context.Background(),time.Second)
 lease,gateErr:=recordingMediaOwnership.acquire(gateCtx);gateCancel()
 if gateErr!=nil {t.Fatal("media gate held across child execution",gateErr)}
 lease.release()
 if _,err:=resumeW.Write([]byte{1});err!=nil {t.Fatal(err)}
 resumeW.Close()
 select {case err:=<-result:if err!=nil {t.Fatal(err)};case <-ctx.Done():t.Fatal("child did not finish")}
 <-done
 assertCreatorClosed(t,creator)
 assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"synthetic child bytes")
}

func TestProductionCaptureUsesCreatorDescriptorAndRefusesReplacement(t *testing.T) {
 for _,replace:=range []bool{false,true} {t.Run(map[bool]string{false:"success",true:"replacement"}[replace],func(t *testing.T){
  bm,job:=captureInventoryJob(t)
  bin:=t.TempDir();named:=filepath.Join(job.dir,"event.mp4")
  script:="#!/bin/sh\nfor last do :; done\n[ \"$last\" = /proc/self/fd/3 ] || exit 9\n"
  if replace {script+="mv -- \"$CREATOR_NAMED\" \"$CREATOR_NAMED.evidence\" || exit 8\nprintf 'replacement' > \"$CREATOR_NAMED\"\n"}
  script+="printf 'synthetic capture' > \"$last\"\n"
  if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
  t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"));t.Setenv("CREATOR_NAMED",named)
  path:=bm.recordEventDuration(job)
  if replace {
   if path!="" || !job.keepStaging {t.Fatal("replacement finalized")}
   assertPublicationBytes(t,named,"replacement")
   assertPublicationBytes(t,named+".evidence","synthetic capture")
   if _,ok:=job.inventory.entries["event.mp4"];ok {t.Fatal("replacement adopted")}
  } else {
   if path!=named {t.Fatal("capture failed")}
   if _,ok:=job.inventory.entries["event.mp4"];!ok {t.Fatal("production capture not adopted")}
   if m:=readRecoveryTest(t,job.dir);m.State!="captured" {t.Fatal(m)}
  }
 })}
}

func TestProductionConcatAdoptsAndClosesOnInvalidInput(t *testing.T) {
 for _,invalid:=range []bool{false,true} {t.Run(map[bool]string{false:"success",true:"invalid second input"}[invalid],func(t *testing.T){
  _,job:=captureInventoryJob(t)
  input:=filepath.Join(job.dir,"event.mp4")
  if invalid {job.preFiles=[]string{input};input=filepath.Join(t.TempDir(),"outside.mp4")}
  err:=job.writeEventConcat(context.Background(),input)
  if (err!=nil)!=invalid {t.Fatal(err)}
  entry,ok:=job.inventory.entries["concat.txt"];if !ok {t.Fatal("concat final evidence missing")}
  assertPublicationBytes(t,filepath.Join(job.dir,"concat.txt"),"file 'event.mp4'\n")
  if err:=job.closeOwnedInventory();err!=nil {t.Fatal(err)}
  assertCreatorClosed(t,entry.file)
 })}
}

func TestCaptureCheckpointFailureClosesCreatorBeforeReturn(t *testing.T) {
 installEventFailureTools(t,0,false)
 bm,job:=captureInventoryJob(t)
 calls:=0
 original:=job.recoveryWriter
 job.recoveryWriter=func(dir string,data []byte)error{
  calls++;if calls==2 {return os.ErrPermission};return original(dir,data)
 }
 if path:=bm.recordEventDuration(job);path!="" || !job.recoveryFailed || !job.keepStaging {t.Fatal("captured checkpoint failure ignored")}
 entry,ok:=job.inventory.entries["event.mp4"];if !ok {t.Fatal("capture evidence missing")}
 // The sole parent descriptor for this inode is now the retained inventory pin.
 // Closure of the creator itself is directly asserted in the helper tests.
 if _,err:=entry.file.Stat();err!=nil {t.Fatal(err)}
 if err:=job.closeOwnedInventory();err!=nil {t.Fatal(err)}
 assertCreatorClosed(t,entry.file)
}

func TestOwnedCreatorRegistrationRefusalClosesAndRetains(t *testing.T) {
 _,job:=captureInventoryJob(t)
 var creator *os.File
 err:=job.withOwnedStagingCreator(context.Background(),"event.mp4",func(file *os.File)error{
  creator=file
  if _,err:=file.WriteString("refused bytes");err!=nil {return err}
  // A root metadata change invalidates admission without changing creator bytes.
  return os.Chmod(job.dir,0750)
 })
 if err==nil || !job.keepStaging {t.Fatal("invalid root registered")}
 assertCreatorClosed(t,creator)
 if _,ok:=job.inventory.entries["event.mp4"];ok {t.Fatal("refused file registered")}
 assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"refused bytes")
}

func TestProductionCaptureProcessFailureRetainsFinalEvidence(t *testing.T) {
 installEventFailureTools(t,1,false)
 bm,job:=captureInventoryJob(t)
 if path:=bm.recordEventDuration(job);path!="" || !job.keepStaging {t.Fatal("process failure finalized")}
 entry,ok:=job.inventory.entries["event.mp4"];if !ok {t.Fatal("failed process evidence missing")}
 assertPublicationBytes(t,filepath.Join(job.dir,"event.mp4"),"synthetic output")
 if m:=readRecoveryTest(t,job.dir);m.State!="capture_failed" {t.Fatal(m)}
 if err:=job.closeOwnedInventory();err!=nil {t.Fatal(err)}
 assertCreatorClosed(t,entry.file)
}
