package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

func outputEvidenceOps() publicationDurabilityOps {
 return publicationDurabilityOps{renameRecordingNoReplace,func(f *os.File)error{return f.Sync()},func(f *os.File)error{return f.Sync()}}
}
func publishedOutputFixture(t *testing.T) (*eventPublishedOutput,*mediaMutationLease,string) {
 t.Helper();lease:=publishedEngineLease(t)
 dir:=filepath.Join(t.TempDir(),"camera");if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 temporary,final:=filepath.Join(dir,"capture.tmp"),filepath.Join(dir,"published.mp4")
 if err:=os.WriteFile(temporary,[]byte("published bytes"),0600);err!=nil {t.Fatal(err)}
 var output *eventPublishedOutput;var originalFile,originalDir *os.File
 err:=publishRecordingDurablyObservedLeased(context.Background(),lease,temporary,final,func(context.Context,string)error{return nil},outputEvidenceOps(),func(file,dir *os.File)error{
  originalFile,originalDir=file,dir
  var err error;output,err=retainEventPublishedOutput(context.Background(),lease,final,file,dir);return err
 })
 if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){if err:=output.Close();err!=nil {t.Error(err)}})
 for _,file:=range []*os.File{originalFile,originalDir} {if _,err:=file.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("publication original not closed",err)}}
 return output,lease,final
}
func TestPublishedOutputPinsExactPublicationDescriptors(t *testing.T) {
 output,lease,final:=publishedOutputFixture(t)
 if err:=output.check(context.Background(),lease);err!=nil {t.Fatal(err)}
 for _,file:=range []*os.File{output.file,output.directory} {
  flags,err:=unix.FcntlInt(file.Fd(),unix.F_GETFD,0);if err!=nil || flags&unix.FD_CLOEXEC==0 {t.Fatal("publication pin not CLOEXEC",flags,err)}
 }
 assertPublicationBytes(t,final,"published bytes")
 file,dir:=output.file,output.directory
 if err:=output.Close();err!=nil {t.Fatal(err)};if err:=output.Close();err!=nil {t.Fatal(err)}
 if err:=output.check(context.Background(),lease);err==nil {t.Fatal("closed evidence accepted")}
 for _,pin:=range []*os.File{file,dir} {if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("publication pin leaked",err)}}
 if !lease.held() {t.Fatal("evidence closed borrowed lease")}
}

func TestPublishedOutputRefusesReplacementMutationAndMissingEntry(t *testing.T) {
 for _,change:=range []string{"replacement","symlink","fifo","directory","missing","modified","camera directory"} {t.Run(change,func(t *testing.T){
  output,lease,final:=publishedOutputFixture(t)
  outside:=filepath.Join(t.TempDir(),"outside");if err:=os.WriteFile(outside,[]byte("outside bytes"),0600);err!=nil {t.Fatal(err)}
  switch change {
  case "modified":if err:=os.WriteFile(final,[]byte("modified bytes"),0600);err!=nil {t.Fatal(err)}
  case "camera directory":
   parent:=filepath.Dir(final);if err:=os.Rename(parent,parent+"-saved");err!=nil {t.Fatal(err)}
   if err:=os.Mkdir(parent,0700);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(final,[]byte("replacement bytes"),0600);err!=nil {t.Fatal(err)}
  default:
   if err:=os.Rename(final,final+"-saved");err!=nil {t.Fatal(err)}
   var err error
   switch change {case "replacement":err=os.WriteFile(final,[]byte("replacement bytes"),0600);case "symlink":err=os.Symlink(outside,final);case "fifo":err=unix.Mkfifo(final,0600);case "directory":err=os.Mkdir(final,0700)}
   if err!=nil {t.Fatal(err)}
  }
  if err:=output.check(context.Background(),lease);err==nil {t.Fatal("changed published evidence accepted",change)}
  assertPublicationBytes(t,outside,"outside bytes")
  if change=="replacement" || change=="camera directory" {assertPublicationBytes(t,final,"replacement bytes")}
  if change=="modified" {assertPublicationBytes(t,final,"modified bytes")}
 })}
}

func TestPublishedOutputConstructorNeverAdoptsReplacementName(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();temporary:=filepath.Join(dir,"capture.tmp");final:=filepath.Join(dir,"published.mp4")
 if err:=os.WriteFile(temporary,[]byte("original bytes"),0600);err!=nil {t.Fatal(err)}
 var output *eventPublishedOutput
 err:=publishRecordingDurablyObservedLeased(context.Background(),lease,temporary,final,func(context.Context,string)error{return nil},outputEvidenceOps(),func(file,root *os.File)error{
  if err:=os.Rename(final,final+"-saved");err!=nil {return err}
  if err:=os.WriteFile(final,[]byte("foreign bytes"),0600);err!=nil {return err}
  var err error;output,err=retainEventPublishedOutput(context.Background(),lease,final,file,root);return err
 })
 if err==nil || output!=nil {if output!=nil {output.Close()};t.Fatal("replacement re-adopted",err)}
 assertPublicationBytes(t,final,"foreign bytes");assertPublicationBytes(t,final+"-saved","original bytes")
}

func TestPublishedOutputDuplicateFailureClosesTransientPin(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();path:=filepath.Join(dir,"published.mp4")
 if err:=os.WriteFile(path,[]byte("bytes"),0600);err!=nil {t.Fatal(err)}
 file,err:=os.Open(path);if err!=nil {t.Fatal(err)};defer file.Close()
 root,err:=os.Open(dir);if err!=nil {t.Fatal(err)};defer root.Close()
 var clone *os.File;calls:=0
 output,err:=retainEventPublishedOutputWithDup(context.Background(),lease,path,file,root,func(file *os.File,name string)(*os.File,error){
  calls++;if calls==2 {return nil,unix.EMFILE};var err error;clone,err=duplicateEventPublicationFile(file,name);return clone,err
 })
 if output!=nil || !errors.Is(err,unix.EMFILE) || clone==nil {t.Fatal(output,err)}
 if _,err:=clone.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("failed constructor leaked clone",err)}
 for _,original:=range []*os.File{file,root} {if _,err:=original.Stat();err!=nil {t.Fatal("constructor closed borrowed source",err)}}
}

func TestPublicationObserverRunsOnlyAfterDurabilityAndIdentitySuccess(t *testing.T) {
 for _,failure:=range []string{"file sync","rename","directory sync","none"} {t.Run(failure,func(t *testing.T){
  lease:=publishedEngineLease(t);dir:=t.TempDir();temporary,final:=filepath.Join(dir,"capture.tmp"),filepath.Join(dir,"published.mp4")
  if err:=os.WriteFile(temporary,[]byte("bytes"),0600);err!=nil {t.Fatal(err)}
  ops:=outputEvidenceOps();events:=[]string{}
  ops.syncFile=func(f *os.File)error{events=append(events,"file sync");if failure=="file sync" {return unix.EIO};return f.Sync()}
  ops.rename=func(from,to string)error{events=append(events,"rename");if failure=="rename" {return unix.EIO};return renameRecordingNoReplace(from,to)}
  ops.syncDirectory=func(f *os.File)error{events=append(events,"directory sync");if failure=="directory sync" {return unix.EIO};return f.Sync()}
  observed:=false
  err:=publishRecordingDurablyObservedLeased(context.Background(),lease,temporary,final,func(context.Context,string)error{return nil},ops,func(file,root *os.File)error{
   observed=true
   if len(events)!=3 || events[0]!="file sync" || events[1]!="rename" || events[2]!="directory sync" {return unix.EINVAL}
   return nil
  })
  if failure=="none" {if err!=nil || !observed {t.Fatal(events,observed,err)}} else if err==nil || observed {t.Fatal("failed publication yielded evidence",failure,events,err)}
 })}
}

func TestEventPublishedOutputLivesThroughTerminalCheckpoint(t *testing.T) {
 for _,checkpointFailure:=range []bool{false,true} {t.Run(map[bool]string{false:"success",true:"checkpoint failure"}[checkpointFailure],func(t *testing.T){
  installEventFailureTools(t,0,false)
  bm,job:=publicationCheckpointJob(t);temporary,final:=filepath.Join(bm.camera.RecordDir,"output.tmp"),filepath.Join(bm.camera.RecordDir,"output.mp4")
  if err:=os.WriteFile(temporary,[]byte("published bytes"),0600);err!=nil {t.Fatal(err)}
  if checkpointFailure {job.recoveryWriter=func(string,[]byte)error{return os.ErrPermission}}
  if err:=bm.publishEventRecording(context.Background(),bm.ctx,job,temporary,final,false);err!=nil {t.Fatal(err)}
  output:=job.recoveryBorrow.output
  if output==nil {t.Fatal("production publication evidence absent")}
  lease:=job.recoveryBorrow.lease
  if err:=output.check(context.Background(),lease);err!=nil {t.Fatal(err)}
  file,root:=output.file,output.directory
  if !checkpointFailure {
   original:=job.recoveryWriter
   job.recoveryWriter=func(dir string,data []byte)error{if err:=output.check(context.Background(),lease);err!=nil {return err};return original(dir,data)}
  }
  bm.releaseJob(job)
  if lease.held() || job.inventory!=nil || job.recoveryBorrow!=nil || len(bm.slots)!=0 {t.Fatal("terminal output ownership leaked")}
  for _,pin:=range []*os.File{file,root} {if _,err:=pin.Stat();!errors.Is(err,os.ErrClosed) {t.Fatal("terminal evidence pin leaked",err)}}
  if job.recoveryFailed!=checkpointFailure {t.Fatal("checkpoint failure semantics changed")}
  assertPublicationBytes(t,final,"published bytes")
 })}
}

func TestEventPublicationLateCloseErrorClearsRetainedOutput(t *testing.T) {
 bm,job:=publicationCheckpointJob(t);temporary,final:=filepath.Join(bm.camera.RecordDir,"output.tmp"),filepath.Join(bm.camera.RecordDir,"output.mp4")
 if err:=os.WriteFile(temporary,[]byte("published bytes"),0600);err!=nil {t.Fatal(err)}
 var output *eventPublishedOutput
 err:=bm.publishEventRecordingWith(context.Background(),bm.ctx,job,temporary,final,false,func(ctx context.Context,lease *mediaMutationLease,from,to string)error{
  return publishRecordingDurablyObservedLeased(ctx,lease,from,to,func(context.Context,string)error{return nil},outputEvidenceOps(),func(file,dir *os.File)error{
   var err error;output,err=retainEventPublishedOutput(ctx,lease,to,file,dir);if err!=nil {return err}
   job.recoveryBorrow.output=output
   return file.Close() // inject a deferred-close error after evidence handoff
  })
 })
 if !errors.Is(err,os.ErrClosed) || output==nil || job.recoveryBorrow!=nil || !job.keepStaging {t.Fatal("late publication failure not retained/cleared",err)}
 if output.file!=nil || output.directory!=nil {t.Fatal("failed publication retained output descriptors")}
 assertPublicationBytes(t,final,"published bytes")
 if m:=readRecoveryTest(t,job.dir);m.State!="pinned" {t.Fatal("failed publication claimed published checkpoint",m)}
}

func TestPublishedOutputRequiresHeldLeaseAndLiveContext(t *testing.T) {
 output,lease,_:=publishedOutputFixture(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=output.check(ctx,lease);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 for _,invalid:=range []*mediaMutationLease{nil,foreign} {if err:=output.check(context.Background(),invalid);err==nil {t.Fatal("invalid lease accepted")}}
 if !lease.held() || !foreign.held() {t.Fatal("evidence inspection disturbed lease owner")}
}

func TestPublishedOutputObserverPinsDestinationAcrossDirectories(t *testing.T) {
 lease:=publishedEngineLease(t);parent:=t.TempDir();fromDir,toDir:=filepath.Join(parent,"source"),filepath.Join(parent,"destination")
 for _,dir:=range []string{fromDir,toDir} {if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}}
 temporary,final:=filepath.Join(fromDir,"capture.tmp"),filepath.Join(toDir,"published.mp4")
 if err:=os.WriteFile(temporary,[]byte("cross-directory bytes"),0600);err!=nil {t.Fatal(err)}
 ops:=outputEvidenceOps();syncs:=0;ops.syncDirectory=func(f *os.File)error{syncs++;return f.Sync()}
 var output *eventPublishedOutput
 err:=publishRecordingDurablyObservedLeased(context.Background(),lease,temporary,final,func(context.Context,string)error{return nil},ops,func(file,dir *os.File)error{
  var err error;output,err=retainEventPublishedOutput(context.Background(),lease,final,file,dir);return err
 })
 if output!=nil {defer output.Close()}
 if err!=nil || output==nil || syncs!=2 {t.Fatal("destination provenance or parent syncs missing",err,syncs)}
 if err:=output.check(context.Background(),lease);err!=nil {t.Fatal(err)}
 assertPublicationBytes(t,final,"cross-directory bytes")
}
