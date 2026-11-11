package main

import (
 "context"
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
)

type publishedDeleteResult struct {Count int;Bytes int64;Failures int}

// Existing explicit deletion scope: top-level published MP4s, never staging.
// Lstat skips stable symlinks/special entries; pathname races are not eliminated.
func deletePublishedRecordings(ctx context.Context,dir string) publishedDeleteResult {
 return deletePublishedRecordingsWithRemove(ctx,dir,os.Remove)
}
func deletePublishedRecordingsWithRemove(ctx context.Context,dir string,remove func(string) error) publishedDeleteResult {
 var result publishedDeleteResult
 if ctx.Err()!=nil || remove==nil {result.Failures++;return result}
 entries,err:=os.ReadDir(dir)
 if err!=nil {result.Failures++;return result}
 for _,entry:=range entries {
  if filepath.Ext(entry.Name())!=".mp4" {continue}
  file:=filepath.Join(dir,entry.Name())
  if ctx.Err()!=nil {result.Failures++;break}
  info,err:=os.Lstat(file)
  if err!=nil {if !os.IsNotExist(err) {result.Failures++};continue}
  if !info.Mode().IsRegular() {continue}
  if ctx.Err()!=nil {result.Failures++;break}
  if err:=remove(file);err!=nil {if !os.IsNotExist(err) {result.Failures++};continue}
  result.Count++;result.Bytes+=info.Size()
 }
 return result
}

func serveDeletePublishedRecordings(w http.ResponseWriter,r *http.Request,n *NVR,remove func(context.Context,string) publishedDeleteResult) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 // Lock order remains recordingControlMu -> NVR.mu. Control serialization
 // is retained across I/O, but registry readers/writers are not held hostage.
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 if n.shuttingDown.Load() {recordingError(w,errRecorderShuttingDown);return}
 n.mu.RLock()
 dirs:=make([]string,0,len(n.cameras))
 for _,cam:=range n.cameras {if cam!=nil {dirs=append(dirs,cam.RecordDir)}}
 n.mu.RUnlock()
 var result publishedDeleteResult
 for _,dir:=range dirs {
  if r.Context().Err()!=nil || n.shuttingDown.Load() || remove==nil {result.Failures++;break}
  next:=remove(r.Context(),dir)
  result.Count+=next.Count;result.Bytes+=next.Bytes;result.Failures+=next.Failures
 }
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]interface{}{"success":result.Failures==0,"deleted":result.Count,"size_mb":float64(result.Bytes)/(1024*1024),"failures":result.Failures})
}
