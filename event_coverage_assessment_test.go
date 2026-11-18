package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCoverageDeclaredUnionClipsAndMergesWithoutReordering(t *testing.T) {
	start:=time.Unix(1000,0).UTC();end:=start.Add(30*time.Second)
	input:=func(a,b int)eventRecoveryInput{return eventRecoveryInput{Start:start.Add(time.Duration(a)*time.Second),End:start.Add(time.Duration(b)*time.Second)}}
	inputs:=[]eventRecoveryInput{input(15,25),input(-5,5),input(4,10),input(25,40),input(8,8)}
	gaps,invalid:=declaredPrebufferGaps(start,end,inputs)
	if invalid!=1 || len(gaps)!=1 || !gaps[0].Start.Equal(start.Add(10*time.Second)) || !gaps[0].End.Equal(start.Add(15*time.Second)) {t.Fatal(gaps,invalid)}
	if !inputs[0].Start.Equal(start.Add(15*time.Second)) {t.Fatal("concat order mutated")}
	gaps,invalid=declaredPrebufferGaps(start,end,nil)
	if invalid!=0 || len(gaps)!=1 || !gaps[0].Start.Equal(start) || !gaps[0].End.Equal(end) {t.Fatal("missing prebuffer became coverage",gaps)}
	gaps,invalid=declaredPrebufferGaps(start,end,[]eventRecoveryInput{input(-1,31)})
	if invalid!=0 || len(gaps)!=0 {t.Fatal("full declaration not recognized",gaps)}
}

func TestCoverageSnapshotUsesFrozenProductionSelectionAndNotRunDuration(t *testing.T) {
	bm,job:=selectionTimingJob(t);p:=job.execution;s:=p.selection
	a:=snapshotEventCoverage(job)
	if a==nil || !a.SelectionBound || !a.DeclaredPrebufferAssessed || a.Complete || a.Status!="unknown" || a.Segments.Missing!=len(job.preFiles) {t.Fatal(a)}
	before,_:=json.Marshal(a.DeclaredPrebufferGaps)
	p.beginCapture(bm,job)
	p.observeCaptureRunStarted(bm,job,s.pinnedAt.Add(time.Second))
	p.observeCaptureRunReturned(bm,job,s.pinnedAt.Add(2*time.Hour),nil,nil)
	a=snapshotEventCoverage(job)
	after,_:=json.Marshal(a.DeclaredPrebufferGaps)
	if a.CaptureRunState!="succeeded" || a.CaptureRunEntered==nil || a.CaptureRunReturned==nil || !bytes.Equal(before,after) || a.Complete {t.Fatal("Run lifetime was treated as media coverage",a)}
	bm.bufferMutex.Lock();bm.buffer=nil;bm.segmentClocks=nil;bm.bufferMutex.Unlock()
	if next:=snapshotEventCoverage(job);next==nil || !next.SelectionBound {t.Fatal("snapshot depended on mutable buffer")}
	job.event.StartTime=job.event.StartTime.Add(time.Second)
	if next:=snapshotEventCoverage(job);next.SelectionBound || next.DeclaredPrebufferAssessed || next.Complete {t.Fatal("changed request accepted",next)}
}

func TestCoverageCheckpointAndReadOnlyInspection(t *testing.T) {
	bm,job:=selectionTimingJob(t)
	if err:=job.recoveryCheckpoint("capturing","");err!=nil {t.Fatal(err)}
	path:=filepath.Join(job.dir,"recovery.json")
	before,err:=os.ReadFile(path);if err!=nil {t.Fatal(err)}
	manifest,err:=parseRecoveryManifest(before,bm.camera.ID)
	if err!=nil || manifest.Coverage==nil || manifest.Coverage.Complete || !manifest.Coverage.SelectionBound {t.Fatal("new production checkpoint unreadable",err)}
	inspection:=inspectEventRecovery(context.Background(),bm.camera.RecordDir,bm.camera.ID,filepath.Base(job.dir))
	if !inspection.Complete || inspection.Coverage==nil || inspection.Coverage.Complete {t.Fatal("stored diagnostic unavailable",inspection)}
	after,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(before,after) {t.Fatal("read-only inspection changed checkpoint",err)}
	if !job.keepStaging {t.Fatal("assessment authorized staging release")}
}

func TestCoverageLegacyShapeAndStickyFailure(t *testing.T) {
	var data []byte
	legacy:=&bufferEvent{recovery:&eventRecoveryManifest{},recoveryWriter:func(_ string,b []byte)error{data=append([]byte(nil),b...);return nil}}
	if err:=legacy.recoveryCheckpoint("capturing","");err!=nil {t.Fatal(err)}
	if bytes.Contains(data,[]byte("coverage_assessment")) {t.Fatal("legacy job gained invented observations")}
	bm,job:=selectionTimingJob(t)
	calls:=0;injected:=errors.New("checkpoint storage failure")
	job.recoveryWriter=func(_ string,_ []byte)error{calls++;return injected}
	if err:=job.recoveryCheckpoint("capturing","");!errors.Is(err,injected) {t.Fatal(err)}
	if err:=job.recoveryCheckpoint("retained","");err==nil || calls!=1 || !job.recoveryFailed {t.Fatal("persistence failure not sticky",err,calls)}
	bm.releaseJob(job)
	if len(bm.slots)!=0 || job.inventory!=nil || !job.keepStaging || calls!=1 {t.Fatal("assessment disturbed failure teardown")}
}

func TestCoverageStrictInspectionRejectsForgedOrMalformedAssessment(t *testing.T) {
	bm,job:=selectionTimingJob(t)
	if err:=job.recoveryCheckpoint("capturing","");err!=nil {t.Fatal(err)}
	original,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
	for _,kind:=range []string{"complete","status","unknown_field","case_alias","missing_field","null","wrong_request","wrong_gaps","bad_count","unknown_reason"} {t.Run(kind,func(t *testing.T){
		var root map[string]json.RawMessage
		if err:=json.Unmarshal(original,&root);err!=nil {t.Fatal(err)}
		var a map[string]json.RawMessage
		if err:=json.Unmarshal(root["coverage_assessment"],&a);err!=nil {t.Fatal(err)}
		switch kind {
		case "complete":a["complete"]=json.RawMessage("true")
		case "status":a["status"]=json.RawMessage(`"complete"`)
		case "unknown_field":a["permit_cleanup"]=json.RawMessage("true")
		case "case_alias":a["Complete"]=json.RawMessage("false")
		case "missing_field":delete(a,"complete")
		case "null":a["segments"]=json.RawMessage("null")
		case "wrong_request":a["requested_start"]=json.RawMessage(`"2000-01-01T00:00:00Z"`)
		case "wrong_gaps":a["declared_prebuffer_gaps_in_request"]=json.RawMessage(`[{"start":"2000-01-01T00:00:00Z","end":"2000-01-02T00:00:00Z"}]`)
		case "bad_count":var s map[string]json.RawMessage;json.Unmarshal(a["segments"],&s);s["observed"]=json.RawMessage("999999");a["segments"],_=json.Marshal(s)
		case "unknown_reason":a["unknowns"]=json.RawMessage(`[]`)
		}
		root["coverage_assessment"],_=json.Marshal(a)
		changed,_:=json.Marshal(root)
		if _,err:=parseRecoveryManifest(changed,bm.camera.ID);err==nil {t.Fatal("forged assessment accepted",kind)}
	})}
}

func TestCoverageMaximumInputsFitsExistingByteAndJSONBudgets(t *testing.T) {
	start:=time.Unix(1000,0).UTC();end:=start.Add(time.Hour)
	m:=&eventRecoveryManifest{Version:1,Revision:1,CameraID:"camera",EventID:"event",RequestedStart:start,RequestedEnd:end,
		DurationSeconds:3600,CreatedAt:start,ObservedAt:end,State:"pinned",CaptureName:"event.mp4",PinnedCount:maxRecoveryInputs}
	for i:=0;i<maxRecoveryInputs;i++ {m.Inputs=append(m.Inputs,eventRecoveryInput{Name:fmt.Sprintf("pre_%03d.mp4",i),Start:start.Add(time.Duration(2*i+1)*time.Second),End:start.Add(time.Duration(2*i+2)*time.Second)})}
	gaps,invalid:=declaredPrebufferGaps(start,end,m.Inputs)
	// Synthetic declarations, NOT a production eligibility witness.
	m.Coverage=&eventCoverageAssessment{Version:1,Status:"unknown",ObservationPoint:"completed_segment_notification_receipt",RequestedStart:start,RequestedEnd:end,
		SelectionBound:true,DeclaredPrebufferAssessed:true,DeclaredPrebufferGaps:gaps,InvalidDeclaredRanges:invalid,
		Segments:eventCoverageSegments{Inputs:maxRecoveryInputs,Missing:maxRecoveryInputs},CaptureRunState:"not_observed",Unknowns:eventCoverageUnknowns()}
	data,err:=json.Marshal(m);if err!=nil {t.Fatal(err)}
	if len(gaps)!=maxRecoveryInputs+1 || len(data)>maxRecoveryManifestBytes {t.Fatal("existing manifest cap exceeded",len(gaps),len(data))}
	if _,err:=parseRecoveryManifest(data,m.CameraID);err!=nil {t.Fatal("existing JSON token/depth budget exceeded",err)}
	m.Coverage=nil
	data,err=json.Marshal(m);if err!=nil {t.Fatal(err)}
	if _,err:=parseRecoveryManifest(data,m.CameraID);err!=nil {t.Fatal("legacy manifest support lost",err)}
}

// Synthetic media bytes, but real registration, frozen pinning, checkpoint writer
// and strict decoding. The observations come from the production segment helper.
func TestCoverageFrozenNotificationPersistsWithoutMutableBuffer(t *testing.T) {
	bm:=bufferFixture(t)
	path:=filepath.Join(bm.camera.RecordDir,"observed.mp4")
	if err:=os.WriteFile(path,bytes.Repeat([]byte("p"),2048),0600);err!=nil {t.Fatal(err)}
	now:=time.Now();start:=now.Add(-5*time.Second)
	clock:=segmentClockSession{id:"coverage-session",startReturned:start}
	o:=clock.notice(now);o.mediaBegin=0;o.mediaEnd=5
	if !bm.registerCompletedObserved(path,start,now,o) {t.Fatal("source registration failed")}
	if err:=bm.TriggerEvent(bufferTestEvent("coverage"));err!=nil {t.Fatal(err)}
	job:=<-bm.eventChan
	t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
	bm.bufferMutex.Lock();bm.segmentClocks=nil;bm.buffer=nil;bm.bufferMutex.Unlock()
	if err:=job.recoveryCheckpoint("capturing","");err!=nil {t.Fatal(err)}
	data,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
	m,err:=parseRecoveryManifest(data,bm.camera.ID);if err!=nil {t.Fatal(err)}
	a:=m.Coverage
	if a==nil || a.Complete || a.Segments.Observed!=1 || a.Segments.Missing!=0 || a.FirstNotification==nil || a.LastNotification==nil || !a.FirstNotification.Equal(now) || !a.LastNotification.Equal(now) {t.Fatal("production observation was lost or promoted into coverage",a)}
}
