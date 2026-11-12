package main

import (
 "context"
 "errors"
 "io"
 "path/filepath"
 "sort"
 "time"

 "golang.org/x/sys/unix"
)

const publishedDeletionEntryLimit=8192
var errPublishedDeletionRefused=errors.New("published deletion refused: invalid ownership, changed directory/entry, or inspection limit")
type publishedDeletionEntry struct {name string;stat unix.Stat_t}

// Caller acquires the shared lease and supplies a pure scope predicate plus a
// registry/policy/shutdown guard. Never acquire/release ownership in this helper.
func deletePublishedLeased(ctx context.Context,lease *mediaMutationLease,dir string,selectFile func(string,time.Time) bool,guard func() error) (publishedDeleteResult,error) {
 return deletePublishedLeasedWithUnlink(ctx,lease,dir,selectFile,guard,unix.Unlinkat)
}

// Fail fast; Count/Bytes describe successful unlinks, Failures=1 means this
// pass ended with an error. There is no recursive sweep or directory removal.
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
 var plan []publishedDeletionEntry
 seen:=0
 for {
  if err:=rootCheck();err!=nil {return result,err}
  names,readErr:=root.Readdirnames(64)
  for _,name:=range names {
   seen++;if seen>publishedDeletionEntryLimit {return result,errPublishedDeletionRefused}
   if filepath.Ext(name)!=".mp4" {continue}
   if err:=rootCheck();err!=nil {return result,err}
   entry:=publishedDeletionEntry{name:name}
   if err:=unix.Fstatat(int(root.Fd()),name,&entry.stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return result,err}
   if entry.stat.Mode&unix.S_IFMT!=unix.S_IFREG {continue}
   if selectFile(name,time.Unix(entry.stat.Mtim.Sec,entry.stat.Mtim.Nsec)) {plan=append(plan,entry)}
  }
  if readErr==io.EOF {break};if readErr!=nil {return result,readErr}
 }
 sort.Slice(plan,func(i,j int) bool{return plan[i].name<plan[j].name})
 validate:=func(entry publishedDeletionEntry) error {
  if err:=rootCheck();err!=nil {return err}
  var current unix.Stat_t
  if err:=unix.Fstatat(int(root.Fd()),entry.name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if !sameCameraDeleteEntry(&entry.stat,&current) {return errPublishedDeletionRefused}
  return nil
 }
 // Complete preflight before the first unlink, then recheck each candidate.
 // Trusted directory ownership remains required: stat/unlink is not atomic.
 for _,entry:=range plan {if err:=validate(entry);err!=nil {return result,err}}
 for _,entry:=range plan {
  if err:=validate(entry);err!=nil {return result,err}
  if err:=unlink(int(root.Fd()),entry.name,0);err!=nil {return result,err}
  result.Count++;result.Bytes+=entry.stat.Size
 }
 return result,nil
}
