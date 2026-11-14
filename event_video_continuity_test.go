package main

import (
 "context"
 "errors"
 "math"
 "reflect"
 "testing"
)

func TestEventVideoContinuityStrictIntervals(t *testing.T) {
 cases := []struct {
  name string
  frames []eventVideoFrameInterval
  numerator, denominator int64
  valid bool
 }{
  {"zero origin", []eventVideoFrameInterval{{0,1},{1,2},{3,1}},1,1,true},
  {"negative origin", []eventVideoFrameInterval{{-3,2},{-1,1},{0,2}},1,1000,true},
  {"positive origin", []eventVideoFrameInterval{{100,2},{102,3}},1,1000,true},
  {"single frame", []eventVideoFrameInterval{{0,1}},1,1,true},
  {"variable durations", []eventVideoFrameInterval{{0,3},{3,2},{5,7}},1001,30000,true},
  {"one tick gap", []eventVideoFrameInterval{{0,1},{2,1}},1,1,false},
  {"overlap", []eventVideoFrameInterval{{0,3},{2,1}},1,1,false},
  {"duplicate", []eventVideoFrameInterval{{0,1},{0,1}},1,1,false},
  {"out of order", []eventVideoFrameInterval{{2,1},{1,1}},1,1,false},
  {"zero duration", []eventVideoFrameInterval{{0,0}},1,1,false},
  {"negative duration", []eventVideoFrameInterval{{0,-1}},1,1,false},
  {"end overflow", []eventVideoFrameInterval{{math.MaxInt64,1}},1,1,false},
  {"end maximum", []eventVideoFrameInterval{{math.MaxInt64-1,1}},1,1,true},
  {"minimum origin", []eventVideoFrameInterval{{math.MinInt64,1}},1,1,true},
  {"empty", nil,1,1,false},
  {"zero numerator", []eventVideoFrameInterval{{0,1}},0,1,false},
  {"negative numerator", []eventVideoFrameInterval{{0,1}},-1,1,false},
  {"zero denominator", []eventVideoFrameInterval{{0,1}},1,0,false},
  {"negative denominator", []eventVideoFrameInterval{{0,1}},1,-1,false},
 }
 for _, tc := range cases { t.Run(tc.name,func(t *testing.T) {
  before := append([]eventVideoFrameInterval(nil),tc.frames...)
  span,err := validateEventVideoContinuity(context.Background(),tc.frames,tc.numerator,tc.denominator)
  if !reflect.DeepEqual(before,tc.frames) { t.Fatal("validator mutated caller observations") }
  if tc.valid {
   if err != nil || span.first != tc.frames[0].pts || span.end != tc.frames[len(tc.frames)-1].pts+tc.frames[len(tc.frames)-1].duration || span.frames != len(tc.frames) || span.numerator != tc.numerator || span.denominator != tc.denominator { t.Fatal("wrong continuity result",span,err) }
  } else if !errors.Is(err,errEventVideoContinuity) || span != (eventVideoContinuity{}) { t.Fatal("refusal returned usable result",span,err) }
 }) }
}

func TestEventVideoContinuityExactLengthNotWallClock(t *testing.T) {
 cases := []struct {
  frames []eventVideoFrameInterval
  numerator, denominator, seconds int64
  want bool
 }{
  {[]eventVideoFrameInterval{{0,30000}},1,1000,30,true},
  {[]eventVideoFrameInterval{{0,29999}},1,1000,30,false},
  {[]eventVideoFrameInterval{{0,30001}},1,1000,30,true},
  {[]eventVideoFrameInterval{{-100,30000}},1,1000,30,true},
  {[]eventVideoFrameInterval{{100,30000}},1,1000,30,true},
  {[]eventVideoFrameInterval{{0,30000}},1001,30000,1001,true},
  {[]eventVideoFrameInterval{{0,29999}},1001,30000,1001,false},
  {[]eventVideoFrameInterval{{math.MinInt64,math.MaxInt64},{-1,math.MaxInt64}},1,1,math.MaxInt64,true},
  {[]eventVideoFrameInterval{{0,math.MaxInt64}},math.MaxInt64,math.MaxInt64,math.MaxInt64,true},
  {[]eventVideoFrameInterval{{0,math.MaxInt64-1}},math.MaxInt64,math.MaxInt64,math.MaxInt64,false},
  {[]eventVideoFrameInterval{{0,1}},1,1,0,false},
  {[]eventVideoFrameInterval{{0,1}},1,1,-1,false},
 }
 for i,tc := range cases {
  span,err := validateEventVideoContinuity(context.Background(),tc.frames,tc.numerator,tc.denominator)
  if err != nil { t.Fatal(i,err) }
  if got := span.atLeastSeconds(tc.seconds); got != tc.want { t.Fatal(i,span,got,tc.want) }
 }
 for _, span := range []eventVideoContinuity{
  {}, {first:0,end:1,numerator:1,denominator:1},
  {first:1,end:1,numerator:1,denominator:1,frames:1},
  {first:2,end:1,numerator:1,denominator:1,frames:1},
  {first:0,end:1,numerator:0,denominator:1,frames:1},
  {first:0,end:1,numerator:1,denominator:0,frames:1},
  {first:0,end:1,numerator:1,denominator:1,frames:maxEventVideoContinuityFrames+1},
 } { if span.atLeastSeconds(1) { t.Fatal("invalid arithmetic result accepted",span) } }
}

// Deterministic exhaustive perturbations, not synthetic footage certification.
func TestEventVideoContinuityRejectsEveryPerturbedBoundary(t *testing.T) {
 for count:=2;count<=64;count++ {
  frames:=make([]eventVideoFrameInterval,count)
  var pts int64 = -100
  for i:=range frames { frames[i]=eventVideoFrameInterval{pts,int64(i%7+1)};pts+=frames[i].duration }
  if _,err:=validateEventVideoContinuity(context.Background(),frames,1,1000);err!=nil { t.Fatal(count,err) }
  for i:=1;i<count;i++ { for _,delta:=range []int64{-1,1} {
   frames[i].pts+=delta
   span,err:=validateEventVideoContinuity(context.Background(),frames,1,1000)
   frames[i].pts-=delta
   if !errors.Is(err,errEventVideoContinuity) || span!=(eventVideoContinuity{}) { t.Fatal("perturbed boundary accepted",count,i,delta,span,err) }
  } }
 }
}

type eventContinuityCancelContext struct { context.Context; remaining int }
func (ctx *eventContinuityCancelContext) Err() error {
 ctx.remaining--
 if ctx.remaining<=0 { return context.Canceled }
 return nil
}
func TestEventVideoContinuityCancellationAndResourceCeiling(t *testing.T) {
 frames:=[]eventVideoFrameInterval{{0,1},{1,1},{2,1}}
 // Entry, each interval and the post-loop check must all observe cancellation.
 for call:=1;call<=len(frames)+2;call++ {
  ctx:=&eventContinuityCancelContext{Context:context.Background(),remaining:call}
  span,err:=validateEventVideoContinuity(ctx,frames,1,1)
  if !errors.Is(err,context.Canceled) || span!=(eventVideoContinuity{}) { t.Fatal(call,span,err) }
 }
 oversized:=make([]eventVideoFrameInterval,maxEventVideoContinuityFrames+1)
 span,err:=validateEventVideoContinuity(context.Background(),oversized,1,1)
 if !errors.Is(err,errEventVideoContinuity) || span!=(eventVideoContinuity{}) { t.Fatal("unbounded observation accepted",span,err) }
}
