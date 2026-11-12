package main

import (
 "context"
 "encoding/json"
 "net/http"
 "sort"
 "time"
)

type publishedDeletionTarget struct {id,dir string;camera *Camera}
type publishedDeletionOperation func(context.Context,*mediaMutationLease,string,func() error) (publishedDeleteResult,error)

func deleteAllPublishedLeased(ctx context.Context,lease *mediaMutationLease,dir string,guard func() error) (publishedDeleteResult,error) {
 return deletePublishedLeased(ctx,lease,dir,func(string,time.Time) bool{return true},guard)
}

// Sole media-admission layer for this explicit deletion path. The operation
// borrows the lease and guard; it must not acquire media or recordingControlMu.
func deleteRegisteredPublished(ctx context.Context,n *NVR,target publishedDeletionTarget,operation publishedDeletionOperation) (publishedDeleteResult,error) {
 refused:=publishedDeleteResult{Failures:1}
 if operation==nil {return refused,errPublishedDeletionRefused}
 if err:=ctx.Err();err!=nil {return refused,err}
 if n.shuttingDown.Load() {return refused,errRecorderShuttingDown}
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return refused,err}
 defer lease.release()
 guard:=func() error {
  if err:=ctx.Err();err!=nil {return err}
  if n.shuttingDown.Load() {return errRecorderShuttingDown}
  n.mu.RLock()
  same:=n.cameras[target.id]==target.camera && target.camera!=nil && target.camera.ID==target.id && target.camera.RecordDir==target.dir
  n.mu.RUnlock()
  if !same {return errRecordingStateChanged}
  return nil
 }
 if err:=guard();err!=nil {return refused,err}
 result,err:=operation(ctx,lease,target.dir,guard)
 if err!=nil && result.Failures==0 {result.Failures=1}
 return result,err
}

func serveManagedPublishedDeletion(w http.ResponseWriter,r *http.Request,n *NVR,operation publishedDeletionOperation) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 // recordingControlMu -> media admission -> brief registry checks. Workers
 // must never wait for recordingControlMu while holding media ownership.
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 if n.shuttingDown.Load() {recordingError(w,errRecorderShuttingDown);return}
 n.mu.RLock()
 targets:=make([]publishedDeletionTarget,0,len(n.cameras))
 for id,cam:=range n.cameras {if cam!=nil {targets=append(targets,publishedDeletionTarget{id,cam.RecordDir,cam})}}
 n.mu.RUnlock()
 sort.Slice(targets,func(i,j int) bool{return targets[i].id<targets[j].id})
 var result publishedDeleteResult
 if operation==nil {result.Failures=1} else {
  for _,target:=range targets {
   if r.Context().Err()!=nil || n.shuttingDown.Load() {result.Failures++;break}
   next,_:=deleteRegisteredPublished(r.Context(),n,target,operation)
   result.Count+=next.Count;result.Bytes+=next.Bytes;result.Failures+=next.Failures
   if r.Context().Err()!=nil || n.shuttingDown.Load() {break}
  }
 }
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]interface{}{"success":result.Failures==0,"deleted":result.Count,"size_mb":float64(result.Bytes)/(1024*1024),"failures":result.Failures})
}
