package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "testing"
 "time"
)

// An expired job keeps its record, listed as expired, until its session clears it.
func TestExportJobsExpiredStayVisibleUntilCleared(t *testing.T) {
 s:=testExportJobs(t,8,16)
 render:=func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"ok");return err}
 job,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,job.ID,"ready")
 s.expire(time.Now().Add(2*time.Hour))
 s.expire(time.Now().Add(3*time.Hour))
 views,err:=s.list("owner")
 if err!=nil || len(views)!=1 || views[0].State!="expired" || views[0].Reason!=exportReasonExpired {t.Fatalf("expired job not listed as expired: %+v %v",views,err)}
 if _,_,err:=s.acquire("owner",job.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("expired output is downloadable")}
 if err:=s.cancelJob("owner",job.ID);err!=nil {t.Fatalf("canceling an expired job failed: %v",err)}
 if view,err:=s.status("owner",job.ID);err!=nil || view.State!="expired" || view.Reason!=exportReasonExpired {t.Fatalf("cancel rewrote an expired job: %+v %v",view,err)}
 s.mu.Lock();reserved:=s.reserved;s.mu.Unlock()
 if reserved!=0 {t.Fatalf("expired job still holds %d reserved bytes",reserved)}
 if n,err:=s.clearOwner("owner");err!=nil || n!=1 {t.Fatalf("clear removed %d records: %v",n,err)}
 if after,err:=s.list("owner");err!=nil || len(after)!=0 {t.Fatalf("cleared expired record is still listed: %+v %v",after,err)}
}

// Expired records never block new work: when the store is full the oldest are evicted.
func TestExportJobsFullStoreEvictsOldestExpiredRecord(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=64;limits.MaxJobs=2;limits.MaxQueued=2;limits.Parallel=1;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 render:=func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"ok");return err}
 first,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,first.ID,"ready")
 second,err:=s.submit("owner",render);if err!=nil {t.Fatal(err)}
 awaitExportJob(t,s,second.ID,"ready")
 if _,err:=s.submit("owner",render);!errors.Is(err,errExportJobsBusy) {t.Fatal("a full store with nothing expired admitted a job")}
 s.expire(time.Now().Add(2*time.Hour))
 third,err:=s.submit("owner",render);if err!=nil {t.Fatalf("expired records were not evicted to make room: %v",err)}
 views,err:=s.list("owner");if err!=nil {t.Fatal(err)}
 if len(views)!=2 || views[0].ID!=second.ID || views[0].State!="expired" || views[1].ID!=third.ID {t.Fatalf("wrong records after eviction: %+v",views)}
 if _,err:=s.status("owner",first.ID);!errors.Is(err,errExportJobMissing) {t.Fatal("the oldest expired record was not the one evicted")}
}

// A page that was reloaded only has the server's list, so the list must carry the
// camera, mode and requested range of each job.
func TestExportListKeepsRequestDetailsForReload(t *testing.T) {
 _,service,h,cookie:=exportHTTPFixture(t)
 service.plan=func(ctx context.Context,req exportCreateRequest)(exportJobRenderer,error){
  if sink,ok:=ctx.Value(exportSavedRangeKey{}).(*exportSavedRange);ok {sink.CameraName="Front door"}
  return func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"0123456789");return err},nil
 }
 created:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody)
 if created.Code!=202 {t.Fatal(created.Code,created.Body.String())}
 var first exportHTTPView
 if err:=json.Unmarshal(created.Body.Bytes(),&first);err!=nil {t.Fatal(err)}
 list:=listHTTPExports(t,h,cookie)
 if len(list.Jobs)!=1 {t.Fatalf("unexpected list: %+v",list)}
 for _,view:=range []exportHTTPView{first,list.Jobs[0]} {
  if view.CameraID!="one" || view.CameraName!="Front door" || view.Mode!="copy" || view.RequestedStart!="2026-11-21T00:00:00Z" || view.RequestedEnd!="2026-11-21T00:00:10Z" {t.Fatalf("request details missing: %+v",view)}
 }
 status:=exportHTTPRequest(h,cookie,"GET",exportStatusPath+"?id="+first.ID,"")
 var polled exportHTTPView
 if status.Code!=200 || json.Unmarshal(status.Body.Bytes(),&polled)!=nil || polled.CameraName!="Front door" || polled.RequestedEnd!="2026-11-21T00:00:10Z" {t.Fatal(status.Code,status.Body.String())}
}
