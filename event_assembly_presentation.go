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
// With prebuffer: output must at least contain the captured component's count
// and length. These lower bounds do NOT prove that every selected prefix was
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
 return published.frames>=capture.frames && order>=0
}
