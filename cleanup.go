package main

import (
 "context"
 "encoding/json"
 "io"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"
)

type orphanCleanupResult struct {
 Count int `json:"count"`
 Bytes int64 `json:"bytes"`
 SkippedRecoveryDirs int `json:"skipped_recovery_dirs"`
 Failures int `json:"failures"`
}

// Preserve all root .tmp entries, including empty files: age and size cannot
// establish whether a recorder owns a file. Event staging is also opaque here.
// An inspection error fails closed rather than treating a directory as empty.
func recoveryArtifactsPresent(dir string) (bool,error) {
 info,err:=os.Lstat(dir)
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 entries,err:=os.ReadDir(dir)
 if err!=nil {return true,err}
 for _,entry:=range entries {if strings.HasSuffix(entry.Name(),".tmp") {return true,nil}}
 staging:=filepath.Join(dir,"temp")
 info,err=os.Lstat(staging)
 if os.IsNotExist(err) {return false,nil}
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 f,err:=os.Open(staging)
 if err!=nil {return true,err}
 defer f.Close()
 names,err:=f.Readdirnames(1)
 if len(names)>0 {return true,nil}
 if err==io.EOF {return false,nil}
 return true,err
}

func cleanupOrphanedAt(root string,validDirs map[string]bool) (orphanCleanupResult,error) {
 var result orphanCleanupResult
 entries,err:=os.ReadDir(root)
 if err!=nil {return result,err}
 for _,entry:=range entries {
  if !entry.IsDir() || validDirs[entry.Name()] {continue}
  dir:=filepath.Join(root,entry.Name())
  retained,err:=recoveryArtifactsPresent(dir)
  if retained || err!=nil {
   result.SkippedRecoveryDirs++
   if err!=nil {result.Failures++}
   continue
  }
  files,err:=filepath.Glob(filepath.Join(dir,"*.mp4"))
  if err!=nil {result.Failures++;continue}
  for _,file:=range files {
   info,err:=os.Lstat(file)
   if err!=nil {if !os.IsNotExist(err) {result.Failures++};continue}
   if !info.Mode().IsRegular() {continue}
   if err:=os.Remove(file);err!=nil {result.Failures++;continue}
   result.Count++;result.Bytes+=info.Size()
  }
  // Never recursively remove directories or their opaque contents.
  _=os.Remove(dir)
 }
 return result,nil
}

func cleanupOrphanedRecordingsHandler(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 // Serialize with recorder mode changes and camera deletion. This does not
 // make arbitrary external filesystem edits safe during cleanup.
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 nvr.mu.RLock()
 validDirs:=make(map[string]bool,len(nvr.cameras))
 for _,cam:=range nvr.cameras {validDirs[filepath.Base(cam.RecordDir)]=true}
 nvr.mu.RUnlock()
 result,err:=cleanupOrphanedAt("/recordings",validDirs)
 if err!=nil {http.Error(w,"Failed to read recordings directory",500);return}
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]interface{}{
  "success":result.Failures==0,"count":result.Count,"size_mb":float64(result.Bytes)/(1024*1024),
  "skipped_recovery_dirs":result.SkippedRecoveryDirs,"failures":result.Failures,
 })
}

func cleanupEvents() { cleanupEventsFor(context.Background(),nvr) }

func cleanupEventsFor(ctx context.Context,n *NVR) {
 if ctx.Err()!=nil {return}
 n.mu.Lock()
 if ctx.Err()!=nil {n.mu.Unlock();return}
 retentionMap:=make(map[string]time.Duration)
 for _,cam:=range n.cameras {
  days:=cam.RetentionDays;if days==0 {days=7}
  retentionMap[cam.Name]=time.Duration(days)*24*time.Hour
 }
 now:=time.Now().UTC()
 var validEvents []Event
 deletedCount:=0
 for _,event:=range n.events {
  if ctx.Err()!=nil {n.mu.Unlock();return}
  retention,exists:=retentionMap[event.CameraName]
  if !exists || event.StartTime.Before(now.Add(-retention)) {deletedCount++;continue}
  validEvents=append(validEvents,event)
 }
 eventsChanged:=len(validEvents)!=len(n.events)
 n.events=validEvents
 n.mu.Unlock()
 if eventsChanged {
  log.Printf("Cleaned up %d old/orphaned events from memory",deletedCount)
  if err:=n.save();err!=nil {log.Printf("Failed to save events after cleanup: %v",err)}
 }
}

func cleanupRoutine(ctx context.Context,n *NVR) {
 ticker:=time.NewTicker(5*time.Minute);defer ticker.Stop()
 log.Println("Cleanup routine started")
 runCleanupTicks(ctx,n,ticker.C)
}

func runCleanupTicks(ctx context.Context,n *NVR,ticks <-chan time.Time) {
 for {
  select {
  case <-ctx.Done():return
  case _,ok:=<-ticks:
   if !ok || ctx.Err()!=nil {return}
   cleanupEventsFor(ctx,n)
   if ctx.Err()!=nil {return}
   type target struct {dir string;retention time.Duration}
   n.mu.RLock()
   targets:=make([]target,0,len(n.cameras))
   for _,cam:=range n.cameras {
    days:=cam.RetentionDays;if days==0 {days=7}
    targets=append(targets,target{cam.RecordDir,time.Duration(days)*24*time.Hour})
   }
   n.mu.RUnlock()
   // Filesystem work must not hold the camera-registry lock. These are
   // per-cycle snapshots, not authority to remove staging or recurse.
   for _,item:=range targets {
    if ctx.Err()!=nil {return}
    cleanupOldRecordingsContext(ctx,item.dir,item.retention)
    if ctx.Err()!=nil {return}
    cleanupTempFiles(item.dir)
   }
  }
 }
}

// Published .mp4 files, including published _partial.mp4 files, remain subject
// to the existing camera retention policy. Opaque staging is never traversed.
func cleanupOldRecordings(dir string,retention time.Duration) {
 cleanupOldRecordingsContext(context.Background(),dir,retention)
}

func cleanupOldRecordingsContext(ctx context.Context,dir string,retention time.Duration) {
 if ctx.Err()!=nil {return}
 files,err:=filepath.Glob(filepath.Join(dir,"*.mp4"))
 if err!=nil {return}
 cutoff:=time.Now().UTC().Add(-retention)
 for _,file:=range files {
  if ctx.Err()!=nil {return}
  name:=filepath.Base(file)
  if len(name)<15 {continue}
  info,err:=os.Lstat(file)
  if err!=nil || !info.Mode().IsRegular() {continue}
  start,err:=time.ParseInLocation("20060102_150405",name[:15],time.UTC)
  if err!=nil {start=info.ModTime().UTC()}
  if !start.Before(cutoff) {continue}
  if ctx.Err()!=nil {return}
  if err:=os.Remove(file);err!=nil {log.Printf("Unable to remove expired recording %s",name)}
 }
}

// Kept as a compatibility boundary for existing callers. A .tmp file can be
// active or retained recovery evidence; neither age nor zero length is proof
// that deletion is safe. Only its owning recorder may remove an empty output.
func cleanupTempFiles(_ string) {}
