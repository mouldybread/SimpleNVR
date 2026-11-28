package main

// Experimental test-only streaming comparator. No production caller or receipt.
import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "sync"
 "time"

 "golang.org/x/sys/unix"
)

var errCorrStreamLimit=errors.New("stream comparison byte ceiling")
var errCorrStreamMismatch=errors.New("stream content mismatch")
var errCorrStreamEmpty=errors.New("empty normalized stream")

type corrStreamPair struct {
 Equal bool
 ReaderWorkersStarted,ReaderWorkersJoined int
 ReferenceBytes,ActualBytes int64
}
type corrStreamReport struct {
 ChildrenStarted,ChildrenJoined,PeakUnjoinedChildren int
 ChildPIDs []int
 PipeDescriptorsCreated int
 ComparatorWorkersStarted,ComparatorWorkersJoined int
 WatcherStarted,WatcherJoined bool
 ownedPipes []*os.File // closed-object trace for tests; omitted from JSON
 BorrowedMediaDescriptors int
 LogicalInputPassBytes int64
 NormalizedScratchBytes int64 // always zero: no normalized scratch files exist
 Elapsed time.Duration
 Video,Audio corrStreamPair
}
type corrStreamCompletion struct {audio bool;pair corrStreamPair;err error}
type corrStreamFactory func(context.Context,[]string)*exec.Cmd

// One independent worker per stream; a blocked read is interrupted by closing
// owned pipes on context cancellation. No donor descriptors are ever closed.
func corrStreamCompare(parent context.Context,a,b io.Reader)(pair corrStreamPair,result error) {
 ctx,cancel:=context.WithCancel(parent)
 x,y:=make([]byte,64<<10),make([]byte,64<<10)
 type chunk struct{n int;err error}
 request:=[]chan int64{make(chan int64),make(chan int64)}
 response:=[]chan chunk{make(chan chunk),make(chan chunk)}
 readers:=[]io.Reader{a,b};buffers:=[][]byte{x,y}
 var joined sync.WaitGroup
 var totals [2]int64
 defer func(){
  cancel()
  // These are disposable normalized-pipe readers, NEVER media donors.
  for _,reader:=range readers {if closer,ok:=reader.(io.Closer);ok {_=closer.Close()}}
  joined.Wait();pair.ReaderWorkersJoined=pair.ReaderWorkersStarted
  pair.ReferenceBytes=totals[0];pair.ActualBytes=totals[1]
  if totals[0]>corrPrototypeStreamLimit || totals[1]>corrPrototypeStreamLimit {pair.Equal=false;result=errors.Join(result,errCorrStreamLimit)}
 }()
 for side:=0;side<2;side++ {
  side:=side;joined.Add(1);pair.ReaderWorkersStarted++
  go func(){defer joined.Done();for {
   var count int64
   select {case count=<-request[side]:case <-ctx.Done():return}
   n,err:=io.ReadFull(readers[side],buffers[side][:count]);totals[side]+=int64(n)
   select {case response[side]<-chunk{n,err}:case <-ctx.Done():return}
  }}()
 }
 for {
  if err:=ctx.Err();err!=nil {return pair,err}
  count:=int64(len(x))
  if remaining:=corrPrototypeStreamLimit-pair.ReferenceBytes+1;remaining<count {count=remaining}
  if remaining:=corrPrototypeStreamLimit-pair.ActualBytes+1;remaining<count {count=remaining}
  // Both sides must read concurrently. Sequential full-chunk reads can stall
  // a valid pair when one child fills its OTHER stream's pipe first.
  for side:=0;side<2;side++ {select {case request[side]<-count:case <-ctx.Done():return pair,ctx.Err()}}
  chunks:=[2]chunk{}
  for side:=0;side<2;side++ {select {case chunks[side]=<-response[side]:case <-ctx.Done():return pair,ctx.Err()}}
  n,ea:=chunks[0].n,chunks[0].err;m,eb:=chunks[1].n,chunks[1].err
  pair.ReferenceBytes+=int64(n);pair.ActualBytes+=int64(m)
  if pair.ReferenceBytes>corrPrototypeStreamLimit || pair.ActualBytes>corrPrototypeStreamLimit {return pair,errCorrStreamLimit}
  terminal:=func(err error)bool{return err==io.EOF || err==io.ErrUnexpectedEOF}
  if ea!=nil && !terminal(ea) {return pair,ea}
  if eb!=nil && !terminal(eb) {return pair,eb}
  if n!=m || !bytes.Equal(x[:n],y[:m]) {return pair,errCorrStreamMismatch}
  if terminal(ea) || terminal(eb) {
   if !terminal(ea) || !terminal(eb) {return pair,errCorrStreamMismatch}
   if pair.ReferenceBytes==0 {return pair,errCorrStreamEmpty}
   pair.Equal=true;return pair,nil
  }
 }
}

func corrStreamArguments(reference bool,files int)([]string,string,error) {
 args,plan,err:=corrPrototypeArguments(reference,files,false)
 if err!=nil {return nil,"",err}
 // Reuse the exact baseline video extraction and input plan; append audio as
 // a second disposable output, using one extra inherited pipe descriptor.
 args=append(args,"-map","0:a:0","-c:a","copy","-bsf:a","setts=pts=N:dts=N","-f","adts",fmt.Sprintf("pipe:%d",3+files))
 return args,plan,nil
}

// hook boundaries: 0 before initial validation, 1/2 after each child starts,
// 3/4 after each comparator result (stream order is intentionally unspecified),
// 5 after both children join. Hooks run synchronously on the owner goroutine.
func corrStreamRun(parent context.Context,refs []*os.File,actual *os.File,factory corrStreamFactory,hook func(int),guard func(context.Context)error)(report corrStreamReport,result error) {
 started:=time.Now()
 ctx,cancel:=context.WithTimeout(parent,corrPrototypeBudget)
 var pipes []*os.File
 var commands []*exec.Cmd
 var waited []bool
 var workers sync.WaitGroup
 var watcherDone chan struct{}
 var results chan corrStreamCompletion
 received:=0
 closePipes:=func(){for _,file:=range pipes {_=file.Close()}}
 defer func(){
  if ctx.Err()==context.DeadlineExceeded {result=errors.Join(result,ctx.Err())}
  cancel();closePipes()
  for i,cmd:=range commands {if !waited[i] {result=errors.Join(result,cmd.Wait());waited[i]=true;report.ChildrenJoined++}}
  workers.Wait();report.ComparatorWorkersJoined=report.ComparatorWorkersStarted
  for results!=nil && received<report.ComparatorWorkersStarted {
   item:=<-results;received++;result=errors.Join(result,item.err)
   if item.audio {report.Audio=item.pair} else {report.Video=item.pair}
  }
  if watcherDone!=nil {<-watcherDone;report.WatcherJoined=true}
  report.Elapsed=time.Since(started)
 }()
 if factory==nil {factory=func(ctx context.Context,args []string)*exec.Cmd{return exec.CommandContext(ctx,"ffmpeg",args...)}}
 if len(refs)<1 || len(refs)>maxRecoveryInputs+1 || actual==nil {return report,errCorrPrototype}
 donors:=append(append([]*os.File(nil),refs...),actual)
 snapshots:=make([]unix.Stat_t,len(donors))
 for i,file:=range donors {
  if err:=ctx.Err();err!=nil {return report,err}
  if file==nil {return report,errCorrPrototype}
  if err:=unix.Fstat(int(file.Fd()),&snapshots[i]);err!=nil {return report,err}
  s:=snapshots[i]
  if s.Mode&unix.S_IFMT!=unix.S_IFREG || s.Size<=0 || s.Size>corrPrototypeInputLimit || s.Nlink==0 {return report,errCorrPrototype}
  report.LogicalInputPassBytes+=s.Size
 }
 // Preserve baseline admission conservatively, although this version performs
 // one logical input pass instead of two. No larger-input allowance is added.
 if report.LogicalInputPassBytes>corrPrototypeTotalInputLimit/2 {return report,errCorrPrototype}
 report.BorrowedMediaDescriptors=len(donors)
 check:=func(boundary int)error{
  if hook!=nil {hook(boundary)}
  if err:=ctx.Err();err!=nil {return err}
  if guard!=nil {if err:=guard(ctx);err!=nil {return err}}
  for i,file:=range donors {
   var current unix.Stat_t
   if err:=unix.Fstat(int(file.Fd()),&current);err!=nil {return err}
   if !eventPresentationDescriptorUnchanged(snapshots[i],current) {return errCorrPrototype}
  }
  return nil
 }
 if err:=check(0);err!=nil {return report,err}
 var readers,writers [2][2]*os.File // side(reference/actual), stream(video/audio)
 for side:=0;side<2;side++ {for kind:=0;kind<2;kind++ {
  r,w,err:=os.Pipe();if err!=nil {return report,err}
  readers[side][kind]=r;writers[side][kind]=w;pipes=append(pipes,r,w)
  report.ownedPipes=append(report.ownedPipes,r,w);report.PipeDescriptorsCreated=len(pipes)
 }}
 report.PipeDescriptorsCreated=len(pipes)
 // No pipe list mutations occur after the watcher starts.
 watcherDone=make(chan struct{});report.WatcherStarted=true
 go func(){defer close(watcherDone);<-ctx.Done();closePipes()}()
 diagnostics:=[]*eventPresentationProbeBuffer{}
 for side:=0;side<2;side++ {
  files:=refs;if side==1 {files=[]*os.File{actual}}
  args,plan,err:=corrStreamArguments(side==0,len(files));if err!=nil {return report,err}
  cmd:=factory(ctx,args);if cmd==nil {return report,errCorrPrototype}
  cmd.ExtraFiles=append(append([]*os.File(nil),files...),writers[side][1])
  cmd.Stdout=writers[side][0];cmd.WaitDelay=2*time.Second
  diagnostic:=&eventPresentationProbeBuffer{limit:eventPresentationProbeDiagnosticBytes};cmd.Stderr=diagnostic
  if side==0 {cmd.Stdin=bytes.NewBufferString(plan)}
  if err:=cmd.Start();err!=nil {return report,err}
  commands=append(commands,cmd);waited=append(waited,false);diagnostics=append(diagnostics,diagnostic)
  report.ChildrenStarted++;report.ChildPIDs=append(report.ChildPIDs,cmd.Process.Pid)
  report.PeakUnjoinedChildren=report.ChildrenStarted
  // Parent writer copies must close immediately, otherwise EOF can never be
  // observed after the child exits. Child inherited copies remain independent.
  _=writers[side][0].Close();_=writers[side][1].Close()
  if err:=check(side+1);err!=nil {return report,err}
 }
 results=make(chan corrStreamCompletion,2)
 for kind:=0;kind<2;kind++ {
  kind:=kind;workers.Add(1);report.ComparatorWorkersStarted++
  go func(){defer workers.Done();pair,err:=corrStreamCompare(ctx,readers[0][kind],readers[1][kind]);results<-corrStreamCompletion{kind==1,pair,err}}()
 }
 for count:=0;count<2;count++ {
  item:=<-results;received++
  if item.audio {report.Audio=item.pair} else {report.Video=item.pair}
  if item.err!=nil {return report,errors.Join(item.err,ctx.Err(),parent.Err())}
  if err:=check(3+count);err!=nil {return report,err}
 }
 for i,cmd:=range commands {
  err:=cmd.Wait();waited[i]=true;report.ChildrenJoined++
  if err!=nil {return report,errors.Join(errCorrPrototype,err,parent.Err())}
  if diagnostics[i].refused || diagnostics[i].Len()!=0 {return report,errCorrPrototype}
 }
 return report,check(5)
}

func corrStreamInspect(parent context.Context,lease *mediaMutationLease,e *eventDurationEvidence,factory corrStreamFactory,hook func(int))(corrStreamReport,error) {
 if err:=e.bound(parent,lease);err!=nil {return corrStreamReport{},err}
 refs:=make([]*os.File,0,len(e.execution.selection.inputs)+1)
 for _,input:=range e.execution.selection.inputs {refs=append(refs,input.file)}
 refs=append(refs,e.capture)
 guard:=func(ctx context.Context)error{
  if err:=e.bound(ctx,lease);err!=nil {return err}
  if err:=e.bm.ctx.Err();err!=nil {return err}
  if err:=e.inventory.Preflight(ctx,lease);err!=nil {return err}
  if err:=e.output.check(ctx,lease);err!=nil {return err}
  return e.execution.concat.check(ctx,lease,e.execution,e.bm,e.job)
 }
 return corrStreamRun(parent,refs,e.output.file,factory,hook,guard)
}
