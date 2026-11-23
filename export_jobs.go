package main

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "sort"
 "strconv"
 "sync"
 "time"
)

var (
 errExportJobsBusy = errors.New("export job capacity exceeded")
 errExportJobMissing = errors.New("export job unavailable")
 errExportJobsClosed = errors.New("export service stopping")
 errExportJobSize = errors.New("export output size limit exceeded")
 errExportJobEmpty = errors.New("empty export")
)

// Stable machine-readable reasons reported for failed, canceled and expired jobs.
const (
 exportReasonOutputLimit = "output_limit"
 exportReasonEstimateExceeded = "estimate_exceeded"
 exportReasonDeadline = "deadline_exceeded"
 exportReasonEmptyOutput = "empty_output"
 exportReasonRenderFailed = "render_failed"
 exportReasonCanceled = "canceled"
 exportReasonCleared = "cleared"
 exportReasonExpired = "expired"
 exportReasonStopping = "service_stopping"
)

type exportJobLimits struct {
 MaxBytes, TotalBytes int64
 MaxJobs, MaxQueued, Parallel int
 Lifetime, Preparation time.Duration
}

func defaultExportJobLimits() exportJobLimits {
 return exportJobLimits{MaxBytes:8<<30, TotalBytes:16<<30, MaxJobs:8, MaxQueued:4, Parallel:1, Lifetime:30*time.Minute, Preparation:30*time.Minute}
}

// queueLimit is the number of jobs that may wait to start. Zero (an unset field in
// a literal) falls back to MaxJobs, which retained records already bound.
func (l exportJobLimits) queueLimit() int {
 if l.MaxQueued>0 {return l.MaxQueued}
 return l.MaxJobs
}

func (l exportJobLimits) validate() error {
 if l.MaxBytes<1 || l.TotalBytes<l.MaxBytes || l.MaxJobs<1 || l.MaxJobs>64 || l.MaxQueued<0 || l.MaxQueued>64 || l.Parallel<1 || l.Parallel>l.MaxJobs || l.Lifetime<=0 || l.Lifetime>24*time.Hour || l.Preparation<=0 || l.Preparation>24*time.Hour {
  return errors.New("invalid export job resource limits")
 }
 return nil
}

// Configuration is read once by the future application startup wiring. No
// environment is read at package initialization and invalid values fail closed.
func loadExportJobLimits(getenv func(string)string) (exportJobLimits,error) {
 limits:=defaultExportJobLimits()
 for name,target:=range map[string]*int64{"NVR_EXPORT_MAX_BYTES":&limits.MaxBytes,"NVR_EXPORT_TOTAL_BYTES":&limits.TotalBytes} {
  if raw:=getenv(name);raw!="" {value,err:=strconv.ParseInt(raw,10,64);if err!=nil {return limits,errors.New("invalid "+name)};*target=value}
 }
 for name,target:=range map[string]*int{"NVR_EXPORT_MAX_JOBS":&limits.MaxJobs,"NVR_EXPORT_MAX_QUEUED":&limits.MaxQueued,"NVR_EXPORT_PARALLEL":&limits.Parallel} {
  if raw:=getenv(name);raw!="" {value,err:=strconv.Atoi(raw);if err!=nil {return limits,errors.New("invalid "+name)};*target=value}
 }
 if raw:=getenv("NVR_EXPORT_MAX_QUEUED");raw!="" && limits.MaxQueued<1 {return limits,errors.New("invalid NVR_EXPORT_MAX_QUEUED")}
 for name,target:=range map[string]*time.Duration{"NVR_EXPORT_TTL":&limits.Lifetime,"NVR_EXPORT_PREPARE_TIMEOUT":&limits.Preparation} {
  if raw:=getenv(name);raw!="" {value,err:=time.ParseDuration(raw);if err!=nil {return limits,errors.New("invalid "+name)};*target=value}
 }
 return limits,limits.validate()
}

// ExpiresAt is the preparation deadline while a job is queued or preparing. When the
// job completes (ready, failed or canceled) it becomes completion time plus Lifetime.
// Reason is a stable code for failed, canceled and expired jobs. EstimatedBytes is the
// size the job reserves while it is queued or preparing; Bytes is the actual size once
// ready. QueuePosition is 1-based and set only while queued.
type exportJobView struct {
 ID string `json:"id"`
 State string `json:"state"`
 Message string `json:"message"`
 Reason string `json:"reason,omitempty"`
 Bytes int64 `json:"bytes"`
 EstimatedBytes int64 `json:"estimated_bytes,omitempty"`
 QueuePosition int `json:"queue_position,omitempty"`
 ExpiresAt time.Time `json:"expires_at"`
}

type preparedExportJob struct {
 view exportJobView
 owner string
 ctx context.Context
 cancel context.CancelFunc
 render exportJobRenderer
 file *os.File
 created time.Time
 running bool
 counted bool
 removed bool
 readers int
 estimate int64
 reserved int64
}

// Renderers must finish/join all subprocesses before returning and validate
// their output before success. They never own or retain the output writer.
type exportJobRenderer func(context.Context,io.Writer) error

type exportJobStore struct {
 mu sync.Mutex
 limits exportJobLimits
 directory string
 jobs map[string]*preparedExportJob
 queue []*preparedExportJob
 active int
 reserved int64
 ctx context.Context
 cancel context.CancelFunc
 workers sync.WaitGroup
 closing, workersDone, drainedClosed bool
 closeOnce sync.Once
 drained chan struct{}
 cleanupErr error
}

// parentDirectory must be a private, trusted export scratch filesystem, not
// camera storage. A unique directory and immediately unlinked outputs avoid
// crash leftovers and never require globbing/deleting existing recordings.
func newExportJobStore(parent context.Context,parentDirectory string,limits exportJobLimits) (*exportJobStore,error) {
 if err:=limits.validate();err!=nil {return nil,err}
 if err:=parent.Err();err!=nil {return nil,err}
 directory,err:=os.MkdirTemp(parentDirectory,"simplenvr-exports-");if err!=nil {return nil,err}
 ctx,cancel:=context.WithCancel(parent)
 store:=&exportJobStore{limits:limits,directory:directory,jobs:make(map[string]*preparedExportJob),ctx:ctx,cancel:cancel,drained:make(chan struct{})}
 store.workers.Add(1)
 go func(){
  defer store.workers.Done()
  interval:=limits.Lifetime/2;if interval>time.Minute {interval=time.Minute};if interval<time.Millisecond {interval=time.Millisecond}
  ticker:=time.NewTicker(interval);defer ticker.Stop()
  for {select {case <-ctx.Done():return;case now:=<-ticker.C:store.expire(now)}}
 }()
 return store,nil
}

// viewLocked returns a job's public view with the live queue position and estimate.
func (s *exportJobStore) viewLocked(job *preparedExportJob) exportJobView {
 view:=job.view
 switch view.State {
 case "queued":
  view.EstimatedBytes=job.estimate
  for i,queued:=range s.queue {if queued==job {view.QueuePosition=i+1;break}}
 case "preparing":
  view.EstimatedBytes=job.estimate
 }
 return view
}

// submit admits a job that reserves the per-job maximum when it starts.
func (s *exportJobStore) submit(owner string,render exportJobRenderer) (exportJobView,error) {
 return s.submitEstimated(owner,0,render)
}

// submitEstimated queues a job. It reserves estimate bytes (clamped to MaxBytes; zero or
// less means MaxBytes) only when it starts, and that reservation is also the job's
// output cap. Admission fails with errExportJobsBusy when the queue or the retained
// job records are full.
func (s *exportJobStore) submitEstimated(owner string,estimate int64,render exportJobRenderer) (exportJobView,error) {
 if owner=="" || render==nil {return exportJobView{},errors.New("export owner and renderer required")}
 raw:=make([]byte,32);if _,err:=rand.Read(raw);err!=nil {return exportJobView{},err}
 id:=hex.EncodeToString(raw)
 s.mu.Lock();defer s.mu.Unlock()
 if s.closing || s.ctx.Err()!=nil {return exportJobView{},errExportJobsClosed}
 if estimate<=0 || estimate>s.limits.MaxBytes {estimate=s.limits.MaxBytes}
 if len(s.jobs)>=s.limits.MaxJobs || len(s.queue)>=s.limits.queueLimit() {return exportJobView{},errExportJobsBusy}
 ctx,cancel:=context.WithTimeout(s.ctx,s.limits.Preparation)
 job:=&preparedExportJob{view:exportJobView{ID:id,State:"queued",Message:"Queued for export preparation",ExpiresAt:time.Now().Add(s.limits.Preparation)},owner:owner,ctx:ctx,cancel:cancel,render:render,estimate:estimate,created:time.Now()}
 s.jobs[id]=job;s.queue=append(s.queue,job)
 view:=s.viewLocked(job)
 // A queued job has no goroutine, so its cancellation or preparation deadline is observed here.
 context.AfterFunc(ctx,func(){s.abortQueued(job)})
 s.dispatchLocked()
 return view,nil
}

// dispatchLocked starts queued jobs in order while a preparation slot and the byte
// budget allow. The head of the queue is never skipped, so a large job cannot be
// starved by smaller ones behind it.
func (s *exportJobStore) dispatchLocked() {
 for len(s.queue)>0 && !s.closing && s.ctx.Err()==nil {
  job:=s.queue[0]
  if job.view.State!="queued" {s.queue=s.queue[1:];continue}
  if s.active>=s.limits.Parallel || job.estimate>s.limits.TotalBytes-s.reserved {return}
  s.queue=s.queue[1:]
  job.reserved=job.estimate;s.reserved+=job.reserved
  job.running=true;job.counted=true;s.active++
  job.view.State="preparing";job.view.Message="Preparing export on server"
  s.workers.Add(1)
  go s.run(job)
 }
}

func (s *exportJobStore) removeQueuedLocked(job *preparedExportJob) {
 for i,queued:=range s.queue {
  if queued==job {s.queue=append(s.queue[:i:i],s.queue[i+1:]...);return}
 }
}

// removeIfDoneLocked drops a cleared job's record once nothing is still using it.
func (s *exportJobStore) removeIfDoneLocked(job *preparedExportJob) {
 if job.removed && !job.running && job.readers==0 {delete(s.jobs,job.view.ID)}
}

// abortQueued ends a job that is still waiting when its context ends: preparation
// deadline, cancellation or shutdown. Running jobs finish through run/finish instead.
func (s *exportJobStore) abortQueued(job *preparedExportJob) {
 s.mu.Lock();defer s.mu.Unlock()
 if job.view.State!="queued" {return}
 s.removeQueuedLocked(job)
 if s.closing || s.ctx.Err()!=nil || !errors.Is(job.ctx.Err(),context.DeadlineExceeded) {
  job.view.State="canceled";job.view.Message="Export canceled";job.view.Reason=exportReasonCanceled
  if s.closing || s.ctx.Err()!=nil {job.view.Reason=exportReasonStopping}
 } else {
  job.view.State="failed";job.view.Message="Export preparation deadline exceeded";job.view.Reason=exportReasonDeadline
 }
 job.view.ExpiresAt=time.Now().Add(s.limits.Lifetime)
 s.discardLocked(job)
 s.removeIfDoneLocked(job)
}

type boundedExportJobWriter struct {
 ctx context.Context
 file *os.File
 limit,written int64
 exceeded bool
}
func (w *boundedExportJobWriter) Write(data []byte) (int,error) {
 if err:=w.ctx.Err();err!=nil {return 0,err}
 if int64(len(data))>w.limit-w.written {w.exceeded=true;return 0,errExportJobSize}
 n,err:=w.file.Write(data);w.written+=int64(n);return n,err
}

func (s *exportJobStore) run(job *preparedExportJob) {
 defer s.workers.Done();defer job.cancel()
 if err:=job.ctx.Err();err!=nil {s.finish(job,nil,0,err);return}
 file,err:=os.CreateTemp(s.directory,"output-*.mp4")
 if err!=nil {s.finish(job,nil,0,err);return}
 // CreateTemp is owner-only. Unlink before handing the descriptor to a
 // renderer; the kernel releases output bytes after the last reader closes.
 if err:=os.Remove(file.Name());err!=nil {closeErr:=file.Close();removeErr:=os.Remove(file.Name());s.mu.Lock();s.cleanupErr=errors.Join(s.cleanupErr,closeErr,removeErr);s.mu.Unlock();s.finish(job,nil,0,err);return}
 // The job's reservation is its output cap, so the byte budget stays an honest bound.
 writer:=&boundedExportJobWriter{ctx:job.ctx,file:file,limit:job.reserved}
 err=job.render(job.ctx,writer)
 if writer.exceeded {err=errExportJobSize}
 if err==nil {err=job.ctx.Err()}
 if err==nil && writer.written==0 {err=errExportJobEmpty}
 s.finish(job,file,writer.written,err)
}

func (s *exportJobStore) finish(job *preparedExportJob,file *os.File,size int64,err error) {
 s.mu.Lock();defer s.mu.Unlock()
 if job.counted {job.counted=false;s.active--}
 job.running=false;job.file=file
 if job.view.State=="canceled" || job.view.State=="expired" || s.closing || s.ctx.Err()!=nil {
  if job.view.State!="expired" && job.view.State!="canceled" {
   job.view.State="canceled";job.view.Message="Export canceled";job.view.Reason=exportReasonCanceled
   if s.closing || s.ctx.Err()!=nil {job.view.Reason=exportReasonStopping}
  }
 } else if err!=nil {
  job.view.State="failed";job.view.Message="Export preparation failed; no download is ready";job.view.Reason=exportReasonRenderFailed
  switch {
  case errors.Is(err,errExportJobSize):
   job.view.Message="Export exceeds the configured server output limit";job.view.Reason=exportReasonOutputLimit
   if job.estimate<s.limits.MaxBytes {job.view.Message="Export exceeded its estimated output size; try a shorter range";job.view.Reason=exportReasonEstimateExceeded}
  case errors.Is(err,context.DeadlineExceeded):
   job.view.Message="Export preparation deadline exceeded";job.view.Reason=exportReasonDeadline
  case errors.Is(err,errExportJobEmpty):
   job.view.Reason=exportReasonEmptyOutput
  }
 } else {
  job.view.State="ready";job.view.Message="Ready to download";job.view.Reason="";job.view.Bytes=size
  // Charge the actual size from now on and free the unused part of the estimate.
  if size<job.reserved {s.reserved-=job.reserved-size;job.reserved=size}
 }
 // The retention timer starts when the job completes.
 job.view.ExpiresAt=time.Now().Add(s.limits.Lifetime)
 if job.view.State!="ready" {s.discardLocked(job)}
 if s.closing || job.view.State=="expired" || job.removed {delete(s.jobs,job.view.ID)}
 s.dispatchLocked()
}

// discardLocked releases a job's output and reservation, then starts any queued job
// that now fits.
func (s *exportJobStore) discardLocked(job *preparedExportJob) {
 if job.running || job.readers>0 {return}
 if job.file!=nil {s.cleanupErr=errors.Join(s.cleanupErr,job.file.Close());job.file=nil}
 s.reserved-=job.reserved;job.reserved=0
 s.dispatchLocked()
}

// expire drops completed jobs whose retention ended. Queued and preparing jobs are
// never expired here: the preparation timeout bounds them.
func (s *exportJobStore) expire(now time.Time) {
 s.mu.Lock();defer s.mu.Unlock()
 for id,job:=range s.jobs {
  if job.running || job.view.State=="queued" {continue}
  if now.Before(job.view.ExpiresAt) {continue}
  job.view.State="expired";job.view.Message="Prepared export expired";job.view.Reason=exportReasonExpired;job.cancel()
  s.discardLocked(job)
  if !job.running && job.readers==0 {delete(s.jobs,id)}
 }
}

func (s *exportJobStore) status(owner,id string) (exportJobView,error) {
 s.expire(time.Now())
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if s.closing || s.ctx.Err()!=nil {return exportJobView{},errExportJobsClosed}
 if owner=="" || job==nil || job.owner!=owner || job.removed {return exportJobView{},errExportJobMissing}
 return s.viewLocked(job),nil
}

// list returns the owner's jobs in creation order. Other sessions' jobs and jobs that
// were cleared are never included.
func (s *exportJobStore) list(owner string) ([]exportJobView,error) {
 s.expire(time.Now())
 s.mu.Lock();defer s.mu.Unlock()
 if s.closing || s.ctx.Err()!=nil {return nil,errExportJobsClosed}
 if owner=="" {return nil,errExportJobMissing}
 jobs:=make([]*preparedExportJob,0,len(s.jobs))
 for _,job:=range s.jobs {if job.owner==owner && !job.removed {jobs=append(jobs,job)}}
 sort.Slice(jobs,func(i,j int) bool {
  if jobs[i].created.Equal(jobs[j].created) {return jobs[i].view.ID<jobs[j].view.ID}
  return jobs[i].created.Before(jobs[j].created)
 })
 views:=make([]exportJobView,0,len(jobs))
 for _,job:=range jobs {views=append(views,s.viewLocked(job))}
 return views,nil
}

// cancelLocked cancels a job, takes it out of the queue and releases what it holds.
func (s *exportJobStore) cancelLocked(job *preparedExportJob,reason,message string) {
 wasQueued:=job.view.State=="queued"
 job.cancel();job.view.State="canceled";job.view.Message=message;job.view.Reason=reason
 if wasQueued {job.view.ExpiresAt=time.Now().Add(s.limits.Lifetime)}
 s.removeQueuedLocked(job)
 s.discardLocked(job)
}

func (s *exportJobStore) cancelJob(owner,id string) error {
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if owner=="" || job==nil || job.owner!=owner || job.removed {return errExportJobMissing}
 s.cancelLocked(job,exportReasonCanceled,"Export canceled")
 return nil
}

// clearOwner cancels and releases every job the owner has and removes the records so
// they no longer count against the retained-job limit. A running job, or a ready job
// with an active download, is hidden at once and removed when it stops or its lease ends.
func (s *exportJobStore) clearOwner(owner string) (int,error) {
 s.mu.Lock();defer s.mu.Unlock()
 if s.closing || s.ctx.Err()!=nil {return 0,errExportJobsClosed}
 if owner=="" {return 0,errExportJobMissing}
 owned:=make([]*preparedExportJob,0)
 for _,job:=range s.jobs {
  if job.owner==owner && !job.removed {owned=append(owned,job)}
 }
 // Take every owned job out of the queue before releasing any capacity, so the
 // dispatch that each cancel triggers can start only other sessions' jobs.
 for _,job:=range owned {job.removed=true;s.removeQueuedLocked(job)}
 for _,job:=range owned {
  s.cancelLocked(job,exportReasonCleared,"Export cleared")
  s.removeIfDoneLocked(job)
 }
 return len(owned),nil
}

// Each download gets an independent SectionReader; concurrent Range requests
// never share a seek offset. Expiry/cancel refuses new leases but does not close
// the descriptor out from under an existing transfer. release must be deferred.
func (s *exportJobStore) acquire(owner,id string) (*io.SectionReader,func(),error) {
 s.expire(time.Now())
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if s.closing || s.ctx.Err()!=nil {return nil,nil,errExportJobsClosed}
 if owner=="" || job==nil || job.owner!=owner || job.removed || job.view.State!="ready" || job.file==nil {return nil,nil,errExportJobMissing}
 job.readers++
 reader:=io.NewSectionReader(job.file,0,job.view.Bytes)
 var once sync.Once
 release:=func(){once.Do(func(){
  s.mu.Lock();defer s.mu.Unlock()
  job.readers--
  if job.view.State!="ready" || s.closing || s.ctx.Err()!=nil {s.discardLocked(job)}
  if job.readers==0 && !job.running && (s.closing || job.view.State=="expired") {delete(s.jobs,id)}
  s.removeIfDoneLocked(job)
  s.maybeDrainedLocked()
 })}
 return reader,release,nil
}

func (s *exportJobStore) maybeDrainedLocked() {
 if !s.closing || !s.workersDone || s.drainedClosed {return}
 for _,job:=range s.jobs {if job.readers>0 || job.running {return}}
 s.cleanupErr=errors.Join(s.cleanupErr,os.Remove(s.directory))
 s.drainedClosed=true;close(s.drained)
}

// Application wiring must cancel/drain HTTP downloads before waiting here. A
// timed-out Close never closes active readers; callers may retry waiting later.
func (s *exportJobStore) Close(ctx context.Context) error {
 s.closeOnce.Do(func(){
  s.mu.Lock();s.closing=true;s.cancel();s.queue=nil
  for _,job:=range s.jobs {job.cancel();job.view.State="canceled";job.view.Message="Export service stopping";job.view.Reason=exportReasonStopping}
  s.mu.Unlock()
  go func(){
   s.workers.Wait()
   s.mu.Lock();defer s.mu.Unlock()
   s.workersDone=true
   for id,job:=range s.jobs {s.discardLocked(job);if job.readers==0 {delete(s.jobs,id)}}
   s.maybeDrainedLocked()
  }()
 })
 select {
 case <-s.drained:s.mu.Lock();defer s.mu.Unlock();return s.cleanupErr
 case <-ctx.Done():return ctx.Err()
 }
}
