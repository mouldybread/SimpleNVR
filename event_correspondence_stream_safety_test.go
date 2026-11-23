package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strconv"
 "strings"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

// Re-exec this test binary as a controlled child: no shell, grandchildren,
// mutable global runner, network access or wall-clock sleeps on success paths.
func TestCorrStreamChild(t *testing.T) {
 if os.Getenv("CORR_STREAM_CHILD")!="1" {return}
 fd,err:=strconv.Atoi(os.Getenv("CORR_STREAM_AUDIO_FD"));if err!=nil {os.Exit(91)}
 audio:=os.NewFile(uintptr(fd),"audio")
 mode:=os.Getenv("CORR_STREAM_MODE")
 // Opposed writes must exceed pipe capacity plus a comparator's 64 KiB
 // read chunk. The original 128 KiB payload can otherwise complete on
 // larger pipes. Constrain only these controlled child outputs, before
 // writing anything; never change the collector or borrowed media FDs.
 if mode=="cross video" || mode=="cross audio" {
  for _,pipe:=range []*os.File{os.Stdout,audio} {
   capacity,err:=unix.FcntlInt(pipe.Fd(),unix.F_SETPIPE_SZ,4096)
   if err!=nil || capacity<=0 || capacity>=64<<10 {
    _,_=fmt.Fprintf(os.Stderr,"opposed-order pipe setup failed: capacity=%d err=%v\n",capacity,err)
    os.Exit(97)
   }
  }
 }
 v,a:=[]byte("ABCD-video"),[]byte("ABCD-audio")
 switch mode {
 case "video mismatch":v=[]byte("CDAB-video")
 case "audio mismatch":a=[]byte("CDAB-audio")
 case "late mismatch":v=append(bytes.Repeat([]byte("x"),130000),'y')
 case "late reference":v=append(bytes.Repeat([]byte("x"),130000),'z')
 case "extra":v=append(v,'x')
 case "empty":os.Exit(0)
 case "stall":time.Sleep(10*time.Second);os.Exit(0)
 case "cross video":v=bytes.Repeat([]byte("x"),128<<10);a=bytes.Repeat([]byte("y"),128<<10)
 case "cross audio":
  if _,err:=audio.Write(bytes.Repeat([]byte("y"),128<<10));err!=nil {os.Exit(92)}
  if _,err:=os.Stdout.Write(bytes.Repeat([]byte("x"),128<<10));err!=nil {os.Exit(93)}
  os.Exit(0)
 case "oversize":
  chunk:=bytes.Repeat([]byte("x"),64<<10)
  for i:=0;i<513;i++ {if _,err:=os.Stdout.Write(chunk);err!=nil {os.Exit(94)}}
  // Stay alive, and keep the audio pipe open, until the parent refuses the stream and
  // kills this child. Exiting right after the last chunk closes the audio pipe, so the
  // audio comparator can report an empty stream before the video comparator reads the
  // byte past the ceiling, and the parent returns whichever comparator error arrives first.
  time.Sleep(10*time.Second);os.Exit(0)
 }
 if _,err:=os.Stdout.Write(v);err!=nil {os.Exit(95)}
 if _,err:=audio.Write(a);err!=nil {os.Exit(96)}
 if mode=="diagnostic" {_,_=os.Stderr.WriteString("diagnosed")}
 if mode=="diagnostic ceiling" {_,_=os.Stderr.Write(bytes.Repeat([]byte("d"),eventPresentationProbeDiagnosticBytes+1))}
 if mode=="nonzero" {os.Exit(7)}
 if mode=="late stall" {_=os.Stdout.Close();_=audio.Close();time.Sleep(10*time.Second)}
 os.Exit(0)
}

func corrStreamChildFactory(t *testing.T,modes []string,failSecond bool)corrStreamFactory {
 t.Helper();binary,err:=os.Executable();if err!=nil {t.Fatal(err)}
 calls:=0
 return func(ctx context.Context,args []string)*exec.Cmd{
  calls++
  if failSecond && calls==2 {return exec.CommandContext(ctx,filepath.Join(t.TempDir(),"absent"))}
  fd:=strings.TrimPrefix(args[len(args)-1],"pipe:")
  mode:=modes[0];if calls==2 {mode=modes[1]}
  cmd:=exec.CommandContext(ctx,binary,"-test.run=^TestCorrStreamChild$")
  cmd.Env=append(os.Environ(),"CORR_STREAM_CHILD=1","GORACE=atexit_sleep_ms=0","CORR_STREAM_MODE="+mode,"CORR_STREAM_AUDIO_FD="+fd)
  return cmd
 }
}

func corrStreamDonors(t *testing.T)([]*os.File,*os.File) {
 t.Helper();files:=[]*os.File{}
 for i:=0;i<2;i++ {
  file,err:=os.CreateTemp(t.TempDir(),"media");if err!=nil {t.Fatal(err)}
  if _,err:=file.WriteString("owned synthetic source");err!=nil {t.Fatal(err)}
  t.Cleanup(func(){_=file.Close()});files=append(files,file)
 }
 return files[:1],files[1]
}
func corrStreamAssertJoined(t *testing.T,r corrStreamReport) {
 t.Helper()
 if r.ChildrenStarted!=r.ChildrenJoined || r.ComparatorWorkersStarted!=r.ComparatorWorkersJoined || (r.WatcherStarted && !r.WatcherJoined) {t.Fatal("unjoined work",r)}
 if len(r.ownedPipes)!=r.PipeDescriptorsCreated {t.Fatal("pipe accounting",r)}
 for _,file:=range r.ownedPipes {if _,err:=file.Stat();err==nil {t.Fatal("owned pipe remains open")}}
 for _,pid:=range r.ChildPIDs {
  var status unix.WaitStatus
  if _,err:=unix.Wait4(pid,&status,unix.WNOHANG,nil);!errors.Is(err,unix.ECHILD) {t.Fatal("child not reaped",pid,err)}
 }
 if r.Video.ReaderWorkersStarted!=r.Video.ReaderWorkersJoined || r.Audio.ReaderWorkersStarted!=r.Audio.ReaderWorkersJoined {t.Fatal("unjoined reader workers",r)}
 if r.ChildrenStarted>2 || r.PeakUnjoinedChildren>2 || r.NormalizedScratchBytes!=0 {t.Fatal("streaming resource contract",r)}
}

func TestCorrStreamComparisonEdges(t *testing.T) {
 for _,kind:=range []string{"equal","empty","reorder","late difference","trailing bytes","cancelled"} {t.Run(kind,func(t *testing.T){
  a,b:=[]byte("ABCD"),[]byte("ABCD")
  switch kind {
  case "empty":a=nil;b=nil
  case "reorder":b=[]byte("CDAB")
  case "late difference":a=bytes.Repeat([]byte("x"),130001);b=append([]byte(nil),a...);b[len(b)-1]='z'
  case "trailing bytes":b=append(b,'x')
  }
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();if kind=="cancelled" {cancel()}
  pair,err:=corrStreamCompare(ctx,bytes.NewReader(a),bytes.NewReader(b))
  if (err==nil)!=(kind=="equal") || pair.Equal!=(kind=="equal") {t.Fatal(pair,err)}
 })}
 // The cap allows exactly the cap, and only one sentinel byte beyond it is
 // consumed to establish a limit violation. No unbounded per-frame collection.
 for _,extra:=range []int{0,1} {
  data:=bytes.Repeat([]byte("x"),int(corrPrototypeStreamLimit)+extra)
  pair,err:=corrStreamCompare(context.Background(),bytes.NewReader(data),bytes.NewReader(data))
  if extra==0 {if err!=nil || !pair.Equal {t.Fatal(pair,err)}} else if !errors.Is(err,errCorrStreamLimit) || pair.Equal || pair.ReferenceBytes>corrPrototypeStreamLimit+1 || pair.ActualBytes>corrPrototypeStreamLimit+1 {t.Fatal(pair,err)}
 }
}

func TestCorrStreamChildFaultsAndBackpressure(t *testing.T) {
 cases:=[]struct{name,left,right string;want error;failSecond bool}{
  {"equal","normal","normal",nil,false},
  {"video mismatch","normal","video mismatch",errCorrStreamMismatch,false},
  {"audio mismatch","normal","audio mismatch",errCorrStreamMismatch,false},
  {"late mismatch","late reference","late mismatch",errCorrStreamMismatch,false},
  {"trailing bytes","normal","extra",errCorrStreamMismatch,false},
  {"empty","empty","empty",errCorrStreamEmpty,false},
  {"diagnostic","normal","diagnostic",errCorrPrototype,false},
  {"diagnostic ceiling","normal","diagnostic ceiling",errCorrPrototype,false},
  {"nonzero after bytes","normal","nonzero",errCorrPrototype,false},
  {"both stalled","stall","stall",context.DeadlineExceeded,false},
  {"opposed pipe order","cross video","cross audio",context.DeadlineExceeded,false},
  {"stalled after EOF","late stall","late stall",context.DeadlineExceeded,false},
  {"stream ceiling","oversize","oversize",errCorrStreamLimit,false},
  {"partial launch","normal","normal",nil,true},
 }
 refs,actual:=corrStreamDonors(t)
 // Positive lifecycle control before injecting child faults.
 warm,err:=corrStreamRun(context.Background(),refs,actual,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil,nil)
 if err!=nil {t.Fatal(err)};corrStreamAssertJoined(t,warm)
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  budget:=5*time.Second;if tc.want==context.DeadlineExceeded {budget=400*time.Millisecond}
  ctx,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
  r,err:=corrStreamRun(ctx,refs,actual,corrStreamChildFactory(t,[]string{tc.left,tc.right},tc.failSecond),nil,nil)
  corrStreamAssertJoined(t,r)
  if tc.failSecond {if err==nil || r.ChildrenStarted!=1 {t.Fatal(r,err)}} else if tc.want==nil {if err!=nil || !r.Video.Equal || !r.Audio.Equal {t.Fatal(r,err)}} else if !errors.Is(err,tc.want) {t.Fatal(tc.name,r,err,tc.want)}
  for _,file:=range append(append([]*os.File(nil),refs...),actual) {if _,err:=file.Stat();err!=nil {t.Fatal("donor closed",err)}}
 })}
}

func TestCorrStreamLiveJobBoundaries(t *testing.T) {
 kinds:=[]string{"selected descriptor","selected bytes","capture bytes","output bytes","output binding","job request","inventory binding","borrowed lease","released lease","manager cancellation","caller cancellation"}
 for _,kind:=range kinds {for phase:=0;phase<=5;phase++ {t.Run(fmt.Sprintf("%s/%d",kind,phase),func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,true);e:=job.recoveryBorrow.duration
  duration,inventory:=job.event.Duration,job.inventory
  selected,output,borrowedLease:=job.execution.selection.inputs[0].file,e.output.file,job.recoveryBorrow.lease
  defer func(){
   job.event.Duration=duration;job.inventory=inventory
   job.execution.selection.inputs[0].file=selected;e.output.file=output;job.recoveryBorrow.lease=borrowedLease
   bm.releaseJob(job);assertJobInventoryClosed(t,inventory)
   if !job.keepStaging || job.recoveryBorrow!=nil || job.inventory!=nil || lease.held() || len(bm.slots)!=0 {t.Error("terminal invariant")}
  }()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  var preserved func()bool
  hook:=func(boundary int){if boundary!=phase {return}
   switch kind {
   case "selected descriptor":job.execution.selection.inputs[0].file=e.capture
   case "selected bytes","capture bytes","output bytes":
    path:=job.preFiles[0];if kind=="capture bytes" {path=filepath.Join(job.dir,"event.mp4")};if kind=="output bytes" {path=e.output.path}
    f,err:=os.OpenFile(path,os.O_WRONLY|os.O_APPEND,0);if err!=nil {t.Fatal(err)}
    _,err=f.WriteString("injected");closeErr:=f.Close();if err!=nil || closeErr!=nil {t.Fatal(err,closeErr)}
   case "output binding":e.output.file=e.capture
   case "job request":job.event.Duration++
   case "inventory binding":job.inventory=nil
   case "borrowed lease":job.recoveryBorrow.lease=nil
   case "released lease":lease.release()
   case "manager cancellation":bm.cancel()
   case "caller cancellation":cancel()
   }
   tree:=eventStorageTree(t,job.dir);published,err:=os.ReadFile(e.output.path);if err!=nil {t.Fatal(err)}
   preserved=func()bool{now,err:=os.ReadFile(e.output.path);return err==nil && bytes.Equal(now,published) && reflect.DeepEqual(tree,eventStorageTree(t,job.dir))}
  }
  r,err:=corrStreamInspect(ctx,lease,e,corrStreamChildFactory(t,[]string{"normal","normal"},false),hook)
  corrStreamAssertJoined(t,r)
  wantChildren:=2;if phase<2 {wantChildren=phase}
  if err==nil || r.ChildrenStarted!=wantChildren || preserved==nil || !preserved() {t.Fatal("live mutation accepted or evidence changed",r,err)}
  if kind!="released lease" && !lease.held() {t.Fatal("borrowed lease released")}
 })}}
}

func TestCorrStreamUnmodifiedLiveJobAndAdmission(t *testing.T) {
 for _,prefix:=range []bool{false,true} {t.Run(fmt.Sprint(prefix),func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,prefix);defer bm.releaseJob(job)
  e:=job.recoveryBorrow.duration;before:=eventStorageTree(t,job.dir)
  ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
  deadline,_:=ctx.Deadline()
  base:=corrStreamChildFactory(t,[]string{"normal","normal"},false)
  factory:=func(child context.Context,args []string)*exec.Cmd{
   actual,ok:=child.Deadline();if !ok || !actual.Equal(deadline) {t.Fatal("renewed deadline")}
   return base(child,args)
  }
  r,err:=corrStreamInspect(ctx,lease,e,factory,nil);corrStreamAssertJoined(t,r)
  if err!=nil || !r.Video.Equal || !r.Audio.Equal || !lease.held() || !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal(r,err)}
 })}
 var absent *eventDurationEvidence
 r,err:=corrStreamInspect(context.Background(),nil,absent,nil,nil)
 if err==nil || r.ChildrenStarted!=0 {t.Fatal(r,err)}
 refs,actual:=corrStreamDonors(t)
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 r,err=corrStreamRun(ctx,refs,actual,nil,nil,nil)
 if !errors.Is(err,context.Canceled) || r.ChildrenStarted!=0 || r.PipeDescriptorsCreated!=0 {t.Fatal(r,err)}
}

func TestCorrStreamPlansAndEarlyRefusal(t *testing.T) {
 for _,count:=range []int{1,3,maxRecoveryInputs+1} {
  args,plan,err:=corrStreamArguments(true,count);if err!=nil {t.Fatal(err)}
  if args[len(args)-1]!=fmt.Sprintf("pipe:%d",3+count) || strings.Count(plan,"file 'file:/proc/self/fd/")!=count {t.Fatal(args,plan)}
  if !strings.Contains(strings.Join(args," "),"-bsf:a setts=pts=N:dts=N") {t.Fatal("audio content normalization changed")}
 }
 refs,actual:=corrStreamDonors(t)
 factory:=func(context.Context,[]string)*exec.Cmd{t.Fatal("invalid admission reached child factory");return nil}
 for _,kind:=range []string{"guard","no references","too many references","nil actual","oversized input"} {t.Run(kind,func(t *testing.T){
  selected:=refs;output:=actual;var guard func(context.Context)error
  switch kind {
  case "guard":guard=func(context.Context)error{return errCorrPrototype}
  case "no references":selected=nil
  case "too many references":selected=make([]*os.File,maxRecoveryInputs+2)
  case "nil actual":output=nil
  case "oversized input":
   f,err:=os.CreateTemp(t.TempDir(),"oversize");if err!=nil {t.Fatal(err)};defer f.Close()
   if err:=f.Truncate(corrPrototypeInputLimit+1);err!=nil {t.Fatal(err)};selected=[]*os.File{f}
  }
  r,err:=corrStreamRun(context.Background(),selected,output,factory,nil,guard)
  if err==nil || r.ChildrenStarted!=0 || r.PipeDescriptorsCreated!=0 {t.Fatal(r,err)}
 })}
}
