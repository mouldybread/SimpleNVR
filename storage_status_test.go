package main

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"
)

func completeStorageStatusFixture() recordingStorageInspection {
 zero:=uint64(0)
 return recordingStorageInspection{MinimumFreeBytes:&zero,MaximumStagingBytes:&zero,MaximumStagingDirs:&zero,Capacity:&recordingStorageCapacity{AvailableBytes:123},Staging:&recordingStagingSummary{},Errors:[]string{}}
}

func TestAvailableStorageBytesRejectsOverflowAndInvalidUnits(t *testing.T) {
 if value,err:=availableStorageBytes(2,4096);err!=nil || value!=8192 {t.Fatal(value,err)}
 if value,err:=availableStorageBytes(0,4096);err!=nil || value!=0 {t.Fatal(value,err)}
 for _,item:=range []struct{blocks uint64;size int64}{{^uint64(0),4096},{1,0},{1,-1}} {if _,err:=availableStorageBytes(item.blocks,item.size);err==nil {t.Fatal("invalid capacity accepted")}}
}

func TestStorageStatusActualInspectionIsReadOnlyAndReportsThresholds(t *testing.T) {
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","18446744073709551615");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES","5");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS","1")
 dir:=t.TempDir();putStagingAdmissionFile(t,filepath.Join(dir,"capture.mp4.tmp"),"12");putStagingAdmissionFile(t,filepath.Join(dir,"temp","event-a","pre.mp4"),"345")
 cam:=&Camera{ID:"a",RecordDir:dir,RTSPUrl:"rtsp://fixture-user:fixture-password@fixture.invalid/live"};n:=&NVR{cameras:map[string]*Camera{"a":cam}}
 before:=eventStorageTree(t,dir);w:=httptest.NewRecorder()
 storageStatusHandlerWithState(w,httptest.NewRequest("GET","/api/storage/status?camera_id=a",nil),n,inspectRecordingStorage,make(chan struct{},2))
 if w.Code!=200 {t.Fatalf("status %d: %s",w.Code,w.Body.String())}
 var report recordingStorageInspection;if err:=json.Unmarshal(w.Body.Bytes(),&report);err!=nil {t.Fatal(err)}
 if !report.Complete || report.Staging==nil || report.Staging.LogicalBytes!=5 || report.Staging.Directories!=1 || report.FreeReserveMet==nil || *report.FreeReserveMet || report.StagingBelowLimits==nil || *report.StagingBelowLimits {t.Fatalf("incorrect report: %+v",report)}
 if w.Header().Get("Cache-Control")!="no-store" || strings.Contains(w.Body.String(),dir) || strings.Contains(w.Body.String(),"fixture-password") {t.Fatal("storage report exposed paths/credentials or was cacheable")}
 if !reflect.DeepEqual(before,eventStorageTree(t,dir)) {t.Fatal("inspection changed recovery artifacts")}
}

func TestStorageStatusValidatesRequestsAndBoundsConcurrentWork(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:"unused"}}};calls:=0;slots:=make(chan struct{},2)
 inspect:=func(ctx context.Context,dir string) recordingStorageInspection {calls++;if _,ok:=ctx.Deadline();!ok {t.Error("inspection has no deadline")};return completeStorageStatusFixture()}
 for _,item:=range []struct{method,query string;status int}{{"POST","camera_id=a",405},{"GET","",400},{"GET","camera_id=",400},{"GET","camera_id=a&camera_id=a",400},{"GET","camera_id=a&other=1",400},{"GET","camera_id=%zz",400},{"GET","camera_id=missing",404}} {
  r:=httptest.NewRequest(item.method,"/api/storage/status",nil);r.URL.RawQuery=item.query;w:=httptest.NewRecorder();storageStatusHandlerWithState(w,r,n,inspect,slots)
  if w.Code!=item.status {t.Fatalf("%s %q: %d",item.method,item.query,w.Code)}
 }
 if calls!=0 {t.Fatal("invalid request performed inspection")}
 slots<-struct{}{};slots<-struct{}{}
 w:=httptest.NewRecorder();storageStatusHandlerWithState(w,httptest.NewRequest("GET","/api/storage/status?camera_id=a",nil),n,inspect,slots)
 if w.Code!=503 || calls!=0 {t.Fatal("inspection capacity not enforced")}
 <-slots;<-slots
 w=httptest.NewRecorder();storageStatusHandlerWithState(w,httptest.NewRequest("GET","/api/storage/status?camera_id=a",nil),n,inspect,slots)
 if w.Code!=200 || calls!=1 || len(slots)!=0 {t.Fatal("inspection slot leaked")}
}

func TestStorageStatusOmitsUnknownUsageRatherThanReportingZero(t *testing.T) {
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS","0")
 dir,outside:=t.TempDir(),t.TempDir();if err:=os.Symlink(outside,filepath.Join(dir,"temp"));err!=nil {t.Fatal(err)}
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:dir}}};w:=httptest.NewRecorder()
 storageStatusHandlerWithState(w,httptest.NewRequest("GET","/api/storage/status?camera_id=a",nil),n,inspectRecordingStorage,make(chan struct{},2))
 var raw map[string]json.RawMessage;if err:=json.Unmarshal(w.Body.Bytes(),&raw);err!=nil {t.Fatal(err)}
 if w.Code!=503 || string(raw["inspection_complete"])!="false" || raw["staging"]!=nil || raw["capacity"]==nil {t.Fatalf("partial inspection misreported: %d %s",w.Code,w.Body.String())}
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","bad")
 report:=inspectRecordingStorage(context.Background(),t.TempDir());if report.Complete || report.MinimumFreeBytes!=nil {t.Fatal("invalid policy reported as configured zero")}
}

func TestStorageStatusRequiresSessionAndRejectsEventOnlyBearer(t *testing.T) {
 a,_:=authFixture(t);n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a"}}};calls:=0
 handler:=a.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){storageStatusHandlerWithState(w,r,n,func(context.Context,string) recordingStorageInspection {calls++;return completeStorageStatusFixture()},make(chan struct{},2))}))
 for _,bearer:=range []string{"","Bearer 0123456789012345678901234567890123456789012"} {
  r:=httptest.NewRequest("GET","https://nvr.test/api/storage/status?camera_id=a",nil);if bearer!="" {r.Header.Set("Authorization",bearer)}
  w:=httptest.NewRecorder();handler.ServeHTTP(w,r);if w.Code!=401 {t.Fatal("storage route accepted missing/sessionless event credentials",w.Code)}
 }
 if calls!=0 {t.Fatal("unauthenticated request performed inspection")}
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 r:=httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()));r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 login:=httptest.NewRecorder();handler.ServeHTTP(login,r);if login.Code!=303 {t.Fatal("login failed",login.Code)}
 r=httptest.NewRequest("GET","https://nvr.test/api/storage/status?camera_id=a",nil);for _,cookie:=range login.Result().Cookies(){r.AddCookie(cookie)}
 w:=httptest.NewRecorder();handler.ServeHTTP(w,r);if w.Code!=200 || calls!=1 {t.Fatal("authenticated inspection failed",w.Code)}
}

func TestShutdownCancelsAndJoinsStorageStatusBeforePersistence(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a"}}};slots:=make(chan struct{},2)
 entered,canceled,release,requestDone:=make(chan struct{}),make(chan struct{}),make(chan struct{}),make(chan struct{})
 var once sync.Once;defer once.Do(func(){close(release)})
 inspect:=func(ctx context.Context,_ string) recordingStorageInspection {close(entered);<-ctx.Done();close(canceled);<-release;return recordingStorageInspection{Errors:[]string{"inspection_canceled"}}}
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){storageStatusHandlerWithState(w,r,n,inspect,slots)}))
 go func(){defer close(requestDone);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/storage/status?camera_id=a",nil))}()
 awaitShutdownStep(t,entered,"inspection not entered")
 var persisted atomic.Bool;done:=make(chan error,1);ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 go func(){done<-shutdownApplication(ctx,n,gate,nil,func() error {persisted.Store(true);return nil})}()
 awaitShutdownStep(t,canceled,"inspection not canceled by shutdown");if persisted.Load() {t.Fatal("persisted before inspection joined")}
 once.Do(func(){close(release)});awaitShutdownStep(t,requestDone,"inspection request did not finish")
 select {case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(3*time.Second):t.Fatal("shutdown did not join inspection")}
 if !persisted.Load() || len(slots)!=0 {t.Fatal("inspection join/slot release failed")}
}
