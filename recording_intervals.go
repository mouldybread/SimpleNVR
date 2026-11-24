package main

import (
 "context"
 "errors"
 "net/http"
 "os"
 "time"
)

const recordingMetadataBudget=30*time.Second
const maxRecordingScanEntries=8192
var errRecordingMetadata=errors.New("recording metadata unavailable")
var errRecordingChanged=errors.New("recording changed during inspection")
var errRecordingScanLimit=errors.New("recording scan limit exceeded")

func sameMediaFile(a,b os.FileInfo) bool {
 return a!=nil && b!=nil && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a,b) && a.Size()==b.Size() && a.ModTime().Equal(b.ModTime())
}

type recordingInterval struct {CameraID,CameraName,Path string;Start,End time.Time;Size int64;identity os.FileInfo}
func intervalsOverlap(start,end,windowStart,windowEnd time.Time) bool {return start.Before(windowEnd) && end.After(windowStart)}

// Selection uses stamped names only. Export validation probes remain separate.
func collectRecordingIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow) ([]recordingInterval,error) {
 scan,err:=collectStampedIntervals(ctx,cameras,window);if err!=nil {return nil,err}
 return scan.Intervals,nil
}
func writeRecordingIntervalError(w http.ResponseWriter,r *http.Request,err error) {
 if r.Context().Err()!=nil {return}
 status:=http.StatusServiceUnavailable
 if errors.Is(err,context.DeadlineExceeded) {status=http.StatusGatewayTimeout}
 w.Header().Set("X-Recording-Metadata-Status","unavailable")
 http.Error(w,"Recording metadata unavailable; no partial timeline or export was returned. Retry or inspect stored media.",status)
}
