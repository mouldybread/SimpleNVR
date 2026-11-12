package main

import (
 "context"
 "log"
 "sync"
 "time"
)

// Lock order: media ownership -> retentionPolicyMu -> NVR.mu. Policy
// writers do not acquire media ownership. Cleanup holds a policy read lock
// across deletion, but never holds NVR.mu across admission or filesystem I/O.
var retentionPolicyMu sync.RWMutex

func setCameraRetention(n *NVR,id string,days int) error {
 if days<1 || days>maxCameraRetentionDays {return errCameraRetention}
 retentionPolicyMu.Lock();defer retentionPolicyMu.Unlock()
 n.mu.Lock();defer n.mu.Unlock()
 cam:=n.cameras[id];if cam==nil {return errCameraMissing}
 cam.RetentionDays=days
 return nil
}

func setAllCameraRetention(n *NVR,days int) error {
 if days<1 || days>maxCameraRetentionDays {return errCameraRetention}
 retentionPolicyMu.Lock();defer retentionPolicyMu.Unlock()
 n.mu.Lock();defer n.mu.Unlock()
 for _,cam:=range n.cameras {if cam!=nil {cam.RetentionDays=days}}
 return nil
}

type retentionCleanupSnapshot struct {
 id string
 camera *Camera
 dir string
 days int
 duration time.Duration
}

func snapshotRetentionCleanup(n *NVR) []retentionCleanupSnapshot {
 n.mu.RLock();defer n.mu.RUnlock()
 targets:=make([]retentionCleanupSnapshot,0,len(n.cameras))
 for id,cam:=range n.cameras {
  if cam==nil {continue}
  duration,err:=cameraRetentionDuration(cam.RetentionDays);if err!=nil {continue}
  targets=append(targets,retentionCleanupSnapshot{id,cam,cam.RecordDir,cam.RetentionDays,duration})
 }
 return targets
}

func cleanupRetentionSnapshot(ctx context.Context,n *NVR,target retentionCleanupSnapshot) bool {
 return admitRetentionDeletion(ctx,n,target,recordingMediaOwnership,cleanupExpiredPublishedLeased)
}

// The injected function lets tests hold the deletion phase without timing
// filesystem operations. true means the validated pass was invoked, not that
// any file was removed or every filesystem operation succeeded.
func cleanupRetentionSnapshotWith(ctx context.Context,n *NVR,target retentionCleanupSnapshot,remove func(context.Context,string,time.Duration)) bool {
 return cleanupRetentionSnapshotUsingGate(ctx,n,target,recordingMediaOwnership,remove)
}

type retentionDeletionOperation func(context.Context,*mediaMutationLease,string,time.Duration,func() error) error

// Keep existing void-callback test seams; production uses the leased operation.
func retentionRemovalAdapter(remove func(context.Context,string,time.Duration)) retentionDeletionOperation {
 if remove==nil {return nil}
 return func(ctx context.Context,_ *mediaMutationLease,dir string,duration time.Duration,guard func() error) error {
  if err:=guard();err!=nil {return err}
  remove(ctx,dir,duration);return nil
 }
}
func cleanupRetentionSnapshotUsingGate(ctx context.Context,n *NVR,target retentionCleanupSnapshot,gate *mediaMutationGate,remove func(context.Context,string,time.Duration)) bool {
 return admitRetentionDeletion(ctx,n,target,gate,retentionRemovalAdapter(remove))
}
func cleanupRetentionSnapshotLeased(ctx context.Context,n *NVR,target retentionCleanupSnapshot,lease *mediaMutationLease,remove func(context.Context,string,time.Duration)) bool {
 return runRetentionDeletionLeased(ctx,n,target,lease,retentionRemovalAdapter(remove))
}

// Sole media acquisition layer. Never wait while holding policy or registry locks.
func admitRetentionDeletion(ctx context.Context,n *NVR,target retentionCleanupSnapshot,gate *mediaMutationGate,remove retentionDeletionOperation) bool {
 if ctx.Err()!=nil || remove==nil || gate==nil || n.shuttingDown.Load() {return false}
 lease,err:=gate.acquire(ctx);if err!=nil {return false}
 defer lease.release()
 return runRetentionDeletionLeased(ctx,n,target,lease,remove)
}

func runRetentionDeletionLeased(ctx context.Context,n *NVR,target retentionCleanupSnapshot,lease *mediaMutationLease,remove retentionDeletionOperation) bool {
 if !lease.held() || remove==nil || ctx.Err()!=nil || n.shuttingDown.Load() {return false}
 retentionPolicyMu.RLock();defer retentionPolicyMu.RUnlock()
 // This guard borrows the policy read lock; never reacquire it from the engine.
 guard:=func() error {
  if err:=ctx.Err();err!=nil {return err}
  if n.shuttingDown.Load() {return errRecorderShuttingDown}
  n.mu.RLock()
  cam:=n.cameras[target.id]
  valid:=cam!=nil && cam==target.camera && cam.ID==target.id && cam.RecordDir==target.dir && cam.RetentionDays==target.days
  if valid {duration,err:=cameraRetentionDuration(cam.RetentionDays);valid=err==nil && duration==target.duration}
  n.mu.RUnlock()
  if !valid {return errRecordingStateChanged}
  if err:=ctx.Err();err!=nil {return err}
  if n.shuttingDown.Load() {return errRecorderShuttingDown}
  return nil
 }
 if err:=guard();err!=nil {return false}
 if err:=remove(ctx,lease,target.dir,target.duration,guard);err!=nil {log.Printf("Retention deletion incomplete for camera %s; inspect media",target.id)}
 if ctx.Err()==nil {cleanupTempFiles(target.dir)}
 // true still means invoked, not complete filesystem success.
 return true
}
