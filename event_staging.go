package main

import (
 "context"
 "errors"
 "io"
 "os"
 "strconv"
 "strings"
 "time"

 "golang.org/x/sys/unix"
)

const (eventStagingScanEntries=8192;eventStagingScanDepth=4)
var (
 errEventStagingPolicy=errors.New("invalid event staging admission configuration")
 errEventStagingUnavailable=errors.New("unable to complete bounded event staging inspection")
 errEventStagingLimit=errors.New("configured event staging admission limit reached")
)

type eventStagingPolicy struct {bytes,dirs uint64}
type eventStagingUsage struct {bytes,dirs uint64;entries int}

func configuredEventStagingPolicy() (eventStagingPolicy,error) {
 var policy eventStagingPolicy
 for _,item:=range []struct{name string;value *uint64}{
  {"SIMPLENVR_MAX_EVENT_STAGING_BYTES",&policy.bytes},
  {"SIMPLENVR_MAX_EVENT_STAGING_DIRS",&policy.dirs},
 } {
  raw:=os.Getenv(item.name);if raw=="" {continue}
  value,err:=strconv.ParseUint(raw,10,64);if err!=nil {return policy,errEventStagingPolicy}
  *item.value=value
 }
 return policy,nil
}

func addEventStagingSize(usage *eventStagingUsage,size int64) error {
 if size<0 || uint64(size)>^uint64(0)-usage.bytes {return errEventStagingUnavailable}
 usage.bytes+=uint64(size)
 return nil
}

// Descriptor-relative child lookup avoids following staging symlinks or
// re-resolving directory ancestors during traversal. The camera root's parent
// path must still be trusted; this is not an atomic filesystem snapshot.
func scanEventStaging(ctx context.Context,dir string,maxEntries,maxDepth int) (eventStagingUsage,error) {
 var usage eventStagingUsage
 if ctx.Err()!=nil {return usage,ctx.Err()}
 if maxEntries<1 || maxDepth<0 {return usage,errEventStagingUnavailable}
 flags:=unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC
 fd,err:=unix.Open(dir,flags,0);if err!=nil {return usage,errEventStagingUnavailable}
 root:=os.NewFile(uintptr(fd),"camera-staging-root");defer root.Close()
 var walk func(*os.File,bool,int) error
 walk=func(parent *os.File,staging bool,depth int) error {
  if ctx.Err()!=nil {return ctx.Err()}
  if staging && depth>maxDepth {return errEventStagingUnavailable}
  for {
   if ctx.Err()!=nil {return ctx.Err()}
   names,readErr:=parent.Readdirnames(64)
   for _,name:=range names {
    if ctx.Err()!=nil {return ctx.Err()}
    usage.entries++;if usage.entries>maxEntries {return errEventStagingUnavailable}
    if !staging && name!="temp" && !strings.HasSuffix(name,".tmp") {continue}
    var stat unix.Stat_t
    if err:=unix.Fstatat(int(parent.Fd()),name,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return errEventStagingUnavailable}
    kind:=stat.Mode&unix.S_IFMT
    if !staging && name!="temp" {
     if kind!=unix.S_IFREG {return errEventStagingUnavailable}
     if err:=addEventStagingSize(&usage,stat.Size);err!=nil {return err}
     continue
    }
    if kind==unix.S_IFDIR {
     nextDepth:=0
     if staging {nextDepth=depth+1;if depth==0 {usage.dirs++}}
     if nextDepth>maxDepth {return errEventStagingUnavailable}
     childFD,err:=unix.Openat(int(parent.Fd()),name,flags,0);if err!=nil {return errEventStagingUnavailable}
     child:=os.NewFile(uintptr(childFD),"event-staging-directory")
     walkErr:=walk(child,true,nextDepth);closeErr:=child.Close()
     if walkErr!=nil {return walkErr};if closeErr!=nil {return errEventStagingUnavailable}
    } else if staging && kind==unix.S_IFREG {
     if err:=addEventStagingSize(&usage,stat.Size);err!=nil {return err}
    } else {return errEventStagingUnavailable}
   }
   if readErr==io.EOF {return nil}
   if readErr!=nil {return errEventStagingUnavailable}
  }
 }
 err=walk(root,false,0)
 return usage,err
}

func checkEventStagingAdmission(parent context.Context,dir string) error {
 policy,err:=configuredEventStagingPolicy();if err!=nil {return err}
 if policy.bytes==0 && policy.dirs==0 {return nil}
 ctx,cancel:=context.WithTimeout(parent,5*time.Second);defer cancel()
 usage,err:=scanEventStaging(ctx,dir,eventStagingScanEntries,eventStagingScanDepth)
 if err!=nil {return errEventStagingUnavailable}
 if (policy.bytes>0 && usage.bytes>=policy.bytes) || (policy.dirs>0 && usage.dirs>=policy.dirs) {return errEventStagingLimit}
 return nil
}
