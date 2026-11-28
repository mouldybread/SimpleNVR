package main

import (
 "context"
 "log"
 "path/filepath"
)

// The managed worker owns Done throughout setup retries and normal recording.
// A setup failure is not a user stop and must not alter persisted intent.
// Per-call dependencies let lifecycle tests avoid storage and media processes.
func recordCameraEventBasedWithSetup(ctx context.Context,n *NVR,cam *Camera,ensure func(string)error,retry func(context.Context)bool,run func(*BufferManager)) {
 defer cam.recordWG.Done()
 bm:=newBufferManager(ctx,cam)
 defer bm.cancel()
 for {
  if ctx.Err()!=nil {return}
  ready:=true
  for _,dir:=range []string{filepath.Join(cam.RecordDir,"buffer"),filepath.Join(cam.RecordDir,"temp")} {
   if ctx.Err()!=nil {return}
   if err:=ensure(dir);err!=nil {
    log.Printf("Unable to initialize event storage for camera %s; recording intent retained, retrying",cam.ID)
    ready=false
    break
   }
  }
  if ready {break}
  if !retry(ctx) {return}
 }
 n.mu.Lock()
 if ctx.Err()!=nil || n.shuttingDown.Load() || n.cameras[cam.ID]!=cam {n.mu.Unlock();return}
 cam.bufferManager=bm
 n.mu.Unlock()
 defer func(){n.mu.Lock();if cam.bufferManager==bm {cam.bufferManager=nil};n.mu.Unlock()}()
 run(bm)
}
