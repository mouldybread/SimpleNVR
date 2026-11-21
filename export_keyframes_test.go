package main

import (
 "context"
 "errors"
 "io"
 "strings"
 "testing"
 "time"
)
func TestCopyKeyframeBoundaries(t *testing.T){
 keys:=[]time.Duration{0,time.Second,2*time.Second,3*time.Second}
 for _,tc:=range []struct{target time.Duration;later bool;want time.Duration}{{1500*time.Millisecond,false,time.Second},{1500*time.Millisecond,true,2*time.Second},{time.Second,false,time.Second},{3500*time.Millisecond,true,4*time.Second},{4*time.Second,true,4*time.Second}}{got,err:=copyBoundary(keys,tc.target,4*time.Second,tc.later);if err!=nil || got!=tc.want{t.Fatalf("%+v: %v %v",tc,got,err)}}
 if _,err:=copyBoundary([]time.Duration{time.Second},500*time.Millisecond,4*time.Second,false);!errors.Is(err,errCopyReordered){t.Fatal("missing safe start accepted")}
}
func TestCopyApprovalBeforeJobAdmission(t *testing.T){
 _,service,h,cookie:=exportHTTPFixture(t)
 calls:=0
 service.plan=func(ctx context.Context,req exportCreateRequest)(exportJobRenderer,error){calls++;if req.AcceptedStart==""{return nil,&exportCopyProposal{Code:"copy_range_approval_required",RequestedStart:req.Start,RequestedEnd:req.End,ExportStart:"2026-11-21T00:00:00Z",ExportEnd:"2026-11-21T00:00:11Z",AudioAllowanceMS:64}};return func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"video");return err},nil}
 first:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody)
 if first.Code!=409 || !strings.Contains(first.Body.String(),"copy_range_approval_required"){t.Fatal(first.Code,first.Body.String())}
 service.store.mu.Lock();count:=len(service.store.jobs);reserved:=service.store.reserved;service.store.mu.Unlock()
 if count!=0 || reserved!=0{t.Fatal("proposal reserved a job or output storage")}
 approved:=strings.TrimSuffix(exportHTTPCreateBody,"}")+`,"accepted_start":"2026-11-21T00:00:00Z","accepted_end":"2026-11-21T00:00:11Z"}`
 next:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,approved);if next.Code!=202 || calls!=2{t.Fatal(next.Code,next.Body.String(),calls)}
 wrong:=strings.TrimSuffix(exportHTTPCreateBody,"}")+`,"mode":"accurate","accepted_start":"2026-11-21T00:00:00Z","accepted_end":"2026-11-21T00:00:11Z"}`
 if got:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,wrong).Code;got!=400{t.Fatal("accurate mode accepted copy approval",got)}
}
func TestCopyStaleApprovalRequiresNewConsent(t *testing.T){
 start:=time.Date(2026,11,21,0,0,0,0,time.UTC)
 requested:=exportPlan{Start:start.Add(1500*time.Millisecond),End:start.Add(2500*time.Millisecond)}
 actual:=exportPlan{Start:start.Add(time.Second),End:start.Add(3*time.Second)}
 req:=exportCreateRequest{Start:requested.Start.Format(time.RFC3339Nano),End:requested.End.Format(time.RFC3339Nano),Mode:"copy",AcceptedStart:start.Format(time.RFC3339Nano),AcceptedEnd:actual.End.Format(time.RFC3339Nano)}
 var proposal *exportCopyProposal
 if err:=copyProposal(req,requested,actual,nil);!errors.As(err,&proposal){t.Fatal("stale interval was approved")}
 req.AcceptedStart=actual.Start.Format(time.RFC3339Nano)
 if err:=copyProposal(req,requested,actual,nil);err!=nil{t.Fatal(err)}
}
