package main

import (
 "context"
 "errors"
 "io"
 "math"
 "strings"
 "testing"
 "time"
)
func TestCopyKeyframeBoundaries(t *testing.T){
 keys:=[]time.Duration{0,time.Second,2*time.Second,3*time.Second}
 for _,tc:=range []struct{target time.Duration;later bool;want time.Duration}{{1500*time.Millisecond,false,time.Second},{1500*time.Millisecond,true,2*time.Second},{time.Second,false,time.Second},{3500*time.Millisecond,true,4*time.Second},{4*time.Second,true,4*time.Second}}{got,err:=copyBoundary(keys,tc.target,4*time.Second,tc.later);if err!=nil || got!=tc.want{t.Fatalf("%+v: %v %v",tc,got,err)}}
 if _,err:=copyBoundary([]time.Duration{time.Second},500*time.Millisecond,4*time.Second,false);!errors.Is(err,errCopyUnsafeCut){t.Fatal("missing safe start accepted")}
}
func TestCopyUnsafeCutError(t *testing.T){
 if !errors.Is(errCopyReordered,errCopyUnsafeCut){t.Fatal("legacy error name no longer matches")}
 if strings.Contains(strings.ToLower(errCopyUnsafeCut.Error()),"accurate"){t.Fatal("message refers to a mode that is being removed")}
}
func TestNalHasIDR(t *testing.T){
 sample:=func(kinds ...byte)[]byte{var out []byte;for _,kind:=range kinds{out=append(out,0,0,0,2,kind,0x80)};return out}
 for _,tc:=range []struct{name string;data []byte;want bool}{
  {"idr slice",sample(0x65),true},
  {"sei then idr",sample(0x06,0x65),true},
  {"non-idr slice",sample(0x41),false},
  {"recovery point sei with non-idr slice",sample(0x06,0x01),false},
  {"empty",nil,false},
  {"length past end",[]byte{0,0,0,9,0x65},false},
  {"zero length",[]byte{0,0,0,0},false},
  {"short header",[]byte{0,0},false},
 }{if got:=nalHasIDR(tc.data);got!=tc.want{t.Fatalf("%s: got %v want %v",tc.name,got,tc.want)}}
}
func TestCopyOutpointUsesDecodeTimeAtIDR(t *testing.T){
 index:=copyFrameIndex{origin:1,keys:[]time.Duration{0,2*time.Second,4*time.Second},ends:[]time.Duration{-80*time.Millisecond,1920*time.Millisecond,3920*time.Millisecond}}
 clip:=recordingInterval{Start:time.Unix(0,0),End:time.Unix(6,0)}
 for _,tc:=range []struct{out time.Duration;want float64}{{4*time.Second,4.92},{2*time.Second,2.92},{3*time.Second,4},{6*time.Second,7}}{
  if got:=copyOutpoint(index,exportPlanPart{Clip:clip,Out:tc.out});math.Abs(got-tc.want)>1e-9{t.Fatalf("out %v: got %v want %v",tc.out,got,tc.want)}
 }
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
