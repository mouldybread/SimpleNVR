package main

import (
 "context"
 "errors"
 "os"

 "golang.org/x/sys/unix"
)

// A stamped basename selects exactly one UTC start-day directory. Legacy
// basenames retain root lookup; stamped files are never adopted from the root
// or searched for recursively. Inspection does not create a missing day.
type recoveryPublishedLocation struct {
 root,parent *os.File
 day string
 identity unix.Stat_t
}

func openRecoveryPublishedLocation(root *os.File,name string) (*recoveryPublishedLocation,error) {
 location:=&recoveryPublishedLocation{root:root,parent:root}
 stamp,err:=parseStampedName(name)
 if err!=nil {return location,nil}
 location.day=stampedDayName(stamp.Start)
 location.parent=nil
 var named unix.Stat_t
 if err:=unix.Fstatat(int(root.Fd()),location.day,&named,unix.AT_SYMLINK_NOFOLLOW);err!=nil {
  if errors.Is(err,unix.ENOENT) {return location,nil}
  return nil,errRecoveryInspection
 }
 parent,identity,err:=openRecoveryChild(root,location.day)
 if err!=nil {return nil,err}
 location.parent=parent;location.identity=identity
 return location,nil
}

func (location *recoveryPublishedLocation) observe(ctx context.Context,name string) (recoveryArtifactObservation,error) {
 if location.parent==nil {
  item:=recoveryArtifactObservation{Role:"published_output"}
  if ctx.Err()!=nil {return item,errRecoveryInspection}
  return item,nil
 }
 return observeRecoveryArtifact(ctx,location.parent,name,"published_output",nil)
}

// Keep the opened descriptor alive through final manifest revalidation. A
// missing day must still be absent; an opened day must still name the same
// directory. A rename/replacement cannot turn this observation into completion.
func (location *recoveryPublishedLocation) current() error {
 if location==nil || location.day=="" {return nil}
 var named unix.Stat_t
 err:=unix.Fstatat(int(location.root.Fd()),location.day,&named,unix.AT_SYMLINK_NOFOLLOW)
 if location.parent==nil {
  if errors.Is(err,unix.ENOENT) {return nil}
  return errRecoveryInspection
 }
 if err!=nil || !sameRecoveryObject(&location.identity,&named) {return errRecoveryInspection}
 return nil
}

func (location *recoveryPublishedLocation) close() error {
 if location==nil || location.day=="" || location.parent==nil {return nil}
 return location.parent.Close()
}
