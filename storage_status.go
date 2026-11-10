package main

import (
 "context"
 "encoding/json"
 "net/http"
 "net/url"
 "time"

 "golang.org/x/sys/unix"
)

var storageInspectionSlots=make(chan struct{},2)

type recordingStorageCapacity struct {
 AvailableBytes uint64 `json:"available_bytes"`
 FreeInodes *uint64 `json:"free_inodes,omitempty"`
}
type recordingStagingSummary struct {
 LogicalBytes uint64 `json:"logical_bytes"`
 Directories uint64 `json:"directories"`
 Entries int `json:"entries_inspected"`
 EntryLimit int `json:"entry_limit"`
 DepthLimit int `json:"depth_limit"`
}
type recordingStorageInspection struct {
 CameraID string `json:"camera_id"`
 ObservedAt time.Time `json:"observed_at"`
 Complete bool `json:"inspection_complete"`
 MinimumFreeBytes *uint64 `json:"minimum_free_bytes,omitempty"`
 MaximumStagingBytes *uint64 `json:"maximum_staging_bytes,omitempty"`
 MaximumStagingDirs *uint64 `json:"maximum_staging_directories,omitempty"`
 Capacity *recordingStorageCapacity `json:"capacity,omitempty"`
 Staging *recordingStagingSummary `json:"staging,omitempty"`
 FreeReserveMet *bool `json:"free_reserve_met,omitempty"`
 StagingBelowLimits *bool `json:"staging_below_limits,omitempty"`
 Errors []string `json:"inspection_errors"`
}

func (report *recordingStorageInspection) finish() {
 if report.Errors==nil {report.Errors=[]string{}}
 known:=report.MinimumFreeBytes!=nil && report.MaximumStagingBytes!=nil && report.MaximumStagingDirs!=nil && report.Capacity!=nil && report.Staging!=nil
 if !known && len(report.Errors)==0 {report.Errors=append(report.Errors,"inspection_incomplete")}
 report.Complete=known && len(report.Errors)==0
 report.ObservedAt=time.Now().UTC()
}

func availableStorageBytes(blocks uint64,size int64) (uint64,error) {
 if size<=0 || blocks>^uint64(0)/uint64(size) {return 0,errStorageUnavailable}
 return blocks*uint64(size),nil
}

func readRecordingStorageCapacity(dir string) (*recordingStorageCapacity,error) {
 fd,err:=unix.Open(dir,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return nil,errStorageUnavailable}
 defer unix.Close(fd)
 var stat unix.Statfs_t
 if err:=unix.Fstatfs(fd,&stat);err!=nil {return nil,errStorageUnavailable}
 available,err:=availableStorageBytes(uint64(stat.Bavail),int64(stat.Bsize));if err!=nil {return nil,err}
 capacity:=&recordingStorageCapacity{AvailableBytes:available}
 if stat.Files>0 {free:=uint64(stat.Ffree);capacity.FreeInodes=&free}
 return capacity,nil
}

func inspectRecordingStorage(ctx context.Context,dir string) recordingStorageInspection {
 report:=recordingStorageInspection{}
 reserve,reserveErr:=recordingStorageReserve()
 if reserveErr!=nil {report.Errors=append(report.Errors,"invalid_free_space_policy")} else {report.MinimumFreeBytes=&reserve}
 policy,policyErr:=configuredEventStagingPolicy()
 if policyErr!=nil {report.Errors=append(report.Errors,"invalid_staging_policy")} else {report.MaximumStagingBytes=&policy.bytes;report.MaximumStagingDirs=&policy.dirs}
 if ctx.Err()!=nil {report.Errors=append(report.Errors,"inspection_canceled");report.finish();return report}
 capacity,err:=readRecordingStorageCapacity(dir)
 if err!=nil {report.Errors=append(report.Errors,"capacity_unavailable")} else {
  report.Capacity=capacity
  if reserveErr==nil && reserve>0 {met:=capacity.AvailableBytes>=reserve;report.FreeReserveMet=&met}
 }
 usage,err:=scanEventStaging(ctx,dir,eventStagingScanEntries,eventStagingScanDepth)
 if err!=nil {report.Errors=append(report.Errors,"staging_inspection_incomplete")} else {
  report.Staging=&recordingStagingSummary{LogicalBytes:usage.bytes,Directories:usage.dirs,Entries:usage.entries,EntryLimit:eventStagingScanEntries,DepthLimit:eventStagingScanDepth}
  if policyErr==nil && (policy.bytes>0 || policy.dirs>0) {
   below:=(policy.bytes==0 || usage.bytes<policy.bytes) && (policy.dirs==0 || usage.dirs<policy.dirs)
   report.StagingBelowLimits=&below
  }
 }
 if ctx.Err()!=nil {report.Errors=append(report.Errors,"inspection_canceled")}
 report.finish();return report
}

func storageStatusHandler(w http.ResponseWriter,r *http.Request) {
 storageStatusHandlerWithState(w,r,nvr,inspectRecordingStorage,storageInspectionSlots)
}

func storageStatusHandlerWithState(w http.ResponseWriter,r *http.Request,n *NVR,inspect func(context.Context,string) recordingStorageInspection,slots chan struct{}) {
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodGet {w.Header().Set("Allow","GET");http.Error(w,"Method not allowed",405);return}
 query,err:=url.ParseQuery(r.URL.RawQuery)
 ids:=query["camera_id"]
 if err!=nil || len(query)!=1 || len(ids)!=1 || ids[0]=="" || len(ids[0])>256 {http.Error(w,"Exactly one camera_id query parameter is required",400);return}
 n.mu.RLock();cam:=n.cameras[ids[0]];dir:="";if cam!=nil {dir=cam.RecordDir};n.mu.RUnlock()
 if cam==nil {http.Error(w,"Camera not found",404);return}
 select {case slots<-struct{}{}:defer func(){<-slots}();default:w.Header().Set("Retry-After","1");http.Error(w,"Storage inspection at capacity",503);return}
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 report:=inspect(ctx,dir);report.CameraID=ids[0]
 if ctx.Err()!=nil {report.Errors=append(report.Errors,"inspection_canceled")}
 report.finish()
 status:=200;if !report.Complete {status=503;w.Header().Set("Retry-After","1")}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.WriteHeader(status);json.NewEncoder(w).Encode(report)
}
