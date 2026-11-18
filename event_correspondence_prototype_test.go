package main

// Test-only exploration. No production caller, receipt, or cleanup capability.
import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "time"

 "golang.org/x/sys/unix"
)

const corrPrototypeStreamLimit int64 = 32 << 20
const corrPrototypeInputLimit int64 = 128 << 20
const corrPrototypeTotalInputLimit int64 = 512 << 20
const corrPrototypeBudget = 20*time.Second
var errCorrPrototype = errors.New("offline correspondence prototype refused")

type corrPrototypeReport struct {
 ProcessesStarted int
 LastChildPID int // test-only join verification; never a durable identity
 NormalizedBytes int64
 ComparedBytes int64
 InputSizePassBudget int64 // logical sizes times passes, NOT measured device I/O
 BorrowedMediaDescriptors int
 PeakScratchDescriptors int
 Elapsed time.Duration
 VideoEqual, AudioEqual bool
}

type corrPrototypeSink struct {
 ctx context.Context
 writer io.Writer
 written int64
}
func (w *corrPrototypeSink) Write(p []byte)(int,error) {
 if err:=w.ctx.Err();err!=nil {return 0,err}
 if int64(len(p))>corrPrototypeStreamLimit-w.written {return 0,errCorrPrototype}
 n,err:=w.writer.Write(p);w.written+=int64(n)
 return n,err
}

func corrPrototypeEqual(ctx context.Context,a,b *os.File)(bool,int64,error) {
 if err:=ctx.Err();err!=nil {return false,0,err}
 sa,err:=a.Stat();if err!=nil {return false,0,err}
 sb,err:=b.Stat();if err!=nil {return false,0,err}
 if !sa.Mode().IsRegular() || !sb.Mode().IsRegular() || sa.Size()<=0 || sb.Size()<=0 || sa.Size()>corrPrototypeStreamLimit || sb.Size()>corrPrototypeStreamLimit {return false,0,errCorrPrototype}
 if sa.Size()!=sb.Size() {return false,0,nil}
 x,y:=make([]byte,64<<10),make([]byte,64<<10)
 var compared int64
 for offset:=int64(0);offset<sa.Size(); {
  if err:=ctx.Err();err!=nil {return false,compared,err}
  count:=int64(len(x));if remaining:=sa.Size()-offset;remaining<count {count=remaining}
  n,ea:=a.ReadAt(x[:count],offset);m,eb:=b.ReadAt(y[:count],offset)
  if ea!=nil || eb!=nil || int64(n)!=count || int64(m)!=count {return false,compared,errors.Join(errCorrPrototype,ea,eb)}
  compared+=int64(n+m)
  if !bytes.Equal(x[:n],y[:m]) {return false,compared,nil}
  offset+=count
 }
 return true,compared,nil
}

func corrPrototypeArguments(reference bool,files int,audio bool)([]string,string,error) {
 if files<1 || files>maxRecoveryInputs+1 {return nil,"",errCorrPrototype}
 args:=[]string{"-hide_banner","-loglevel","error","-nostdin","-protocol_whitelist","file,pipe"}
 var plan string
 if reference {
  args=append(args,"-f","concat","-safe","0","-i","pipe:0")
  for i:=0;i<files;i++ {plan+=fmt.Sprintf("file 'file:/proc/self/fd/%d'\n",i+3)}
 } else {
  if files!=1 {return nil,"",errCorrPrototype}
  args=append(args,"-f","mov","-i","/proc/self/fd/3")
 }
 if audio {
  // Disposable content sink ONLY. Never recording or timing evidence.
  args=append(args,"-map","0:a:0","-c:a","copy","-bsf:a","setts=pts=N:dts=N","-f","adts","pipe:1")
 } else {
  args=append(args,"-map","0:v:0","-c:v","copy","-bsf:v","h264_mp4toannexb","-f","h264","pipe:1")
 }
 return args,plan,nil
}

func corrPrototypeNormalize(ctx context.Context,files []*os.File,reference,audio bool,dst *os.File,report *corrPrototypeReport) error {
 if err:=ctx.Err();err!=nil {return err}
 if report.ProcessesStarted>=4 {return errCorrPrototype}
 args,plan,err:=corrPrototypeArguments(reference,len(files),audio);if err!=nil {return err}
 for _,file:=range files {
  if file==nil {return errCorrPrototype}
  info,err:=file.Stat();if err!=nil {return err}
  if !info.Mode().IsRegular() {return errCorrPrototype}
 }
 cmd:=exec.CommandContext(ctx,"ffmpeg",args...)
 cmd.ExtraFiles=files;cmd.WaitDelay=2*time.Second
 if reference {cmd.Stdin=bytes.NewBufferString(plan)}
 sink:=&corrPrototypeSink{ctx:ctx,writer:dst}
 diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes}
 cmd.Stdout=sink;cmd.Stderr=diagnostic
 if err:=cmd.Start();err!=nil {return err}
 report.ProcessesStarted++;report.LastChildPID=cmd.Process.Pid
 err=cmd.Wait()
 report.NormalizedBytes+=sink.written
 if err!=nil || ctx.Err()!=nil || diagnostic.refused || diagnostic.Len()!=0 || sink.written==0 {return errors.Join(errCorrPrototype,err,ctx.Err())}
 return nil
}

type corrPrototypeNormalizer func(context.Context,[]*os.File,bool,bool,*os.File,*corrPrototypeReport)error

// Test-only synchronous adapter. Does not change the production inspection path.
func corrPrototypeInspect(parent context.Context,lease *mediaMutationLease,e *eventDurationEvidence,scratch string)(corrPrototypeReport,error) {
 return corrPrototypeInspectWithNormalizer(parent,lease,e,scratch,corrPrototypeNormalize)
}

// The seam injects synchronous faults, not proof of media equivalence.
func corrPrototypeInspectWithNormalizer(parent context.Context,lease *mediaMutationLease,e *eventDurationEvidence,scratch string,normalize corrPrototypeNormalizer)(report corrPrototypeReport,result error) {
 started:=time.Now();defer func(){report.Elapsed=time.Since(started)}()
 ctx,cancel:=context.WithTimeout(parent,corrPrototypeBudget);defer cancel()
 if normalize==nil {return report,errCorrPrototype}
 if err:=e.bound(ctx,lease);err!=nil {return report,err}
 selection:=e.execution.selection
 if len(selection.inputs)>maxRecoveryInputs {return report,errCorrPrototype}
 files:=make([]*os.File,0,len(selection.inputs)+2)
 for _,input:=range selection.inputs {files=append(files,input.file)}
 files=append(files,e.capture,e.output.file)
 snapshots:=make([]unix.Stat_t,len(files))
 for i,file:=range files {
  snapshot,err:=eventPresentationDescriptorStat(ctx,lease,file);if err!=nil {return report,err}
  if snapshot.Size>corrPrototypeInputLimit {return report,errCorrPrototype}
  snapshots[i]=snapshot;report.InputSizePassBudget+=2*snapshot.Size
 }
 if report.InputSizePassBudget>corrPrototypeTotalInputLimit {return report,errCorrPrototype}
 report.BorrowedMediaDescriptors=len(files)
 check:=func()error {
  if err:=e.bound(ctx,lease);err!=nil {return err}
  if err:=e.bm.ctx.Err();err!=nil {return err}
  if err:=e.inventory.Preflight(ctx,lease);err!=nil {return err}
  if err:=e.output.check(ctx,lease);err!=nil {return err}
  if err:=e.execution.concat.check(ctx,lease,e.execution,e.bm,e.job);err!=nil {return err}
  for i,file:=range files {
   current,err:=eventPresentationDescriptorStat(ctx,lease,file);if err!=nil {return err}
   if !eventPresentationDescriptorUnchanged(snapshots[i],current) {return errCorrPrototype}
  }
  return ctx.Err()
 }
 if err:=check();err!=nil {return report,err}
 for _,audio:=range []bool{false,true} {
  equal,err:=func()(equal bool,result error) {
   a,err:=os.CreateTemp(scratch,"corr-reference-*");if err!=nil {return false,err}
   defer func(){result=errors.Join(result,a.Close(),os.Remove(a.Name()))}()
   b,err:=os.CreateTemp(scratch,"corr-output-*");if err!=nil {return false,err}
   defer func(){result=errors.Join(result,b.Close(),os.Remove(b.Name()))}()
   report.PeakScratchDescriptors=2
   if err:=normalize(ctx,files[:len(files)-1],true,audio,a,&report);err!=nil {return false,err}
   if err:=check();err!=nil {return false,err}
   if err:=normalize(ctx,files[len(files)-1:],false,audio,b,&report);err!=nil {return false,err}
   if err:=check();err!=nil {return false,err}
   equal,count,err:=corrPrototypeEqual(ctx,a,b);report.ComparedBytes+=count
   return equal,err
  }()
  if err!=nil {return report,err}
  if audio {report.AudioEqual=equal} else {report.VideoEqual=equal}
  if !equal {return report,errCorrPrototype}
 }
 return report,check()
}
