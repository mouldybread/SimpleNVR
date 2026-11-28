package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"

 "golang.org/x/sys/unix"
)

// Physical enumeration is bounded per opened folder, including malformed and
// out-of-window entries. Only valid overlapping names consume the per-call bound.
const (
 maxStampedFolderEntries=8192
 maxStampedCandidates=20000
)

// stampedScanStats counts the work done by one call. It travels with the call's
// accumulator and is never global, so concurrent scans cannot disturb each other.
// DayLookups counts day directories requested, whether or not they exist.
// EntriesRead counts all names returned by Readdirnames, including batch read-ahead
// on early failure. It is not the number of names processed before failure.
// RelevantCandidates counts valid, correct-day, overlapping names identified before
// the candidate-limit check, including candidate 20,001 refused without inspection.
// MetadataCalls counts candidate inspections actually started. On failure the scan
// result is discarded, while these counters remain observable.
type stampedScanStats struct {DayLookups,DaysOpened,EntriesRead,RelevantCandidates,MetadataCalls int}

// stampedInspector inspects one named entry of a held day directory. Production
// uses stampedCandidateInfo; tests may wrap it to inject failures.
type stampedInspector func(dir *os.File,name string) (os.FileInfo,error)

// Unparseable counts .mp4 names in scanned day folders that are not a valid stamped
// name for that folder. They are never probed and never returned as intervals.
// Unreadable counts valid in-window candidates that could not be used (empty, not a
// regular file, or gone before inspection) and were skipped; it is only ever non-zero
// for a scan started with SkipUnreadable, which only the timeline uses.
type stampedScan struct {Intervals []recordingInterval;Unparseable,Unreadable int;Stats stampedScanStats;inspect stampedInspector;skipUnreadable bool}

// stampedScanOptions selects how an unusable in-window candidate is handled. The
// zero value is strict: the whole call fails. Downloads and exports must stay strict.
type stampedScanOptions struct {SkipUnreadable bool}

// Name-only selection: no ffprobe or media reads. Only the UTC day folders that
// can hold overlapping recordings are opened, with no-follow descriptors. Any
// candidate inspection failure discards the entire result, not just that entry.
func collectStampedIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow) (stampedScan,error) {
 scan,_,err:=collectStampedIntervalsObserved(ctx,cameras,window,nil)
 return scan,err
}

// The timeline degrades instead of failing on one unusable recording: the file is left
// out of the result, counted in Unreadable and reported to the client. Cancellation,
// every other error, a folder that cannot be read completely and all scan bounds still
// fail the whole call, and skipped candidates still count toward the candidate bound.
func collectTimelineIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow) (stampedScan,error) {
 scan,_,err:=collectStampedIntervalsWith(ctx,cameras,window,nil,stampedScanOptions{SkipUnreadable:true})
 return scan,err
}

// collectStampedIntervalsObserved is collectStampedIntervals with an optional
// inspector and the work counters returned even when the call fails. The scan
// itself is empty on failure: counters are the only thing a failed call reports.
func collectStampedIntervalsObserved(ctx context.Context,cameras []timelineCamera,window timelineWindow,inspect stampedInspector) (stampedScan,stampedScanStats,error) {
 return collectStampedIntervalsWith(ctx,cameras,window,inspect,stampedScanOptions{})
}

func collectStampedIntervalsWith(ctx context.Context,cameras []timelineCamera,window timelineWindow,inspect stampedInspector,options stampedScanOptions) (stampedScan,stampedScanStats,error) {
 var scan stampedScan
 if inspect==nil {inspect=stampedCandidateInfo}
 scan.inspect=inspect
 scan.skipUnreadable=options.SkipUnreadable
 if err:=ctx.Err();err!=nil {return stampedScan{},scan.Stats,err}
 days,err:=stampedScanDays(window.Start,window.End);if err!=nil {return stampedScan{},scan.Stats,err}
 scan.Intervals=make([]recordingInterval,0)
 for _,cam:=range cameras {
  if err:=ctx.Err();err!=nil {return stampedScan{},scan.Stats,err}
  if err:=scanStampedCamera(ctx,cam,days,window,&scan);err!=nil {return stampedScan{},scan.Stats,err}
 }
 sort.Slice(scan.Intervals,func(i,j int) bool {
  a,b:=scan.Intervals[i],scan.Intervals[j]
  if a.Start.Equal(b.Start) {return a.Path<b.Path}
  return a.Start.Before(b.Start)
 })
 if err:=ctx.Err();err!=nil {return stampedScan{},scan.Stats,err}
 scan.inspect=nil
 return scan,scan.Stats,nil
}

func scanStampedCamera(ctx context.Context,cam timelineCamera,days []string,window timelineWindow,scan *stampedScan) error {
 if !filepath.IsAbs(cam.Dir) {return errRecordingMetadata}
 root,err:=openCameraDeleteDirectory(unix.AT_FDCWD,cam.Dir)
 if errors.Is(err,os.ErrNotExist) {return nil}
 if err!=nil {return errRecordingMetadata}
 defer root.Close()
 for _,day:=range days {
  if err:=ctx.Err();err!=nil {return err}
  if err:=scanStampedDay(ctx,cam,root,day,window,scan);err!=nil {return err}
 }
 return nil
}

// Inspect only the named entry in the held day directory. O_PATH never opens a
// FIFO/device for data; O_NOFOLLOW pins a symlink itself so Stat rejects its type.
// FileInfo retains the OS identity used by export pinning and sameMediaFile.
func stampedCandidateInfo(dir *os.File,name string) (os.FileInfo,error) {
 fd,err:=unix.Openat(int(dir.Fd()),name,unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return nil,errRecordingMetadata}
 file:=os.NewFile(uintptr(fd),name)
 info,statErr:=file.Stat()
 closeErr:=file.Close()
 if statErr!=nil || closeErr!=nil || !info.Mode().IsRegular() || info.Size()<=0 {return nil,errRecordingMetadata}
 return info,nil
}

func scanStampedDay(ctx context.Context,cam timelineCamera,root *os.File,day string,window timelineWindow,scan *stampedScan) error {
 inspect:=scan.inspect
 if inspect==nil {inspect=stampedCandidateInfo}
 scan.Stats.DayLookups++
 dir,err:=openCameraDeleteDirectory(int(root.Fd()),day)
 // Missing, symlinked or non-directory day entries are skipped, never followed.
 if errors.Is(err,os.ErrNotExist) || errors.Is(err,unix.ELOOP) || errors.Is(err,unix.ENOTDIR) {return nil}
 if err!=nil {return errRecordingMetadata}
 defer dir.Close()
 scan.Stats.DaysOpened++
 seen:=0
 for {
  if err:=ctx.Err();err!=nil {return err}
  names,readErr:=dir.Readdirnames(256)
  scan.Stats.EntriesRead+=len(names)
  for _,name:=range names {
   seen++
   if seen>maxStampedFolderEntries {return errRecordingScanLimit}
   if err:=ctx.Err();err!=nil {return err}
   if !strings.HasSuffix(name,".mp4") {continue}
   stamp,err:=parseStampedName(name)
   if err!=nil || !stamp.inDay(day) {scan.Unparseable++;continue}
   if !stampedInWindow(stamp,window.Start,window.End) {continue}
   scan.Stats.RelevantCandidates++
   // One call-wide budget across all cameras and day folders. Skipped candidates
   // count too, so tolerating unreadable files can never hide an oversized scan.
   // Check before inspecting candidate 20,001.
   if scan.Stats.RelevantCandidates>maxStampedCandidates {return errRecordingScanLimit}
   scan.Stats.MetadataCalls++
   info,err:=inspect(dir,name)
   if err!=nil {
    // Only an unusable recording is skipped, and only when the caller allows it.
    // Cancellation and every other error still fail the call.
    if scan.skipUnreadable && errors.Is(err,errRecordingMetadata) {scan.Unreadable++;continue}
    return err
   }
   if err:=ctx.Err();err!=nil {return err}
   path:=filepath.Join(cam.Dir,day,name)
   scan.Intervals=append(scan.Intervals,recordingInterval{cam.ID,cam.Name,path,stamp.Start,stamp.End,info.Size(),info})
  }
  if readErr==io.EOF {return nil}
  if readErr!=nil {return errRecordingMetadata}
 }
}
