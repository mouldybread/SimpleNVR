package main

import (
 "context"
 "log"
 "os"
 "path/filepath"
 "time"
)

func cleanupEvents() { cleanupEventsFor(context.Background(),nvr) }

func cleanupEventsFor(ctx context.Context,n *NVR) {
 cleanupEventsForWithSave(ctx,n,n.save)
}

// Persistence injection keeps expiry tests isolated from the real config path.
func cleanupEventsForWithSave(ctx context.Context,n *NVR,persist func() error) {
 if ctx.Err()!=nil {return}
 n.mu.Lock()
 if ctx.Err()!=nil {n.mu.Unlock();return}
 retentionMap:=make(map[string]time.Duration)
 invalidRetention:=make(map[string]bool)
 for _,cam:=range n.cameras {
  retention,err:=cameraRetentionDuration(cam.RetentionDays)
  if err!=nil {invalidRetention[cam.Name]=true;log.Printf("Skipping invalid retention policy for camera %s",cam.ID);continue}
  // Events lack camera IDs. Do not choose a shorter policy based on map
  // iteration order when several registered cameras share the same name.
  if retention>retentionMap[cam.Name] {retentionMap[cam.Name]=retention}
 }
 now:=time.Now().UTC()
 var validEvents []Event
 deletedCount:=0
 for _,event:=range n.events {
  if ctx.Err()!=nil {n.mu.Unlock();return}
  // An invalid policy is not equivalent to a missing camera. Preserve its
  // events, including ambiguous names shared with another camera.
  if invalidRetention[event.CameraName] {validEvents=append(validEvents,event);continue}
  retention,exists:=retentionMap[event.CameraName]
  if !exists || event.StartTime.Before(now.Add(-retention)) {deletedCount++;continue}
  validEvents=append(validEvents,event)
 }
 eventsChanged:=len(validEvents)!=len(n.events)
 n.events=validEvents
 n.mu.Unlock()
 if eventsChanged {
  log.Printf("Cleaned up %d old/orphaned events from memory",deletedCount)
  if err:=persist();err!=nil {log.Printf("Failed to save events after cleanup: %v",err)}
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
   for _,target:=range snapshotRetentionCleanup(n) {
    if ctx.Err()!=nil {return}
    cleanupRetentionSnapshot(ctx,n,target)
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
 if ctx.Err()!=nil || retention<=0 {return}
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
