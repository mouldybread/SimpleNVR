package main

import (
 "context"
 "errors"
 "os"

 "golang.org/x/sys/unix"
)

// No check-then-rename fallback: unsupported kernels/filesystems fail closed.
func renameRecordingNoReplace(temporary,final string) error {
 if err:=unix.Renameat2(unix.AT_FDCWD,temporary,unix.AT_FDCWD,final,unix.RENAME_NOREPLACE);err!=nil {
  return &os.LinkError{Op:"rename_noreplace",Old:temporary,New:final,Err:err}
 }
 return nil
}

// Per-call operations permit deterministic failure/collision tests without
// mutating process-global function hooks or replacing media validation in use.
func publishRecordingWithOps(ctx context.Context,temporary,final string,validate func(context.Context,string) error,rename func(string,string) error) error {
 if validate==nil || rename==nil {return errors.New("recording publication operations missing")}
 if err:=ctx.Err();err!=nil {return err}
 if _,err:=os.Lstat(final);err==nil {return os.ErrExist} else if !os.IsNotExist(err) {return err}
 before,err:=os.Lstat(temporary)
 if err!=nil {return err}
 if !before.Mode().IsRegular() || before.Size()==0 {return errors.New("recording is not a nonempty regular file")}
 if err:=validate(ctx,temporary);err!=nil {return err}
 if err:=ctx.Err();err!=nil {return err}
 after,err:=os.Lstat(temporary)
 if err!=nil {return err}
 if !after.Mode().IsRegular() || !os.SameFile(before,after) || before.Size()!=after.Size() || !before.ModTime().Equal(after.ModTime()) {return errors.New("recording changed during validation")}
 if err:=ctx.Err();err!=nil {return err}
 return rename(temporary,final)
}
