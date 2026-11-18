package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "reflect"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

// Genuine production witness bindings, synthetic media bytes. Only separate
// native fixtures test encoded content. No positive coverage claim is made.
func corrSafetyFixture(t *testing.T,prefix bool)(*BufferManager,*bufferEvent,*mediaMutationLease) {
 t.Helper()
 if !prefix {_,bm,job,_,lease:=releaseRevalidationFixture(t);return bm,job,lease}
 installEventFailureTools(t,0,false)
 bm,job:=selectionTimingJob(t)
 installSyntheticCoarseTimeline(job.execution.selection)
 t.Setenv("CORR_PROBE_COUNTER",filepath.Join(t.TempDir(),"count"))
 installEventSpanProbe(t,`case "$*" in
 *duration_ts,time_base*) ;;
 *) printf 'synthetic-codec\n'; exit 0;;
esac
count=0
[ ! -f "$CORR_PROBE_COUNTER" ] || count=$(cat "$CORR_PROBE_COUNTER")
count=$((count+1))
printf '%s' "$count" > "$CORR_PROBE_COUNTER" || exit 9
if [ "$count" = 1 ]; then
printf '%s' '{"streams":[{"index":0,"codec_type":"video","duration_ts":30000,"time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":30000}]}'
else
printf '%s' '{"streams":[{"index":0,"codec_type":"video","duration_ts":40000,"time_base":"1/1000"}],"frames":[{"media_type":"video","stream_index":0,"pts":0,"duration":5000},{"media_type":"video","stream_index":0,"pts":5000,"duration":5000},{"media_type":"video","stream_index":0,"pts":10000,"duration":30000}]}'
fi
`)
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture")}
 if bm.concatenateEventRecording(job,input)=="" {t.Fatal("assembly")}
 if job.recoveryBorrow==nil || job.recoveryBorrow.duration==nil {t.Fatal("missing bound observations")}
 return bm,job,job.recoveryBorrow.lease
}

func corrAssertScratchClosed(t *testing.T,dir string,opened []*os.File) {
 t.Helper()
 entries,err:=os.ReadDir(dir);if err!=nil || len(entries)!=0 {t.Fatal("scratch leak",entries,err)}
 for _,file:=range opened {if _,err:=file.Stat();err==nil {t.Fatal("scratch descriptor remains open")}}
}

func TestCorrPrototypeLiveMutationMatrix(t *testing.T) {
 kinds:=[]string{"selected descriptor","selected bytes","selected path replacement","output path replacement","concat bytes","capture bytes","output bytes","output binding","job request","inventory binding","borrowed lease","released lease","manager cancellation","caller cancellation"}
 for _,kind:=range kinds {for phase:=0;phase<=4;phase++ {t.Run(fmt.Sprintf("%s/after-%d",kind,phase),func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,true)
  evidence:=job.recoveryBorrow.duration
  savedDuration:=job.event.Duration;savedInventory:=job.inventory
  savedLease:=job.recoveryBorrow.lease;savedOutput:=evidence.output.file
  savedSelected:=job.execution.selection.inputs[0].file
  defer func(){
   job.event.Duration=savedDuration;job.inventory=savedInventory
   job.recoveryBorrow.lease=savedLease;evidence.output.file=savedOutput
   job.execution.selection.inputs[0].file=savedSelected
   bm.releaseJob(job)
   assertJobInventoryClosed(t,savedInventory)
   if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || lease.held() || len(bm.slots)!=0 {t.Error("terminal ownership/staging invariant")}
  }()
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  outputBefore,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
  // eventStorageTree's concrete return type is intentionally not assumed.
  var preserved func()bool
  mutate:=func(){
   switch kind {
   case "selected descriptor":job.execution.selection.inputs[0].file=evidence.capture
   case "selected bytes","concat bytes","capture bytes","output bytes":
    path:=filepath.Join(job.dir,"event.mp4");if kind=="output bytes" {path=evidence.output.path};if kind=="selected bytes" {path=job.preFiles[0]};if kind=="concat bytes" {path=filepath.Join(job.dir,"concat.txt")}
    f,err:=os.OpenFile(path,os.O_WRONLY|os.O_APPEND,0);if err!=nil {t.Fatal(err)}
    _,writeErr:=f.WriteString("injected change");closeErr:=f.Close();if writeErr!=nil || closeErr!=nil {t.Fatal(writeErr,closeErr)}
   case "selected path replacement","output path replacement":
    path:=job.preFiles[0];if kind=="output path replacement" {path=evidence.output.path}
    if err:=os.Rename(path,filepath.Join(t.TempDir(),"saved"));err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(path,[]byte("replacement"),0600);err!=nil {t.Fatal(err)}
   case "output binding":evidence.output.file=evidence.capture
   case "job request":job.event.Duration++
   case "inventory binding":job.inventory=nil
   case "borrowed lease":job.recoveryBorrow.lease=nil
   case "released lease":lease.release()
   case "manager cancellation":bm.cancel()
   case "caller cancellation":cancel()
   }
   tree:=eventStorageTree(t,job.dir)
   output,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
   preserved=func()bool {current,err:=os.ReadFile(evidence.output.path);return err==nil && reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) && string(current)==string(output)}
  }
  if phase==0 {mutate()}
  scratch:=t.TempDir();var opened []*os.File;calls:=0
  normalize:=func(c context.Context,files []*os.File,reference,audio bool,dst *os.File,r *corrPrototypeReport)error{
   calls++;opened=append(opened,dst)
   if calls%2==1 {if len(files)!=3 || files[0]!=savedSelected || files[2]!=evidence.capture || !reference {t.Fatal("reference order/descriptor changed")}} else if len(files)!=1 || files[0]!=savedOutput || reference {t.Fatal("output descriptor changed")}
   n,err:=dst.WriteString("synthetic equal payload");r.ProcessesStarted++;r.NormalizedBytes+=int64(n)
   if calls==phase {mutate()}
   return err
  }
  report,err:=corrPrototypeInspectWithNormalizer(ctx,lease,evidence,scratch,normalize)
  if err==nil || calls!=phase || report.ProcessesStarted!=phase {t.Fatal("mutation survived boundary",report,err,calls,phase)}
  if preserved==nil || !preserved() {t.Fatal("adapter altered recording/checkpoint evidence")}
  if kind!="released lease" && !lease.held() {t.Fatal("adapter released caller lease")}
  if kind!="output bytes" && kind!="output path replacement" {assertPublicationBytes(t,evidence.output.path,string(outputBefore))}
  corrAssertScratchClosed(t,scratch,opened)
 })}}
}

func TestCorrPrototypeSharedDeadlineRestorationAndAccounting(t *testing.T) {
 for _,prefix:=range []bool{false,true} {t.Run(fmt.Sprint(prefix),func(t *testing.T){
  bm,job,lease:=corrSafetyFixture(t,prefix)
  evidence:=job.recoveryBorrow.duration
  tree:=eventStorageTree(t,job.dir)
  output,err:=os.ReadFile(evidence.output.path);if err!=nil {t.Fatal(err)}
  parent,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  wantDeadline,_:=parent.Deadline()
  scratch:=t.TempDir();var opened []*os.File;var childContext context.Context
  calls:=0;var logical int64
  normalize:=func(ctx context.Context,files []*os.File,reference,audio bool,dst *os.File,r *corrPrototypeReport)error{
   calls++;opened=append(opened,dst)
   deadline,ok:=ctx.Deadline();if !ok || !deadline.Equal(wantDeadline) {t.Fatal("deadline renewed",deadline,wantDeadline)}
   if childContext==nil {childContext=ctx} else if childContext!=ctx {t.Fatal("new per-comparison context")}
   for _,file:=range files {stat,err:=file.Stat();if err!=nil {t.Fatal(err)};logical+=stat.Size()}
   n,err:=dst.WriteString("equal payload");r.ProcessesStarted++;r.NormalizedBytes+=int64(n)
   return err
  }
  // A prior rejected binding must not poison or replace genuine observations.
  request:=job.event.Duration;job.event.Duration++
  refused,err:=corrPrototypeInspectWithNormalizer(parent,lease,evidence,scratch,normalize)
  if err==nil || refused.ProcessesStarted!=0 || calls!=0 {t.Fatal("bad initial binding spawned")}
  job.event.Duration=request
  report,err:=corrPrototypeInspectWithNormalizer(parent,lease,evidence,scratch,normalize)
  if err!=nil || calls!=4 || report.ProcessesStarted!=4 || !report.VideoEqual || !report.AudioEqual || report.NormalizedBytes!=52 || report.ComparedBytes!=52 || report.InputSizePassBudget!=logical || report.PeakScratchDescriptors!=2 || report.BorrowedMediaDescriptors!=len(job.execution.selection.inputs)+2 || report.Elapsed<=0 {t.Fatal(report,err,calls,logical)}
  if !lease.held() || job.recoveryBorrow.duration!=evidence || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("success altered job/ownership/evidence")}
  assertPublicationBytes(t,evidence.output.path,string(output))
  corrAssertScratchClosed(t,scratch,opened)
  bm.releaseJob(job)
  if !job.keepStaging || lease.held() || len(bm.slots)!=0 {t.Fatal("terminal teardown")}
 })}
}

func TestCorrPrototypeMidCollectionFailuresCloseScratch(t *testing.T) {
 for _,kind:=range []string{"error","cancel","video mismatch","audio mismatch"} {for phase:=1;phase<=4;phase++ {t.Run(fmt.Sprintf("%s/%d",kind,phase),func(t *testing.T){
  if (kind=="video mismatch" && phase!=2) || (kind=="audio mismatch" && phase!=4) {return}
  bm,job,lease:=corrSafetyFixture(t,false);defer bm.releaseJob(job)
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  tree:=eventStorageTree(t,job.dir);output,err:=os.ReadFile(job.recoveryBorrow.output.path);if err!=nil {t.Fatal(err)}
  scratch:=t.TempDir();var opened []*os.File;calls:=0
  injected:=errors.New("injected normalizer failure")
  normalize:=func(ctx context.Context,files []*os.File,reference,audio bool,dst *os.File,r *corrPrototypeReport)error{
   calls++;opened=append(opened,dst);r.ProcessesStarted++
   text:="equal";if calls==phase && (kind=="video mismatch" || kind=="audio mismatch") {text="wrong"}
   n,err:=dst.WriteString(text);r.NormalizedBytes+=int64(n);if err!=nil {return err}
   if calls==phase {if kind=="error" {return injected};if kind=="cancel" {cancel();return ctx.Err()}}
   return nil
  }
  report,err:=corrPrototypeInspectWithNormalizer(ctx,lease,job.recoveryBorrow.duration,scratch,normalize)
  if err==nil || calls!=phase || report.ProcessesStarted!=phase {t.Fatal(report,err,calls)}
  if kind=="error" && !errors.Is(err,injected) {t.Fatal(err)}
  if kind=="cancel" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
  if !lease.held() || !reflect.DeepEqual(tree,eventStorageTree(t,job.dir)) {t.Fatal("failure changed job/lease")}
  assertPublicationBytes(t,job.recoveryBorrow.output.path,string(output));corrAssertScratchClosed(t,scratch,opened)
 })}}
}

func TestCorrPrototypeRealChildrenAreJoined(t *testing.T) {
 for _,kind:=range []string{"success","diagnostic","nonzero","timeout","output ceiling","diagnostic ceiling","empty"} {t.Run(kind,func(t *testing.T){
  dir:=t.TempDir()
  script:="#!/bin/sh\nprintf payload\n"
  switch kind {
  case "diagnostic":script+="printf diagnosed >&2\n"
  case "nonzero":script+="exit 7\n"
  case "timeout":script="#!/bin/sh\nexec /bin/sleep 10\n"
  case "output ceiling":script="#!/bin/sh\nexec /usr/bin/head -c 33554433 /dev/zero\n"
  case "diagnostic ceiling":script="#!/bin/sh\nexec /usr/bin/head -c 16385 /dev/zero >&2\n"
  case "empty":script="#!/bin/sh\nexit 0\n"
  }
  if err:=os.WriteFile(filepath.Join(dir,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
  t.Setenv("PATH",dir)
  source,err:=os.CreateTemp(t.TempDir(),"source");if err!=nil {t.Fatal(err)};defer source.Close()
  dst,err:=os.CreateTemp(t.TempDir(),"sink");if err!=nil {t.Fatal(err)};defer dst.Close()
  budget:=3*time.Second;if kind=="timeout" {budget=100*time.Millisecond}
  ctx,cancel:=context.WithTimeout(context.Background(),budget);defer cancel()
  report:=corrPrototypeReport{};started:=time.Now()
  err=corrPrototypeNormalize(ctx,[]*os.File{source},false,false,dst,&report)
  if (err==nil)!=(kind=="success") || report.ProcessesStarted!=1 || report.LastChildPID<=0 {t.Fatal(report,err)}
  var status unix.WaitStatus
  if _,waitErr:=unix.Wait4(report.LastChildPID,&status,unix.WNOHANG,nil);!errors.Is(waitErr,unix.ECHILD) {t.Fatal("normalizer did not reap child",waitErr)}
  if time.Since(started)>budget+2500*time.Millisecond {t.Fatal("join exceeded grace")}
  if report.NormalizedBytes>corrPrototypeStreamLimit {t.Fatal("output byte ceiling bypassed",report)}
  if _,err:=source.Stat();err!=nil {t.Fatal("donor closed",err)}
 })}
}

func TestCorrPrototypeNormalizerAdmissionDoesNotSpawn(t *testing.T) {
 for _,kind:=range []string{"cancelled","process ceiling","missing executable","closed donor"} {t.Run(kind,func(t *testing.T){
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  src,err:=os.CreateTemp(t.TempDir(),"source");if err!=nil {t.Fatal(err)};defer src.Close()
  dst,err:=os.CreateTemp(t.TempDir(),"sink");if err!=nil {t.Fatal(err)};defer dst.Close()
  report:=corrPrototypeReport{}
  switch kind {
  case "cancelled":cancel()
  case "process ceiling":report.ProcessesStarted=4
  case "missing executable":t.Setenv("PATH",t.TempDir())
  case "closed donor":if err:=src.Close();err!=nil {t.Fatal(err)}
  }
  err=corrPrototypeNormalize(ctx,[]*os.File{src},false,false,dst,&report)
  want:=0;if kind=="process ceiling" {want=4}
  if err==nil || report.ProcessesStarted!=want || report.LastChildPID!=0 {t.Fatal(report,err)}
 })}
}
