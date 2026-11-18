package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func realPublicationDurabilityOps() publicationDurabilityOps {
 return publicationDurabilityOps{renameRecordingNoReplace,func(f *os.File) error {return f.Sync()},func(f *os.File) error {return f.Sync()}}
}

func TestDurablePublicationOrdersFileRenameAndBothParentSyncs(t *testing.T) {
 for _,crossDirectory:=range []bool{false,true} {t.Run(map[bool]string{false:"same-parent",true:"different-parents"}[crossDirectory],func(t *testing.T){
  sourceDir,finalDir:=t.TempDir(),t.TempDir();if !crossDirectory {finalDir=sourceDir}
  source:=publicationSource(t,sourceDir,"capture.tmp","candidate evidence");final:=filepath.Join(finalDir,"final.mp4")
  var stages []string;ops:=realPublicationDurabilityOps()
  ops.syncFile=func(f *os.File) error {stages=append(stages,"file-sync");assertPublicationBytes(t,source,"candidate evidence");return f.Sync()}
  ops.rename=func(a,b string) error {stages=append(stages,"rename");return renameRecordingNoReplace(a,b)}
  ops.syncDirectory=func(f *os.File) error {stages=append(stages,"directory-sync");assertPublicationBytes(t,final,"candidate evidence");return f.Sync()}
  validate:=func(context.Context,string) error {stages=append(stages,"validate");return nil}
  if err:=publishRecordingDurably(context.Background(),source,final,validate,ops);err!=nil {t.Fatal(err)}
  expected:="validate,file-sync,rename,directory-sync";if crossDirectory {expected+=",directory-sync"}
  if strings.Join(stages,",")!=expected {t.Fatal("incorrect durability ordering",stages)}
  if _,err:=os.Lstat(source);!os.IsNotExist(err) {t.Fatal("source not renamed",err)}
 })}
}

func TestDurablePublicationFileSyncFailurePreservesCandidate(t *testing.T) {
 dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","retained input");final:=filepath.Join(dir,"final.mp4")
 ops:=realPublicationDurabilityOps();renames,dirSyncs:=0,0
 ops.syncFile=func(*os.File) error {return os.ErrPermission}
 ops.rename=func(string,string) error {renames++;return nil}
 ops.syncDirectory=func(*os.File) error {dirSyncs++;return nil}
 err:=publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops)
 if !errors.Is(err,os.ErrPermission) || renames!=0 || dirSyncs!=0 {t.Fatal("sync failure advanced publication",err,renames,dirSyncs)}
 assertPublicationBytes(t,source,"retained input")
 if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal("unsynced output published",err)}
}

func TestDurablePublicationPostRenameSyncFailureIsNotRolledBackOrRetried(t *testing.T) {
 sourceDir,finalDir:=t.TempDir(),t.TempDir()
 source:=publicationSource(t,sourceDir,"capture.tmp","published but uncertain");final:=filepath.Join(finalDir,"final.mp4")
 ops:=realPublicationDurabilityOps();syncs,renames:=0,0
 ops.rename=func(a,b string) error {renames++;return renameRecordingNoReplace(a,b)}
 ops.syncDirectory=func(f *os.File) error {syncs++;if syncs==1 {return os.ErrPermission};return f.Sync()}
 err:=publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops)
 if !errors.Is(err,os.ErrPermission) || syncs!=2 || renames!=1 {t.Fatal("ambiguous publication hidden or other parent not synced",err,syncs,renames)}
 assertPublicationBytes(t,final,"published but uncertain")
 if _,err:=os.Lstat(source);!os.IsNotExist(err) {t.Fatal("rename was rolled back",err)}
 publicationSource(t,sourceDir,"capture.tmp","retry evidence")
 if err:=publishRecordingWithOps(context.Background(),source,final,acceptPublicationFixture,renameRecordingNoReplace);!os.IsExist(err) {t.Fatal("retry overwrote final or broke collision classification",err)}
 assertPublicationBytes(t,final,"published but uncertain");assertPublicationBytes(t,source,"retry evidence")
}

func TestDurablePublicationCancellationAroundCommitBoundary(t *testing.T) {
 for _,when:=range []string{"file-sync","rename"} {t.Run(map[bool]string{false:"",true:""}[false],func(t *testing.T){
  dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","capture");final:=filepath.Join(dir,"final.mp4")
  ctx,cancel:=context.WithCancel(context.Background());defer cancel();ops:=realPublicationDurabilityOps();renames,syncs:=0,0
  ops.syncFile=func(f *os.File) error {err:=f.Sync();if when=="file-sync" {cancel()};return err}
  ops.rename=func(a,b string) error {renames++;err:=renameRecordingNoReplace(a,b);if err==nil && when=="rename" {cancel()};return err}
  ops.syncDirectory=func(f *os.File) error {syncs++;return f.Sync()}
  err:=publishRecordingDurably(ctx,source,final,acceptPublicationFixture,ops)
  if !errors.Is(err,context.Canceled) {t.Fatal("cancellation not reported",err)}
  if when=="file-sync" {if renames!=0 || syncs!=0 {t.Fatal("canceled publication renamed")};assertPublicationBytes(t,source,"capture")} else {
   if renames!=1 || syncs!=1 {t.Fatal("post-rename durability abandoned",renames,syncs)};assertPublicationBytes(t,final,"capture")
  }
 })}
}

func TestDurablePublicationRechecksSourceAfterFileSync(t *testing.T) {
 for _,kind:=range []string{"growth","replacement","symlink"} {t.Run(kind,func(t *testing.T){
  dir:=t.TempDir();source:=publicationSource(t,dir,"capture.tmp","original");final:=filepath.Join(dir,"final.mp4");retained:=filepath.Join(dir,"retained.tmp")
  ops:=realPublicationDurabilityOps();renames:=0
  ops.syncFile=func(f *os.File) error {
   if err:=f.Sync();err!=nil {return err}
   if kind=="growth" {return os.WriteFile(source,[]byte("grown candidate bytes"),0600)}
   if err:=os.Rename(source,retained);err!=nil {return err}
   if kind=="symlink" {return os.Symlink(retained,source)}
   return os.WriteFile(source,[]byte("replacement"),0600)
  }
  ops.rename=func(string,string) error {renames++;return nil}
  if err:=publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops);err==nil || renames!=0 {t.Fatal("changed source published",err,renames)}
  if kind!="growth" {assertPublicationBytes(t,retained,"original")}
  if _,err:=os.Lstat(source);err!=nil {t.Fatal("changed candidate removed",err)}
 })}
}

func TestDurablePublicationDetectsReplacedParentAndPublishedIdentity(t *testing.T) {
 for _,when:=range []string{"file-sync","rename"} {t.Run(when,func(t *testing.T){
  parent:=t.TempDir();dir:=filepath.Join(parent,"camera");moved:=filepath.Join(parent,"retained")
  if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
  source:=publicationSource(t,dir,"capture.tmp","original");final:=filepath.Join(dir,"final.mp4");ops:=realPublicationDurabilityOps();renames:=0
  if when=="file-sync" {
   ops.syncFile=func(f *os.File) error {if err:=f.Sync();err!=nil {return err};if err:=os.Rename(dir,moved);err!=nil {return err};if err:=os.Mkdir(dir,0700);err!=nil {return err};return os.Link(filepath.Join(moved,"capture.tmp"),source)}
   ops.rename=func(string,string) error {renames++;return nil}
  } else {
   ops.rename=func(a,b string) error {renames++;if err:=renameRecordingNoReplace(a,b);err!=nil {return err};if err:=os.Rename(b,filepath.Join(dir,"retained.tmp"));err!=nil {return err};return os.WriteFile(b,[]byte("replacement"),0600)}
  }
  if err:=publishRecordingDurably(context.Background(),source,final,acceptPublicationFixture,ops);err==nil {t.Fatal("replacement was accepted")}
  if when=="file-sync" {if renames!=0 {t.Fatal("changed parent used")};assertPublicationBytes(t,filepath.Join(moved,"capture.tmp"),"original")} else {assertPublicationBytes(t,filepath.Join(dir,"retained.tmp"),"original");assertPublicationBytes(t,final,"replacement")}
 })}
}
