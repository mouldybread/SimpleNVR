package main

import (
 "context"
 "errors"
 "os"
 "fmt"
 "path/filepath"

 "golang.org/x/sys/unix"
)

// No check-then-rename fallback: unsupported kernels/filesystems fail closed.
func renameRecordingNoReplace(temporary,final string) error {
 if err:=unix.Renameat2(unix.AT_FDCWD,temporary,unix.AT_FDCWD,final,unix.RENAME_NOREPLACE);err!=nil {
  return &os.LinkError{Op:"rename_noreplace",Old:temporary,New:final,Err:err}
 }
 return nil
}

// Per-call operations retain the existing test seam. Production always uses
// real file and directory Sync; failures must not authorize staging cleanup.
func publishRecordingWithOps(ctx context.Context,temporary,final string,validate func(context.Context,string) error,rename func(string,string) error) error {
 return publishRecordingDurably(ctx,temporary,final,validate,publicationDurabilityOps{rename,func(f *os.File) error {return f.Sync()},func(f *os.File) error {return f.Sync()}})
}
type publicationDurabilityOps struct {
 rename func(string,string) error
 syncFile func(*os.File) error
 syncDirectory func(*os.File) error
}
func samePublicationFile(before,after os.FileInfo) bool {
 return before!=nil && after!=nil && after.Mode().IsRegular() && os.SameFile(before,after) && before.Size()==after.Size() && before.ModTime().Equal(after.ModTime())
}

// Sync the validated inode before no-replace rename, then sync BOTH directory
// entries when publishing across directories. Post-rename errors are ambiguous
// publication outcomes: never unlink the final or imply rollback occurred.
func publishRecordingDurably(ctx context.Context,temporary,final string,validate func(context.Context,string) error,ops publicationDurabilityOps) (result error) {
 if validate==nil || ops.rename==nil || ops.syncFile==nil || ops.syncDirectory==nil {return errors.New("recording publication operations missing")}
 if err:=ctx.Err();err!=nil {return err}
 if _,err:=os.Lstat(final);err==nil {return os.ErrExist} else if !os.IsNotExist(err) {return err}
 before,err:=os.Lstat(temporary);if err!=nil {return err}
 if !before.Mode().IsRegular() || before.Size()==0 {return errors.New("recording is not a nonempty regular file")}
 pinned,err:=os.OpenFile(temporary,os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK,0);if err!=nil {return err}
 defer func(){if err:=pinned.Close();err!=nil {result=errors.Join(result,err)}}()
 opened,err:=pinned.Stat();if err!=nil {return err}
 if !samePublicationFile(before,opened) {return errors.New("recording changed before validation")}
 parents:=[]string{filepath.Dir(temporary)}
 if filepath.Clean(filepath.Dir(final))!=filepath.Clean(parents[0]) {parents=append(parents,filepath.Dir(final))}
 directories:=make([]*os.File,0,len(parents))
 defer func(){for _,directory:=range directories {if err:=directory.Close();err!=nil {result=errors.Join(result,err)}}}()
 for _,parent:=range parents {
  directory,err:=os.OpenFile(parent,os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW,0);if err!=nil {return err}
  directories=append(directories,directory)
 }
 check:=func() error {
  if err:=ctx.Err();err!=nil {return err}
  after,err:=os.Lstat(temporary);if err!=nil {return err}
  current,err:=pinned.Stat();if err!=nil {return err}
  if !samePublicationFile(before,after) || !samePublicationFile(before,current) {return errors.New("recording changed during validation or sync")}
  for i,directory:=range directories {
   held,err:=directory.Stat();if err!=nil {return err}
   named,err:=os.Lstat(parents[i]);if err!=nil {return err}
   if !named.IsDir() || named.Mode()&os.ModeSymlink!=0 || !os.SameFile(held,named) {return errors.New("recording publication directory changed")}
  }
  return nil
 }
 if err:=validate(ctx,temporary);err!=nil {return err}
 if err:=check();err!=nil {return err}
 if err:=ops.syncFile(pinned);err!=nil {return fmt.Errorf("sync recording before publication: %w",err)}
 if err:=check();err!=nil {return err}
 if err:=ops.rename(temporary,final);err!=nil {return err}
 // Once rename succeeds, finish durability work even if cancellation arrives.
 // Return cancellation afterward so event callers conservatively retain inputs.
 for _,directory:=range directories {if err:=ops.syncDirectory(directory);err!=nil {result=errors.Join(result,fmt.Errorf("sync recording publication directory: %w",err))}}
 published,err:=os.Lstat(final)
 if err!=nil {result=errors.Join(result,err)} else if !samePublicationFile(before,published) {result=errors.Join(result,errors.New("published recording identity changed"))}
 return errors.Join(result,ctx.Err())
}
