package main

import (
 "os"
 "time"

 "golang.org/x/sys/unix"
)

// Observation and injection belong to one pass, never mutable package state.
// Reads include EOF/error attempts; EntriesRead includes the whole returned batch.
// EntriesProcessed includes the overflow entry that triggers refusal. MetadataCalls
// includes directory guards and identity checks as well as candidate metadata.
// Mutation successes count syscalls, not durable completion (sync can still fail).
type publishedDeletionStats struct {
 RootReads,DayReads,EntriesRead,EntriesProcessed int
 DayFolders,DaysOpened,DaysSkipped int
 MetadataCalls,UnlinkAttempts,Unlinks,RmdirAttempts,Rmdirs,SyncAttempts int
}
type publishedDeletionWork struct {
 Stats publishedDeletionStats
 cutoff *time.Time
 readHook func(*os.File,int)([]string,error)
 statHook func(int,*unix.Stat_t) error
 statatHook func(int,string,*unix.Stat_t,int) error
 syncHook func(*os.File) error
}
func newPublishedDeletionWork() *publishedDeletionWork {
 return &publishedDeletionWork{
  readHook:func(file *os.File,n int)([]string,error){return file.Readdirnames(n)},
  statHook:unix.Fstat,statatHook:unix.Fstatat,
  syncHook:func(file *os.File) error{return file.Sync()},
 }
}
func(w *publishedDeletionWork)read(file *os.File,root bool)([]string,error) {
 if root {w.Stats.RootReads++} else {w.Stats.DayReads++}
 names,err:=w.readHook(file,64);w.Stats.EntriesRead+=len(names)
 return names,err
}
func(w *publishedDeletionWork)stat(fd int,out *unix.Stat_t) error {
 w.Stats.MetadataCalls++;return w.statHook(fd,out)
}
func(w *publishedDeletionWork)statat(fd int,name string,out *unix.Stat_t,flags int) error {
 w.Stats.MetadataCalls++;return w.statatHook(fd,name,out,flags)
}
func(w *publishedDeletionWork)sync(file *os.File) error {
 w.Stats.SyncAttempts++;return w.syncHook(file)
}
func(w *publishedDeletionWork)remove(unlink func(int,string,int) error,fd int,name string,flags int) error {
 if flags==unix.AT_REMOVEDIR {w.Stats.RmdirAttempts++} else {w.Stats.UnlinkAttempts++}
 err:=unlink(fd,name,flags)
 if err==nil {if flags==unix.AT_REMOVEDIR {w.Stats.Rmdirs++} else {w.Stats.Unlinks++}}
 return err
}

type retentionDayClass int
const (
 retentionOldDay retentionDayClass=iota
 retentionBoundaryDay
 retentionRetainedDay
)

// Only valid calendar-day names reach this classifier. Start-day bounds describe
// correctly placed stamped recordings of at most 24 hours, not arbitrary children.
// Even old days still require the original individual-file predicate and checks.
func classifyRetentionDay(day string,cutoff time.Time) retentionDayClass {
 cutoffDay:=stampedDayStart(cutoff)
 if day>stampedDayName(cutoffDay) {return retentionRetainedDay}
 if day>=stampedDayName(cutoffDay.AddDate(0,0,-1)) {return retentionBoundaryDay}
 return retentionOldDay
}

// Retention alone chooses option 2: newer day contents are conservatively left
// untouched, including misplaced/malformed expired files, until their day ages in.
// A nil work is convenient for production; a supplied work exposes this pass.
func deleteExpiredPublishedAt(ctxContext interface{}) {}
