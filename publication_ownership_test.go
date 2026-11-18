package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

type publicationOwnershipWaitContext struct {context.Context;waiting chan struct{};once sync.Once}
func (ctx *publicationOwnershipWaitContext) Done() <-chan struct{} {ctx.once.Do(func(){close(ctx.waiting)});return ctx.Context.Done()}
func publicationOwnershipResult(t *testing.T,result <-chan error) error {
 t.Helper();select {case err:=<-result:return err;case <-time.After(3*time.Second):t.Fatal("publication ownership operation did not join");return nil}
}

func TestPublicationOwnershipCanceledWaitPreservesCandidate(t *testing.T) {
 dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","retain");final:=filepath.Join(dir,"final.mp4")
 owner,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)};defer owner.release()
 parent,cancel:=context.WithCancel(context.Background());defer cancel()
 ctx:=&publicationOwnershipWaitContext{Context:parent,waiting:make(chan struct{})}
 validated:=false;result:=make(chan error,1)
 go func(){result<-publishRecordingDurably(ctx,source,final,func(context.Context,string) error {validated=true;return nil},realPublicationDurabilityOps())}()
 awaitShutdownStep(t,ctx.waiting,"publisher did not enter ownership wait")
 cancel()
 if err:=publicationOwnershipResult(t,result);!errors.Is(err,context.Canceled) || validated {t.Fatal(err,validated)}
 assertPublicationBytes(t,source,"retain")
 if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal("canceled admission published output",err)}
 if !owner.held() {t.Fatal("canceled publisher released another owner")}
}

func TestPublicationOwnershipExcludesDeletionThroughFinalDirectorySync(t *testing.T) {
 for _,kind:=range []string{"explicit","retention"} {t.Run(kind,func(t *testing.T){
  n,cam:=lifecycleFixture(ModeContinuous);cam.RecordDir=t.TempDir();cam.RetentionDays=1
  target:=snapshotRetentionCleanup(n)[0]
  source:=publicationSource(t,t.TempDir(),"capture.tmp","candidate")
  final:=filepath.Join(cam.RecordDir,"20000101_000000_final.mp4")
  entered,release:=make(chan struct{}),make(chan struct{});var once sync.Once;defer once.Do(func(){close(release)})
  ops:=realPublicationDurabilityOps();syncs:=0
  ops.syncDirectory=func(f *os.File) error {syncs++;if syncs==2 {close(entered);<-release};return f.Sync()}
  published:=make(chan error,1)
  go func(){published<-publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops)}()
  awaitShutdownStep(t,entered,"publisher did not reach second parent sync")
  if len(recordingMediaOwnership.token)!=1 {t.Fatal("publisher lost ownership before final sync")}
  requireRegistryAvailable(t,n)
  parent,cancel:=context.WithCancel(context.Background());defer cancel()
  ctx:=&publicationOwnershipWaitContext{Context:parent,waiting:make(chan struct{})}
  deleted:=make(chan error,1)
  go func(){
   if kind=="retention" {if !cleanupRetentionSnapshot(ctx,n,target) {deleted<-errors.New("retention refused valid target")} else {deleted<-nil};return}
   result,err:=deleteRegisteredPublished(ctx,n,publishedDeletionTarget{cam.ID,cam.RecordDir,cam},deleteAllPublishedLeased)
   if err==nil && result.Count!=1 {err=errors.New("explicit deletion count mismatch")};deleted<-err
  }()
  awaitShutdownStep(t,ctx.waiting,"deletion did not wait on publication ownership")
  select {case <-deleted:t.Fatal("deletion overlapped publication durability");default:}
  assertPublicationBytes(t,final,"candidate")
  once.Do(func(){close(release)})
  if err:=publicationOwnershipResult(t,published);err!=nil {t.Fatal(err)}
  if err:=publicationOwnershipResult(t,deleted);err!=nil {t.Fatal(err)}
  if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal("following deletion did not run",err)}
  if len(recordingMediaOwnership.token)!=0 {t.Fatal("publication/deletion leaked lease")}
 })}
}

func TestBorrowedPublicationEntryPreservesCallerLease(t *testing.T) {
 // Probe executables are synthetic: this checks wiring, not media correctness.
 installEventFailureTools(t,0,false)
 lease:=publishedEngineLease(t);dir:=t.TempDir()
 source:=publicationSource(t,dir,"capture.tmp","candidate");final:=filepath.Join(dir,"final.mp4")
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 result:=make(chan error,1);go func(){result<-publishRecordingLeased(ctx,lease,source,final)}()
 if err:=publicationOwnershipResult(t,result);err!=nil {t.Fatal(err)}
 if !lease.held() {t.Fatal("borrower released caller lease")}
 assertPublicationBytes(t,final,"candidate")
}

func TestPublicationOwnershipFailurePathsReleaseLease(t *testing.T) {
 for _,phase:=range []string{"file-sync","directory-sync"} {t.Run(phase,func(t *testing.T){
  dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","evidence");final:=filepath.Join(dir,"final.mp4")
  ops:=realPublicationDurabilityOps()
  if phase=="file-sync" {ops.syncFile=func(*os.File) error{return os.ErrPermission}} else {ops.syncDirectory=func(*os.File) error{return os.ErrPermission}}
  err:=publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops)
  if !errors.Is(err,os.ErrPermission) {t.Fatal(err)}
  if phase=="file-sync" {assertPublicationBytes(t,source,"evidence");if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal(err)}} else {assertPublicationBytes(t,final,"evidence")}
  if len(recordingMediaOwnership.token)!=0 {t.Fatal("failed publication leaked lease")}
 })}
}

func TestLeasedPublicationRejectsMissingReleasedAndForeignLeases(t *testing.T) {
 lease:=publishedEngineLease(t);lease.release()
 foreign,err:=newMediaMutationGate().acquire(context.Background());if err!=nil {t.Fatal(err)};defer foreign.release()
 dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","evidence");final:=filepath.Join(dir,"final.mp4")
 for _,invalid:=range []*mediaMutationLease{nil,lease,foreign} {
  validated:=false
  err:=publishRecordingDurablyLeased(context.Background(),invalid,source,final,func(context.Context,string) error{validated=true;return nil},realPublicationDurabilityOps())
  if !errors.Is(err,errPublicationOwnership) || validated {t.Fatal("invalid ownership admitted",err,validated)}
 }
 if !foreign.held() {t.Fatal("invalid borrower disturbed foreign owner")}
 assertPublicationBytes(t,source,"evidence")
}
