package main

import (
 "context"
 "time"

 "golang.org/x/sys/unix"
)

// A valid stamped name expires on its END time: it is deleted only when the end is
// strictly before the cutoff, so a recording that still overlaps the retention window
// is kept and equality is retained. Other names keep the previous rules: short names
// are retained, a valid UTC filename prefix wins, otherwise use mtime.
func publishedExpiredBefore(cutoff time.Time) func(string,time.Time) bool {
 return func(name string,modified time.Time) bool {
  if stamp,err:=parseStampedName(name);err==nil {return stamp.End.Before(cutoff)}
  if len(name)<15 {return false}
  start,err:=time.ParseInLocation("20060102_150405",name[:15],time.UTC)
  if err!=nil {start=modified.UTC()}
  return start.Before(cutoff)
 }
}

// Pass an immutable cutoff for deterministic tests and one policy instant per pass.
// Work belongs to this pass and must not be shared or reused between calls.
func deleteExpiredPublishedLeasedAt(ctx context.Context,lease *mediaMutationLease,dir string,cutoff time.Time,guard func() error,work *publishedDeletionWork) (publishedDeleteResult,error) {
 if work==nil {work=newPublishedDeletionWork()}
 cutoff=cutoff.UTC();work.cutoff=&cutoff
 return deletePublishedLeasedWithWork(ctx,lease,dir,publishedExpiredBefore(cutoff),guard,unix.Unlinkat,work)
}

// Borrow both the media lease and caller's policy guard. No nested acquisition.
func cleanupExpiredPublishedLeased(ctx context.Context,lease *mediaMutationLease,dir string,retention time.Duration,guard func() error) error {
 if retention<=0 {return errCameraRetention}
 _,err:=deleteExpiredPublishedLeasedAt(ctx,lease,dir,time.Now().UTC().Add(-retention),guard,nil)
 return err
}
