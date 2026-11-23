package main

import (
 "context"
 "errors"
 "io"
 "strings"
 "testing"
 "time"
)

func awaitExportStoreIdle(t *testing.T,s *exportJobStore) {
 t.Helper();deadline:=time.After(3*time.Second);ticker:=time.NewTicker(time.Millisecond);defer ticker.Stop()
 for {
  s.mu.Lock();records,reserved:=len(s.jobs),s.reserved;s.mu.Unlock()
  if records==0 && reserved==0 {return}
  select {case <-deadline:t.Fatalf("store did not go idle: %d records, %d bytes reserved",records,reserved);case <-ticker.C:}
 }
}

func TestExportJobsReportFailureReasons(t *testing.T) {
 write:=func(text string) exportJobRenderer {return func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,text);return err}}
 for _,tc:=range []struct{name string;estimate int64;maxBytes int64;render exportJobRenderer;reason,message string}{
  {"output limit",0,4,write("12345"),exportReasonOutputLimit,"output limit"},
  {"estimate exceeded",10,100,write(strings.Repeat("x",20)),exportReasonEstimateExceeded,"estimated"},
  {"empty output",0,100,func(context.Context,io.Writer)error{return nil},exportReasonEmptyOutput,"preparation failed"},
  {"render failed",0,100,func(context.Context,io.Writer)error{return errors.New("rtsp://user:SECRET@camera/private")},exportReasonRenderFailed,"preparation failed"},
 } {t.Run(tc.name,func(t *testing.T){
  s:=testExportJobs(t,tc.maxBytes,tc.maxBytes)
  job,err:=s.submitEstimated("owner",tc.estimate,tc.render);if err!=nil {t.Fatal(err)}
  failed:=awaitExportJob(t,s,job.ID,"failed")
  if failed.Reason!=tc.reason || !strings.Contains(failed.Message,tc.message) || strings.Contains(failed.Message,"SECRET") {t.Fatalf("%+v",failed)}
 })}
}

func TestExportJobsDeadlineReasonWhileRunningAndQueued(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=16;limits.Parallel=1;limits.Preparation=50*time.Millisecond;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 blocking:=func(ctx context.Context,w io.Writer)error{<-ctx.Done();return ctx.Err()}
 running,err:=s.submit("owner",blocking);if err!=nil {t.Fatal(err)}
 queued,err:=s.submit("owner",blocking);if err!=nil {t.Fatal(err)}
 for _,id:=range []string{running.ID,queued.ID} {
  failed:=awaitExportJob(t,s,id,"failed")
  if failed.Reason!=exportReasonDeadline || !strings.Contains(failed.Message,"deadline") {t.Fatalf("%+v",failed)}
 }
}

func TestExportJobsCancelAndShutdownReasons(t *testing.T) {
 s:=testExportJobs(t,8,24)
 hold:=make(chan struct{})
 blocking:=func(ctx context.Context,w io.Writer)error{select {case <-hold:case <-ctx.Done():return ctx.Err()};return nil}
 running,err:=s.submit("owner",blocking);if err!=nil {t.Fatal(err)}
 queued,err:=s.submit("owner",blocking);if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,running.ID,"preparing")
 if err:=s.cancelJob("owner",queued.ID);err!=nil {t.Fatal(err)}
 if view,err:=s.status("owner",queued.ID);err!=nil || view.State!="canceled" || view.Reason!=exportReasonCanceled {t.Fatalf("%+v %v",view,err)}
 if err:=s.cancelJob("owner",running.ID);err!=nil {t.Fatal(err)}
 canceled:=awaitExportJob(t,s,running.ID,"canceled")
 if canceled.Reason!=exportReasonCanceled {t.Fatalf("%+v",canceled)}
 ready,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"ok");return err});if err!=nil {t.Fatal(err)}
 if view:=awaitExportJob(t,s,ready.ID,"ready");view.Reason!="" {t.Fatalf("ready job has a reason: %+v",view)}
 s.expire(time.Now().Add(2*time.Hour))
 if _,err:=s.status("owner",ready.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("expired job still listed")}
}

func TestExportJobsListAndClear(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=16;limits.MaxJobs=4;limits.MaxQueued=4;limits.Parallel=1;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 ready,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"x");return err});if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,ready.ID,"ready")
 reader,release,err:=s.acquire("owner",ready.ID);if err!=nil {t.Fatal(err)};defer release()
 hold:=make(chan struct{})
 running,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{select {case <-hold:case <-ctx.Done():return ctx.Err()};_,err:=io.WriteString(w,"y");return err});if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,running.ID,"preparing")
 queued,err:=s.submit("owner",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"z");return err});if err!=nil {t.Fatal(err)}
 other,err:=s.submit("other",func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"o");return err});if err!=nil {t.Fatal(err)}
 views,err:=s.list("owner");if err!=nil {t.Fatal(err)}
 if len(views)!=3 || views[0].ID!=ready.ID || views[1].ID!=running.ID || views[2].ID!=queued.ID {t.Fatalf("list order or contents wrong: %+v",views)}
 if views[0].State!="ready" || views[0].Bytes!=1 || views[1].State!="preparing" || views[1].EstimatedBytes!=8 || views[2].State!="queued" || views[2].QueuePosition!=1 || views[2].EstimatedBytes!=8 {t.Fatalf("job fields wrong: %+v",views)}
 if mine,err:=s.list("nobody");err!=nil || len(mine)!=0 {t.Fatalf("unknown owner saw jobs: %+v %v",mine,err)}
 if n,err:=s.clearOwner("nobody");err!=nil || n!=0 {t.Fatalf("cleared another session's jobs: %d %v",n,err)}
 cleared,err:=s.clearOwner("owner");if err!=nil || cleared!=3 {t.Fatalf("cleared %d: %v",cleared,err)}
 if after,err:=s.list("owner");err!=nil || len(after)!=0 {t.Fatalf("cleared jobs still listed: %+v %v",after,err)}
 for _,id:=range []string{ready.ID,running.ID,queued.ID} {
  if _,err:=s.status("owner",id);!errors.Is(err,errExportJobMissing) {t.Fatal("cleared job still has status",id)}
  if _,_,err:=s.acquire("owner",id);!errors.Is(err,errExportJobMissing) {t.Fatal("cleared job still downloadable",id)}
  if err:=s.cancelJob("owner",id);!errors.Is(err,errExportJobMissing) {t.Fatal("cleared job can still be canceled",id)}
 }
 if view,err:=s.status("other",other.ID);err!=nil || view.State=="canceled" {t.Fatalf("another session's job was affected: %+v %v",view,err)}
 data,err:=io.ReadAll(reader);if err!=nil || string(data)!="x" {t.Fatalf("clearing interrupted an active download: %q %v",data,err)}
 release()
 close(hold)
 if view:=awaitExportJob(t,s,other.ID,"ready");view.Bytes!=1 {t.Fatalf("%+v",view)}
 if n,err:=s.clearOwner("other");err!=nil || n!=1 {t.Fatalf("cleared %d: %v",n,err)}
 awaitExportStoreIdle(t,s)
}

func TestExportJobsClearFreesCapacity(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=8;limits.MaxJobs=2;limits.MaxQueued=2;limits.Parallel=1;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 render:=func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"v");return err}
 for i:=0;i<2;i++ {job,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)};awaitExportJob(t,s,job.ID,"ready")}
 if _,err:=s.submit("owner",render);!errors.Is(err,errExportJobsBusy) {t.Fatal("retained-job limit not enforced")}
 if n,err:=s.clearOwner("owner");err!=nil || n!=2 {t.Fatalf("cleared %d: %v",n,err)}
 next,err:=s.submit("owner",render);if err!=nil {t.Fatalf("cleared records still count against the limit: %v",err)}
 awaitExportJob(t,s,next.ID,"ready")
}
