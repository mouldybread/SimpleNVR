package main

import (
 "errors"
 "net/http"
 "sort"
 "time"
)

var errExportPlanInvalid=errors.New("invalid export intervals")
var errExportPlanGap=errors.New("requested export has missing coverage")
var errExportPlanOverlap=errors.New("requested export has ambiguous overlapping coverage")

type exportPlanPart struct {
 Clip recordingInterval
 Start,End time.Time
 In,Out time.Duration
}
type exportPlan struct {
 Start,End time.Time
 MediaStart,MediaEnd time.Time
 Parts []exportPlanPart
 NeedsTrim bool
}

// Planning is metadata-only. Offsets are not sent to the stream-copy encoder
// until a separately verified trim execution policy is implemented.
func buildExportPlan(clips []recordingInterval,window timelineWindow) (exportPlan,error) {
 plan:=exportPlan{Start:window.Start,End:window.End}
 span:=window.End.Sub(window.Start)
 if !window.End.After(window.Start) || !window.Start.Add(span).Equal(window.End) {return exportPlan{},errExportPlanInvalid}
 if len(clips)>maxPinnedExportInputs {return exportPlan{},errExportInputLimit}
 camera:=""
 for _,clip:=range clips {
  duration:=clip.End.Sub(clip.Start)
  if clip.Path=="" || clip.CameraID=="" || clip.Size<=0 || !clip.End.After(clip.Start) || !clip.Start.Add(duration).Equal(clip.End) {return exportPlan{},errExportPlanInvalid}
  if !intervalsOverlap(clip.Start,clip.End,window.Start,window.End) {continue}
  if camera=="" {camera=clip.CameraID} else if clip.CameraID!=camera {return exportPlan{},errExportPlanInvalid}
  start,end:=clip.Start,clip.End
  if start.Before(window.Start) {start=window.Start}
  if end.After(window.End) {end=window.End}
  plan.Parts=append(plan.Parts,exportPlanPart{Clip:clip,Start:start,End:end,In:start.Sub(clip.Start),Out:end.Sub(clip.Start)})
 }
 sort.Slice(plan.Parts,func(i,j int) bool {
  a,b:=plan.Parts[i],plan.Parts[j]
  if a.Start.Equal(b.Start) {if a.End.Equal(b.End) {return a.Clip.Path<b.Clip.Path};return a.End.Before(b.End)}
  return a.Start.Before(b.Start)
 })
 cursor:=window.Start
 for _,part:=range plan.Parts {
  if part.Start.Before(cursor) {return exportPlan{},errExportPlanOverlap}
  if part.Start.After(cursor) {return exportPlan{},errExportPlanGap}
  if !part.Start.Equal(part.Clip.Start) || !part.End.Equal(part.Clip.End) {plan.NeedsTrim=true}
  cursor=part.End
 }
 if cursor.Before(window.End) || len(plan.Parts)==0 {return exportPlan{},errExportPlanGap}
 plan.MediaStart=plan.Parts[0].Clip.Start
 plan.MediaEnd=plan.Parts[len(plan.Parts)-1].Clip.End
 return plan,nil
}
func (plan exportPlan) clips() []recordingInterval {
 result:=make([]recordingInterval,0,len(plan.Parts))
 for _,part:=range plan.Parts {result=append(result,part.Clip)}
 return result
}
func writeExportPlanError(w http.ResponseWriter,err error) {
 status,kind,message:=http.StatusBadRequest,"invalid","Invalid recording intervals for export"
 switch {
 case errors.Is(err,errExportInputLimit):status,kind,message=413,"input-limit","Too many export clips; reduce the requested time range"
 case errors.Is(err,errExportPlanGap):status,kind,message=409,"gap","Requested range has missing recording coverage; no export was started"
 case errors.Is(err,errExportPlanOverlap):status,kind,message=409,"overlap","Requested range has overlapping recordings; no automatic clip choice was made"
 }
 w.Header().Set("X-Export-Plan-Status",kind)
 http.Error(w,message,status)
}
func (plan exportPlan) setHeaders(w http.ResponseWriter) {
 w.Header().Set("X-Export-Plan-Status","validated")
 w.Header().Set("X-Export-Range-Policy","whole-clips-untrimmed")
 w.Header().Set("X-Export-Requested-Start",plan.Start.UTC().Format(time.RFC3339Nano))
 w.Header().Set("X-Export-Requested-End",plan.End.UTC().Format(time.RFC3339Nano))
 w.Header().Set("X-Export-Media-Start",plan.MediaStart.UTC().Format(time.RFC3339Nano))
 w.Header().Set("X-Export-Media-End",plan.MediaEnd.UTC().Format(time.RFC3339Nano))
 trim:="false";if plan.NeedsTrim {trim="true"}
 w.Header().Set("X-Export-Trim-Required",trim)
}
