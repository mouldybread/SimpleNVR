package main

import (
 "context"
 "errors"
 "fmt"
 "log"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "sync"
)

const recordingStderrLimit = 8192

// exec.Cmd joins its stderr-copy goroutine before Run returns. Read only then.
type recordingStderrTail struct { data []byte }
func (b *recordingStderrTail) Write(p []byte) (int,error) {
 n:=len(p)
 if n>=recordingStderrLimit { b.data=append(b.data[:0],p[n-recordingStderrLimit:]...);return n,nil }
 if extra:=len(b.data)+n-recordingStderrLimit;extra>0 { copy(b.data,b.data[extra:]);b.data=b.data[:len(b.data)-extra] }
 b.data=append(b.data,p...);return n,nil
}

type continuousRecordingNotice struct {
 Code string
 Message string
 Detail string
 PublicationMessage string
 Retrying bool
}

// Error and the public notice are built only from constants and integers.
// Underlying errors remain available to errors.Is/As, never to rendering/logs.
type continuousRecordingError struct {
 notice continuousRecordingNotice
 causes []error
}
func (e *continuousRecordingError) Error() string {
 text:=e.notice.Code+": "+e.notice.Message
 if e.notice.Detail!="" {text+=" ("+e.notice.Detail+")"}
 if e.notice.PublicationMessage!="" {text+=" "+e.notice.PublicationMessage}
 return text
}
func (e *continuousRecordingError) Unwrap() []error {return e.causes}

var describeFailure = regexp.MustCompile(`(?m)method DESCRIBE failed: ([1-5][0-9]{2})(?:[ (]|$)`)

func continuousAttemptResult(parent,segment context.Context,runErr,publicationErr error,tempPath string,stderr []byte) error {
 if parent.Err()!=nil {return parent.Err()}
 if runErr==nil && segment.Err()==nil && publicationErr==nil {return nil}
 notice:=continuousRecordingNotice{Code:"recorder_failed",Message:"Recording failed. Check the camera or stream server."}
 if runErr!=nil {
  if match:=describeFailure.FindSubmatch(stderr);len(match)>1 {
   notice.Code="upstream_rtsp_unavailable"
   notice.Message="Couldn't open the upstream RTSP stream. Check the camera or stream server. If this stream comes from go2rtc, check its configuration and camera source."
   notice.Detail="DESCRIBE "+string(match[1])
  } else {
   var exit *exec.ExitError
   if errors.As(runErr,&exit) && exit.ExitCode()>=0 {notice.Detail=fmt.Sprintf("FFmpeg exit %d",exit.ExitCode())}
  }
 } else if segment.Err()!=nil {
  notice.Code="recording_timeout";notice.Message="Recording exceeded its capture deadline."
 }
 if publicationErr!=nil {
  // Only a direct lstat of the temporary output can be the secondary
  // missing-output symptom. Destination, rename and later validation errors
  // are never suppressed by a broad os.IsNotExist check. Verify the parent
  // still exists; a missing storage directory is a storage failure.
  var pe *os.PathError
  absent:=errors.As(publicationErr,&pe) && pe.Op=="lstat" && pe.Path==tempPath && errors.Is(pe.Err,os.ErrNotExist)
  parentInfo,parentErr:=os.Stat(filepath.Dir(tempPath))
  absent=absent && parentErr==nil && parentInfo.IsDir()
  if absent && runErr==nil && segment.Err()==nil {
   notice.Code="recording_output_missing";notice.Message="Recorder finished without creating its output file."
  } else if !absent || (runErr==nil && segment.Err()==nil) {
   if runErr==nil && segment.Err()==nil {notice.Code="recording_publication_failed";notice.Message="Recording could not be validated or published. Check recording storage."} else {notice.PublicationMessage="Recording storage or publication also failed; check recording storage."}
  }
 }
 return &continuousRecordingError{notice:notice,causes:[]error{runErr,segment.Err(),publicationErr}}
}

type continuousHealthWorker struct {ctx context.Context;notice continuousRecordingNotice}
var continuousHealth = struct {sync.Mutex; workers map[*Camera]*continuousHealthWorker}{workers:make(map[*Camera]*continuousHealthWorker)}

func beginContinuousHealth(ctx context.Context,cam *Camera) *continuousHealthWorker {
 worker:=&continuousHealthWorker{ctx:ctx}
 continuousHealth.Lock();continuousHealth.workers[cam]=worker;continuousHealth.Unlock()
 return worker
}
func endContinuousHealth(cam *Camera,worker *continuousHealthWorker) {
 continuousHealth.Lock();defer continuousHealth.Unlock()
 if continuousHealth.workers[cam]==worker {delete(continuousHealth.workers,cam)}
}
func updateContinuousHealth(cam *Camera,worker *continuousHealthWorker,err error) {
 continuousHealth.Lock();defer continuousHealth.Unlock()
 if continuousHealth.workers[cam]!=worker || worker.ctx.Err()!=nil {return}
 if err==nil {worker.notice=continuousRecordingNotice{};return}
 var failure *continuousRecordingError
 if errors.As(err,&failure) {worker.notice=failure.notice} else {worker.notice=continuousRecordingNotice{Code:"recorder_failed",Message:"Recording failed. Check the camera, stream server, or recording storage."}}
 worker.notice.Retrying=true
}
func snapshotContinuousHealth(cam *Camera) continuousRecordingNotice {
 continuousHealth.Lock();defer continuousHealth.Unlock()
 worker:=continuousHealth.workers[cam]
 if worker==nil || worker.ctx.Err()!=nil {return continuousRecordingNotice{}}
 return worker.notice
}

func runContinuousRecorder(ctx context.Context,cam *Camera,attempt func(context.Context,*Camera,int) error,wait func(context.Context) bool) {
 worker:=beginContinuousHealth(ctx,cam);defer endContinuousHealth(cam,worker)
 for ctx.Err()==nil {
  err:=attempt(ctx,cam,600)
  if ctx.Err()!=nil {return}
  updateContinuousHealth(cam,worker,err)
  if err!=nil {
   // Log the safe snapshot, never an arbitrary subprocess or storage error.
   notice:=snapshotContinuousHealth(cam)
   if notice.Code!="" {log.Printf("Recording error for camera %s: %s",cam.ID,(&continuousRecordingError{notice:notice}).Error())}
   if !wait(ctx) {return}
  }
 }
}
