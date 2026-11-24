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

// Unparseable counts .mp4 names in scanned day folders that are not a valid stamped
// name for that folder. They are never probed and never returned as intervals.
type stampedScan struct {Intervals []recordingInterval;Unparseable int}

// Name-only selection: no ffprobe or media reads. Only the UTC day folders that
// can hold overlapping recordings are opened, with no-follow descriptors. Any
// candidate inspection failure discards the entire result, not just that entry.
func collectStampedIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow) (stampedScan,error) {
 if err:=ctx.Err();err!=nil {return stampedScan{},err}
 days,err:=stampedScanDays(window.Start,window.End);if err!=nil {return stampedScan{},err}
 var scan stampedScan
 scan.Intervals=make([]recordingInterval,0)
 for _,cam:=range cameras {
  if err:=ctx.Err();err!=nil {return stampedScan{},err}
  if err:=scanStampedCamera(ctx,cam,days,window,&scan);err!=nil {return stampedScan{},err}
 }
 sort.Slice(scan.Intervals,func(i,j int) bool {
  a,b:=scan.Intervals[i],scan.Intervals[j]
  if a.Start.Equal(b.Start) {return a.Path<b.Path}
  return a.Start.Before(b.Start)
 })
 if err:=ctx.Err();err!=nil {return stampedScan{},err}
 return scan,nil
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
 dir,err:=openCameraDeleteDirectory(int(root.Fd()),day)
 // Missing, symlinked or non-directory day entries are skipped, never followed.
 if errors.Is(err,os.ErrNotExist) || errors.Is(err,unix.ELOOP) || errors.Is(err,unix.ENOTDIR) {return nil}
 if err!=nil {return errRecordingMetadata}
 defer dir.Close()
 seen:=0
 for {
  if err:=ctx.Err();err!=nil {return err}
  names,readErr:=dir.Readdirnames(256)
  for _,name:=range names {
   seen++;if seen>maxStampedFolderEntries {return errRecordingScanLimit}
   if err:=ctx.Err();err!=nil {return err}
   if !strings.HasSuffix(name,".mp4") {continue}
   stamp,err:=parseStampedName(name)
   if err!=nil || !stamp.inDay(day) {scan.Unparseable++;continue}
   if !stampedInWindow(stamp,window.Start,window.End) {continue}
   // Every previously accepted candidate has exactly one interval. Any rejected
   // candidate aborts the call, so this length is the call-wide candidate count.
   // Check before inspecting candidate 20,001, across all cameras and day folders.
   if len(scan.Intervals)>=maxStampedCandidates {return errRecordingScanLimit}
   info,err:=stampedCandidateInfo(dir,name)
   if err!=nil {return err}
   if err:=ctx.Err();err!=nil {return err}
   path:=filepath.Join(cam.Dir,day,name)
   scan.Intervals=append(scan.Intervals,recordingInterval{cam.ID,cam.Name,path,stamp.Start,stamp.End,info.Size(),info})
  }
  if readErr==io.EOF {return nil}
  if readErr!=nil {return errRecordingMetadata}
 }
}
