package main

import (
 "context"
 "time"
)

// Preserve the existing selection exactly: short names are retained, a valid
// UTC filename prefix wins, otherwise use mtime; equality is not expired.
func publishedExpiredBefore(cutoff time.Time) func(string,time.Time) bool {
 return func(name string,modified time.Time) bool {
  if len(name)<15 {return false}
  start,err:=time.ParseInLocation("20060102_150405",name[:15],time.UTC)
  if err!=nil {start=modified.UTC()}
  return start.Before(cutoff)
 }
}

// Borrow both the media lease and caller's policy guard. No nested acquisition.
func cleanupExpiredPublishedLeased(ctx context.Context,lease *mediaMutationLease,dir string,retention time.Duration,guard func() error) error {
 if retention<=0 {return errCameraRetention}
 _,err:=deletePublishedLeased(ctx,lease,dir,publishedExpiredBefore(time.Now().UTC().Add(-retention)),guard)
 return err
}
