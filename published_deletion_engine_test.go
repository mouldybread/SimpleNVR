package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func publishedEngineLease(t *testing.T) *mediaMutationLease {
 t.Helper();lease,err:=recordingMediaOwnership.acquire(context.Background());if err!=nil {t.Fatal(err)}
 t.Cleanup(lease.release);return lease
}
func publishedEngineAll(string,time.Time) bool {return true}
func publishedEngineGuard() error {return nil}

func TestPublishedEngineScopeAndBorrowedLease(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();outside:=filepath.Join(t.TempDir(),"outside.mp4")
 for name,data:=range map[string]string{"a.mp4":"abc","keep.mp4":"keep","x.mp4.tmp":"tmp","temp/event/input.mp4":"input","buffer/pre.mp4":"pre"} {putCleanupFixture(t,filepath.Join(dir,name),data)}
 putCleanupFixture(t,outside,"outside")
 if err:=os.Symlink(outside,filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
 if err:=unix.Mkfifo(filepath.Join(dir,"fifo.mp4"),0600);err!=nil {t.Fatal(err)}
 result,err:=deletePublishedLeased(context.Background(),lease,dir,func(name string,_ time.Time) bool{return name!="keep.mp4"},publishedEngineGuard)
 if err!=nil || result.Count!=1 || result.Bytes!=3 || result.Failures!=0 || !lease.held() {t.Fatal(result,err)}
 for _,name:=range []string{"keep.mp4","x.mp4.tmp","temp/event/input.mp4","buffer/pre.mp4","link.mp4","fifo.mp4"} {if _,err:=os.Lstat(filepath.Join(dir,name));err!=nil {t.Fatal(name,err)}}
 if got,err:=os.ReadFile(outside);err!=nil || string(got)!="outside" {t.Fatal("outside changed",err)}
}

func TestPublishedEnginePreflightRejectsChangedEntry(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir()
 for _,name:=range []string{"a.mp4","b.mp4"} {putCleanupFixture(t,filepath.Join(dir,name),"original")}
 selected,calls:=0,0
 result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,func(string,time.Time) bool {selected++;if selected==2 {putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"changed evidence longer")};return true},publishedEngineGuard,func(int,string,int) error {calls++;return nil})
 if !errors.Is(err,errPublishedDeletionRefused) || calls!=0 || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err,calls)}
}

func TestPublishedEnginePartialFailureCancellationAndLateArrival(t *testing.T) {
 for _,kind:=range []string{"error","cancel","late"} {t.Run(kind,func(t *testing.T){
  lease:=publishedEngineLease(t);dir:=t.TempDir();ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  for _,name:=range []string{"a.mp4","b.mp4"} {putCleanupFixture(t,filepath.Join(dir,name),"bytes")}
  result,err:=deletePublishedLeasedWithUnlink(ctx,lease,dir,publishedEngineAll,publishedEngineGuard,func(fd int,name string,flags int) error {
   if kind=="error" && name=="b.mp4" {return os.ErrPermission}
   if kind=="late" && name=="a.mp4" {putCleanupFixture(t,filepath.Join(dir,"late.mp4"),"late")}
   err:=unix.Unlinkat(fd,name,flags);if kind=="cancel" {cancel()};return err
  })
  want:=1;if kind=="late" {want=2}
  if result.Count!=want || result.Bytes!=int64(want*5) {t.Fatal(result,err)}
  if kind=="late" {if err!=nil {t.Fatal(err)};if got,err:=os.ReadFile(filepath.Join(dir,"late.mp4"));err!=nil || string(got)!="late" {t.Fatal("late entry touched",err)}} else {
   if err==nil || result.Failures!=1 {t.Fatal(result,err)}
   if kind=="cancel" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
   if got,err:=os.ReadFile(filepath.Join(dir,"b.mp4"));err!=nil || string(got)!="bytes" {t.Fatal("remaining bytes lost",err)}
  }
 })}
}

func TestPublishedEngineRootReplacementCannotRedirectUnlink(t *testing.T) {
 lease:=publishedEngineLease(t);parent:=t.TempDir();dir:=filepath.Join(parent,"root");moved:=filepath.Join(parent,"moved")
 for _,name:=range []string{"a.mp4","b.mp4"} {putCleanupFixture(t,filepath.Join(dir,name),"original")}
 result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,publishedEngineAll,publishedEngineGuard,func(fd int,name string,flags int) error {
  if name=="a.mp4" {if err:=os.Rename(dir,moved);err!=nil {return err};putCleanupFixture(t,filepath.Join(dir,"replacement.mp4"),"replacement")}
  return unix.Unlinkat(fd,name,flags)
 })
 if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=1 {t.Fatal(result,err)}
 if got,err:=os.ReadFile(filepath.Join(dir,"replacement.mp4"));err!=nil || string(got)!="replacement" {t.Fatal("replacement touched",err)}
 if _,err:=os.Stat(filepath.Join(moved,"b.mp4"));err!=nil {t.Fatal("remaining original lost",err)}
}

func TestPublishedEngineRefusesMissingOwnershipAndCanceledAdmission(t *testing.T) {
 dir:=t.TempDir();putCleanupFixture(t,filepath.Join(dir,"a.mp4"),"keep")
 result,err:=deletePublishedLeased(context.Background(),nil,dir,publishedEngineAll,publishedEngineGuard)
 if err==nil || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err)}
 lease:=publishedEngineLease(t);ctx,cancel:=context.WithCancel(context.Background());cancel()
 result,err=deletePublishedLeased(ctx,lease,dir,publishedEngineAll,publishedEngineGuard)
 if !errors.Is(err,context.Canceled) || result.Count!=0 {t.Fatal(result,err)}
}

func TestPublishedEngineParentReplacementAndInspectionBound(t *testing.T) {
 for _,kind:=range []string{"parent","limit"} {t.Run(kind,func(t *testing.T){
  lease:=publishedEngineLease(t);parent:=filepath.Join(t.TempDir(),"parent");dir:=filepath.Join(parent,"root")
  for _,name:=range []string{"a.mp4","b.mp4"} {putCleanupFixture(t,filepath.Join(dir,name),"original")}
  if kind=="limit" {for i:=0;i<publishedDeletionEntryLimit;i++ {if err:=os.WriteFile(filepath.Join(dir,fmt.Sprintf("opaque-%d",i)),nil,0600);err!=nil {t.Fatal(err)}}}
  selected,calls:=0,0
  result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,func(string,time.Time) bool {
   selected++
   if kind=="parent" && selected==2 {
    if err:=os.Rename(parent,parent+"-moved");err!=nil {t.Fatal(err)}
    putCleanupFixture(t,filepath.Join(dir,"replacement.mp4"),"replacement")
   }
   return true
  },publishedEngineGuard,func(int,string,int) error {calls++;return nil})
  if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || calls!=0 {t.Fatal(result,err,calls)}
  if kind=="parent" {if got,err:=os.ReadFile(filepath.Join(dir,"replacement.mp4"));err!=nil || string(got)!="replacement" {t.Fatal("replacement touched",err)}}
 })}
}
