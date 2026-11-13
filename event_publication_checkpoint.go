package main

import "context"

// Acquisition ownership may transfer to terminal release, never to a child or
// goroutine. Checkpoint writers only borrow. The job owner must call releaseJob
// after finalization returns, including on a published-checkpoint failure.
type eventRecoveryBorrow struct {ctx context.Context;lease *mediaMutationLease;terminal bool}

func (bm *BufferManager) publishEventRecording(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool) error {
 return bm.publishEventRecordingWith(ctx,captureCtx,job,temporary,final,incomplete,publishRecordingLeased)
}

// This joins publication, checkpoint and terminal retention, NOT cleanup authority.
// Terminal release still retains every job. Registry/complete-capture eligibility
// and checkpoint-last/input/root release remain separate integration work.
func (bm *BufferManager) publishEventRecordingWith(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool,publish func(context.Context,*mediaMutationLease,string,string)error) error {
 if job==nil || publish==nil || job.recoveryBorrow!=nil {return errPublicationOwnership}
 job.keepStaging=true
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return err}
 transferred:=false
 defer func(){if !transferred {job.recoveryBorrow=nil;lease.release()}}()
 if job.inventory!=nil {
  if job.inventory.path!=job.dir {return errStagingInventory}
  if err:=job.inventory.check(ctx,lease);err!=nil {return err}
 }
 job.recoveryBorrow=&eventRecoveryBorrow{ctx:ctx,lease:lease}
 if err:=publish(ctx,lease,temporary,final);err!=nil {return err}
 // A readable partial remains distinct from a complete capture. Preserve the
 // existing successful-publication return even if its checkpoint fails.
 checkpointErr:=job.recoveryCheckpoint("published","")
 job.keepStaging=incomplete || captureCtx.Err()!=nil || bm.ctx.Err()!=nil || checkpointErr!=nil || job.recoveryFailed
 // Worker terminal release owns this same lease from here, even if the
 // published checkpoint failed. No further capture/concat precedes that release.
 job.recoveryBorrow.terminal=true
 transferred=true
 return nil
}
