package main

import (
 "context"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// Channel handshakes control phase ordering; the timeout is only a deadlock
// watchdog, not a work/performance assertion. Publication must acquire ownership
// before creating its day folder, including when retention has just removed it.
func TestRetentionPublicationRecreatesDayOnlyAfterLeaseRelease(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();start:=scanTime("20261101","100000")
 old:=scanFixture(t,dir,start,start.Add(time.Minute),"old")
 temporary:=publicationSource(t,dir,"pending.mp4.tmp","new published bytes")
 started,retentionDone:=make(chan struct{}),make(chan struct{})
 type outcome struct {path string;err error}
 done:=make(chan outcome,1)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 go func() {
  close(started)
  own,err:=recordingMediaOwnership.acquire(ctx)
  if err!=nil {done<-outcome{err:err};return}
  defer own.release()
  validate:=func(ctx context.Context,path string)error {
   select {case <-retentionDone:default:t.Error("publication entered before retention completed")}
   return acceptPublicationFixture(ctx,path)
  }
  path,err:=publishStampedLeasedWith(ctx,own,temporary,dir,start,start.Add(time.Minute),"new",false,validate)
  done<-outcome{path,err}
 }()
 // Ensure every failure path releases ownership and joins the publisher before
 // temporary-directory cleanup, even when an assertion calls Fatal.
 defer func(){cancel();lease.release();select {case <-done:case <-time.After(5*time.Second):t.Error("publisher cleanup did not join")}}()
 <-started
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,scanTime("20261110","000000"),publishedEngineGuard,nil)
 if err!=nil || result.Count!=1 {t.Fatal(result,err)}
 if _,err:=os.Lstat(filepath.Dir(old));!os.IsNotExist(err) {t.Fatal("old empty day was not removed",err)}
 assertPublicationBytes(t,temporary,"new published bytes")
 close(retentionDone);lease.release()
 select {
 case out:=<-done:
  // Put the outcome back so the deferred join observes the completed worker.
  done<-out
  if out.err!=nil {t.Fatal(out.err)}
  assertPublicationBytes(t,out.path,"new published bytes")
  if filepath.Dir(out.path)!=filepath.Dir(old) {t.Fatal(out.path)}
  if _,err:=os.Lstat(temporary);!os.IsNotExist(err) {t.Fatal("published temporary retained",err)}
 case <-time.After(5*time.Second):t.Fatal("publication did not finish after retention released ownership")
 }
}
