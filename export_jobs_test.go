package main

import (
 "context"
 "errors"
 "io"
 "os"
 "strings"
 "testing"
 "time"
)

func testExportJobs(t *testing.T,maxBytes int64,total int64) *exportJobStore {
 t.Helper()
 limits:=defaultExportJobLimits();limits.MaxBytes=maxBytes;limits.TotalBytes=total;limits.Lifetime=time.Hour
 return testExportJobsWithLimits(t,limits)
}

func testExportJobsWithLimits(t *testing.T,limits exportJobLimits) *exportJobStore {
 t.Helper()
 store,err:=newExportJobStore(context.Background(),t.TempDir(),limits);if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel();if err:=store.Close(ctx);err!=nil {t.Error(err)}})
 return store
}

func awaitExportJob(t *testing.T,s *exportJobStore,id,state string) exportJobView {
 t.Helper();deadline:=time.After(3*time.Second);ticker:=time.NewTicker(time.Millisecond);defer ticker.Stop()
 for {
  view,err:=s.status("owner",id);if err!=nil {t.Fatal(err)}
  if view.State==state {return view}
  select {case <-deadline:t.Fatalf("wanted %s, got %+v",state,view);case <-ticker.C:}
 }
}

func TestExportJobLimits(t *testing.T) {
 defaults,err:=loadExportJobLimits(func(string)string{return ""});if err!=nil || defaults.MaxBytes!=8<<30 || defaults.TotalBytes!=16<<30 || defaults.MaxQueued!=4 {t.Fatalf("%+v %v",defaults,err)}
 for _,values:=range []map[string]string{
  {"NVR_EXPORT_MAX_BYTES":"0"},
  {"NVR_EXPORT_MAX_BYTES":"no"},
  {"NVR_EXPORT_MAX_BYTES":"9223372036854775808"},
  {"NVR_EXPORT_TOTAL_BYTES":"1"},
  {"NVR_EXPORT_MAX_JOBS":"65"},
  {"NVR_EXPORT_MAX_QUEUED":"0"},
  {"NVR_EXPORT_MAX_QUEUED":"65"},
  {"NVR_EXPORT_PARALLEL":"0"},
  {"NVR_EXPORT_TTL":"0s"},
  {"NVR_EXPORT_PREPARE_TIMEOUT":"25h"},
 } {if _,err:=loadExportJobLimits(func(k string)string{return values[k]});err==nil {t.Fatalf("accepted invalid limits: %v",values)}}
 values:=map[string]string{"NVR_EXPORT_MAX_BYTES":"1073741824","NVR_EXPORT_TOTAL_BYTES":"2147483648","NVR_EXPORT_TTL":"1h","NVR_EXPORT_MAX_QUEUED":"2"}
 custom,err:=loadExportJobLimits(func(k string)string{return values[k]});if err!=nil || custom.MaxBytes!=1<<30 || custom.Lifetime!=time.Hour || custom.MaxQueued!=2 {t.Fatalf("%+v %v",custom,err)}
}

func TestExportJobsReadyAuthorizationAndIndependentReaders(t *testing.T) {
 s:=testExportJobs(t,1024,2048)
 view,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"abcdef");return err});if err!=nil {t.Fatal(err)}
 ready:=awaitExportJob(t,s,view.ID,"ready");if ready.Bytes!=6 {t.Fatal(ready)}
 if _,err:=s.status("other",view.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("cross-owner status allowed")}
 if err:=s.cancelJob("other",view.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("cross-owner cancellation allowed")}
 if _,_,err:=s.acquire("other",view.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("cross-owner download allowed")}
 a,releaseA,err:=s.acquire("owner",view.ID);if err!=nil {t.Fatal(err)};defer releaseA()
 b,releaseB,err:=s.acquire("owner",view.ID);if err!=nil {t.Fatal(err)};defer releaseB()
 if _,err:=a.Seek(3,io.SeekStart);err!=nil {t.Fatal(err)}
 first,_:=io.ReadAll(a);second,_:=io.ReadAll(b)
 if string(first)!="def" || string(second)!="abcdef" {t.Fatalf("shared download offsets: %q %q",first,second)}
 files,err:=os.ReadDir(s.directory);if err!=nil || len(files)!=0 {t.Fatalf("named temporary output remains: %v %v",files,err)}
}

func TestExportJobsBoundOutputAndSanitizeErrors(t *testing.T) {
 for _,tc:=range []struct{name string;render exportJobRenderer;message string}{
  {"size",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"12345");return err},"output limit"},
  {"ignored write failure",func(ctx context.Context,w io.Writer)error{w.Write([]byte("12345"));return nil},"output limit"},
  {"empty",func(context.Context,io.Writer)error{return nil},"preparation failed"},
  {"secret",func(context.Context,io.Writer)error{return errors.New("rtsp://user:SECRET@camera/private")},"preparation failed"},
 } {t.Run(tc.name,func(t *testing.T){
  s:=testExportJobs(t,4,4);job,err:=s.submit("owner",tc.render);if err!=nil {t.Fatal(err)}
  failed:=awaitExportJob(t,s,job.ID,"failed")
  if !strings.Contains(failed.Message,tc.message) || strings.Contains(failed.Message,"SECRET") {t.Fatal(failed)}
  s.mu.Lock();reserved:=s.reserved;s.mu.Unlock();if reserved!=0 {t.Fatal("failed output reservation leaked")}
  if _,_,err:=s.acquire("owner",job.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("failed output downloadable")}
 })}
}

func TestExportJobsExpiryRetainsActiveTransferAndQuota(t *testing.T) {
 s:=testExportJobs(t,8,8)
 render:=func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"video");return err}
 job,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)};awaitExportJob(t,s,job.ID,"ready")
 reader,release,err:=s.acquire("owner",job.ID);if err!=nil {t.Fatal(err)};defer release()
 s.expire(time.Now().Add(2*time.Hour))
 if _,_,err:=s.acquire("owner",job.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("expired output newly downloadable")}
 next,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)}
 if view,err:=s.status("owner",next.ID);err!=nil || view.State!="queued" {t.Fatalf("job started although an active transfer still holds the budget: %+v %v",view,err)}
 data,err:=io.ReadAll(reader);if err!=nil || string(data)!="video" {t.Fatalf("expiry interrupted active download: %q %v",data,err)}
 release();release()
 awaitExportJob(t,s,next.ID,"ready")
}

func TestExportJobsCancelQueueAndShutdown(t *testing.T) {
 s:=testExportJobs(t,8,24)
 started:=make(chan struct{});stopped:=make(chan struct{})
 first,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{close(started);<-ctx.Done();close(stopped);return ctx.Err()});if err!=nil {t.Fatal(err)}
 select {case <-started:case <-time.After(3*time.Second):t.Fatal("renderer never started")}
 called:=make(chan struct{},1)
 second,err:=s.submit("owner",func(context.Context,io.Writer)error{called<-struct{}{};return nil});if err!=nil {t.Fatal(err)}
 if err:=s.cancelJob("owner",second.ID);err!=nil {t.Fatal(err)}
 if err:=s.cancelJob("owner",first.ID);err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 if err:=s.Close(ctx);err!=nil {t.Fatal(err)}
 select {case <-stopped:default:t.Fatal("shutdown returned before renderer joined")}
 select {case <-called:t.Fatal("canceled queued renderer ran");default:}
 if _,err:=os.Stat(s.directory);!os.IsNotExist(err) {t.Fatalf("scratch directory not removed: %v",err)}
 if _,err:=s.submit("owner",func(context.Context,io.Writer)error{return nil});!errors.Is(err,errExportJobsClosed) {t.Fatal("shutdown admitted new export")}
}

func TestExportJobsShutdownWaitsForDownloadLease(t *testing.T) {
 s:=testExportJobs(t,8,8)
 job,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"video");return err});if err!=nil {t.Fatal(err)};awaitExportJob(t,s,job.ID,"ready")
 reader,release,err:=s.acquire("owner",job.ID);if err!=nil {t.Fatal(err)};defer release()
 short,cancel:=context.WithTimeout(context.Background(),10*time.Millisecond);defer cancel()
 if err:=s.Close(short);!errors.Is(err,context.DeadlineExceeded) {t.Fatalf("did not wait for reader: %v",err)}
 data,err:=io.ReadAll(reader);if err!=nil || string(data)!="video" {t.Fatal("shutdown closed active reader")}
 release()
 ctx,stop:=context.WithTimeout(context.Background(),3*time.Second);defer stop()
 if err:=s.Close(ctx);err!=nil {t.Fatal(err)}
}

func TestExportJobsQueueLimitAndOrder(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=64;limits.MaxQueued=1;limits.Parallel=1;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 started:=make(chan struct{});hold:=make(chan struct{});order:=make(chan string,2)
 first,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{
  order<-"first";close(started)
  select {case <-hold:case <-ctx.Done():return ctx.Err()}
  _,err:=io.WriteString(w,"one");return err
 });if err!=nil {t.Fatal(err)}
 select {case <-started:case <-time.After(3*time.Second):t.Fatal("first renderer never started")}
 second,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{order<-"second";_,err:=io.WriteString(w,"two");return err});if err!=nil {t.Fatal(err)}
 if second.State!="queued" {t.Fatalf("second job state %q, want queued",second.State)}
 if view,err:=s.status("owner",second.ID);err!=nil || view.State!="queued" {t.Fatalf("second job started while the only slot was busy: %+v %v",view,err)}
 if _,err:=s.submit("owner",func(context.Context,io.Writer)error{return nil});!errors.Is(err,errExportJobsBusy) {t.Fatal("queue limit not enforced")}
 close(hold)
 awaitExportJob(t,s,first.ID,"ready");awaitExportJob(t,s,second.ID,"ready")
 if a,b:=<-order,<-order;a!="first" || b!="second" {t.Fatalf("jobs ran out of order: %s %s",a,b)}
}

func TestExportJobsReserveEstimateNotMaximum(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=100;limits.TotalBytes=100;limits.Parallel=3;limits.MaxQueued=4;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 hold:=make(chan struct{})
 blocking:=func(ctx context.Context,w io.Writer)error{
  select {case <-hold:case <-ctx.Done():return ctx.Err()}
  _,err:=io.WriteString(w,"x");return err
 }
 a,err:=s.submitEstimated("owner",30,blocking);if err!=nil {t.Fatal(err)}
 b,err:=s.submitEstimated("owner",30,blocking);if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,a.ID,"preparing");awaitExportJob(t,s,b.ID,"preparing")
 s.mu.Lock();reserved:=s.reserved;s.mu.Unlock()
 if reserved!=60 {t.Fatalf("reserved %d bytes for two 30-byte estimates, want 60",reserved)}
 c,err:=s.submitEstimated("owner",50,func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"c");return err});if err!=nil {t.Fatal(err)}
 if view,err:=s.status("owner",c.ID);err!=nil || view.State!="queued" {t.Fatalf("job started without room for its estimate: %+v %v",view,err)}
 close(hold)
 awaitExportJob(t,s,c.ID,"ready")
}

func TestExportJobsEstimateClampedToMaximum(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=100;limits.TotalBytes=100;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 hold:=make(chan struct{})
 job,err:=s.submitEstimated("owner",1000,func(ctx context.Context,w io.Writer)error{
  select {case <-hold:case <-ctx.Done():return ctx.Err()}
  _,err:=io.WriteString(w,"d");return err
 });if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,job.ID,"preparing")
 s.mu.Lock();reserved:=s.reserved;s.mu.Unlock()
 if reserved!=100 {t.Fatalf("an estimate above the per-job maximum reserved %d bytes, want 100",reserved)}
 close(hold)
 ready:=awaitExportJob(t,s,job.ID,"ready")
 s.mu.Lock();reserved=s.reserved;s.mu.Unlock()
 if ready.Bytes!=1 || reserved!=1 {t.Fatalf("reservation did not shrink to the actual output size: bytes %d reserved %d",ready.Bytes,reserved)}
}

func TestExportJobsExpiryStartsAtCompletion(t *testing.T) {
 s:=testExportJobs(t,8,16)
 hold:=make(chan struct{})
 running,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{
  select {case <-hold:case <-ctx.Done():return ctx.Err()}
  _,err:=io.WriteString(w,"ok");return err
 });if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,running.ID,"preparing")
 queued,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"ok");return err});if err!=nil {t.Fatal(err)}
 s.expire(time.Now().Add(2*time.Hour))
 if view,err:=s.status("owner",running.ID);err!=nil || view.State!="preparing" {t.Fatalf("running job expired before completing: %+v %v",view,err)}
 if view,err:=s.status("owner",queued.ID);err!=nil || view.State!="queued" {t.Fatalf("queued job expired before starting: %+v %v",view,err)}
 completed:=time.Now()
 close(hold)
 ready:=awaitExportJob(t,s,running.ID,"ready")
 if ready.ExpiresAt.Before(completed.Add(time.Hour-time.Minute)) {t.Fatalf("expiry timer did not start at completion: %v",ready.ExpiresAt)}
 awaitExportJob(t,s,queued.ID,"ready")
}
