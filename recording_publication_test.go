package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "sync"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func publicationSource(t *testing.T,dir,name,content string) string {
 t.Helper();path:=filepath.Join(dir,name)
 if err:=os.WriteFile(path,[]byte(content),0600);err!=nil {t.Fatal(err)}
 return path
}
func assertPublicationBytes(t *testing.T,path,want string) {
 t.Helper();got,err:=os.ReadFile(path);if err!=nil || string(got)!=want {t.Fatalf("%s: %q %v",filepath.Base(path),got,err)}
}
func acceptPublicationFixture(context.Context,string) error {return nil}

func TestPublicationDestinationCreatedAtRenameIsNeverOverwritten(t *testing.T) {
 dir:=t.TempDir();source:=publicationSource(t,dir,"source.tmp","candidate evidence");final:=filepath.Join(dir,"final.mp4")
 rename:=func(source,target string)error {
  if err:=os.WriteFile(target,[]byte("concurrent existing footage"),0600);err!=nil {return err}
  return renameRecordingNoReplace(source,target)
 }
 err:=publishRecordingWithOps(context.Background(),source,final,acceptPublicationFixture,rename)
 if !os.IsExist(err) {t.Fatalf("late collision not rejected: %v",err)}
 assertPublicationBytes(t,final,"concurrent existing footage");assertPublicationBytes(t,source,"candidate evidence")
}

func TestAtomicPublicationHasOneWinnerAndPreservesEveryLoser(t *testing.T) {
 const workers=16
 dir:=t.TempDir();final:=filepath.Join(dir,"final.mp4")
 sources:=make([]string,workers);for i:=range sources {sources[i]=publicationSource(t,dir,fmt.Sprintf("source-%d.tmp",i),fmt.Sprintf("evidence-%d",i))}
 ready:=sync.WaitGroup{};ready.Add(workers);start:=make(chan struct{});results:=make(chan struct{index int;err error},workers)
 for i:=range sources {go func(index int){ready.Done();<-start;results<-struct{index int;err error}{index,renameRecordingNoReplace(sources[index],final)}}(i)}
 ready.Wait();close(start)
 winner:=-1
 for range sources {
  result:=<-results
  if result.err==nil {if winner!=-1 {t.Fatal("multiple publishers won")};winner=result.index} else {
   if !os.IsExist(result.err) {t.Fatalf("unexpected publication failure: %v",result.err)}
   assertPublicationBytes(t,sources[result.index],fmt.Sprintf("evidence-%d",result.index))
  }
 }
 if winner<0 {t.Fatal("no publisher won")}
 assertPublicationBytes(t,final,fmt.Sprintf("evidence-%d",winner))
 if _,err:=os.Lstat(sources[winner]);!os.IsNotExist(err) {t.Fatal("winner source was not renamed",err)}
}

func TestPublicationProtectsSymlinkAndDirectoryDestinations(t *testing.T) {
 for _,kind:=range []string{"symlink","directory"} {t.Run(kind,func(t *testing.T){
  dir:=t.TempDir();source:=publicationSource(t,dir,"source.tmp","candidate evidence");final:=filepath.Join(dir,"final.mp4")
  outside:=publicationSource(t,dir,"outside.mp4","existing target")
  var err error;if kind=="symlink" {err=os.Symlink(outside,final)} else {err=os.Mkdir(final,0700)};if err!=nil {t.Fatal(err)}
  if err:=renameRecordingNoReplace(source,final);!os.IsExist(err) {t.Fatalf("destination not protected: %v",err)}
  assertPublicationBytes(t,source,"candidate evidence");assertPublicationBytes(t,outside,"existing target")
  info,err:=os.Lstat(final);if err!=nil {t.Fatal(err)}
  if kind=="symlink" && info.Mode()&os.ModeSymlink==0 {t.Fatal("symlink overwritten")}
  if kind=="directory" && !info.IsDir() {t.Fatal("directory overwritten")}
 })}
}

func TestPublicationFailureAndCancellationPreserveCandidate(t *testing.T) {
 for _,failure:=range []error{unix.ENOSPC,unix.EDQUOT,unix.EIO,unix.EROFS,unix.EACCES,unix.EXDEV,unix.ENOSYS,unix.EINVAL,unix.EOPNOTSUPP} {
  t.Run(failure.Error(),func(t *testing.T){
   dir:=t.TempDir();source:=publicationSource(t,dir,"source.tmp","retained evidence");final:=filepath.Join(dir,"final.mp4");calls:=0
   err:=publishRecordingWithOps(context.Background(),source,final,acceptPublicationFixture,func(string,string)error{calls++;return &os.LinkError{Op:"injected",Old:source,New:final,Err:failure}})
   if !errors.Is(err,failure) || calls!=1 {t.Fatal("publication failure lost or retried",err,calls)}
   assertPublicationBytes(t,source,"retained evidence");if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal("failed output published",err)}
  })
 }
 dir:=t.TempDir();source:=publicationSource(t,dir,"source.tmp","retained evidence");final:=filepath.Join(dir,"final.mp4")
 ctx,cancel:=context.WithCancel(context.Background());calls:=0
 validate:=func(context.Context,string)error{cancel();return nil}
 err:=publishRecordingWithOps(ctx,source,final,validate,func(string,string)error{calls++;return nil})
 if !errors.Is(err,context.Canceled) || calls!=0 {t.Fatal("canceled publication continued",err)}
 assertPublicationBytes(t,source,"retained evidence")
}

func TestPublicationRejectsSourceChangesDuringValidation(t *testing.T) {
 for _,kind:=range []string{"replacement","growth","symlink"} {t.Run(kind,func(t *testing.T){
  dir:=t.TempDir();source:=publicationSource(t,dir,"source.tmp","original evidence");final:=filepath.Join(dir,"final.mp4");calls:=0
  retained:=filepath.Join(dir,"retained-original.tmp")
  validate:=func(context.Context,string)error {
   if kind=="growth" {f,err:=os.OpenFile(source,os.O_APPEND|os.O_WRONLY,0);if err!=nil {return err};_,writeErr:=f.WriteString("extra");return errors.Join(writeErr,f.Close())}
   if err:=os.Rename(source,retained);err!=nil {return err}
   if kind=="symlink" {return os.Symlink(retained,source)}
   return os.WriteFile(source,[]byte("replacement bytes"),0600)
  }
  err:=publishRecordingWithOps(context.Background(),source,final,validate,func(string,string)error{calls++;return nil})
  if err==nil || calls!=0 {t.Fatal("changed source published")}
  if _,err:=os.Lstat(source);err!=nil {t.Fatal("changed candidate removed",err)}
  if kind!="growth" {assertPublicationBytes(t,retained,"original evidence")}
 })}
}

func TestPublicRecordingPublisherUsesRealMediaAndNoReplace(t *testing.T) {
 requireFFmpeg(t)
 dir:=t.TempDir();source:=filepath.Join(dir,"capture.mp4.tmp");final:=filepath.Join(dir,"capture.mp4")
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-t","0.5","-c:v","mpeg4","-f","mp4","-y",source)
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatalf("synthetic media failed: %v %s",err,output)}
 candidate,err:=os.ReadFile(source);if err!=nil {t.Fatal(err)}
 if err:=publishRecording(ctx,source,final);err!=nil {t.Fatal("real-media publication failed",err)}
 published,err:=os.ReadFile(final);if err!=nil || string(published)!=string(candidate) {t.Fatal("published bytes changed",err)}
 if err:=os.WriteFile(source,candidate,0600);err!=nil {t.Fatal(err)}
 if err:=publishRecording(ctx,source,final);!os.IsExist(err) {t.Fatal("public publisher overwrote destination",err)}
 if info,err:=os.Stat(source);err!=nil || info.Size()!=int64(len(candidate)) {t.Fatal("collision discarded candidate",err)}
}
