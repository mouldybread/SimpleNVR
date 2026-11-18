package main

import (
 "context"
 "errors"
)

// Join in the caller, not an abandoned coordinator that can later start a save.
// A blocked worker/syscall may still outlive the deadline; failure is explicit.
func completeApplicationShutdown(ctx context.Context,workersDone,mutationsDone <-chan struct{},httpDone <-chan error,persist func() error) error {
 for _,joined:=range []<-chan struct{}{workersDone,mutationsDone} {
  if err:=ctx.Err();err!=nil {return err}
  select {case <-joined:case <-ctx.Done():return ctx.Err()}
 }
 if err:=ctx.Err();err!=nil {return err}
 var httpErr error
 select {case httpErr=<-httpDone:case <-ctx.Done():return ctx.Err()}
 if err:=ctx.Err();err!=nil {return errors.Join(httpErr,err)}
 if persist==nil {return errors.Join(httpErr,errors.New("shutdown persistence callback missing"))}
 // Once entered, persistence is still an uninterruptible operation. Keep the
 // caller's deadline without claiming to cancel or roll back a started save.
 done:=make(chan error,1)
 go func(){
  if err:=ctx.Err();err!=nil {done<-err;return}
  done<-persist()
 }()
 select {
 case err:=<-done:return errors.Join(httpErr,err,ctx.Err())
 case <-ctx.Done():return errors.Join(httpErr,ctx.Err())
 }
}
