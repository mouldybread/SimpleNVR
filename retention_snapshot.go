package main

import (
 "context"
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
 return cleanupRetentionSnapshotWith(ctx,n,target,cleanupOldRecordingsContext)
}

// The injected function lets tests hold the deletion phase without timing
// filesystem operations. true means the validated pass was invoked, not that
// any file was removed or every filesystem operation succeeded.
func cleanupRetentionSnapshotWith(ctx context.Context,n *NVR,target retentionCleanupSnapshot,remove func(context.Context,string,time.Duration)) bool {
 return cleanupRetentionSnapshotUsingGate(ctx,n,target,recordingMediaOwnership,remove)
}

// This is the sole admission layer for managed retention. The callback borrows
// ownership and must not acquire the gate again. Injection uses the same path.
func cleanupRetentionSnapshotUsingGate(ctx context.Context,n *NVR,target retentionCleanupSnapshot,gate *mediaMutationGate,remove func(context.Context,string,time.Duration)) bool {
 if ctx.Err()!=nil || remove==nil || gate==nil || n.shuttingDown.Load() {return false}
 lease,err:=gate.acquire(ctx);if err!=nil {return false}
 defer lease.release()
 return cleanupRetentionSnapshotLeased(ctx,n,target,lease,remove)
}

// Receiving a lease avoids hidden reentrant acquisition. Registry identity,
// path, policy, shutdown and cancellation are revalidated AFTER admission.
func cleanupRetentionSnapshotLeased(ctx context.Context,n *NVR,target retentionCleanupSnapshot,lease *mediaMutationLease,remove func(context.Context,string,time.Duration)) bool {
 if !lease.held() || remove==nil || ctx.Err()!=nil || n.shuttingDown.Load() {return false}
 retentionPolicyMu.RLock();defer retentionPolicyMu.RUnlock()
 if ctx.Err()!=nil || n.shuttingDown.Load() {return false}
 n.mu.RLock()
 cam:=n.cameras[target.id]
 valid:=cam!=nil && cam==target.camera && cam.ID==target.id && cam.RecordDir==target.dir && cam.RetentionDays==target.days
 if valid {duration,err:=cameraRetentionDuration(cam.RetentionDays);valid=err==nil && duration==target.duration}
 n.mu.RUnlock()
 if !valid || ctx.Err()!=nil || n.shuttingDown.Load() {return false}
 // Registry access remains available during I/O. Policy writers stay excluded
 // until this pass returns; the acquisition layer retains media ownership.
 remove(ctx,target.dir,target.duration)
 if ctx.Err()==nil {cleanupTempFiles(target.dir)}
 return true
}
