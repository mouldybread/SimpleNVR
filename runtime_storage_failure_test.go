package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "syscall"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func runtimeStorageMedia(t *testing.T) string {
 t.Helper();requireFFmpeg(t)
 source:=filepath.Join(t.TempDir(),"runtime-source.mp4")
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=256x144:rate=10","-filter_threads","1","-vf","noise=alls=100:allf=t+u","-t","2","-c:v","mpeg4","-q:v","2","-threads","1","-an","-y",source)
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("runtime storage media: %v %s",err,output)}
 return source
}

// Recheck the bounded mount before filling; the caller has already observed an
// actual FFmpeg executable and nonempty output on this filesystem.
func exhaustActiveStorage(t *testing.T,root string) {
 t.Helper()
 var fs unix.Statfs_t
 if err:=unix.Statfs(root,&fs);err!=nil || fs.Type!=unix.TMPFS_MAGIC || uint64(fs.Blocks)*uint64(fs.Bsize)>8<<20 {t.Fatal("refusing to fill unexpected filesystem",err)}
 witness,err:=os.OpenFile(filepath.Join(root,"write-witness.tmp"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil {t.Fatal(err)}
 defer witness.Close()
 fill,err:=os.OpenFile(filepath.Join(root,"runtime-fill.bin"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil {t.Fatal(err)}
 chunk:=make([]byte,64<<10);total:=0;var full error
 for total<8<<20 {n,err:=fill.Write(chunk);total+=n;if err!=nil {full=err;break};if n==0 {break}}
 closeErr:=fill.Close()
 if !errors.Is(full,unix.ENOSPC) || closeErr!=nil {t.Fatal("runtime byte exhaustion not reached",full,closeErr,total)}
 if n,err:=witness.Write([]byte("runtime write witness"));n!=0 || !errors.Is(err,unix.ENOSPC) {t.Fatal("full filesystem did not reject an already-open writer",n,err)}
 capacity,err:=readRecordingStorageCapacity(root)
 if err!=nil || capacity.AvailableBytes!=0 {t.Fatal("runtime filesystem is not byte-full",capacity,err)}
}

func TestStorageFailureMountNamespaceRuntimeWrites(t *testing.T) {
 for _,mode:=range []string{"continuous","capture","assembly"} {t.Run(mode,func(t *testing.T){
  f:=mountedStorageFailureFixture(t,4<<20,256)
  media:=runtimeStorageMedia(t)
  original,err:=os.ReadFile(media);if err!=nil {t.Fatal(err)}
  pidsDir,real:=installActiveRecorderFixture(t,media)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  cam:=&Camera{ID:"runtime",Name:"Runtime",RecordDir:f.root,RTSPUrl:"rtsp://simplenvr-fixture"}
  bm:=newBufferManager(ctx,cam);defer bm.cancel()
  var job *bufferEvent;var checkpoint []byte
  outputPattern:=filepath.Join(f.root,"*.mp4.tmp")
  if mode!="continuous" {
   seed:=filepath.Join(f.root,"seed.mp4")
   if err:=os.WriteFile(seed,original,0600);err!=nil {t.Fatal(err)}
   now:=time.Now().UTC();bm.registerCompleted(seed,now.Add(-2*time.Second),now)
   job,err=bm.pinBuffer(bufferTestEvent("runtime-"+mode));if err!=nil {t.Fatal(err)}
   bm.slots<-struct{}{}
   if mode=="capture" {outputPattern=filepath.Join(job.dir,"event.mp4")} else {
    if err:=os.WriteFile(filepath.Join(job.dir,"event.mp4"),original,0600);err!=nil {t.Fatal(err)}
    t.Setenv("SIMPLENVR_ASSEMBLY_FIXTURE","1")
   }
  }
  done:=make(chan error,1);joined:=false
  defer func(){cancel();if !joined {select {case <-done:case <-time.After(25*time.Second):t.Error("runtime storage process cleanup did not join")}}}()
  go func(){
   switch mode {
   case "continuous":done<-recordSegment(ctx,cam,60)
   case "capture":if output:=bm.recordEventDuration(job);output!="" {done<-errors.New("full-storage capture reported success")} else {done<-nil}
   case "assembly":if output:=bm.concatenateEventRecording(job,filepath.Join(job.dir,"event.mp4"));output!="" {done<-errors.New("full-storage assembly passed failed checkpoint")} else {done<-nil}
   }
  }()
  awaitActiveRecorder(t,"recorder never wrote before storage exhaustion",func()bool{return activeRecorderWriting(outputPattern)})
  pids:=activeRecorderPIDs(t,pidsDir,real,1)
  if job!=nil {checkpoint,err=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}}
  exhaustActiveStorage(t,f.root)
  select {
  case err:=<-done:
   joined=true
   if mode=="continuous" && err==nil {t.Fatal("continuous capture reported success after exhaustion")}
   if mode!="continuous" && err!=nil {t.Fatal(err)}
  case <-time.After(15*time.Second):t.Fatal("runtime write failure did not terminate processing without cancellation")
  }
  if ctx.Err()!=nil || bm.ctx.Err()!=nil {t.Fatal("runtime failure confused with context cancellation")}
  for _,pid:=range pids {if _,err:=os.Stat(fmt.Sprintf("/proc/%d",pid));!os.IsNotExist(err) {t.Fatal("failed recorder child not reaped",err)}}
  if job!=nil {
   if !job.keepStaging || !job.recoveryFailed {t.Fatal("full-storage checkpoint failure did not latch preservation")}
   bm.releaseJob(job)
   if len(bm.slots)!=0 {t.Fatal("failed job leaked capacity")}
   data,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil || !bytes.Equal(data,checkpoint) {t.Fatal("last complete checkpoint lost",err)}
   data,err=os.ReadFile(job.preFiles[0]);if err!=nil || !bytes.Equal(data,original) {t.Fatal("pinned input changed",err)}
   if mode=="capture" {
    if info,err:=os.Stat(filepath.Join(job.dir,"event.mp4"));err!=nil || info.Size()<=1024 {t.Fatal("failed runtime capture bytes lost",err)}
   } else {
    data,err=os.ReadFile(filepath.Join(job.dir,"event.mp4"));if err!=nil || !bytes.Equal(data,original) {t.Fatal("original assembly capture changed",err)}
    if _,err:=os.Stat(filepath.Join(job.dir,"concat.txt"));err!=nil {t.Fatal("assembly concat evidence lost",err)}
   }
  }
  if mode!="capture" {
   // A failed output may remain unpublished or be minimally readable/published.
   // Neither outcome permits losing all observed nonempty output evidence.
   retained:=false
   for _,pattern:=range []string{filepath.Join(f.root,"*.mp4.tmp"),filepath.Join(f.root,"*_partial.mp4")} {if activeRecorderWriting(pattern) {retained=true}}
   if !retained {t.Fatal("nonempty failed runtime output lost")}
  }
  data,err:=os.ReadFile(media);if err!=nil || !bytes.Equal(data,original) {t.Fatal("healthy source changed",err)}
  f.assertPreserved(t)
 })}
}

func TestStorageFailureMountNamespaceUnprivilegedPermissions(t *testing.T) {
 f:=mountedStorageFailureFixture(t,1<<20,128)
 // Only fixture-owned paths change mode, not ancestors or deployment paths.
 if err:=filepath.WalkDir(f.root,func(path string,entry os.DirEntry,err error)error{
  if err!=nil {return err};mode:=os.FileMode(0644);if entry.IsDir() {mode=0555};return os.Chmod(path,mode)
 });err!=nil {t.Fatal(err)}
 dir,err:=os.MkdirTemp("","simplenvr-permission-helper-*");if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){if err:=os.RemoveAll(dir);err!=nil {t.Error(err)}})
 if err:=os.Chmod(dir,0755);err!=nil {t.Fatal(err)}
 executable:=filepath.Join(dir,"permission-test")
 input,err:=os.Open(os.Args[0]);if err!=nil {t.Fatal(err)}
 output,err:=os.OpenFile(executable,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0755);if err!=nil {input.Close();t.Fatal(err)}
 _,copyErr:=io.Copy(output,input)
 if err:=errors.Join(copyErr,input.Close(),output.Close());err!=nil {t.Fatal(err)}
 root,err:=os.Open(f.root);if err!=nil {t.Fatal(err)};defer root.Close()
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,executable,"-test.run=^TestRuntimeStoragePermissionHelper$","-test.v","-test.timeout=15s")
 cmd.Dir=dir;cmd.ExtraFiles=[]*os.File{root}
 cmd.Env=append(os.Environ(),"SIMPLENVR_PERMISSION_HELPER=1")
 cmd.SysProcAttr=&syscall.SysProcAttr{Credential:&syscall.Credential{Uid:65534,Gid:65534,Groups:[]uint32{}}}
 result,err:=cmd.CombinedOutput()
 if err!=nil || !bytes.Contains(result,[]byte("UNPRIVILEGED_STORAGE_FAILURES_VERIFIED")) {t.Fatalf("non-root storage helper failed: %v %s",err,result)}
 f.assertPreserved(t)
 if _,err:=os.Lstat(filepath.Join(f.root,"permission-output.mp4"));!os.IsNotExist(err) {t.Fatal("denied publication created output",err)}
}

func TestRuntimeStoragePermissionHelper(t *testing.T) {
 if os.Getenv("SIMPLENVR_PERMISSION_HELPER")!="1" {return}
 groups,err:=os.Getgroups()
 if err!=nil || os.Geteuid()!=65534 || os.Getegid()!=65534 || len(groups)!=0 {t.Fatal("helper did not drop credentials",err)}
 // The inherited fixture descriptor avoids opening inaccessible ancestor paths;
 // normal directory write permission is still checked for relative operations.
 if err:=os.Chdir("/proc/self/fd/3");err!=nil {t.Fatal(err)}
 var fs unix.Statfs_t
 if err:=unix.Statfs(".",&fs);err!=nil || fs.Type!=unix.TMPFS_MAGIC {t.Fatal("unexpected permission fixture",err)}
 if file,err:=os.OpenFile("permission-witness.tmp",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);!errors.Is(err,unix.EACCES) {if file!=nil {file.Close()};t.Fatal("expected actual non-root EACCES",err)}
 stage:=filepath.Join("temp","event-evidence")
 manifest,err:=os.ReadFile(filepath.Join(stage,"recovery.json"));if err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(stage,bytes.Replace(manifest,[]byte(`"revision":2`),[]byte(`"revision":3`),1));!errors.Is(err,unix.EACCES) {t.Fatal("manifest permission error ignored",err)}
 n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
 if err:=n.saveAt("cameras.json");!errors.Is(err,unix.EACCES) {t.Fatal("configuration permission error ignored",err)}
 if err:=publishRecordingWithOps(context.Background(),"capture.mp4.tmp","permission-output.mp4",acceptPublicationFixture,renameRecordingNoReplace);!errors.Is(err,unix.EACCES) {t.Fatal("publication permission error ignored",err)}
 fmt.Println("UNPRIVILEGED_STORAGE_FAILURES_VERIFIED")
}
