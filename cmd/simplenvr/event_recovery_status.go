package main

import (
 "context"
 "encoding/json"
 "net/http"
 "net/url"
 "time"
)

func recoveryStatusHandler(w http.ResponseWriter,r *http.Request) {
 recoveryStatusHandlerWithState(w,r,nvr,inspectEventRecovery,storageInspectionSlots)
}

func recoveryStatusHandlerWithState(w http.ResponseWriter,r *http.Request,n *NVR,inspect func(context.Context,string,string,string) eventRecoveryInspection,slots chan struct{}) {
 w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
 if r.Method!=http.MethodGet {w.Header().Set("Allow","GET");http.Error(w,"Method not allowed",405);return}
 query,err:=url.ParseQuery(r.URL.RawQuery)
 cameras,staging:=query["camera_id"],query["staging_id"]
 if err!=nil || len(query)!=2 || len(cameras)!=1 || cameras[0]=="" || len(cameras[0])>256 || len(staging)!=1 || !recoveryStagingName.MatchString(staging[0]) {http.Error(w,"Exactly one camera_id and staging_id are required",400);return}
 n.mu.RLock();cam:=n.cameras[cameras[0]];dir:="";if cam!=nil {dir=cam.RecordDir};n.mu.RUnlock()
 if cam==nil {http.Error(w,"Camera not found",404);return}
 select {case slots<-struct{}{}:defer func(){<-slots}();default:w.Header().Set("Retry-After","1");http.Error(w,"Inspection at capacity",503);return}
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 report:=inspect(ctx,dir,cameras[0],staging[0]);report.CameraID=cameras[0];report.StagingID=staging[0];report.ObservedAt=time.Now().UTC()
 if report.Errors==nil {report.Errors=[]string{}}
 if ctx.Err()!=nil {report.Complete=false;report.Errors=append(report.Errors,"inspection_canceled")}
 n.mu.RLock();unchanged:=n.cameras[cameras[0]]==cam && cam.RecordDir==dir;n.mu.RUnlock()
 if !unchanged {report.Complete=false;report.Errors=append(report.Errors,"camera_changed")}
 status:=200
 if !report.Complete || len(report.Errors)>0 {report.Complete=false;status=503;w.Header().Set("Retry-After","1")}
 w.Header().Set("Content-Type","application/json");w.WriteHeader(status);json.NewEncoder(w).Encode(report)
}
