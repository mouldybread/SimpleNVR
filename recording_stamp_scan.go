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

const maxStampedFolderEntries=8192

// Unparseable counts .mp4 names in scanned day folders that are not a valid stamped
// name for that folder. They are never probed and never returned as intervals.
type stampedScan struct {Intervals []recordingInterval;Unparseable int}

// Name-only scan: no ffprobe, no file reads. Only the UTC day folders that can hold an
// overlapping recording are opened (stampedScanDays), each with O_NOFOLLOW, and each
// folder is bounded by maxStampedFolderEntries.
func collectStampedIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow) (stampedScan,error) {
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
 return scan,nil
}

func scanStampedCamera(ctx context.Context,cam timelineCamera,days []string,window timelineWindow,scan *stampedScan) error {
 if !filepath.IsAbs(cam.Dir) {return errRecordingMetadata}
 root,err:=openCameraDeleteDirectory(unix.AT_FDCWD,cam.Dir)
 if errors.Is(err,os.ErrNotExist) {return nil}
 if err!=nil {return errRecordingMetadata}
 defer root.Close()
 for _,day:=range days {
  if err:=scanStampedDay(ctx,cam,root,day,window,scan);err!=nil {return err}
 }
 return nil
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
   path:=filepath.Join(cam.Dir,day,name)
   info,err:=os.Lstat(path)
   if errors.Is(err,os.ErrNotExist) {continue}
   if err!=nil {return errRecordingMetadata}
   if !info.Mode().IsRegular() {continue}
   scan.Intervals=append(scan.Intervals,recordingInterval{cam.ID,cam.Name,path,stamp.Start,stamp.End,info.Size(),info})
  }
  if readErr==io.EOF {return nil}
  if readErr!=nil {return errRecordingMetadata}
 }
}
