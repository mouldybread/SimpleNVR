package main

import (
 "context"
 "errors"
 "fmt"
 "testing"
 "time"
)

// Test assertion policy only, not runtime classification or permission to accept
// missing audio. A semantic refusal can coexist with internal teardown cancel.
func corrTrackMissingAudioRefused(parent context.Context,err error) bool {
 return parent.Err()==nil && err!=nil && !errors.Is(err,context.DeadlineExceeded) &&
  (errors.Is(err,errCorrStreamMismatch) || errors.Is(err,errCorrStreamEmpty) || errors.Is(err,errCorrPrototype))
}
func TestCorrMissingAudioRefusalClassification(t *testing.T) {
 live:=context.Background()
 stopped,cancel:=context.WithCancel(live);cancel()
 expired,expire:=context.WithDeadline(live,time.Now().Add(-time.Second));defer expire()
 for _,tc:=range []struct{name string;parent context.Context;err error;want bool}{
  {"accepted",live,nil,false},
  {"mismatch",live,errCorrStreamMismatch,true},
  {"wrapped mismatch with teardown",live,fmt.Errorf("comparison: %w",errors.Join(errCorrStreamMismatch,context.Canceled,errors.New("signal: killed"))),true},
  {"empty stream with teardown",live,errors.Join(errCorrStreamEmpty,context.Canceled),true},
  {"process refusal with teardown",live,errors.Join(errCorrPrototype,context.Canceled),true},
  {"bare cancellation",live,context.Canceled,false},
  {"bare deadline",live,context.DeadlineExceeded,false},
  {"deadline plus mismatch",live,errors.Join(errCorrStreamMismatch,context.DeadlineExceeded),false},
  {"caller cancelled with mismatch",stopped,errors.Join(errCorrStreamMismatch,context.Canceled),false},
  {"caller expired with mismatch",expired,errCorrStreamMismatch,false},
  {"unrelated process error",live,errors.New("exit status 1"),false},
  {"matching text is not sentinel",live,errors.New("stream content mismatch"),false},
 } {t.Run(tc.name,func(t *testing.T){if got:=corrTrackMissingAudioRefused(tc.parent,tc.err);got!=tc.want {t.Fatal(got,tc.want,tc.err)}})}
}
func TestCorrMissingAudioControlledRefusalAndReuse(t *testing.T) {
 bm,job,lease:=corrSafetyFixture(t,false);defer bm.releaseJob(job)
 evidence:=job.recoveryBorrow.duration;gate:=newCorrAdmissionGate()
 report,err:=gate.inspect(bm.ctx,lease,evidence,corrStreamChildFactory(t,[]string{"normal","empty"},false),nil)
 corrStreamAssertJoined(t,report)
 if !corrTrackMissingAudioRefused(bm.ctx,err) || report.ChildrenStarted!=2 || len(gate.slot)!=0 {t.Fatal(report,err)}
 retried,retryErr:=gate.inspect(bm.ctx,lease,evidence,corrStreamChildFactory(t,[]string{"normal","normal"},false),nil)
 corrStreamAssertJoined(t,retried)
 if retryErr!=nil || !retried.Video.Equal || !retried.Audio.Equal || len(gate.slot)!=0 || !lease.held() || job.recoveryBorrow.duration!=evidence {t.Fatal("semantic refusal poisoned reuse",retried,retryErr)}
}
