package main

import (
 "errors"
 "time"
)

// Layout of a stamped recording: <camera dir>/<UTC start day>/<start>_E<end>_<label>.mp4
// Start and end are UTC. The end is the wall-clock end written at publication.
const (
 stampTimeLayout="20060102_150405"
 stampDayLayout="20060102"
 maxStampedSpan=24*time.Hour
 maxStampedScanDays=400
 minStampedNameLen=37
)
var errStampedName=errors.New("recording name has no valid start and end time")
var errStampedWindow=errors.New("recording window must be positive")
var errStampedScanDays=errors.New("recording window spans too many day directories")

type stampedName struct {Start,End time.Time}

func stampedFileName(start,end time.Time,label string) string {
 return start.UTC().Format(stampTimeLayout)+"_E"+end.UTC().Format(stampTimeLayout)+"_"+label+".mp4"
}
func stampedDayName(start time.Time) string {return start.UTC().Format(stampDayLayout)}
func stampedDayStart(t time.Time) time.Time {y,m,d:=t.UTC().Date();return time.Date(y,m,d,0,0,0,0,time.UTC)}

// A name is accepted only when it has the exact stamped shape and a positive
// span of at most maxStampedSpan. Anything else is an error the caller must handle.
func parseStampedName(name string) (stampedName,error) {
 if len(name)<minStampedNameLen || name[15:17]!="_E" || name[32]!='_' || name[len(name)-4:]!=".mp4" {return stampedName{},errStampedName}
 start,err:=time.ParseInLocation(stampTimeLayout,name[:15],time.UTC)
 if err!=nil {return stampedName{},errStampedName}
 end,err:=time.ParseInLocation(stampTimeLayout,name[17:32],time.UTC)
 if err!=nil || !end.After(start) || end.Sub(start)>maxStampedSpan {return stampedName{},errStampedName}
 return stampedName{start,end},nil
}

func (n stampedName) inDay(day string) bool {return stampedDayName(n.Start)==day}

// Inclusion rule: a recording overlaps the window when start<windowEnd and
// end>windowStart. Touching edges do not overlap.
func stampedInWindow(n stampedName,start,end time.Time) bool {return n.Start.Before(end) && n.End.After(start)}

// Day directories that can hold an overlapping recording: from the UTC day of
// (windowStart-maxStampedSpan) to the UTC day of the last instant before windowEnd.
func stampedScanDays(start,end time.Time) ([]string,error) {
 if !end.After(start) {return nil,errStampedWindow}
 first:=stampedDayStart(start.Add(-maxStampedSpan));last:=stampedDayStart(end.Add(-time.Nanosecond))
 days:=make([]string,0,4)
 for day:=first;!day.After(last);day=day.AddDate(0,0,1) {
  if len(days)>=maxStampedScanDays {return nil,errStampedScanDays}
  days=append(days,stampedDayName(day))
 }
 return days,nil
}
