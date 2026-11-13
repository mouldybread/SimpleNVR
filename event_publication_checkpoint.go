package main

import "context"

// Scoped to one serialized synchronous finalization call. Never retained across
// capture/concat execution or terminal teardown; a borrower never releases it.
type eventRecoveryBorrow struct {ctx context.Context;lease *mediaMutationLease}

func (bm *BufferManager) publishEventRecording(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool) error {
 return bm.publishEventRecordingWith(ctx,captureCtx,job,temporary,final,incomplete,publishRecordingLeased)
}

// This joins publication and the published checkpoint, NOT cleanup authority.
// Terminal release still retains every job. Registry/complete-capture eligibility
// and checkpoint-last/input/root release remain separate integration work.
func (bm *BufferManager) publishEventRecordingWith(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool,publish func(context.Context,*mediaMutationLease,string,string)error) error {
 if job==nil || publish==nil || job.recoveryBorrow!=nil {return errPublicationOwnership}
 job.keepStaging=true
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return err}
 defer lease.release()
 if job.inventory!=nil {
  if job.inventory.path!=job.dir {return errStagingInventory}
  if err:=job.inventory.check(ctx,lease);err!=nil {return err}
 }
 job.recoveryBorrow=&eventRecoveryBorrow{ctx,lease}
 defer func(){job.recoveryBorrow=nil}()
 if err:=publish(ctx,lease,temporary,final);err!=nil {return err}
 // A readable partial remains distinct from a complete capture. Preserve the
 // existing successful-publication return even if its checkpoint fails.
 checkpointErr:=job.recoveryCheckpoint("published","")
 job.keepStaging=incomplete || captureCtx.Err()!=nil || bm.ctx.Err()!=nil || checkpointErr!=nil || job.recoveryFailed
 return nil
}
