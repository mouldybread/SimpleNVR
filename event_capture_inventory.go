package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "strings"

 "golang.org/x/sys/unix"
)

// Creation and registration borrow separate short leases. The caller serializes
// this job's inventory; no lease spans the writer or a child process Run/Wait.
// At most one creator and one transient Remember duplicate are added to the
// inventory's bounded persistent descriptors. No failure removes disk evidence.
func (job *bufferEvent) withOwnedStagingCreator(ctx context.Context,name string,write func(*os.File) error) (result error) {
 if job.recoveryBorrow!=nil {return errPublicationOwnership}
 if write==nil || (name!="event.mp4" && name!="concat.txt") {return errStagingInventory}
 inventory:=job.inventory
 if inventory==nil || inventory.path!=job.dir {return errStagingInventory}
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return err}
 file,err:=createOwnedStagingFile(ctx,lease,inventory,name)
 lease.release()
 if err!=nil {return err}
 defer func(){result=errors.Join(result,file.Close());if result!=nil {job.keepStaging=true}}()
 if err:=ctx.Err();err!=nil {result=err} else {result=write(file)}
 result=errors.Join(result,ctx.Err())
 // A joined failed/canceled writer may still have recovery bytes. Record their
 // final identity with an independently bounded metadata context, not the
 // canceled capture context. This never certifies successful/complete capture.
 finalizeCtx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace);defer cancel()
 finalLease,err:=recordingMediaOwnership.acquire(finalizeCtx)
 if err!=nil {return errors.Join(result,err)}
 defer finalLease.release()
 return errors.Join(result,inventory.Remember(finalizeCtx,finalLease,name,file))
}

// A borrowed-lease helper: exclusive, owner-only, descriptor-relative creation
// into the already pinned root, never an open/truncate of a discovered name.
func createOwnedStagingFile(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,name string) (*os.File,error) {
 if name!="event.mp4" && name!="concat.txt" {return nil,errStagingInventory}
 if err:=inventory.check(ctx,lease);err!=nil {return nil,err}
 if inventory.entries==nil || len(inventory.entries)>=ownedStagingEntryLimit {return nil,errStagingInventory}
 if _,exists:=inventory.entries[name];exists {return nil,errStagingInventory}
 fd,err:=unix.Openat(int(inventory.root.Fd()),name,unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC,0600)
 if err!=nil {return nil,err}
 file:=os.NewFile(uintptr(fd),name)
 fail:=func(err error)(*os.File,error){return nil,errors.Join(err,file.Close())}
 if err:=inventory.check(ctx,lease);err!=nil {return fail(err)}
 var held,named unix.Stat_t
 if err:=unix.Fstat(fd,&held);err!=nil {return fail(err)}
 if err:=unix.Fstatat(int(inventory.root.Fd()),name,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return fail(err)}
 if held.Mode&unix.S_IFMT!=unix.S_IFREG || !stagingInventoryIdentity(held,named) {return fail(errStagingInventory)}
 return file,nil
}

func (bm *BufferManager) captureEventFile(ctx context.Context,job *bufferEvent,path string) error {
 if job.recoveryBorrow!=nil {return errPublicationOwnership}
 args:=[]string{"-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-i",bm.camera.RTSPUrl,"-t",strconv.Itoa(job.event.Duration),"-c","copy","-f","mp4","-movflags","faststart","-y"}
 if job.inventory==nil {
  // Compatibility for manually constructed legacy jobs. Production jobs own
  // an inventory before reaching capture; terminal jobs are never re-executed.
  return ffmpegCommand(ctx,append(args,path)...).Run()
 }
 if path!=filepath.Join(job.dir,"event.mp4") {return errStagingInventory}
 return job.withOwnedStagingCreator(ctx,"event.mp4",func(file *os.File)error{
  // ExtraFiles[0] is fd 3 in this child. procfs reopens the creator inode as a
  // seekable file, including FFmpeg's faststart reopen; it is NOT pipe:3.
  // There is deliberately no pathname-output fallback if procfs is unavailable.
  cmd:=ffmpegCommand(ctx,append(args,"/proc/self/fd/3")...)
  cmd.ExtraFiles=[]*os.File{file}
  return cmd.Run() // includes existing interrupt/grace/kill/join behavior
 })
}

func (job *bufferEvent) writeEventConcat(ctx context.Context,eventFile string) (result error) {
 if job.recoveryBorrow!=nil {return errPublicationOwnership}
 write:=func(list *os.File)error{
  files:=append(append([]string(nil),job.preFiles...),eventFile)
  for _,file:=range files {
   if filepath.Dir(file)!=job.dir {return errStagingInventory}
   base:=filepath.Base(file)
   if strings.ContainsAny(base,"'\r\n\\") {return errStagingInventory}
   if _,err:=fmt.Fprintf(list,"file '%s'\n",base);err!=nil {return err}
  }
  return nil
 }
 if job.inventory!=nil {return job.withOwnedStagingCreator(ctx,"concat.txt",write)}
 list,err:=os.Create(filepath.Join(job.dir,"concat.txt"));if err!=nil {return err}
 defer func(){result=errors.Join(result,list.Close())}()
 return write(list)
}
