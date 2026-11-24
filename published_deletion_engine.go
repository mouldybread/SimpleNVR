package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "sort"
 "time"

 "golang.org/x/sys/unix"
)

const publishedDeletionEntryLimit=8192
var errPublishedDeletionRefused=errors.New("published deletion refused: invalid ownership, changed directory/entry, or inspection limit")
// day is nil for a file directly in the camera directory. A day folder is opened
// no-follow, pinned by identity, and only its regular .mp4 files are candidates.
type publishedDeletionDay struct {name string;file *os.File;id unix.Stat_t;deleted int}
type publishedDeletionEntry struct {name string;day *publishedDeletionDay;stat unix.Stat_t}

func isStampedDayName(name string) bool {
 if len(name)!=len(stampDayLayout) {return false}
 _,err:=time.Parse(stampDayLayout,name)
 return err==nil
}

// Caller acquires the shared lease and supplies a pure scope predicate plus a
// registry/policy/shutdown guard. Never acquire/release ownership in this helper.
func deletePublishedLeased(ctx context.Context,lease *mediaMutationLease,dir string,selectFile func(string,time.Time) bool,guard func() error) (publishedDeleteResult,error) {
 return deletePublishedLeasedWithUnlink(ctx,lease,dir,selectFile,guard,unix.Unlinkat)
}

// Fail fast; Count/Bytes describe successful unlinks, Failures=1 means this
// pass ended with an error. Files are unlinked individually; a stamped day folder
// is removed only by an atomic empty-directory rmdir after its files were deleted,
// so a late entry keeps the folder. There is no recursive sweep.
func deletePublishedLeasedWithUnlink(ctx context.Context,lease *mediaMutationLease,dir string,selectFile func(string,time.Time) bool,guard func() error,unlink func(int,string,int) error) (result publishedDeleteResult,resultErr error) {
 defer func(){if resultErr!=nil {result.Failures=1}}()
 check:=func() error {
  if err:=ctx.Err();err!=nil {return err}
  if !lease.held() || lease.gate!=recordingMediaOwnership || selectFile==nil || guard==nil || unlink==nil {return errPublishedDeletionRefused}
  return guard()
 }
 if err:=check();err!=nil {return result,err}
 dir=filepath.Clean(dir)
 if !filepath.IsAbs(dir) || dir==string(filepath.Separator) {return result,errPublishedDeletionRefused}
 parent,err:=openCameraDeleteDirectory(unix.AT_FDCWD,filepath.Dir(dir));if err!=nil {return result,err}
 defer func(){resultErr=errors.Join(resultErr,parent.Close())}()
 root,err:=openCameraDeleteDirectory(int(parent.Fd()),filepath.Base(dir));if err!=nil {return result,err}
 defer func(){resultErr=errors.Join(resultErr,root.Close())}()
 defer func(){if result.Count>0 {resultErr=errors.Join(resultErr,root.Sync())}}()
 var parentID,rootID unix.Stat_t
 if err:=unix.Fstat(int(parent.Fd()),&parentID);err!=nil {return result,err}
 if err:=unix.Fstat(int(root.Fd()),&rootID);err!=nil {return result,err}
 var days []*publishedDeletionDay
 defer func(){for _,day:=range days {if day.deleted>0 {resultErr=errors.Join(resultErr,day.file.Sync())};resultErr=errors.Join(resultErr,day.file.Close())}}()
 sameDir:=func(fd int,name string,want unix.Stat_t) error {
  var current unix.Stat_t
  if err:=unix.Fstatat(fd,name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if current.Dev!=want.Dev || current.Ino!=want.Ino || current.Mode&unix.S_IFMT!=unix.S_IFDIR {return errPublishedDeletionRefused}
  return nil
 }
 rootCheck:=func() error {
  if err:=check();err!=nil {return err}
  if err:=sameDir(unix.AT_FDCWD,filepath.Dir(dir),parentID);err!=nil {return err}
  return sameDir(int(parent.Fd()),filepath.Base(dir),rootID)
 }
 dayCheck:=func(day *publishedDeletionDay) error {
  if err:=rootCheck();err!=nil {return err}
  return sameDir(int(root.Fd()),day.name,day.id)
 }
 var plan []publishedDeletionEntry
 seen:=0
 for {
  if err:=rootCheck();err!=nil {return result,err}
  names,readErr:=root.Readdirnames(64)
  for _,name:=range names {
   seen++;if seen>publishedDeletionEntryLimit {return result,errPublishedDeletionRefused}
   if isStampedDayName(name) {
    if err:=rootCheck();err!=nil {return result,err}
    var listed unix.Stat_t
    if err:=unix.Fstatat(int(root.Fd()),name,&listed,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return result,err}
    // Symlinks and other non-directories named like a day are never followed.
    if listed.Mode&unix.S_IFMT!=unix.S_IFDIR {continue}
    if len(days)>=maxStampedScanDays {return result,errPublishedDeletionRefused}
    file,err:=openCameraDeleteDirectory(int(root.Fd()),name);if err!=nil {return result,err}
    day:=&publishedDeletionDay{name:name,file:file};days=append(days,day)
    if err:=unix.Fstat(int(file.Fd()),&day.id);err!=nil {return result,err}
    if day.id.Dev!=listed.Dev || day.id.Ino!=listed.Ino {return result,errPublishedDeletionRefused}
    continue
   }
   if filepath.Ext(name)!=".mp4" {continue}
   if err:=rootCheck();err!=nil {return result,err}
   entry:=publishedDeletionEntry{name:name}
   if err:=unix.Fstatat(int(root.Fd()),name,&entry.stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return result,err}
   if entry.stat.Mode&unix.S_IFMT!=unix.S_IFREG {continue}
   if selectFile(name,time.Unix(entry.stat.Mtim.Sec,entry.stat.Mtim.Nsec)) {plan=append(plan,entry)}
  }
  if readErr==io.EOF {break};if readErr!=nil {return result,readErr}
 }
 for _,day:=range days {
  count:=0
  for {
   if err:=dayCheck(day);err!=nil {return result,err}
   names,readErr:=day.file.Readdirnames(64)
   for _,name:=range names {
    count++;if count>publishedDeletionEntryLimit {return result,errPublishedDeletionRefused}
    if filepath.Ext(name)!=".mp4" {continue}
    if err:=dayCheck(day);err!=nil {return result,err}
    entry:=publishedDeletionEntry{name:name,day:day}
    if err:=unix.Fstatat(int(day.file.Fd()),name,&entry.stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return result,err}
    if entry.stat.Mode&unix.S_IFMT!=unix.S_IFREG {continue}
    if selectFile(name,time.Unix(entry.stat.Mtim.Sec,entry.stat.Mtim.Nsec)) {plan=append(plan,entry)}
   }
   if readErr==io.EOF {break};if readErr!=nil {return result,readErr}
  }
 }
 dayOf:=func(entry publishedDeletionEntry) string {if entry.day==nil {return ""};return entry.day.name}
 sort.Slice(plan,func(i,j int) bool{
  di,dj:=dayOf(plan[i]),dayOf(plan[j])
  if di!=dj {return di<dj}
  return plan[i].name<plan[j].name
 })
 dirFD:=func(entry publishedDeletionEntry) int {if entry.day==nil {return int(root.Fd())};return int(entry.day.file.Fd())}
 validate:=func(entry publishedDeletionEntry) error {
  if entry.day==nil {if err:=rootCheck();err!=nil {return err}} else {if err:=dayCheck(entry.day);err!=nil {return err}}
  var current unix.Stat_t
  if err:=unix.Fstatat(dirFD(entry),entry.name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if !sameCameraDeleteEntry(&entry.stat,&current) {return errPublishedDeletionRefused}
  return nil
 }
 // Complete preflight before the first unlink, then recheck each candidate.
 // Trusted directory ownership remains required: stat/unlink is not atomic.
 for _,entry:=range plan {if err:=validate(entry);err!=nil {return result,err}}
 for _,entry:=range plan {
  if err:=validate(entry);err!=nil {return result,err}
  if err:=unlink(dirFD(entry),entry.name,0);err!=nil {return result,err}
  result.Count++;result.Bytes+=entry.stat.Size
  if entry.day!=nil {entry.day.deleted++}
 }
 // An emptied day folder is removed only by atomic rmdir. A late or retained entry
 // makes it fail with ENOTEMPTY, which keeps the folder and is not an error.
 for _,day:=range days {
  if day.deleted==0 {continue}
  if err:=dayCheck(day);err!=nil {return result,err}
  if err:=unlink(int(root.Fd()),day.name,unix.AT_REMOVEDIR);err!=nil && !errors.Is(err,unix.ENOTEMPTY) && !errors.Is(err,unix.EEXIST) {return result,err}
 }
 return result,nil
}
