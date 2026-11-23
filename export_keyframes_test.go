package main

import (
 "context"
 "encoding/json"
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
func TestCopyRangeExtensionSavedWithoutApproval(t *testing.T){
 _,service,h,cookie:=exportHTTPFixture(t)
 savedStart,savedEnd:=time.Date(2026,11,20,23,59,59,0,time.UTC),time.Date(2026,11,21,0,0,11,0,time.UTC)
 calls:=0
 service.plan=func(ctx context.Context,req exportCreateRequest)(exportJobRenderer,error){
  calls++
  sink,ok:=ctx.Value(exportSavedRangeKey{}).(*exportSavedRange)
  if !ok{return nil,errors.New("planner has no saved-range sink")}
  sink.Start,sink.End=savedStart,savedEnd
  return func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"video");return err},nil
 }
 created:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody)
 if created.Code!=202 || calls!=1{t.Fatal(created.Code,created.Body.String(),calls)}
 if strings.Contains(created.Body.String(),"approval"){t.Fatal("approval vocabulary remains in the create response")}
 var view exportHTTPView;if err:=json.Unmarshal(created.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
 if view.SavedStart!="2026-11-20T23:59:59Z" || view.SavedEnd!="2026-11-21T00:00:11Z"{t.Fatalf("saved range not reported: %+v",view)}
 status:=exportHTTPRequest(h,cookie,"GET",exportStatusPath+"?id="+view.ID,"")
 var polled exportHTTPView
 if status.Code!=200 || json.Unmarshal(status.Body.Bytes(),&polled)!=nil || polled.SavedStart!=view.SavedStart || polled.SavedEnd!=view.SavedEnd{t.Fatal(status.Code,status.Body.String())}
}
func TestCopyApprovalFieldsRemoved(t *testing.T){
 _,_,h,cookie:=exportHTTPFixture(t)
 body:=strings.TrimSuffix(exportHTTPCreateBody,"}")+`,"accepted_start":"2026-11-21T00:00:00Z","accepted_end":"2026-11-21T00:00:11Z"}`
 if got:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,body).Code;got!=400{t.Fatal("legacy approval fields accepted",got)}
}
