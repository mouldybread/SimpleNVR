package main

import (
 "context"
 "errors"
 "io"
 "testing"
 "time"
)

// The UI exports up to eight selected cameras at once, one job each. With the default
// limits all eight must be admitted (one preparing, seven queued) and a ninth refused.
func TestExportJobsDefaultLimitsAdmitEightCameras(t *testing.T) {
 limits:=defaultExportJobLimits();limits.MaxBytes=8;limits.TotalBytes=16;limits.Lifetime=time.Hour
 s:=testExportJobsWithLimits(t,limits)
 blocking:=func(ctx context.Context,w io.Writer)error{<-ctx.Done();return ctx.Err()}
 for i:=1;i<=8;i++ {
  if _,err:=s.submit("owner",blocking);err!=nil {t.Fatalf("export %d of 8 refused: %v",i,err)}
 }
 views,err:=s.list("owner");if err!=nil {t.Fatal(err)}
 preparing,queued:=0,0
 for _,view:=range views {
  switch view.State {
  case "preparing":preparing++
  case "queued":queued++
  }
 }
 if len(views)!=8 || preparing!=1 || queued!=7 {t.Fatalf("want 1 preparing and 7 queued of 8, got %d preparing and %d queued of %d",preparing,queued,len(views))}
 if _,err:=s.submit("owner",blocking);!errors.Is(err,errExportJobsBusy) {t.Fatalf("ninth export was not refused: %v",err)}
 if n,err:=s.clearOwner("owner");err!=nil || n!=8 {t.Fatalf("clear released %d jobs: %v",n,err)}
 if _,err:=s.submit("owner",blocking);err!=nil {t.Fatalf("export refused after clear: %v",err)}
}
