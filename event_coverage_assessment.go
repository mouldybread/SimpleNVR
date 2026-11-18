package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"
)

type eventCoverageGap struct {
	Start time.Time `json:"start"`
	End time.Time `json:"end"`
}

type eventCoverageSegments struct {
	Inputs int `json:"inputs"`
	Observed int `json:"observed"`
	Missing int `json:"missing"`
	Uncertain int `json:"uncertain"`
	Backwards int `json:"backwards"`
	SessionChanges int `json:"session_changes"`
	SequenceBreaks int `json:"sequence_breaks"`
	DeclaredGaps int `json:"declared_csv_gaps"`
	DeclaredOverlaps int `json:"declared_csv_overlaps"`
}

// A persisted DIAGNOSTIC, never an authorization receipt. Complete remains false.
// No media I/O, clock sampling, lease, descriptor or background worker is added.
type eventCoverageAssessment struct {
	Version int `json:"version"`
	Status string `json:"status"`
	Complete bool `json:"complete"`
	ObservationPoint string `json:"observation_point"`
	RequestedStart time.Time `json:"requested_start"`
	RequestedEnd time.Time `json:"requested_end"`
	SelectionBound bool `json:"selection_bound"`
	DeclaredPrebufferAssessed bool `json:"declared_prebuffer_assessed"`
	DeclaredPrebufferGaps []eventCoverageGap `json:"declared_prebuffer_gaps_in_request"`
	InvalidDeclaredRanges int `json:"invalid_declared_ranges"`
	Segments eventCoverageSegments `json:"segments"`
	FirstNotification *time.Time `json:"first_notification,omitempty"`
	LastNotification *time.Time `json:"last_notification,omitempty"`
	CaptureRunState string `json:"capture_run_state"`
	CaptureRunEntered *time.Time `json:"capture_run_entered,omitempty"`
	CaptureRunReturned *time.Time `json:"capture_run_returned,omitempty"`
	Unknowns []string `json:"unknowns"`
}

func eventCoverageUnknowns() []string {
	return []string{"segment_notification_delay_unbounded", "capture_media_bounds_unobserved", "assembled_output_correspondence_unverified"}
}

// This is the union of DECLARED prebuffer ranges clipped to the request. It is
// not a media inspection or concat reorder. Capture may fill these gaps; its
// process lifetime is deliberately NOT inserted as a media-covered interval.
func declaredPrebufferGaps(start, end time.Time, inputs []eventRecoveryInput) ([]eventCoverageGap, int) {
	gaps:=make([]eventCoverageGap,0)
	if start.IsZero() || !end.After(start) || len(inputs)>maxRecoveryInputs { return gaps,len(inputs) }
	ranges:=make([]eventCoverageGap,0,len(inputs));invalid:=0
	for _,input:=range inputs {
		if input.Start.IsZero() || input.End.IsZero() || !input.End.After(input.Start) {invalid++;continue}
		a,b:=input.Start,input.End
		if a.Before(start) {a=start};if b.After(end) {b=end}
		if b.After(a) {ranges=append(ranges,eventCoverageGap{Start:a.UTC(),End:b.UTC()})}
	}
	sort.Slice(ranges,func(i,j int)bool{return ranges[i].Start.Before(ranges[j].Start)})
	cursor:=start.UTC()
	for _,r:=range ranges {
		if r.Start.After(cursor) {gaps=append(gaps,eventCoverageGap{Start:cursor,End:r.Start})}
		if r.End.After(cursor) {cursor=r.End}
	}
	if end.After(cursor) {gaps=append(gaps,eventCoverageGap{Start:cursor,End:end.UTC()})}
	return gaps,invalid
}

// Snapshot on the owner thread immediately before an EXISTING checkpoint write.
// Legacy/no-selection jobs retain their old JSON shape. Refused bindings lose
// diagnostic credit rather than modifying publication or inventing coverage.
func snapshotEventCoverage(job *bufferEvent) *eventCoverageAssessment {
	if job==nil || job.recovery==nil || job.execution==nil || job.execution.selection==nil {return nil}
	m,p:=job.recovery,job.execution;t:=p.selection
	a:=&eventCoverageAssessment{Version:1,Status:"unknown",ObservationPoint:"completed_segment_notification_receipt",
		RequestedStart:m.RequestedStart,RequestedEnd:m.RequestedEnd,DeclaredPrebufferGaps:make([]eventCoverageGap,0),
		Segments:eventCoverageSegments{Inputs:len(m.Inputs),Missing:len(m.Inputs)},CaptureRunState:"unavailable",Unknowns:eventCoverageUnknowns()}
	if !p.bound(p.bm,job) || !t.requestedStart.Equal(m.RequestedStart) || !m.RequestedEnd.Equal(m.RequestedStart.Add(time.Duration(t.duration)*time.Second)) || len(m.Inputs)!=len(t.inputs) {return a}
	for i,input:=range t.inputs {
		if m.Inputs[i].Name!=fmt.Sprintf("pre_%03d.mp4",i) || !m.Inputs[i].Start.Equal(input.declaredStart) || !m.Inputs[i].End.Equal(input.declaredEnd) {return a}
	}
	a.SelectionBound=true
	s:=t.segmentClockSummary()
	a.Segments=eventCoverageSegments{Inputs:s.inputs,Observed:s.observed,Missing:s.missing,Uncertain:s.uncertain,
		Backwards:s.backwards,SessionChanges:s.sessionChanges,SequenceBreaks:s.sequenceBreaks,DeclaredGaps:s.declaredGaps,DeclaredOverlaps:s.declaredOverlaps}
	if !m.RequestedStart.IsZero() && m.RequestedEnd.After(m.RequestedStart) {
		a.DeclaredPrebufferAssessed=true
		a.DeclaredPrebufferGaps,a.InvalidDeclaredRanges=declaredPrebufferGaps(m.RequestedStart,m.RequestedEnd,m.Inputs)
	}
	for _,input:=range t.inputs {
		o:=input.segmentClock
		if !o.observed || o.received.IsZero() {continue}
		at:=o.received.UTC()
		if a.FirstNotification==nil || at.Before(*a.FirstNotification) {v:=at;a.FirstNotification=&v}
		if a.LastNotification==nil || at.After(*a.LastNotification) {v:=at;a.LastNotification=&v}
	}
	switch t.runStage {
	case eventCaptureRunNotObserved:a.CaptureRunState="not_observed"
	case eventCaptureRunRunning:a.CaptureRunState="running";v:=t.runEntered.UTC();a.CaptureRunEntered=&v
	case eventCaptureRunSucceeded:
		a.CaptureRunState="succeeded";v,w:=t.runEntered.UTC(),t.runReturned.UTC();a.CaptureRunEntered=&v;a.CaptureRunReturned=&w
	}
	return a
}

// The existing strict inspector validates new optional JSON instead of accepting
// arbitrary fields. Stored diagnostics are untrusted and never authorize cleanup.
func validateEventCoverageJSON(data []byte,m *eventRecoveryManifest) error {
	required:=[]string{"version","status","complete","observation_point","requested_start","requested_end","selection_bound","declared_prebuffer_assessed","declared_prebuffer_gaps_in_request","invalid_declared_ranges","segments","capture_run_state","unknowns"}
	optional:=[]string{"first_notification","last_notification","capture_run_entered","capture_run_returned"}
	if err:=exactRecoveryKeys(data,required,optional);err!=nil {return err}
	var raw struct {Segments json.RawMessage `json:"segments"`; Gaps []json.RawMessage `json:"declared_prebuffer_gaps_in_request"`}
	if err:=json.Unmarshal(data,&raw);err!=nil || len(raw.Gaps)>maxRecoveryInputs+1 {return errRecoveryInspection}
	if err:=exactRecoveryKeys(raw.Segments,[]string{"inputs","observed","missing","uncertain","backwards","session_changes","sequence_breaks","declared_csv_gaps","declared_csv_overlaps"},nil);err!=nil {return err}
	for _,gap:=range raw.Gaps {if err:=exactRecoveryKeys(gap,[]string{"start","end"},nil);err!=nil {return err}}
	a:=m.Coverage
	if a==nil || a.Version!=1 || a.Status!="unknown" || a.Complete || a.ObservationPoint!="completed_segment_notification_receipt" || !a.RequestedStart.Equal(m.RequestedStart) || !a.RequestedEnd.Equal(m.RequestedEnd) || !reflect.DeepEqual(a.Unknowns,eventCoverageUnknowns()) || a.DeclaredPrebufferGaps==nil {return errRecoveryInspection}
	s:=a.Segments
	if s.Inputs!=len(m.Inputs) || s.Inputs>maxRecoveryInputs || s.Observed<0 || s.Observed>s.Inputs || s.Missing<0 || s.Missing>s.Inputs || s.Observed+s.Missing!=s.Inputs || s.Uncertain<0 || s.Uncertain>2*s.Inputs {return errRecoveryInspection}
	for _,n:=range []int{s.Backwards,s.SessionChanges,s.SequenceBreaks,s.DeclaredGaps,s.DeclaredOverlaps} {if n<0 || n>s.Inputs {return errRecoveryInspection}}
	if a.DeclaredPrebufferAssessed {
		gaps,invalid:=declaredPrebufferGaps(m.RequestedStart,m.RequestedEnd,m.Inputs)
		if !a.SelectionBound || invalid!=a.InvalidDeclaredRanges || !reflect.DeepEqual(gaps,a.DeclaredPrebufferGaps) {return errRecoveryInspection}
	} else if a.InvalidDeclaredRanges!=0 || len(a.DeclaredPrebufferGaps)!=0 {return errRecoveryInspection}
	if !a.SelectionBound && (a.DeclaredPrebufferAssessed || s.Observed!=0 || s.Uncertain!=0 || s.Backwards!=0 || s.SessionChanges!=0 || s.SequenceBreaks!=0 || s.DeclaredGaps!=0 || s.DeclaredOverlaps!=0 || a.FirstNotification!=nil || a.LastNotification!=nil || a.CaptureRunState!="unavailable") {return errRecoveryInspection}
	if (a.FirstNotification==nil)!=(a.LastNotification==nil) {return errRecoveryInspection}
	if a.FirstNotification!=nil && (a.FirstNotification.IsZero() || a.LastNotification.IsZero() || a.LastNotification.Before(*a.FirstNotification) || s.Observed==0) {return errRecoveryInspection}
	switch a.CaptureRunState {
	case "not_observed","unavailable":if a.CaptureRunEntered!=nil || a.CaptureRunReturned!=nil {return errRecoveryInspection}
	case "running":if !a.SelectionBound || a.CaptureRunEntered==nil || a.CaptureRunEntered.IsZero() || a.CaptureRunReturned!=nil {return errRecoveryInspection}
	case "succeeded":if !a.SelectionBound || a.CaptureRunEntered==nil || a.CaptureRunReturned==nil || a.CaptureRunEntered.IsZero() || a.CaptureRunReturned.IsZero() || a.CaptureRunReturned.Before(*a.CaptureRunEntered) {return errRecoveryInspection}
	default:return errRecoveryInspection
	}
	return nil
}
