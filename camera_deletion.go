package main

import (
 "context"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "sort"

 "golang.org/x/sys/unix"
)

const cameraDeleteEntryLimit=8192
var errCameraDeletePersistence=errors.New("camera deletion state could not be persisted")
var errCameraDeleteBlocked=errors.New("camera deletion refused: retained evidence, unexpected entries, or changed directory; camera remains registered and stopped after a stop has completed")

type cameraDeletionDeps struct {
 validate func(string,string) error
 remove func(context.Context,string,func() error) error
 persist func() error
}
func cameraDeletionDependencies(n *NVR) cameraDeletionDeps {
 return cameraDeletionDeps{validate:validateCameraPath,remove:removeCameraDirectory,persist:n.save}
}
func cameraDeletionError(w http.ResponseWriter,err error) {
 if errors.Is(err,errCameraDeletePersistence) {http.Error(w,err.Error(),500);return}
 if errors.Is(err,errRecorderShuttingDown) {recordingError(w,errRecorderShuttingDown);return}
 if errors.Is(err,errCameraDeleteBlocked) {http.Error(w,err.Error(),http.StatusConflict);return}
 recordingError(w,err)
}

// Caller serialization covers stop/join, filesystem work and both persistence
// boundaries. NVR.mu is never held across worker waits or filesystem I/O.
// A nil selection means the current registry, in deterministic ID order.
func deleteManagedCameras(ctx context.Context,n *NVR,selection []string,deps cameraDeletionDeps) error {
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 if err:=ctx.Err();err!=nil {return err}
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 if deps.validate==nil || deps.remove==nil || deps.persist==nil {return errors.New("camera deletion dependencies unavailable")}
 type target struct {id,dir string;cam *Camera}
 n.mu.RLock()
 ids:=append([]string(nil),selection...)
 if selection==nil {for id:=range n.cameras {ids=append(ids,id)};sort.Strings(ids)}
 targets:=make([]target,0,len(ids))
 for _,id:=range ids {
  cam:=n.cameras[id]
  if cam==nil {n.mu.RUnlock();return errCameraMissing}
  targets=append(targets,target{id,cam.RecordDir,cam})
 }
 n.mu.RUnlock()
 var result error
 for _,item:=range targets {
  guard:=func() error {
   if err:=ctx.Err();err!=nil {return err}
   if n.shuttingDown.Load() {return errRecorderShuttingDown}
   n.mu.RLock();same:=n.cameras[item.id]==item.cam && item.cam.ID==item.id && item.cam.RecordDir==item.dir;n.mu.RUnlock()
   if !same {return errRecordingStateChanged}
   return nil
  }
  if err:=guard();err!=nil {return errors.Join(result,err)}
  if err:=deps.validate(item.id,item.dir);err!=nil {result=errors.Join(result,err);continue}
  if err:=guard();err!=nil {return errors.Join(result,err)}
  stopManagedCamera(n,item.cam)
  // Even cancellation/shutdown during join must not skip persistence of this
  // explicit API stop. A failed save prohibits the first destructive unlink.
  if err:=deps.persist();err!=nil {return errors.Join(result,fmt.Errorf("%w: camera stopped; no file deletion attempted for this camera: %w",errCameraDeletePersistence,err))}
  if err:=guard();err!=nil {return errors.Join(result,err)}
  if err:=deps.remove(ctx,item.dir,guard);err!=nil {result=errors.Join(result,err);continue}
  if err:=guard();err!=nil {return errors.Join(result,err)}
  n.mu.Lock()
  if n.cameras[item.id]!=item.cam || item.cam.ID!=item.id || item.cam.RecordDir!=item.dir {n.mu.Unlock();return errors.Join(result,errRecordingStateChanged)}
  delete(n.cameras,item.id)
  order:=make([]string,0,len(n.cameraOrder))
  for _,id:=range n.cameraOrder {if id!=item.id {order=append(order,id)}}
  n.cameraOrder=order
  n.mu.Unlock()
  // Persist each completed camera before proceeding to another. No rollback
  // can restore already-unlinked recordings on a final-save failure.
  if err:=deps.persist();err!=nil {return errors.Join(result,fmt.Errorf("%w: camera removed in memory but final save failed: %w",errCameraDeletePersistence,err))}
 }
 return result
}

type cameraDeleteEntry struct {name string;stat unix.Stat_t;directory bool}
func sameCameraDeleteEntry(a,b *unix.Stat_t) bool {
 return a.Dev==b.Dev && a.Ino==b.Ino && a.Mode==b.Mode && a.Size==b.Size && a.Mtim==b.Mtim && a.Ctim==b.Ctim
}
func openCameraDeleteDirectory(parent int,name string) (*os.File,error) {
 fd,err:=unix.Openat(parent,name,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return nil,err}
 return os.NewFile(uintptr(fd),"camera-delete-directory"),nil
}

// Only direct regular .mp4 files and EMPTY buffer/temp directories are eligible.
// No recursive traversal/removal and no symlink following. Inspection is bounded
// and completes before the first unlink; late changes may still cause partial
// deletion, but nonempty directories cannot be recursively destroyed.
func removeCameraDirectory(ctx context.Context,dir string,guard func() error) error {
 return removeCameraDirectoryWithUnlink(ctx,dir,guard,unix.Unlinkat)
}
func removeCameraDirectoryWithUnlink(ctx context.Context,dir string,guard func() error,unlink func(int,string,int) error) error {
 check:=func() error {if err:=ctx.Err();err!=nil {return err};if guard==nil || unlink==nil {return errCameraDeleteBlocked};return guard()}
 if err:=check();err!=nil {return err}
 parent,err:=openCameraDeleteDirectory(unix.AT_FDCWD,filepath.Dir(dir));if err!=nil {return err}
 defer parent.Close()
 name:=filepath.Base(dir)
 root,err:=openCameraDeleteDirectory(int(parent.Fd()),name)
 // A missing directory is not inferred to be empty: it can be a lost mount.
 if err!=nil {return fmt.Errorf("camera directory unavailable; registration retained: %w",err)}
 defer root.Close()
 var identity unix.Stat_t
 if err:=unix.Fstat(int(root.Fd()),&identity);err!=nil {return err}
 rootCheck:=func() error {
  if err:=check();err!=nil {return err}
  var current unix.Stat_t
  if err:=unix.Fstatat(int(parent.Fd()),name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if current.Dev!=identity.Dev || current.Ino!=identity.Ino || current.Mode&unix.S_IFMT!=unix.S_IFDIR {return errCameraDeleteBlocked}
  return nil
 }
 var plan []cameraDeleteEntry
 for {
  if err:=rootCheck();err!=nil {return err}
  names,readErr:=root.Readdirnames(64)
  for _,entry:=range names {
   if len(plan)>=cameraDeleteEntryLimit {return errCameraDeleteBlocked}
   if err:=rootCheck();err!=nil {return err}
   next:=cameraDeleteEntry{name:entry}
   if err:=unix.Fstatat(int(root.Fd()),entry,&next.stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
   switch next.stat.Mode&unix.S_IFMT {
   case unix.S_IFREG:
    if filepath.Ext(entry)!=".mp4" {return errCameraDeleteBlocked}
   case unix.S_IFDIR:
    if entry!="buffer" && entry!="temp" {return errCameraDeleteBlocked}
    child,err:=openCameraDeleteDirectory(int(root.Fd()),entry);if err!=nil {return err}
    var opened unix.Stat_t
    statErr:=unix.Fstat(int(child.Fd()),&opened)
    children,childErr:=child.Readdirnames(1);closeErr:=child.Close()
    if statErr!=nil {return statErr}
    if !sameCameraDeleteEntry(&next.stat,&opened) || len(children)!=0 {return errCameraDeleteBlocked}
    if childErr!=io.EOF {if childErr!=nil {return childErr};return errCameraDeleteBlocked}
    if closeErr!=nil {return closeErr}
    next.directory=true
   default:return errCameraDeleteBlocked
   }
   plan=append(plan,next)
  }
  if readErr==io.EOF {break};if readErr!=nil {return readErr}
 }
 // Revalidate the whole plan before unlinking any entry, then each entry again
 // immediately before its unlink. Trusted directory ownership is still required:
 // stat/unlink is not an atomic compare-and-unlink operation.
 validate:=func(entry cameraDeleteEntry) error {
  if err:=rootCheck();err!=nil {return err}
  var current unix.Stat_t
  if err:=unix.Fstatat(int(root.Fd()),entry.name,&current,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
  if !sameCameraDeleteEntry(&entry.stat,&current) {return errCameraDeleteBlocked}
  return nil
 }
 for _,entry:=range plan {if err:=validate(entry);err!=nil {return err}}
 for _,entry:=range plan {
  if err:=validate(entry);err!=nil {return err}
  flags:=0;if entry.directory {flags=unix.AT_REMOVEDIR}
  if err:=unlink(int(root.Fd()),entry.name,flags);err!=nil {return err}
 }
 if err:=root.Sync();err!=nil {return err}
 if err:=rootCheck();err!=nil {return err}
 // Atomic rmdir refuses late entries. Never replace this with RemoveAll.
 if err:=unlink(int(parent.Fd()),name,unix.AT_REMOVEDIR);err!=nil {return err}
 return parent.Sync()
}
