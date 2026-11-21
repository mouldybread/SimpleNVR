package main

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "strconv"
 "sync"
 "time"
)

var (
 errExportJobsBusy = errors.New("export job capacity exceeded")
 errExportJobMissing = errors.New("export job unavailable")
 errExportJobsClosed = errors.New("export service stopping")
 errExportJobSize = errors.New("export output size limit exceeded")
)

type exportJobLimits struct {
 MaxBytes, TotalBytes int64
 MaxJobs, Parallel int
 Lifetime, Preparation time.Duration
}

func defaultExportJobLimits() exportJobLimits {
 return exportJobLimits{MaxBytes:8<<30, TotalBytes:16<<30, MaxJobs:8, Parallel:1, Lifetime:30*time.Minute, Preparation:30*time.Minute}
}

func (l exportJobLimits) validate() error {
 if l.MaxBytes<1 || l.TotalBytes<l.MaxBytes || l.MaxJobs<1 || l.MaxJobs>64 || l.Parallel<1 || l.Parallel>l.MaxJobs || l.Lifetime<=0 || l.Lifetime>24*time.Hour || l.Preparation<=0 || l.Preparation>24*time.Hour {
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
 for name,target:=range map[string]*int{"NVR_EXPORT_MAX_JOBS":&limits.MaxJobs,"NVR_EXPORT_PARALLEL":&limits.Parallel} {
  if raw:=getenv(name);raw!="" {value,err:=strconv.Atoi(raw);if err!=nil {return limits,errors.New("invalid "+name)};*target=value}
 }
 for name,target:=range map[string]*time.Duration{"NVR_EXPORT_TTL":&limits.Lifetime,"NVR_EXPORT_PREPARE_TIMEOUT":&limits.Preparation} {
  if raw:=getenv(name);raw!="" {value,err:=time.ParseDuration(raw);if err!=nil {return limits,errors.New("invalid "+name)};*target=value}
 }
 return limits,limits.validate()
}

type exportJobView struct {
 ID string `json:"id"`
 State string `json:"state"`
 Message string `json:"message"`
 Bytes int64 `json:"bytes"`
 ExpiresAt time.Time `json:"expires_at"`
}

type preparedExportJob struct {
 view exportJobView
 owner string
 ctx context.Context
 cancel context.CancelFunc
 file *os.File
 running bool
 readers int
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
 reserved int64
 slots chan struct{}
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
 store:=&exportJobStore{limits:limits,directory:directory,jobs:make(map[string]*preparedExportJob),slots:make(chan struct{},limits.Parallel),ctx:ctx,cancel:cancel,drained:make(chan struct{})}
 store.workers.Add(1)
 go func(){
  defer store.workers.Done()
  interval:=limits.Lifetime/2;if interval>time.Minute {interval=time.Minute};if interval<time.Millisecond {interval=time.Millisecond}
  ticker:=time.NewTicker(interval);defer ticker.Stop()
  for {select {case <-ctx.Done():return;case now:=<-ticker.C:store.expire(now)}}
 }()
 return store,nil
}

func (s *exportJobStore) submit(owner string,render exportJobRenderer) (exportJobView,error) {
 if owner=="" || render==nil {return exportJobView{},errors.New("export owner and renderer required")}
 raw:=make([]byte,32);if _,err:=rand.Read(raw);err!=nil {return exportJobView{},err}
 id:=hex.EncodeToString(raw)
 s.mu.Lock();defer s.mu.Unlock()
 if s.closing || s.ctx.Err()!=nil {return exportJobView{},errExportJobsClosed}
 // Reserve each admitted job's maximum output, including queued jobs and
 // expired outputs still leased to downloads. This bounds total disk use.
 if len(s.jobs)>=s.limits.MaxJobs || s.limits.MaxBytes>s.limits.TotalBytes-s.reserved {return exportJobView{},errExportJobsBusy}
 ctx,cancel:=context.WithTimeout(s.ctx,s.limits.Preparation)
 job:=&preparedExportJob{view:exportJobView{ID:id,State:"queued",Message:"Queued for export preparation",ExpiresAt:time.Now().Add(s.limits.Lifetime)},owner:owner,ctx:ctx,cancel:cancel,running:true,reserved:s.limits.MaxBytes}
 s.jobs[id]=job;s.reserved+=job.reserved
 s.workers.Add(1)
 go s.run(job,render)
 return job.view,nil
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

func (s *exportJobStore) run(job *preparedExportJob,render exportJobRenderer) {
 defer s.workers.Done();defer job.cancel()
 select {
 case s.slots<-struct{}{}:defer func(){<-s.slots}()
 case <-job.ctx.Done():s.finish(job,nil,0,job.ctx.Err());return
 }
 if err:=job.ctx.Err();err!=nil {s.finish(job,nil,0,err);return}
 s.mu.Lock()
 if job.view.State!="queued" {s.mu.Unlock();s.finish(job,nil,0,context.Canceled);return}
 job.view.State="preparing";job.view.Message="Preparing export on server"
 s.mu.Unlock()
 file,err:=os.CreateTemp(s.directory,"output-*.mp4")
 if err!=nil {s.finish(job,nil,0,err);return}
 // CreateTemp is owner-only. Unlink before handing the descriptor to a
 // renderer; the kernel releases output bytes after the last reader closes.
 if err:=os.Remove(file.Name());err!=nil {closeErr:=file.Close();removeErr:=os.Remove(file.Name());s.mu.Lock();s.cleanupErr=errors.Join(s.cleanupErr,closeErr,removeErr);s.mu.Unlock();s.finish(job,nil,0,err);return}
 writer:=&boundedExportJobWriter{ctx:job.ctx,file:file,limit:s.limits.MaxBytes}
 err=render(job.ctx,writer)
 if writer.exceeded {err=errExportJobSize}
 if err==nil {err=job.ctx.Err()}
 if err==nil && writer.written==0 {err=errors.New("empty export")}
 s.finish(job,file,writer.written,err)
}

func (s *exportJobStore) finish(job *preparedExportJob,file *os.File,size int64,err error) {
 s.mu.Lock();defer s.mu.Unlock()
 job.running=false;job.file=file
 if job.view.State=="canceled" || job.view.State=="expired" || s.closing || s.ctx.Err()!=nil {
  if job.view.State!="expired" {job.view.State="canceled";job.view.Message="Export canceled"}
 } else if err!=nil {
  job.view.State="failed";job.view.Message="Export preparation failed; no download is ready"
  if errors.Is(err,errExportJobSize) {job.view.Message="Export exceeds the configured server output limit"}
  if errors.Is(err,context.DeadlineExceeded) {job.view.Message="Export preparation deadline exceeded"}
 } else {
  job.view.State="ready";job.view.Message="Ready to download";job.view.Bytes=size
 }
 job.view.ExpiresAt=time.Now().Add(s.limits.Lifetime)
 if job.view.State!="ready" {s.discardLocked(job)}
 if s.closing || job.view.State=="expired" {delete(s.jobs,job.view.ID)}
}

func (s *exportJobStore) discardLocked(job *preparedExportJob) {
 if job.running || job.readers>0 {return}
 if job.file!=nil {s.cleanupErr=errors.Join(s.cleanupErr,job.file.Close());job.file=nil}
 s.reserved-=job.reserved;job.reserved=0
}

func (s *exportJobStore) expire(now time.Time) {
 s.mu.Lock();defer s.mu.Unlock()
 for id,job:=range s.jobs {
  if now.Before(job.view.ExpiresAt) {continue}
  job.view.State="expired";job.view.Message="Prepared export expired";job.cancel()
  s.discardLocked(job)
  if !job.running && job.readers==0 {delete(s.jobs,id)}
 }
}

func (s *exportJobStore) status(owner,id string) (exportJobView,error) {
 s.expire(time.Now())
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if s.closing || s.ctx.Err()!=nil {return exportJobView{},errExportJobsClosed}
 if owner=="" || job==nil || job.owner!=owner {return exportJobView{},errExportJobMissing}
 return job.view,nil
}

func (s *exportJobStore) cancelJob(owner,id string) error {
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if owner=="" || job==nil || job.owner!=owner {return errExportJobMissing}
 job.cancel();job.view.State="canceled";job.view.Message="Export canceled"
 s.discardLocked(job)
 return nil
}

// Each download gets an independent SectionReader; concurrent Range requests
// never share a seek offset. Expiry/cancel refuses new leases but does not close
// the descriptor out from under an existing transfer. release must be deferred.
func (s *exportJobStore) acquire(owner,id string) (*io.SectionReader,func(),error) {
 s.expire(time.Now())
 s.mu.Lock();defer s.mu.Unlock()
 job:=s.jobs[id]
 if s.closing || s.ctx.Err()!=nil {return nil,nil,errExportJobsClosed}
 if owner=="" || job==nil || job.owner!=owner || job.view.State!="ready" || job.file==nil {return nil,nil,errExportJobMissing}
 job.readers++
 reader:=io.NewSectionReader(job.file,0,job.view.Bytes)
 var once sync.Once
 release:=func(){once.Do(func(){
  s.mu.Lock();defer s.mu.Unlock()
  job.readers--
  if job.view.State!="ready" || s.closing || s.ctx.Err()!=nil {s.discardLocked(job)}
  if job.readers==0 && !job.running && (s.closing || job.view.State=="expired") {delete(s.jobs,id)}
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
  s.mu.Lock();s.closing=true;s.cancel()
  for _,job:=range s.jobs {job.cancel();job.view.State="canceled";job.view.Message="Export service stopping"}
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
