package main

import (
 "context"
 "errors"
 "fmt"
 "log"
 "os"
 "path/filepath"
 "strings"
 "time"

 "golang.org/x/sys/unix"
)

func (bm *BufferManager) pinOwnedBuffer(event Event) (*bufferEvent,error) {
 return bm.pinOwnedBufferWithOpen(event,openOwnedStagingInventory)
}

// Only the inventory opener is injectable; production always uses the real one.
// The existing prebuffer read lock excludes pruning throughout source pinning.
// No adopter may wait for bufferMutex while holding media ownership.
func (bm *BufferManager) pinOwnedBufferWithOpen(event Event,open func(context.Context,*mediaMutationLease,string)(*ownedStagingInventory,error)) (result *bufferEvent,resultErr error) {
 if open==nil {return nil,errStagingInventory}
 parent:=filepath.Join(bm.camera.RecordDir,"temp")
 if err:=os.MkdirAll(parent,0700);err!=nil {return nil,err}
 dir,err:=os.MkdirTemp(parent,"event-");if err!=nil {return nil,err}
 job:=&bufferEvent{event:event,preStart:event.CreatedAt.UTC(),dir:dir,keepStaging:true}
 bm.bufferMutex.RLock();defer bm.bufferMutex.RUnlock()
 cutoff:=time.Now().UTC().Add(-bufferDuration)
 selected:=make([]BufferSegment,0)
 for _,seg:=range bm.buffer {
  if seg.EndTime.After(cutoff) {
   if len(selected)>=maxRecoveryInputs {return nil,errors.New("too many recovery inputs")}
   selected=append(selected,seg)
  }
 }
 // Metadata finalization is intentionally independent of capture cancellation,
 // preserving queued/refused-job checkpoints. The operation has its own budget.
 ctx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace);defer cancel()
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return nil,err}
 defer lease.release()
 job.inventory,err=open(ctx,lease,dir)
 defer func(){if resultErr!=nil {resultErr=errors.Join(resultErr,job.closeOwnedInventory())}}()
 if err!=nil {return nil,err};if job.inventory==nil || job.inventory.path!=dir {return nil,errStagingInventory}
 // Borrow this setup lease for both checkpoints; never recursively acquire it.
 job.recoveryWriter=func(path string,data []byte)error {
  if path!=job.dir {return errStagingInventory}
  return writeOwnedEventRecovery(ctx,lease,job.inventory,data)
 }
 if err:=bm.beginEventRecovery(job,selected);err!=nil {return nil,err}
 for _,seg:=range selected {
  name:=fmt.Sprintf("pre_%03d.mp4",len(job.preFiles))
  if err:=pinOwnedJobInput(ctx,lease,job.inventory,seg.FilePath,name);err!=nil {
   _=job.recoveryCheckpoint("pin_failed","link_failed")
   log.Printf("Retained incomplete prebuffer staging for camera %s event %s; inspection/recovery required",bm.camera.ID,event.ID)
   return nil,err
  }
  if len(job.preFiles)==0 {job.preStart=seg.StartTime}
  job.preFiles=append(job.preFiles,filepath.Join(dir,name))
 }
 if err:=job.recoveryCheckpoint("pinned","");err!=nil {return nil,err}
 // After setup, each serialized checkpoint gets a fresh bounded lease. Neither
 // this job nor its descriptors retain setup ownership across capture/FFmpeg.
 job.recoveryWriter=job.writeOwnedJobRecovery
 return job,nil
}

// The source is an explicitly selected completed prebuffer entry, opened during
// this pin operation, not a filename discovered at cleanup. Link into pinned root
// and verify that the new name refers to that same descriptor before recording it.
func pinOwnedJobInput(ctx context.Context,lease *mediaMutationLease,inventory *ownedStagingInventory,path,name string) (result error) {
 if name=="" || name=="." || name==".." || strings.ContainsAny(name,"/\\\x00") {return errStagingInventory}
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 source,err:=os.OpenFile(path,os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK,0);if err!=nil {return err}
 defer func(){result=errors.Join(result,source.Close())}()
 info,err:=source.Stat();if err!=nil {return err};if !info.Mode().IsRegular() {return errStagingInventory}
 if err:=inventory.check(ctx,lease);err!=nil {return err}
 if err:=unix.Linkat(unix.AT_FDCWD,path,int(inventory.root.Fd()),name,0);err!=nil {return err}
 return inventory.Remember(ctx,lease,name,source)
}

func (job *bufferEvent) writeOwnedJobRecovery(path string,data []byte) error {
 if borrowed:=job.recoveryBorrow;borrowed!=nil {return job.writeOwnedJobRecoveryContext(borrowed.ctx,path,data)}
 ctx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace);defer cancel()
 return job.writeOwnedJobRecoveryContext(ctx,path,data)
}
func (job *bufferEvent) writeOwnedJobRecoveryContext(ctx context.Context,path string,data []byte) error {
 if job.inventory==nil || path!=job.dir || job.inventory.path!=job.dir {return errStagingInventory}
 if borrowed:=job.recoveryBorrow;borrowed!=nil {
  if err:=borrowed.ctx.Err();err!=nil {return err}
  return writeOwnedEventRecovery(ctx,borrowed.lease,job.inventory,data)
 }
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return err}
 defer lease.release()
 return writeOwnedEventRecovery(ctx,lease,job.inventory,data)
}
func (job *bufferEvent) closeOwnedInventory() error {
 if job.inventory==nil {return nil}
 inventory:=job.inventory;job.inventory=nil
 return inventory.Close()
}

func (bm *BufferManager) releaseOwnedJob(job *bufferEvent) {
 borrowed:=job.recoveryBorrow
 defer func(){
  // Close pins and return capacity before releasing transferred acquisition
  // ownership. Checkpoint helpers never release the borrowed lease themselves.
  if borrowed!=nil && borrowed.terminal {
   defer borrowed.lease.release()
   defer func(){borrowed.terminal=false}()
  }
  defer func(){job.recoveryBorrow=nil;<-bm.slots}()
  defer func(){if err:=job.closePublishedOutput();err!=nil {log.Printf("Unable to close event publication evidence for camera %s",bm.camera.ID)}}()
  if err:=job.closeOwnedInventory();err!=nil {log.Printf("Unable to close event inventory for camera %s",bm.camera.ID)}
 }()
 if borrowed!=nil && borrowed.terminal {
  // concatenateEventRecording cancels its finalization context on return.
  // Terminal metadata keeps its established independent grace budget while
  // borrowing the SAME held lease; there is no release/reacquisition window.
  ctx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace);defer cancel()
  borrowed.ctx=ctx
 }
 // Retention is still unconditional, never inferred from a legacy false flag.
 job.keepStaging=true
 _=job.recoveryCheckpoint("retained","")
 log.Printf("Retained event staging for camera %s event %s; inspection/recovery required",bm.camera.ID,job.event.ID)
}
