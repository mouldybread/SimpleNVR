package main

import (
 "context"
 "encoding/csv"
 "errors"
 "fmt"
 "io"
 "log"
 "math"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"
)

const (bufferDuration=30*time.Second; segmentSize=5*time.Second; eventWorkers=2; maxEventSeconds=3600)
var (errEventStopped=errors.New("event recorder stopping or unavailable");errEventBusy=errors.New("event recorder at capacity");errEventInvalid=errors.New("invalid event capture request"))

type BufferSegment struct {StartTime time.Time; EndTime time.Time; FilePath string}
type bufferEvent struct {event Event;preFiles []string;preStart time.Time;dir string;keepStaging bool;recovery *eventRecoveryManifest;recoveryFailed bool;recoveryWriter func(string,[]byte) error;inventory *ownedStagingInventory;recoveryBorrow *eventRecoveryBorrow;execution *eventExecutionProvenance}
type BufferManager struct {
 camera *Camera
 buffer []BufferSegment
 bufferMutex sync.RWMutex
 ctx context.Context
 cancel context.CancelFunc
 queueMu sync.Mutex
 stopping bool
 eventChan chan *bufferEvent
 slots chan struct{}
}

func newBufferManager(ctx context.Context,cam *Camera) *BufferManager {
 child,cancel:=context.WithCancel(ctx)
 return &BufferManager{camera:cam,ctx:child,cancel:cancel,eventChan:make(chan *bufferEvent,eventWorkers),slots:make(chan struct{},eventWorkers)}
}

func recordCamera(ctx context.Context,cam *Camera) {
 defer cam.recordWG.Done()
 for ctx.Err()==nil {
  if err:=recordSegment(ctx,cam,600);err!=nil && ctx.Err()==nil {log.Printf("Recording error for camera %s: %v",cam.ID,err);if !waitRecordingRetry(ctx) {return}}
 }
}

func waitRecordingRetry(ctx context.Context) bool {
 timer:=time.NewTimer(5*time.Second);defer timer.Stop()
 select {case <-ctx.Done():return false;case <-timer.C:return true}
}

func recordCameraEventBased(ctx context.Context,cam *Camera) {
 defer cam.recordWG.Done()
 bm:=newBufferManager(ctx,cam)
 defer bm.cancel()
 for _,dir:=range []string{filepath.Join(cam.RecordDir,"buffer"),filepath.Join(cam.RecordDir,"temp")} {
  if err:=ensureEventStorageDirectory(dir);err!=nil {
   log.Printf("Unable to initialize event storage for camera %s",cam.ID)
   nvr.mu.Lock();cam.IsRecording=false;nvr.mu.Unlock();return
  }
 }
 nvr.mu.Lock();cam.bufferManager=bm;nvr.mu.Unlock()
 defer func(){nvr.mu.Lock();if cam.bufferManager==bm {cam.bufferManager=nil};nvr.mu.Unlock()}()
 bm.runWorkers([]func(){bm.segmentBuffer,bm.pruneLoop},bm.handleEvent)
 bm.cleanupBuffer()
}

// All Add calls finish before Wait. Event workers never spawn untracked work.
// Unprocessed jobs retain their recovery checkpoint even without prebuffer pins.
func (bm *BufferManager) runWorkers(tasks []func(),handle func(*bufferEvent)) {
 var children sync.WaitGroup
 children.Add(len(tasks)+eventWorkers)
 for _,task:=range tasks {go func(task func()){defer children.Done();task()}(task)}
 for i:=0;i<eventWorkers;i++ {go func(){
  defer children.Done()
  for {select {
  case <-bm.ctx.Done():return
  case job:=<-bm.eventChan:
   func(){defer bm.releaseJob(job);if bm.ctx.Err()==nil {handle(job)} else {job.keepStaging=true}}()
  }}
 }()}
 <-bm.ctx.Done()
 bm.queueMu.Lock();bm.stopping=true;bm.cancel();bm.queueMu.Unlock()
 children.Wait()
 for {select {case job:=<-bm.eventChan:job.keepStaging=true;bm.releaseJob(job);default:return}}
}

func (bm *BufferManager) TriggerEvent(event Event) error {
 bm.queueMu.Lock();defer bm.queueMu.Unlock()
 if bm.stopping || bm.ctx.Err()!=nil {return errEventStopped}
 if event.ID=="" || event.Duration<1 || event.Duration>maxEventSeconds {return errEventInvalid}
 select {case bm.slots<-struct{}{}:default:return errEventBusy}
 job,err:=bm.pinBuffer(event)
 if err!=nil {<-bm.slots;return fmt.Errorf("pin completed prebuffer: %w",err)}
 select {
 case bm.eventChan<-job:return nil
 case <-bm.ctx.Done():job.keepStaging=true;bm.releaseJob(job);return errEventStopped
 default:job.keepStaging=true;bm.releaseJob(job);return errEventBusy
 }
}

func (bm *BufferManager) pinBuffer(event Event) (*bufferEvent,error) {return bm.pinOwnedBuffer(event)}

func (bm *BufferManager) releaseJob(job *bufferEvent) {bm.releaseOwnedJob(job)}

func (bm *BufferManager) segmentBuffer() {
 dir:=filepath.Join(bm.camera.RecordDir,"buffer")
 for bm.ctx.Err()==nil {
  nonce:=generateID("buffer",time.Now())
  pattern:="buffer_%Y%m%d_%H%M%S_"+nonce+".mp4"
  cmd:=ffmpegCommand(bm.ctx,"-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-i",bm.camera.RTSPUrl,"-c","copy","-f","segment","-segment_time","5","-segment_format","mp4","-reset_timestamps","1","-strftime","1","-segment_list","pipe:1","-segment_list_type","csv","-segment_list_size","0",pattern)
  cmd.Dir=dir;cmd.Env=append(os.Environ(),"TZ=UTC")
  stdout,err:=cmd.StdoutPipe()
  if err!=nil {if !waitRecordingRetry(bm.ctx) {return};continue}
  if err=cmd.Start();err!=nil {stdout.Close();if !waitRecordingRetry(bm.ctx) {return};continue}
  reader:=csv.NewReader(stdout);reader.FieldsPerRecord=-1
  for {
   row,readErr:=reader.Read()
   if readErr==io.EOF {break}
   if readErr!=nil {if bm.ctx.Err()!=nil {break};continue}
   if len(row)<3 {continue}
   base:=filepath.Base(row[0])
   if !strings.HasPrefix(base,"buffer_") || !strings.HasSuffix(base,"_"+nonce+".mp4") || len(base)<22 {continue}
   start,parseErr:=time.Parse("20060102_150405",base[7:22]);if parseErr!=nil {continue}
   begin,e1:=strconv.ParseFloat(row[1],64);end,e2:=strconv.ParseFloat(row[2],64)
   duration:=end-begin
   if e1!=nil || e2!=nil || math.IsNaN(duration) || math.IsInf(duration,0) || duration<=0 || duration>maxEventSeconds {continue}
   bm.registerCompleted(filepath.Join(dir,base),start,start.Add(time.Duration(duration*float64(time.Second))))
  }
  err=cmd.Wait();stdout.Close()
  bm.discardUnclosed(filepath.Join(dir,"*_"+nonce+".mp4"))
  if bm.ctx.Err()!=nil {return}
  log.Printf("Buffer segmenter exited for camera %s: %v",bm.camera.ID,err)
  if !waitRecordingRetry(bm.ctx) {return}
 }
}

func (bm *BufferManager) registerCompleted(path string,start,end time.Time) {
 info,err:=os.Lstat(path);if err!=nil || !info.Mode().IsRegular() || info.Size()<1000 {return}
 bm.bufferMutex.Lock();defer bm.bufferMutex.Unlock()
 bm.buffer=append(bm.buffer,BufferSegment{StartTime:start,EndTime:end,FilePath:path})
 sort.Slice(bm.buffer,func(i,j int) bool {return bm.buffer[i].StartTime.Before(bm.buffer[j].StartTime)})
}

func (bm *BufferManager) discardUnclosed(pattern string) {
 bm.bufferMutex.Lock();defer bm.bufferMutex.Unlock()
 closed:=make(map[string]bool)
 for _,seg:=range bm.buffer {closed[seg.FilePath]=true}
 files,err:=filepath.Glob(pattern)
 if err!=nil {log.Printf("Unable to enumerate unclosed prebuffer for camera %s; staging retained",bm.camera.ID);return}
 for _,file:=range files {
  if closed[file] {continue}
  info,err:=os.Lstat(file)
  if os.IsNotExist(err) {continue}
  if err!=nil || !info.Mode().IsRegular() || info.Size()!=0 {
   log.Printf("Preserved unclosed prebuffer for camera %s; inspection/recovery required",bm.camera.ID)
   continue
  }
  // The segmenter has joined. Only its nonce-scoped empty regular outputs
  // are disposable; missing completion metadata does not condemn real bytes.
  if err:=os.Remove(file);err!=nil && !os.IsNotExist(err) {log.Printf("Unable to remove empty prebuffer output for camera %s",bm.camera.ID)}
 }
}

func (bm *BufferManager) pruneLoop() {
 ticker:=time.NewTicker(segmentSize);defer ticker.Stop()
 for {select {case <-bm.ctx.Done():return;case now:=<-ticker.C:bm.pruneBefore(now.UTC().Add(-bufferDuration))}}
}

func (bm *BufferManager) pruneBefore(cutoff time.Time) {
 bm.bufferMutex.Lock();defer bm.bufferMutex.Unlock()
 kept:=make([]BufferSegment,0,len(bm.buffer))
 for _,seg:=range bm.buffer {
  if seg.EndTime.Before(cutoff) {if err:=os.Remove(seg.FilePath);err!=nil && !os.IsNotExist(err) {kept=append(kept,seg)}} else {kept=append(kept,seg)}
 }
 bm.buffer=kept
}

func (bm *BufferManager) cleanupBuffer() {
 bm.bufferMutex.Lock();defer bm.bufferMutex.Unlock()
 bm.buffer=nil
 // Stop releases the in-memory index, not capture evidence. A new manager
 // must not adopt or prune files left by this or an earlier process.
 logPreservedEventStorage(bm.camera)
}

func (bm *BufferManager) handleEvent(job *bufferEvent) {
 file:=bm.recordEventDuration(job)
 if file=="" {return}
 if bm.ctx.Err()!=nil {job.keepStaging=true;return}
 if final:=bm.concatenateEventRecording(job,file);final!="" {log.Printf("Event output published for camera %s event %s; publication does not certify complete capture",bm.camera.ID,job.event.ID)}
}

func (bm *BufferManager) recordEventDuration(job *bufferEvent) string {
 execution:=job.execution
 execution.beginCapture(bm,job)
 defer execution.endCaptureAttempt()
 file:=filepath.Join(job.dir,"event.mp4")
 job.keepStaging=true
 if err:=job.recoveryCheckpoint("capturing","");err!=nil {return ""}
 execution.requireCheckpoint(job,"capturing")
 ctx,cancel:=context.WithTimeout(bm.ctx,time.Duration(job.event.Duration)*time.Second+15*time.Second);defer cancel()
 err:=bm.captureEventFile(ctx,job,file)
 if err!=nil || ctx.Err()!=nil {
  // Failure, including inability to inspect output, never authorizes deletion.
  job.keepStaging=true
  _=job.recoveryCheckpoint("capture_failed","process_failed_or_canceled")
  log.Printf("Event capture interrupted/failed for camera %s event %s; staging retained for inspection",bm.camera.ID,job.event.ID)
  return ""
 }
 if err:=job.recoveryCheckpoint("captured","");err!=nil {return ""}
 // captureEventFile has joined and closed/registered its creator. Observe the
 // outcome before normal deferred cancellation of this function-local context.
 execution.finishCapture(bm,job,ctx)
 return file
}

func (bm *BufferManager) concatenateEventRecording(job *bufferEvent,eventFile string) string {
 execution:=job.execution
 execution.beginAssembly(bm,job,eventFile)
 defer execution.endAssemblyAttempt()
 job.keepStaging=true
 if err:=job.recoveryCheckpoint("assembling","");err!=nil {return ""}
 execution.requireCheckpoint(job,"assembling")
 if err:=job.writeEventConcat(bm.ctx,eventFile);err!=nil {return ""}
 name:=fmt.Sprintf("%s_%s_%s.mp4",job.preStart.UTC().Format("20060102_150405"),sanitizeFilename(job.event.Label),sanitizeFilename(job.event.ID))
 final:=filepath.Join(bm.camera.RecordDir,name);temp:=final+".tmp"
 ctx,cancel:=context.WithTimeout(bm.ctx,2*time.Minute);defer cancel()
 cmd:=ffmpegCommand(ctx,"-hide_banner","-loglevel","error","-nostats","-f","concat","-safe","1","-i","concat.txt","-c","copy","-f","mp4","-movflags","faststart","-y",temp)
 cmd.Dir=job.dir
 runErr:=cmd.Run()
 incomplete:=runErr!=nil || ctx.Err()!=nil
 execution.joinAssembly(bm,job,ctx,runErr)
 if incomplete {final=partialRecordingName(final)}
 job.recovery.OutputTemporary=filepath.Base(temp)
 job.recovery.OutputPublished=filepath.Base(final)
 job.recovery.Partial=incomplete
 if err:=job.recoveryCheckpoint("publishing","");err!=nil {return ""}
 execution.requireCheckpoint(job,"publishing")
 finalizeCtx,finalizeCancel:=context.WithTimeout(context.WithoutCancel(bm.ctx),recordingFinalizeGrace);defer finalizeCancel()
 if err:=bm.publishEventRecording(finalizeCtx,ctx,job,temp,final,incomplete);err!=nil {_=job.recoveryCheckpoint("publication_failed","validation_or_publication_failed");removeEmptyRecording(temp);log.Printf("Event assembly for camera %s event %s retained for inspection",bm.camera.ID,job.event.ID);return ""}
 return final
}

func probeRecording(parent context.Context,path string) error {
 ctx,cancel:=context.WithTimeout(parent,10*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-select_streams","v:0","-show_entries","stream=codec_name","-of","default=noprint_wrappers=1:nokey=1",path)
 cmd.WaitDelay=5*time.Second
 output,err:=cmd.Output()
 if err!=nil {return err}
 if strings.TrimSpace(string(output))=="" {return errors.New("recording has no video stream")}
 return nil
}

func recordSegment(ctx context.Context,cam *Camera,durationSeconds int) error {
 now:=time.Now().UTC()
 filename:=fmt.Sprintf("%s_%s_%s.mp4",now.Format("20060102_150405"),sanitizeFilename(cam.Name),generateID("segment",now))
 filePath:=filepath.Join(cam.RecordDir,filename);tempPath:=filePath+".tmp"
 segmentCtx,cancel:=context.WithTimeout(ctx,time.Duration(durationSeconds)*time.Second+5*time.Second);defer cancel()
 cmd:=ffmpegCommand(segmentCtx,"-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-fflags","+genpts+igndts+ignidx+discardcorrupt","-i",cam.RTSPUrl,"-t",strconv.Itoa(durationSeconds),"-c","copy","-movflags","faststart","-f","mp4","-y",tempPath)
 runErr:=cmd.Run()
 if runErr!=nil || segmentCtx.Err()!=nil {filePath=partialRecordingName(filePath)}
 finalizeCtx,finalizeCancel:=context.WithTimeout(context.WithoutCancel(ctx),recordingFinalizeGrace);defer finalizeCancel()
 if err:=publishRecording(finalizeCtx,tempPath,filePath);err!=nil {
  removeEmptyRecording(tempPath)
  return errors.Join(runErr,segmentCtx.Err(),err)
 }
 if ctx.Err()!=nil {return ctx.Err()}
 if segmentCtx.Err()!=nil {return segmentCtx.Err()}
 return runErr
}

func sanitizeFilename(name string) string {
 var out strings.Builder
 for _,r:=range name {
  if out.Len()>=80 {break}
  if (r>='a' && r<='z') || (r>='A' && r<='Z') || (r>='0' && r<='9') {out.WriteRune(r)} else {out.WriteByte('_')}
 }
 return out.String()
}

func recoverPartialRecordings() {
 nvr.mu.RLock()
 cameras:=make([]*Camera,0,len(nvr.cameras));for _,cam:=range nvr.cameras {cameras=append(cameras,cam)}
 nvr.mu.RUnlock()
 for _,cam:=range cameras {
  logPreservedEventStorage(cam)
  files,_:=filepath.Glob(filepath.Join(cam.RecordDir,"*.tmp"))
  for _,file:=range files {
   final:=partialRecordingName(strings.TrimSuffix(file,".tmp"))
   ctx,cancel:=context.WithTimeout(context.Background(),recordingFinalizeGrace)
   err:=publishRecording(ctx,file,final);cancel()
   if err!=nil {log.Printf("Partial recording for camera %s retained for inspection",cam.ID)}
  }
 }
}
