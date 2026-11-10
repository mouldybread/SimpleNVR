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
type bufferEvent struct {event Event;preFiles []string;preStart time.Time;dir string}
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
  if err:=os.MkdirAll(dir,0700);err!=nil {
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
func (bm *BufferManager) runWorkers(tasks []func(),handle func(*bufferEvent)) {
 var children sync.WaitGroup
 children.Add(len(tasks)+eventWorkers)
 for _,task:=range tasks {go func(task func()){defer children.Done();task()}(task)}
 for i:=0;i<eventWorkers;i++ {go func(){
  defer children.Done()
  for {select {
  case <-bm.ctx.Done():return
  case job:=<-bm.eventChan:
   func(){defer bm.releaseJob(job);if bm.ctx.Err()==nil {handle(job)}}()
  }}
 }()}
 <-bm.ctx.Done()
 bm.queueMu.Lock();bm.stopping=true;bm.cancel();bm.queueMu.Unlock()
 children.Wait()
 for {select {case job:=<-bm.eventChan:bm.releaseJob(job);default:return}}
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
 case <-bm.ctx.Done():bm.releaseJob(job);return errEventStopped
 default:bm.releaseJob(job);return errEventBusy
 }
}

func (bm *BufferManager) pinBuffer(event Event) (*bufferEvent,error) {
 parent:=filepath.Join(bm.camera.RecordDir,"temp")
 if err:=os.MkdirAll(parent,0700);err!=nil {return nil,err}
 dir,err:=os.MkdirTemp(parent,"event-");if err!=nil {return nil,err}
 job:=&bufferEvent{event:event,preStart:event.CreatedAt.UTC(),dir:dir}
 bm.bufferMutex.RLock();defer bm.bufferMutex.RUnlock()
 cutoff:=time.Now().UTC().Add(-bufferDuration)
 for _,seg:=range bm.buffer {
  if !seg.EndTime.After(cutoff) {continue}
  pinned:=filepath.Join(dir,fmt.Sprintf("pre_%03d.mp4",len(job.preFiles)))
  if err:=os.Link(seg.FilePath,pinned);err!=nil {os.RemoveAll(dir);return nil,err}
  if len(job.preFiles)==0 {job.preStart=seg.StartTime}
  job.preFiles=append(job.preFiles,pinned)
 }
 return job,nil
}

func (bm *BufferManager) releaseJob(job *bufferEvent) {
 if err:=os.RemoveAll(job.dir);err!=nil {log.Printf("Unable to remove event staging for camera %s",bm.camera.ID)}
 <-bm.slots
}

func (bm *BufferManager) segmentBuffer() {
 dir:=filepath.Join(bm.camera.RecordDir,"buffer")
 for bm.ctx.Err()==nil {
  nonce:=generateID("buffer",time.Now())
  pattern:="buffer_%Y%m%d_%H%M%S_"+nonce+".mp4"
  cmd:=exec.CommandContext(bm.ctx,"ffmpeg","-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-i",bm.camera.RTSPUrl,"-c","copy","-f","segment","-segment_time","5","-segment_format","mp4","-reset_timestamps","1","-strftime","1","-segment_list","pipe:1","-segment_list_type","csv","-segment_list_size","0",pattern)
  cmd.Dir=dir;cmd.Env=append(os.Environ(),"TZ=UTC");cmd.WaitDelay=5*time.Second
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
 files,_:=filepath.Glob(pattern)
 for _,file:=range files {if !closed[file] {os.Remove(file)}}
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
 if err:=os.RemoveAll(filepath.Join(bm.camera.RecordDir,"buffer"));err!=nil {log.Printf("Unable to remove stopped buffer for camera %s",bm.camera.ID)}
}

func (bm *BufferManager) handleEvent(job *bufferEvent) {
 file:=bm.recordEventDuration(job)
 if file=="" {return}
 if final:=bm.concatenateEventRecording(job,file);final!="" {log.Printf("Event capture saved for camera %s event %s",bm.camera.ID,job.event.ID)}
}

func (bm *BufferManager) recordEventDuration(job *bufferEvent) string {
 file:=filepath.Join(job.dir,"event.mp4")
 ctx,cancel:=context.WithTimeout(bm.ctx,time.Duration(job.event.Duration)*time.Second+15*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-i",bm.camera.RTSPUrl,"-t",strconv.Itoa(job.event.Duration),"-c","copy","-f","mp4","-movflags","faststart","-y",file)
 cmd.WaitDelay=5*time.Second
 if err:=cmd.Run();err!=nil {log.Printf("Event capture failed for camera %s event %s: %v",bm.camera.ID,job.event.ID,err);return ""}
 return file
}

func (bm *BufferManager) concatenateEventRecording(job *bufferEvent,eventFile string) string {
 list,err:=os.Create(filepath.Join(job.dir,"concat.txt"));if err!=nil {return ""}
 files:=append(append([]string(nil),job.preFiles...),eventFile)
 for _,file:=range files {
  if filepath.Dir(file)!=job.dir {list.Close();return ""}
  base:=filepath.Base(file)
  if strings.ContainsAny(base,"'\r\n\\") {list.Close();return ""}
  if _,err:=fmt.Fprintf(list,"file '%s'\n",base);err!=nil {list.Close();return ""}
 }
 if err:=list.Close();err!=nil {return ""}
 name:=fmt.Sprintf("%s_%s_%s.mp4",job.preStart.UTC().Format("20060102_150405"),sanitizeFilename(job.event.Label),sanitizeFilename(job.event.ID))
 final:=filepath.Join(bm.camera.RecordDir,name);temp:=final+".tmp"
 defer os.Remove(temp)
 ctx,cancel:=context.WithTimeout(bm.ctx,2*time.Minute);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-hide_banner","-loglevel","error","-nostats","-f","concat","-safe","1","-i","concat.txt","-c","copy","-f","mp4","-movflags","faststart","-y",temp)
 cmd.Dir=job.dir;cmd.WaitDelay=5*time.Second
 if err:=cmd.Run();err!=nil {log.Printf("Event assembly failed for camera %s event %s: %v",bm.camera.ID,job.event.ID,err);return ""}
 if err:=probeRecording(ctx,temp);err!=nil {return ""}
 if err:=os.Rename(temp,final);err!=nil {return ""}
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

// Continuous MP4 finalization on cancellation remains a separate pending fix.
func recordSegment(ctx context.Context,cam *Camera,durationSeconds int) error {
 now:=time.Now().UTC()
 filename:=fmt.Sprintf("%s_%s_%s.mp4",now.Format("20060102_150405"),sanitizeFilename(cam.Name),generateID("segment",now))
 filePath:=filepath.Join(cam.RecordDir,filename);tempPath:=filePath+".tmp"
 segmentCtx,cancel:=context.WithTimeout(ctx,time.Duration(durationSeconds)*time.Second+5*time.Second);defer cancel()
 cmd:=exec.CommandContext(segmentCtx,"ffmpeg","-hide_banner","-loglevel","error","-nostats","-rtsp_transport","tcp","-fflags","+genpts+igndts+ignidx+discardcorrupt","-i",cam.RTSPUrl,"-t",strconv.Itoa(durationSeconds),"-c","copy","-movflags","faststart","-f","mp4","-y",tempPath)
 cmd.WaitDelay=5*time.Second
 err:=cmd.Run()
 if ctx.Err()!=nil {return ctx.Err()}
 if segmentCtx.Err()!=nil {os.Remove(tempPath);return segmentCtx.Err()}
 if err!=nil {os.Remove(tempPath);return err}
 info,err:=os.Stat(tempPath);if err!=nil {return err}
 if info.Size()<1000 {os.Remove(tempPath);return errors.New("recording file too small")}
 if err:=os.Rename(tempPath,filePath);err!=nil {return err}
 return nil
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
  // Startup precedes workers. Preserve pinned crash staging rather than silently wiping it.
  os.RemoveAll(filepath.Join(cam.RecordDir,"buffer"))
  if entries,err:=os.ReadDir(filepath.Join(cam.RecordDir,"temp"));err==nil && len(entries)>0 {log.Printf("Preserved unassembled event staging for camera %s; manual recovery/cleanup required",cam.ID)}
  files,_:=filepath.Glob(filepath.Join(cam.RecordDir,"*.tmp"))
  for _,file:=range files {
   if err:=probeRecording(context.Background(),file);err!=nil {log.Printf("Partial recording for camera %s could not be validated; left for inspection",cam.ID);continue}
   final:=strings.TrimSuffix(file,".tmp")
   if _,err:=os.Lstat(final);!os.IsNotExist(err) {continue}
   if err:=os.Rename(file,final);err!=nil {log.Printf("Unable to recover partial recording for camera %s",cam.ID)}
  }
 }
}
