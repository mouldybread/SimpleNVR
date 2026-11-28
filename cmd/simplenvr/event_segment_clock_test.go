package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSegmentClockNoticeAndBackwardsClock(t *testing.T) {
	start:=time.Now()
	s:=segmentClockSession{id:"session-a",startReturned:start}
	a:=s.notice(start.Add(time.Second))
	b:=s.notice(start.Add(2*time.Second))
	if !a.observed || a.sequence!=1 || b.sequence!=2 || !b.previousReceived.Equal(a.received) || a.clockBackwards || b.priorUncertain {t.Fatal("wrong notification observations",a,b)}
	c:=s.notice(start.Add(-time.Second))
	d:=s.notice(start.Add(3*time.Second))
	if !c.clockBackwards || !d.priorUncertain {t.Fatal("clock reversal lost")}
	// Forward steps are retained as samples, not certified as healthy clocks.
	e:=s.notice(start.Add(time.Hour))
	if !e.received.Equal(start.Add(time.Hour)) || !e.priorUncertain {t.Fatal("clock samples rewritten")}
}

func TestSegmentClockDeclaredGapAndSessionDiagnostics(t *testing.T) {
	now:=time.Now()
	makeInput:=func(session string, sequence uint64, begin,end float64) eventSelectedInputTiming {
		return eventSelectedInputTiming{segmentClock:segmentClockObservation{session:session,sequence:sequence,received:now.Add(time.Duration(sequence)*time.Second),mediaBegin:begin,mediaEnd:end,observed:true}}
	}
	selection:=&eventSelectionTiming{inputs:[]eventSelectedInputTiming{
		makeInput("a",1,0,5),makeInput("a",2,6,10),makeInput("a",4,9,15),makeInput("b",1,0,5),{},
	}}
	s:=selection.segmentClockSummary()
	if s.inputs!=5 || s.observed!=4 || s.missing!=1 || s.declaredGaps!=1 || s.declaredOverlaps!=1 || s.sequenceBreaks!=1 || s.sessionChanges!=1 {t.Fatal(s)}
	var absent *eventSelectionTiming
	if absent.segmentClockSummary().inputs!=0 {t.Fatal("nil selection invented inputs")}
}

func TestSegmentClockFreezeRefusesChangedSource(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"segment.mp4")
	if err:=os.WriteFile(path,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
	info,err:=os.Lstat(path);if err!=nil {t.Fatal(err)}
	start:=time.Now().Add(-5*time.Second);end:=start.Add(5*time.Second)
	o:=segmentClockObservation{session:"a",sequence:1,received:time.Now(),observed:true,mediaEnd:5}
	e:=newSegmentClockEntry(path,start,end,info,o)
	var identity unix.Stat_t
	if err:=unix.Stat(path,&identity);err!=nil {t.Fatal(err)}
	segment:=BufferSegment{FilePath:path,StartTime:start,EndTime:end}
	if !e.freeze(segment,identity).observed {t.Fatal("matching observation refused")}
	changed:=identity;changed.Ino++
	if e.freeze(segment,changed).observed {t.Fatal("replacement inode inherited timing")}
	changed=identity;changed.Size++
	if e.freeze(segment,changed).observed {t.Fatal("modified file inherited timing")}
	segment.StartTime=segment.StartTime.Add(time.Second)
	if e.freeze(segment,identity).observed {t.Fatal("different declaration inherited timing")}
}

func TestSegmentClockProductionSelectionCopiesAndRetains(t *testing.T) {
	for _,replace:=range []bool{false,true} {t.Run(map[bool]string{false:"same_source",true:"replaced_source"}[replace],func(t *testing.T){
		bm:=bufferFixture(t)
		path:=filepath.Join(bm.camera.RecordDir,"observed-segment.mp4")
		if err:=os.WriteFile(path,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
		now:=time.Now();start:=now.Add(-5*time.Second)
		o:=segmentClockObservation{session:"a",sequence:1,startReturned:start,received:now,observed:true,mediaEnd:5}
		if !bm.registerCompletedObserved(path,start,now,o) {t.Fatal("registration failed")}
		if replace {
			if err:=os.Rename(path,path+".old");err!=nil {t.Fatal(err)}
			if err:=os.WriteFile(path,[]byte(strings.Repeat("q",2048)),0600);err!=nil {t.Fatal(err)}
		}
		if err:=bm.TriggerEvent(bufferTestEvent("segment-clock"));err!=nil {t.Fatal(err)}
		job:=<-bm.eventChan
		t.Cleanup(func(){if len(bm.slots)!=0 {bm.releaseJob(job)}})
		selection:=job.execution.selection
		if selection==nil || len(selection.inputs)!=1 {t.Fatal("production input not selected")}
		if selection.inputs[0].segmentClock.observed==replace {t.Fatal("origin association wrong")}
		bm.bufferMutex.Lock();bm.segmentClocks=nil;bm.buffer=nil;bm.bufferMutex.Unlock()
		if !selection.bound(job.execution,bm,job) {t.Fatal("timing changed existing binding")}
		s:=selection.segmentClockSummary()
		if !replace && (s.observed!=1 || s.missing!=0) {t.Fatal("frozen observation lost",s)}
		if replace && s.missing!=1 {t.Fatal("foreign source certified",s)}
		bm.releaseJob(job)
		if !job.keepStaging || job.inventory!=nil || len(bm.slots)!=0 {t.Fatal("retention/teardown changed")}
		if _,err:=os.Stat(job.dir);err!=nil {t.Fatal("staging deleted",err)}
	})}
}

func TestSegmentClockMetadataFollowsIndexLifetimeAndDuplicateRefusal(t *testing.T) {
	dir:=t.TempDir();bm:=&BufferManager{camera:&Camera{ID:"clock",RecordDir:dir}}
	path:=filepath.Join(dir,"segment.mp4")
	if err:=os.WriteFile(path,[]byte(strings.Repeat("p",2048)),0600);err!=nil {t.Fatal(err)}
	now:=time.Now();o:=segmentClockObservation{session:"a",sequence:1,received:now,observed:true}
	if !bm.registerCompletedObserved(path,now,now.Add(time.Second),o) || len(bm.segmentClocks)!=1 {t.Fatal("clock not indexed")}
	bm.registerCompleted(path,now,now.Add(time.Second))
	if len(bm.segmentClocks)!=0 {t.Fatal("duplicate name kept ambiguous clock association")}
	bm.pruneBefore(now.Add(time.Hour))
	if len(bm.buffer)!=0 || len(bm.segmentClocks)!=0 {t.Fatal("metadata outlived index")}
	if bm.registerCompletedObserved(filepath.Join(dir,"missing.mp4"),now,now,o) || len(bm.segmentClocks)!=0 {t.Fatal("missing file indexed")}
}

// A synthetic FFmpeg stand-in checks the real CSV loop and cancel/join path.
// It is not a real RTSP, codec, clock-accuracy or application-acceptance test.
func TestSegmentClockProductionCSVReadLoop(t *testing.T) {
	dir:=t.TempDir();bin:=t.TempDir()
	if err:=os.Mkdir(filepath.Join(dir,"buffer"),0700);err!=nil {t.Fatal(err)}
	script:="#!/bin/sh\ncase \"$*\" in *framehash*|*copyts*|*use_wallclock*) exit 9;; esac\nfor pattern do :; done\nsuffix=${pattern#buffer_%Y%m%d_%H%M%S_}\nname=buffer_20261114_120000_$suffix\nprintf '%s' '"+strings.Repeat("p",2048)+"' > \"$name\"\nprintf '\"%s\",0.000000,5.000000\\n' \"$name\"\n"
	if err:=os.WriteFile(filepath.Join(bin,"ffmpeg"),[]byte(script),0700);err!=nil {t.Fatal(err)}
	t.Setenv("PATH",bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx,cancel:=context.WithCancel(context.Background())
	bm:=&BufferManager{camera:&Camera{ID:"clock",RecordDir:dir},ctx:ctx,cancel:cancel}
	done:=make(chan struct{})
	go func(){defer close(done);bm.segmentBuffer()}()
	defer func(){cancel();select{case <-done:case <-time.After(10*time.Second):t.Error("segmenter did not join")}}()
	deadline:=time.NewTimer(3*time.Second);defer deadline.Stop()
	tick:=time.NewTicker(5*time.Millisecond);defer tick.Stop()
	for {
		bm.bufferMutex.RLock()
		ready:=len(bm.buffer)==1 && len(bm.segmentClocks)==1
		var observation segmentClockObservation
		if ready {observation=bm.segmentClocks[bm.buffer[0].FilePath].observation}
		bm.bufferMutex.RUnlock()
		if ready {
			if !observation.observed || observation.received.IsZero() || observation.startReturned.IsZero() || observation.mediaBegin!=0 || observation.mediaEnd!=5 || observation.sequence!=1 {t.Fatal(observation)}
			cancel();return
		}
		select{case <-deadline.C:t.Fatal("CSV completion was not observed");case <-tick.C:}
	}
}
