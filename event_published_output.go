package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "runtime"
 "strings"

 "golang.org/x/sys/unix"
)

// Publication-derived identity evidence, NOT complete capture or permission to
// delete. Two CLOEXEC pins per finalizing job; caller serializes check/Close.
type eventPublishedOutput struct {
 file,directory *os.File
 path string
 identity,directoryID unix.Stat_t
}
func duplicateEventPublicationFile(file *os.File,name string) (*os.File,error) {
 if file==nil {return nil,errPublicationOwnership}
 fd,err:=unix.FcntlInt(file.Fd(),unix.F_DUPFD_CLOEXEC,0)
 runtime.KeepAlive(file)
 if err!=nil {return nil,err}
 return os.NewFile(uintptr(fd),name),nil
}
func sameEventPublicationDirectory(a,b unix.Stat_t) bool {
 return a.Dev==b.Dev && a.Ino==b.Ino && a.Mode==b.Mode && a.Mode&unix.S_IFMT==unix.S_IFDIR && a.Uid==b.Uid && a.Gid==b.Gid
}

// MUST receive the descriptors still held by successful durable publication,
// never files opened by scanning a published pathname after the operation.
func retainEventPublishedOutput(ctx context.Context,lease *mediaMutationLease,path string,file,directory *os.File) (*eventPublishedOutput,error) {
 return retainEventPublishedOutputWithDup(ctx,lease,path,file,directory,duplicateEventPublicationFile)
}
func retainEventPublishedOutputWithDup(ctx context.Context,lease *mediaMutationLease,path string,file,directory *os.File,duplicate func(*os.File,string)(*os.File,error)) (result *eventPublishedOutput,resultErr error) {
 if err:=ctx.Err();err!=nil {return nil,err}
 if duplicate==nil {return nil,errPublicationOwnership}
 if !lease.held() || lease.gate!=recordingMediaOwnership || !filepath.IsAbs(path) || filepath.Clean(path)!=path {return nil,errPublicationOwnership}
 name:=filepath.Base(path)
 if name=="." || name==".." || strings.ContainsAny(name,"/\\\x00") {return nil,errPublicationOwnership}
 retained:=&eventPublishedOutput{path:path}
 defer func(){if resultErr!=nil {resultErr=errors.Join(resultErr,retained.Close())}}()
 var err error
 retained.file,err=duplicate(file,name);if err!=nil {return nil,err};if retained.file==nil {return nil,errPublicationOwnership}
 retained.directory,err=duplicate(directory,filepath.Dir(path));if err!=nil {return nil,err};if retained.directory==nil {return nil,errPublicationOwnership}
 if err:=unix.Fstat(int(retained.file.Fd()),&retained.identity);err!=nil {return nil,err}
 if retained.identity.Mode&unix.S_IFMT!=unix.S_IFREG || retained.identity.Size<=0 {return nil,errPublicationOwnership}
 if err:=unix.Fstat(int(retained.directory.Fd()),&retained.directoryID);err!=nil {return nil,err}
 if err:=retained.check(ctx,lease);err!=nil {return nil,err}
 return retained,nil
}

func (output *eventPublishedOutput) check(ctx context.Context,lease *mediaMutationLease) error {
 if err:=ctx.Err();err!=nil {return err}
 if !lease.held() || lease.gate!=recordingMediaOwnership || output==nil || output.file==nil || output.directory==nil {return errPublicationOwnership}
 var heldDirectory,namedDirectory,held,named unix.Stat_t
 if err:=unix.Fstat(int(output.directory.Fd()),&heldDirectory);err!=nil {return err}
 if err:=unix.Fstatat(unix.AT_FDCWD,filepath.Dir(output.path),&namedDirectory,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
 if !sameEventPublicationDirectory(output.directoryID,heldDirectory) || !sameEventPublicationDirectory(output.directoryID,namedDirectory) {return errPublicationOwnership}
 if err:=unix.Fstat(int(output.file.Fd()),&held);err!=nil {return err}
 if err:=unix.Fstatat(int(output.directory.Fd()),filepath.Base(output.path),&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
 if !stagingInventoryIdentity(output.identity,held) || !stagingInventoryIdentity(output.identity,named) {return errPublicationOwnership}
 return ctx.Err()
}
func (output *eventPublishedOutput) Close() (result error) {
 if output==nil {return nil}
 for _,file:=range []*os.File{output.file,output.directory} {if file!=nil {result=errors.Join(result,file.Close())}}
 output.file=nil;output.directory=nil
 return result
}
func (job *bufferEvent) closePublishedOutput() error {
 if job==nil || job.recoveryBorrow==nil || job.recoveryBorrow.output==nil {return nil}
 output:=job.recoveryBorrow.output;job.recoveryBorrow.output=nil
 return output.Close()
}
