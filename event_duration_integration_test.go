package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "reflect"
 "strconv"
 "strings"
 "testing"
 "time"
)

// Synthetic capture, assembly and duration executables exercise production paths.
// They do NOT establish real camera, gap-free or requested wall-clock coverage.
func durationIntegrationFixture(t *testing.T,kind string) (*BufferManager,*bufferEvent,string) {
 t.Helper();installEventFailureTools(t,0,false)
 bm,job:=publicationCheckpointJob(t)
 counter:=filepath.Join(t.TempDir(),"duration-count")
 t.Setenv("DURATION_COUNTER",counter);t.Setenv("DURATION_CASE",kind)
 installEventSpanProbe(t,`case "$*" in
 *duration_ts,time_base*) ;;
 *) printf 'synthetic-codec\n'; exit 0;;
esac
count=0
[ ! -f "$DURATION_COUNTER" ] || count=$(cat "$DURATION_COUNTER")
count=$((count+1))
printf '%s' "$count" > "$DURATION_COUNTER" || exit 9
if [ "$DURATION_CASE" = barrier ] && [ "$count" = 1 ]; then
 printf r > "$DURATION_READY" || exit 8
 IFS= read -r token < "$DURATION_RESUME" || exit 7
fi
[ "$DURATION_CASE" != 'probe failure' ] || exit 6
if [ "$DURATION_CASE" = 'missing stream' ]; then
 printf '%s' '{"format":{"duration":"999"}}'; exit 0
fi
ticks=30000
if [ "$DURATION_CASE" = 'short capture' ] && [ "$count" = 1 ]; then ticks=29999; fi
if [ "$DURATION_CASE" = 'short output' ] && [ "$count" = 2 ]; then ticks=29999; fi
frames='[{"media_type":"video","stream_index":0,"pts":0,"duration":'"$ticks"'}]'
if [ "$DURATION_CASE" = 'missing frames' ]; then frames='[]'; fi
if [ "$DURATION_CASE" = 'short presentation' ]; then frames='[{"media_type":"video","stream_index":0,"pts":0,"duration":29999}]'; fi
if { [ "$DURATION_CASE" = 'capture gap' ] && [ "$count" = 1 ]; } || { [ "$DURATION_CASE" = 'output gap' ] && [ "$count" = 2 ]; }; then
 frames='[{"media_type":"video","stream_index":0,"pts":0,"duration":15000},{"media_type":"video","stream_index":0,"pts":15001,"duration":15000}]'
fi
if [ "$DURATION_CASE" = 'diagnostic' ]; then printf 'decode error' >&2; fi
printf '{"streams":[{"index":0,"codec_type":"video","duration_ts":%s,"time_base":"1/1000"}],"frames":%s}' "$ticks" "$frames"
`)
 return bm,job,counter
}
func durationIntegrationCount(t *testing.T,path string) int {
 t.Helper();data,err:=os.ReadFile(path)
 if os.IsNotExist(err) {return 0};if err!=nil {t.Fatal(err)}
 count,err:=strconv.Atoi(string(data));if err!=nil {t.Fatal(err)};return count
}
func durationIntegrationCheck(bm *BufferManager,job *bufferEvent) *eventReleaseRevalidation {
 // Synthetic registry witness, independent from production media observations.
 n:=&NVR{cameras:map[string]*Camera{bm.camera.ID:bm.camera}}
 bm.camera.IsRecording=true;bm.camera.RecordingMode=ModeEventBased;bm.camera.bufferManager=bm;bm.camera.recordCancel=func(){}
 check:=newEventReleaseRevalidation(n,bm,job,false)
 job.recoveryBorrow.revalidation=check
 job.recoveryBorrow.revalidationErr=check.check(context.Background(),job.recoveryBorrow.lease)
 return check
}
func TestProductionDurationObservedAfterPublishedCheckpointAndReusedThroughRelease(t *testing.T) {
 for _,expiredMetadata:=range []bool{false,true} {t.Run(fmt.Sprint(expiredMetadata),func(t *testing.T){
  bm,job,counter:=durationIntegrationFixture(t,"success")
  input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
  original:=job.recoveryWriter;published:=false
  var publicationLease *mediaMutationLease
  job.recoveryWriter=func(dir string,data []byte)error{
   var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {return err}
   if m.State=="published" && !published {
    published=true;publicationLease=job.recoveryBorrow.lease
    if durationIntegrationCount(t,counter)!=0 {t.Error("duration inspected before published checkpoint")}
    if err:=original(dir,data);err!=nil {return err}
    if expiredMetadata {
     // Deliberately expire the borrowed metadata context AFTER its real write.
     // The independent live-manager inspection budget must not borrow it.
     ended,cancel:=context.WithCancel(context.Background());cancel();job.recoveryBorrow.ctx=ended
    }
    return nil
   }
   return original(dir,data)
  }
  final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("publication lost")}
  job.recoveryWriter=original
  borrowed:=job.recoveryBorrow
  if !published || borrowed==nil || !borrowed.terminal || borrowed.lease!=publicationLease || !publicationLease.held() || borrowed.duration==nil || borrowed.durationErr!=nil || durationIntegrationCount(t,counter)!=2 {t.Fatal("duration observation/order/lease incorrect")}
  if borrowed.duration.presentation==nil {t.Fatal("production did not bind presentation observations")}
  if !errors.Is(borrowed.ctx.Err(),context.Canceled) {t.Fatal("function-local metadata lifetime changed")}
  check:=durationIntegrationCheck(bm,job)
  for i:=0;i<2;i++ {if err:=check.check(context.Background(),publicationLease);err!=nil {t.Fatal(err)}}
  inventory:=job.inventory
  bm.releaseJob(job)
  if borrowed.revalidationErr!=nil || job.recoveryFailed || !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || publicationLease.held() || len(bm.slots)!=0 || durationIntegrationCount(t,counter)!=2 {t.Fatal("terminal checks re-probed or disturbed metadata/teardown")}
  assertJobInventoryClosed(t,inventory)
  assertPublicationBytes(t,input,"synthetic output");assertPublicationBytes(t,final,"synthetic output")
 })}
}
func TestProductionDurationRefusalPreservesPublicationMetadataAndTeardown(t *testing.T) {
 for _,kind:=range []string{"short capture","short output","missing frames","short presentation","capture gap","output gap","diagnostic","probe failure","missing stream","published checkpoint failure","stop before probe","direct assembly","late foreign"} {t.Run(kind,func(t *testing.T){
  bm,job,counter:=durationIntegrationFixture(t,kind)
  input:=filepath.Join(job.dir,"event.mp4")
  if kind=="direct assembly" {
   if err:=job.withOwnedStagingCreator(bm.ctx,"event.mp4",func(f *os.File)error{_,err:=f.WriteString("synthetic output");return err});err!=nil {t.Fatal(err)}
  } else {input=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}}
  original:=job.recoveryWriter
  job.recoveryWriter=func(dir string,data []byte)error{
   var m eventRecoveryManifest;if err:=json.Unmarshal(data,&m);err!=nil {return err}
   if m.State=="published" {
    if kind=="published checkpoint failure" {return os.ErrPermission}
    if err:=original(dir,data);err!=nil {return err}
    if kind=="stop before probe" {bm.cancel()}
    if kind=="late foreign" {return os.WriteFile(filepath.Join(job.dir,"foreign"),[]byte("retain foreign"),0600)}
    return nil
   }
   return original(dir,data)
  }
  final:=bm.concatenateEventRecording(job,input);if final=="" {t.Fatal("duration refusal erased publication")}
  job.recoveryWriter=original
  borrowed:=job.recoveryBorrow;lease:=borrowed.lease
  if borrowed.duration!=nil || borrowed.durationErr==nil || borrowed.revalidationErr==nil || !borrowed.terminal || !lease.held() {t.Fatal("failed observation accepted or ownership lost",kind)}
  wantProbes:=1;if kind=="short output" || kind=="output gap" {wantProbes=2}
  if kind=="published checkpoint failure" || kind=="stop before probe" || kind=="direct assembly" || kind=="late foreign" {wantProbes=0}
  if got:=durationIntegrationCount(t,counter);got!=wantProbes {t.Fatal("unexpected probe count",got,wantProbes)}
  if job.recoveryFailed!=(kind=="published checkpoint failure") {t.Fatal("duration refusal changed persistence latch")}
  before:=readRecoveryTest(t,job.dir)
  bm.releaseJob(job)
  if !job.keepStaging || job.inventory!=nil || job.recoveryBorrow!=nil || lease.held() || len(bm.slots)!=0 || durationIntegrationCount(t,counter)!=wantProbes {t.Fatal("refusal leaked or retried")}
  after:=readRecoveryTest(t,job.dir)
  if kind=="published checkpoint failure" {
   if !job.recoveryFailed || after.Revision!=before.Revision || after.State!="publishing" {t.Fatal("sticky published-checkpoint failure lost",after)}
  } else if job.recoveryFailed || after.State!="published" || after.Revision!=before.Revision+1 {t.Fatal("independent retained checkpoint lost",after)}
  assertPublicationBytes(t,input,"synthetic output");assertPublicationBytes(t,final,"synthetic output")
  if kind=="late foreign" {assertPublicationBytes(t,filepath.Join(job.dir,"foreign"),"retain foreign")}
 })}
}
func TestDurationRevalidationBindsReceiptBeforeAndAfterIO(t *testing.T) {
 for _,kind:=range []string{"missing","substituted","failed","wrong job","short receipt","removed during IO"} {t.Run(kind,func(t *testing.T){
  _,bm,job,check,lease:=releaseRevalidationFixture(t)
  borrowed:=job.recoveryBorrow;original:=borrowed.duration;copy:=*original
  before:=eventStorageTree(t,job.dir)
  switch kind {
  case "missing":borrowed.duration=nil
  case "substituted":borrowed.duration=&copy
  case "failed":borrowed.durationErr=os.ErrPermission
  case "wrong job":wrong:=*job;original.job=&wrong
  case "short receipt":original.captureSpan=eventVideoSpan{29999,1,1000}
  }
  calls:=0;job.keepStaging=false
  err:=check.checkWithOutput(context.Background(),lease,func(ctx context.Context,lease *mediaMutationLease)error{
   calls++;if kind=="removed during IO" {borrowed.duration=nil};return check.output.check(ctx,lease)
  })
  wantCalls:=0;if kind=="removed during IO" {wantCalls=1}
  if err==nil || calls!=wantCalls || !reflect.DeepEqual(before,eventStorageTree(t,job.dir)) {t.Fatal("receipt substitution accepted or check mutated evidence",kind,err,calls)}
  borrowed.duration=original;borrowed.durationErr=nil;original.job=job;original.captureSpan=copy.captureSpan
  if err:=check.check(context.Background(),lease);err!=nil {t.Fatal("restored genuine receipt no longer valid",err)}
  bm.releaseJob(job)
  if !job.keepStaging || len(bm.slots)!=0 || lease.held() {t.Fatal("receipt refusal disturbed teardown")}
 })}
}
func TestProductionDurationProbeRetainsLeaseWhileChildRuns(t *testing.T) {
 bm,job,_:=durationIntegrationFixture(t,"barrier")
 input:=bm.recordEventDuration(job);if input=="" {t.Fatal("capture failed")}
 readyR,readyW,err:=os.Pipe();if err!=nil {t.Fatal(err)}
 resumeR,resumeW,err:=os.Pipe();if err!=nil {readyR.Close();readyW.Close();t.Fatal(err)}
 t.Setenv("DURATION_READY",fmt.Sprintf("/proc/%d/fd/%d",os.Getpid(),readyW.Fd()))
 t.Setenv("DURATION_RESUME",fmt.Sprintf("/proc/%d/fd/%d",os.Getpid(),resumeR.Fd()))
 result:=make(chan string,1);done:=make(chan struct{})
 go func(){defer close(done);result<-bm.concatenateEventRecording(job,input)}()
 defer func(){
  bm.cancel();readyR.Close();readyW.Close();resumeR.Close();resumeW.Close()
  select {case <-done:case <-time.After(8*time.Second):t.Error("duration finalizer did not join")}
 }()
 if err:=readyR.SetReadDeadline(time.Now().Add(3*time.Second));err!=nil {t.Fatal(err)}
 var one [1]byte;if _,err:=io.ReadFull(readyR,one[:]);err!=nil {t.Fatal("probe did not reach barrier",err)}
 // No job fields are read concurrently with the finalizer; the contender only
 // attempts the shared gate. The pipe barrier holds the actual probe child.
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 waiting:=&jobInventoryWaitContext{Context:parent,waiting:make(chan struct{})}
 contender:=make(chan error,1);contenderDone:=make(chan struct{})
 go func(){defer close(contenderDone);lease,err:=recordingMediaOwnership.acquire(waiting);if err==nil {lease.release()};contender<-err}()
 defer func(){cancel();select {case <-contenderDone:case <-time.After(3*time.Second):t.Error("media contender did not join")}}()
 awaitShutdownStep(t,waiting.waiting,"media contender did not wait during probe");cancel()
 select {case err:=<-contender:if !errors.Is(err,context.Canceled) {t.Fatal("probe released publication ownership",err)};case <-time.After(3*time.Second):t.Fatal("contender did not join")}
 if _,err:=io.WriteString(resumeW,"continue\n");err!=nil {t.Fatal(err)}
 var final string
 select {case final=<-result:case <-time.After(8*time.Second):t.Fatal("duration finalizer did not return")}
 <-done
 if strings.TrimSpace(final)=="" || job.recoveryBorrow.duration==nil || job.recoveryBorrow.durationErr!=nil {t.Fatal("barrier observation failed")}
 bm.releaseJob(job)
 if len(bm.slots)!=0 || job.recoveryBorrow!=nil {t.Fatal("probe owner leaked terminal capacity")}
}
