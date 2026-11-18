package main

import (
 "errors"
 "math/big"
)

var errEventAssemblyPresentation=errors.New("assembled video presentation inconsistent with captured component")

// Cross-file necessary conditions over ALREADY collected, descriptor-bound
// observations. No frames/hashes are collected here; only scalar summaries are
// compared. Production derives hasPrebuffer from the frozen bound selection,
// never a client assertion or the current mutable buffer.
//
// No prebuffer: assembly only remuxes event.mp4, so video frame count and exact
// continuous presentation length must agree. Origin/timebase rebasing is allowed.
// With prebuffer: output must have MORE frames and a LONGER presentation span.
// These necessary bounds do NOT prove that every selected prefix was
// included, that content matches, or that the requested NVR interval is covered.
func eventAssemblyPresentationCompatible(capture, published eventVideoContinuity, hasPrebuffer bool) bool {
 for _,s:=range []eventVideoContinuity{capture,published} {
  if s.frames<=0 || s.frames>maxEventVideoContinuityFrames || s.end<=s.first || s.numerator<=0 || s.denominator<=0 {return false}
 }
 // Subtraction and products can exceed int64. Each operand is bounded int64,
 // so the exact comparison remains bounded without floats, epsilon or slack.
 capturedTicks:=new(big.Int).Sub(big.NewInt(capture.end),big.NewInt(capture.first))
 publishedTicks:=new(big.Int).Sub(big.NewInt(published.end),big.NewInt(published.first))
 left:=new(big.Int).Mul(publishedTicks,big.NewInt(published.numerator))
 left.Mul(left,big.NewInt(capture.denominator))
 right:=new(big.Int).Mul(capturedTicks,big.NewInt(capture.numerator))
 right.Mul(right,big.NewInt(published.denominator))
 order:=left.Cmp(right)
 if !hasPrebuffer {return published.frames==capture.frames && order==0}
 return published.frames>capture.frames && order>0
}

// Production additionally binds the minimum extra-frame budget to the number of
// frozen selected inputs. Each usable selected video clip must contribute at
// least one frame; aggregate counts cannot identify WHICH clip supplied a frame.
// This is a necessary refusal bound, not per-input correspondence or coverage.
func eventAssemblySelectedPrefixCompatible(capture,published eventVideoContinuity,selectedInputs int) bool {
 if selectedInputs<0 || selectedInputs>maxRecoveryInputs || !eventAssemblyPresentationCompatible(capture,published,selectedInputs!=0) {return false}
 // Both observed counts have already passed their existing positive bounded
 // validation; subtraction cannot overflow and cannot invent a contribution.
 return selectedInputs==0 || published.frames-capture.frames>=selectedInputs
}
