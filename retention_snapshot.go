package main

import (
 "context"
 "sync"
 "time"
)

// Always acquire this lock before NVR.mu when both are needed. A cleanup
// pass holds a read lock while API retention mutations take the write lock.
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
 if ctx.Err()!=nil || remove==nil {return false}
 retentionPolicyMu.RLock();defer retentionPolicyMu.RUnlock()
 if ctx.Err()!=nil {return false}
 n.mu.RLock()
 cam:=n.cameras[target.id]
 valid:=cam!=nil && cam==target.camera && cam.ID==target.id && cam.RecordDir==target.dir && cam.RetentionDays==target.days
 if valid {duration,err:=cameraRetentionDuration(cam.RetentionDays);valid=err==nil && duration==target.duration}
 n.mu.RUnlock()
 if !valid || ctx.Err()!=nil {return false}
 // The registry lock is released before I/O; retention writers stay excluded
// until this pass returns. Camera deletion is still an explicit destructive action.
 remove(ctx,target.dir,target.duration)
 if ctx.Err()==nil {cleanupTempFiles(target.dir)}
 return true
}
