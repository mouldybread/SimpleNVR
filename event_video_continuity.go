package main

import (
 "context"
 "errors"
 "math"
 "math/big"
)

var errEventVideoContinuity = errors.New("event video timestamp continuity unavailable")

// A resource ceiling, NOT a coverage tolerance. Larger observations refuse.
const maxEventVideoContinuityFrames = 1 << 20

// Explicit presentation timestamp and positive presentation duration, in the
// SAME video-stream time base. Missing fields must be refused by a future reader;
// zero PTS is valid and must not be used as a missing-field sentinel.
// Decoder/demux packet order is not necessarily presentation order. This pure
// validator never sorts, fills gaps, infers durations, or adjusts timestamps.
type eventVideoFrameInterval struct {
 pts int64
 duration int64
}

// Arithmetic result only: NOT an observed receipt, job binding or cleanup permit.
// No production caller or descriptor/process reader is introduced in this batch.
type eventVideoContinuity struct {
 first, end int64
 numerator, denominator int64
 frames int
}

// Require exact half-open presentation intervals [PTS, PTS+duration), with
// neither gaps nor overlaps. No rounded seconds, epsilon or format fallback.
// A future descriptor-bound reader must supply complete, verified presentation
// observations; passing synthetic intervals here does not establish real media.
func validateEventVideoContinuity(ctx context.Context, frames []eventVideoFrameInterval, numerator, denominator int64) (eventVideoContinuity, error) {
 if err := ctx.Err(); err != nil { return eventVideoContinuity{}, err }
 if numerator <= 0 || denominator <= 0 || len(frames) == 0 || len(frames) > maxEventVideoContinuityFrames {
  return eventVideoContinuity{}, errEventVideoContinuity
 }
 var expected int64
 for i, frame := range frames {
  if err := ctx.Err(); err != nil { return eventVideoContinuity{}, err }
  if frame.duration <= 0 || frame.pts > math.MaxInt64-frame.duration {
   return eventVideoContinuity{}, errEventVideoContinuity
  }
  if i > 0 && frame.pts != expected { return eventVideoContinuity{}, errEventVideoContinuity }
  expected = frame.pts + frame.duration
 }
 if err := ctx.Err(); err != nil { return eventVideoContinuity{}, err }
 return eventVideoContinuity{first:frames[0].pts, end:expected, numerator:numerator, denominator:denominator, frames:len(frames)}, nil
}

// Compare the interval LENGTH only. This does not establish an absolute start,
// requested wall-clock coverage, selected prebuffer coverage or assembled order.
// Subtraction and both products can exceed int64 and must remain exact.
func (span eventVideoContinuity) atLeastSeconds(seconds int64) bool {
 if seconds <= 0 || span.frames <= 0 || span.frames > maxEventVideoContinuityFrames || span.end <= span.first || span.numerator <= 0 || span.denominator <= 0 { return false }
 ticks := new(big.Int).Sub(big.NewInt(span.end), big.NewInt(span.first))
 left := new(big.Int).Mul(ticks, big.NewInt(span.numerator))
 right := new(big.Int).Mul(big.NewInt(seconds), big.NewInt(span.denominator))
 return left.Cmp(right) >= 0
}
