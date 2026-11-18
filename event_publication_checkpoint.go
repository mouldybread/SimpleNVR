package main

import (
 "context"
 "errors"
 "os"
)

// Acquisition ownership may transfer to terminal release, never to a child or
// goroutine. Checkpoint writers only borrow. The job owner must call releaseJob
// after finalization returns, including on a published-checkpoint failure.
type eventRecoveryBorrow struct {ctx context.Context;lease *mediaMutationLease;terminal bool;output *eventPublishedOutput;revalidation *eventReleaseRevalidation;revalidationErr error;duration *eventDurationEvidence;durationErr error}

func (bm *BufferManager) publishEventRecording(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool) error {
 return bm.publishEventRecordingWith(ctx,captureCtx,job,temporary,final,incomplete,func(ctx context.Context,lease *mediaMutationLease,from,to string)error{
  err:=publishRecordingObservedLeased(ctx,lease,from,to,func(file,directory *os.File)error{
   borrowed:=job.recoveryBorrow
   if borrowed==nil || borrowed.lease!=lease || borrowed.output!=nil {return errPublicationOwnership}
   output,err:=retainEventPublishedOutput(ctx,lease,to,file,directory);if err!=nil {return err}
   borrowed.output=output
   return nil
  })
  // Only the actual durable publisher returning success can supply this witness;
  // the generic injected publication callback cannot substitute for it.
  if err==nil {job.execution.published(bm,job,ctx)}
  return err
 })
}

// This joins publication, checkpoint and terminal retention, NOT cleanup authority.
// Terminal release still retains every job. Registry/complete-capture eligibility
// and checkpoint-last/input/root release remain separate integration work.
func (bm *BufferManager) publishEventRecordingWith(ctx,captureCtx context.Context,job *bufferEvent,temporary,final string,incomplete bool,publish func(context.Context,*mediaMutationLease,string,string)error) (resultErr error) {
 if job==nil || publish==nil || job.recoveryBorrow!=nil {return errPublicationOwnership}
 job.keepStaging=true
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return err}
 transferred:=false
 defer func(){if !transferred {resultErr=errors.Join(resultErr,job.closePublishedOutput());job.recoveryBorrow=nil;lease.release()}}()
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
 // Necessary live/evidence checks only: complete-capture authorization is
 // still absent, so a successful revalidation never enables deletion.
 borrowed:=job.recoveryBorrow
 job.execution.finishAssembly(bm,job,ctx,captureCtx,incomplete,checkpointErr)
 // Transfer the SAME acquisition lease before combined duration/presentation inspection.
 // Even a failed published checkpoint still hands ownership to terminal release.
 borrowed.terminal=true
 transferred=true
 borrowed.durationErr=errEventDurationEvidence
 if job.execution.qualifies(bm,job) {
  // The collector's existing aggregate budget is rooted in the live manager,
  // NOT the publication/terminal metadata budget. No checkpoint is written here.
  borrowed.duration,borrowed.durationErr=inspectEventDurationPresentationEvidence(bm.ctx,lease,bm,job)
 }
 borrowed.revalidation=newEventReleaseRevalidation(nvr,bm,job,incomplete || captureCtx.Err()!=nil || checkpointErr!=nil || job.recoveryFailed)
 borrowed.revalidationErr=borrowed.revalidation.check(ctx,lease)
 return nil
}
