package main

import (
	"log"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// One small observation per completed segment, never per frame or packet.
// received is sampled after csv.Reader.Read returns, BEFORE parsing or stat.
// It is a completion-notification receipt, not first/last input-packet time.
type segmentClockObservation struct {
	session string
	sequence uint64
	startReturned, previousReceived, received time.Time
	mediaBegin, mediaEnd float64
	observed, priorUncertain, clockBackwards bool
}

type segmentClockSession struct {
	id string
	startReturned, previousReceived time.Time
	sequence uint64
	uncertain bool
}

// Local to the existing segmenter loop; no lock, descriptor or worker added.
func (s *segmentClockSession) notice(at time.Time) segmentClockObservation {
	s.sequence++
	if s.sequence == 0 { s.uncertain = true }
	o := segmentClockObservation{session:s.id, sequence:s.sequence, startReturned:s.startReturned,
		previousReceived:s.previousReceived, received:at, priorUncertain:s.uncertain,
		observed:s.id!="" && s.sequence!=0 && !s.startReturned.IsZero() && !at.IsZero()}
	previous := s.previousReceived
	if previous.IsZero() { previous = s.startReturned }
	o.clockBackwards = !previous.IsZero() && (at.Before(previous) || at.UnixNano()<previous.UnixNano())
	if o.clockBackwards { s.uncertain = true }
	s.previousReceived = at
	return o
}

type segmentClockEntry struct {
	observation segmentClockObservation
	source string
	declaredStart, declaredEnd time.Time
	device, inode uint64
	size int64
	modified time.Time
}

// Uses the registration's EXISTING Lstat result. This is only coarse origin
// association; it neither weakens inventory identity checks nor grants ownership.
func newSegmentClockEntry(path string, start, end time.Time, info os.FileInfo, observation segmentClockObservation) segmentClockEntry {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !observation.observed { return segmentClockEntry{} }
	return segmentClockEntry{observation:observation, source:path, declaredStart:start, declaredEnd:end,
		device:uint64(stat.Dev), inode:stat.Ino, size:info.Size(), modified:info.ModTime()}
}

// Called under the pre-existing buffer RLock during real pinning. Copy by value.
// Pinning can change ctime/link count; full post-pinning identity stays enforced
// by the existing inventory. Do not treat this subset as content-integrity proof.
func (e segmentClockEntry) freeze(segment BufferSegment, identity unix.Stat_t) segmentClockObservation {
	if !e.observation.observed || e.source!=segment.FilePath || !e.declaredStart.Equal(segment.StartTime) || !e.declaredEnd.Equal(segment.EndTime) || e.device!=uint64(identity.Dev) || e.inode!=identity.Ino || e.size!=identity.Size || !e.modified.Equal(time.Unix(identity.Mtim.Sec, identity.Mtim.Nsec)) {
		return segmentClockObservation{}
	}
	return e.observation
}

type segmentClockSummary struct {
	inputs, observed, missing, uncertain, backwards int
	sessionChanges, sequenceBreaks, declaredGaps, declaredOverlaps int
}

// Diagnostic only. No positive interval-coverage or deletion API exists here.
// CSV gaps/overlaps are DECLARATIONS, not proven missing/duplicated camera media.
func (t *eventSelectionTiming) segmentClockSummary() segmentClockSummary {
	var out segmentClockSummary
	if t == nil { return out }
	out.inputs = len(t.inputs)
	var previous segmentClockObservation
	for _, input := range t.inputs {
		o := input.segmentClock
		if !o.observed { out.missing++; previous=segmentClockObservation{}; continue }
		out.observed++
		if o.priorUncertain { out.uncertain++ }
		if o.clockBackwards { out.backwards++ }
		if previous.observed {
			if previous.session!=o.session { out.sessionChanges++ } else {
				if previous.sequence==^uint64(0) || o.sequence!=previous.sequence+1 { out.sequenceBreaks++ }
				if o.mediaBegin>previous.mediaEnd { out.declaredGaps++ }
				if o.mediaBegin<previous.mediaEnd { out.declaredOverlaps++ }
				if o.received.Before(previous.received) || o.received.UnixNano()<previous.received.UnixNano() { out.uncertain++ }
			}
		}
		previous=o
	}
	return out
}

// Called by the event worker outside media ownership, never while pinning.
func logEventSegmentClock(p *eventExecutionProvenance) {
	if p==nil || p.selection==nil { return }
	s:=p.selection.segmentClockSummary()
	log.Printf("Prebuffer segment notifications: inputs=%d observed=%d missing=%d prior_uncertain=%d clock_backwards=%d session_changes=%d sequence_breaks=%d declared_gaps=%d declared_overlaps=%d; input-arrival coverage unknown; staging retained", s.inputs,s.observed,s.missing,s.uncertain,s.backwards,s.sessionChanges,s.sequenceBreaks,s.declaredGaps,s.declaredOverlaps)
}
