package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http/httptest"
 "os"
 "path/filepath"
 "sort"
 "testing"

 "golang.org/x/sys/unix"
)

type orphanBudgetEntry struct {name string}
func (entry orphanBudgetEntry) Name()string{return entry.name}
func (entry orphanBudgetEntry) IsDir()bool{return false}
func (entry orphanBudgetEntry) Type()os.FileMode{return 0}
func (entry orphanBudgetEntry) Info()(os.FileInfo,error){return nil,os.ErrInvalid}
func orphanBudgetEntries(count int)[]os.DirEntry {
 entries:=make([]os.DirEntry,count)
 for i:=range entries {entries[i]=orphanBudgetEntry{fmt.Sprintf("entry_%04d",count-i)}}
 return entries
}
type orphanBudgetReader struct {
 entries []os.DirEntry
 requests []int
 zero,oversized bool
 failure error
 afterRead func()
}
func (reader *orphanBudgetReader) ReadDir(count int)([]os.DirEntry,error) {
 reader.requests=append(reader.requests,count)
 if reader.afterRead!=nil {reader.afterRead()}
 if reader.zero {return nil,nil}
 if reader.failure!=nil {return reader.entries,reader.failure}
 if len(reader.entries)==0 {return nil,io.EOF}
 size:=count;if size>len(reader.entries) || reader.oversized {size=len(reader.entries)}
 out:=reader.entries[:size];reader.entries=reader.entries[size:]
 return out,nil
}
func orphanBudgetOps(budget *orphanInspectionBudget) orphanCleanupIO {
 return orphanCleanupIO{budget.readDir,budget.inspect,os.Remove,func()bool{return budget.ctx.Err()!=nil}}
}

func TestOrphanBudgetCollectorBoundariesAndIncompleteLists(t *testing.T) {
 kinds:=[]string{"empty","below","exact","over-directory","over-total","one-nonempty","one-empty","read-error","compound-eof-error","zero-progress","canceled-before","canceled-after","deadline-error","oversized-return"}
 for _,kind:=range kinds {t.Run(kind,func(t *testing.T){
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  budget:=newOrphanInspectionBudget(ctx);budget.perDirectory=4;budget.maxEntries=16
  reader:=&orphanBudgetReader{entries:orphanBudgetEntries(3)}
  one:=false;want:=3;var wantErr error
  switch kind {
  case "empty":reader.entries=nil;want=0
  case "exact":reader.entries=orphanBudgetEntries(4);want=4
  case "over-directory":reader.entries=orphanBudgetEntries(5);wantErr=errOrphanInspectionLimit
  case "over-total":budget.maxEntries=2;wantErr=errOrphanInspectionLimit
  case "one-nonempty":one=true;want=1
  case "one-empty":one=true;reader.entries=nil;want=0
  case "read-error":reader.entries=orphanBudgetEntries(1);reader.failure=os.ErrPermission;wantErr=os.ErrPermission
  case "compound-eof-error":reader.entries=orphanBudgetEntries(1);reader.failure=errors.Join(io.EOF,os.ErrPermission);wantErr=os.ErrPermission
  case "zero-progress":reader.zero=true;wantErr=io.ErrNoProgress
  case "canceled-before":cancel();wantErr=context.Canceled
  case "canceled-after":reader.afterRead=cancel;wantErr=context.Canceled
  case "deadline-error":reader.failure=context.DeadlineExceeded;wantErr=context.DeadlineExceeded
  case "oversized-return":reader.entries=orphanBudgetEntries(6);reader.oversized=true;wantErr=errOrphanInspectionLimit
  }
  got,err:=budget.collect(reader,one)
  if wantErr!=nil {
   if !errors.Is(err,wantErr) || got!=nil {t.Fatal("partial/refused listing exposed",len(got),err)}
  } else {
   if err!=nil || len(got)!=want {t.Fatal("positive listing rejected",len(got),err)}
   if !sort.SliceIsSorted(got,func(i,j int)bool{return got[i].Name()<got[j].Name()}) {t.Fatal("deterministic order lost")}
  }
  for _,count:=range reader.requests {if count<1 || count>orphanReadChunk {t.Fatal("unbounded directory read",count)}}
  if kind=="canceled-before" && len(reader.requests)!=0 {t.Fatal("canceled collector read directory")}
 })}
}

func TestOrphanBudgetPassLimitsStopBeforeFurtherRemoval(t *testing.T) {
 for _,kind:=range []string{"root","candidate","entries","before-first","after-first"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first")
  putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
  budget:=newOrphanInspectionBudget(context.Background())
  switch kind {
  case "root":budget.perDirectory=1;if err:=os.Mkdir(filepath.Join(root,"other"),0700);err!=nil {t.Fatal(err)}
  case "candidate":budget.perDirectory=1
  case "entries":budget.maxEntries=5
  case "before-first":budget.maxReads=3
  case "after-first":budget.maxReads=4
  }
  calls:=0;ops:=orphanBudgetOps(budget)
  ops.remove=func(path string)error{calls++;return os.Remove(path)}
  result,err:=cleanupOrphanedWithIO(root,nil,ops)
  if kind=="root" {if !errors.Is(err,errOrphanInspectionLimit) {t.Fatal(err)}} else if err!=nil {t.Fatal(err)}
  want:=0;if kind=="after-first" {want=1}
  if !result.LimitReached || result.Interrupted || result.Count!=want || calls!=want {t.Fatal("limit lost or deletion continued",result,err,calls)}
  if want==1 && (result.Bytes!=5 || result.Failures!=1) {t.Fatal("partial successful-unlink accounting lost",result)}
  assertPublicationBytes(t,filepath.Join(dir,"b.mp4"),"second")
  if want==0 {assertPublicationBytes(t,filepath.Join(dir,"a.mp4"),"first")}
  if _,err:=os.Stat(dir);err!=nil {t.Fatal("incompletely inspected directory removed",err)}
 })}
}

func TestOrphanBudgetStagingEvidenceAndSingleEntryPeeks(t *testing.T) {
 for _,kind:=range []string{"tmp-after-cap","temp-nonempty","buffer-nonempty","temp-empty-cap","buffer-empty-cap"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  putCleanupFixture(t,filepath.Join(dir,"saved.mp4"),"published evidence")
  budget:=newOrphanInspectionBudget(context.Background())
  switch kind {
  case "tmp-after-cap":putCleanupFixture(t,filepath.Join(dir,"z.mp4.tmp"),"");budget.perDirectory=1
  case "temp-nonempty","buffer-nonempty":
   name:="temp";if kind=="buffer-nonempty" {name="buffer"}
   for i:=0;i<3;i++ {putCleanupFixture(t,filepath.Join(dir,name,fmt.Sprintf("evidence-%d",i)),"capture")}
   budget.perDirectory=2
  default:
   name:="temp";if kind=="buffer-empty-cap" {name="buffer"}
   if err:=os.Mkdir(filepath.Join(dir,name),0700);err!=nil {t.Fatal(err)}
   budget.maxReads=2
  }
  before:=recoveryContractTree(t,root)
  result,err:=cleanupOrphanedWithIO(root,nil,orphanBudgetOps(budget))
  limited:=kind!="temp-nonempty" && kind!="buffer-nonempty"
  if err!=nil || result.Count!=0 || result.SkippedRecoveryDirs!=1 || result.LimitReached!=limited {t.Fatal("staging/limit preservation failed",result,err)}
  after:=recoveryContractTree(t,root)
  if len(before)!=len(after) {t.Fatal("staging entries changed")}
  for name,want:=range before {if after[name]!=want {t.Fatal("staging bytes/mode changed",name)}}
  if !limited && budget.entries!=4 {t.Fatal("nonempty staging was fully enumerated instead of peeked",budget.entries)}
 })}
}

func TestOrphanBudgetDirectoryOpenAndCancellation(t *testing.T) {
 for _,kind:=range []string{"regular","symlink","fifo","canceled","missing","normal"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();path:=filepath.Join(root,"candidate")
  switch kind {
  case "regular":if err:=os.WriteFile(path,[]byte("keep"),0600);err!=nil {t.Fatal(err)}
  case "symlink":if err:=os.Symlink(t.TempDir(),path);err!=nil {t.Fatal(err)}
  case "fifo":if err:=unix.Mkfifo(path,0600);err!=nil {t.Fatal(err)}
  case "canceled","normal":if err:=os.Mkdir(path,0700);err!=nil {t.Fatal(err)}
  }
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  if kind=="canceled" {cancel()}
  budget:=newOrphanInspectionBudget(ctx)
  entries,err:=budget.readDir(path)
  if kind=="normal" {if err!=nil || len(entries)!=0 {t.Fatal(err)}} else if err==nil || entries!=nil {t.Fatal("non-directory/canceled listing accepted",kind,err)}
  if kind=="canceled" && (!errors.Is(err,context.Canceled) || budget.reads!=0) {t.Fatal("canceled budget performed an open",err,budget.reads)}
 })}
}

func TestOrphanBudgetHTTPRootPartialAndCancellationResults(t *testing.T) {
 for _,kind:=range []string{"root","partial","cancel","deadline"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();dir:=filepath.Join(root,"orphan")
  putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"first")
  putCleanupFixture(t,filepath.Join(dir,"b.mp4"),"second")
  budget:=newOrphanInspectionBudget(context.Background());ops:=orphanBudgetOps(budget)
  switch kind {
  case "root":budget.perDirectory=1;if err:=os.Mkdir(filepath.Join(root,"other"),0700);err!=nil {t.Fatal(err)}
  case "partial":budget.maxReads=4
  case "cancel":ops.readDir=func(string)([]os.DirEntry,error){return nil,context.Canceled}
  case "deadline":ops.readDir=func(string)([]os.DirEntry,error){return nil,context.DeadlineExceeded}
  }
  n:=&NVR{cameras:make(map[string]*Camera)};w:=httptest.NewRecorder()
  serveOrphanCleanup(w,httptest.NewRequest("POST","/api/recordings/cleanup-orphaned",nil),n,root,ops)
  if kind=="root" {if w.Code!=413 {t.Fatal("root limit status",w.Code)};return}
  var result struct {Success bool `json:"success"`;Count int `json:"count"`;Failures int `json:"failures"`;Interrupted bool `json:"interrupted"`;LimitReached bool `json:"limit_reached"`}
  if err:=json.Unmarshal(w.Body.Bytes(),&result);err!=nil {t.Fatal(err)}
  want:=0;if kind=="partial" {want=1}
  if w.Code!=200 || result.Success || result.Count!=want || result.Failures!=1 || result.Interrupted!=(kind=="cancel" || kind=="deadline") || result.LimitReached!=(kind=="partial") {t.Fatal("limit/cancellation response conflated",w.Code,result)}
  assertPublicationBytes(t,filepath.Join(dir,"b.mp4"),"second")
 })}
}

func TestOrphanBudgetRejectsUnavailableOperations(t *testing.T) {
 for _,kind:=range []string{"read","inspect","remove","interrupted"} {t.Run(kind,func(t *testing.T){
  root:=t.TempDir();ops:=orphanCleanupIOFor(context.Background())
  switch kind {case "read":ops.readDir=nil;case "inspect":ops.inspect=nil;case "remove":ops.remove=nil;case "interrupted":ops.interrupted=nil}
  if result,err:=cleanupOrphanedWithIO(root,nil,ops);!errors.Is(err,os.ErrInvalid) || result.Count!=0 {t.Fatal("unavailable inspection did not refuse",result,err)}
  w:=httptest.NewRecorder();serveOrphanCleanup(w,httptest.NewRequest("POST","/api/recordings/cleanup-orphaned",nil),&NVR{cameras:make(map[string]*Camera)},root,ops)
  if w.Code!=500 {t.Fatal("invalid operations handler did not refuse",w.Code)}
 })}
}

func TestOrphanBudgetRejectsOversizedInjectedRootWithoutPrefix(t *testing.T) {
 root:=t.TempDir();ops:=orphanCleanupIOFor(context.Background());removes:=0
 ops.readDir=func(string)([]os.DirEntry,error){return orphanBudgetEntries(orphanDirectoryEntryLimit+1),nil}
 ops.remove=func(string)error{removes++;return nil}
 result,err:=cleanupOrphanedWithIO(root,nil,ops)
 if !errors.Is(err,errOrphanInspectionLimit) || !result.LimitReached || result.Count!=0 || removes!=0 {t.Fatal("oversized injected listing authorized a prefix",result,err,removes)}
}
