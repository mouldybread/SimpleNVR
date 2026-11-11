package main

import (
 "context"
 "encoding/json"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "strings"
)

type orphanCleanupResult struct {
 Count int `json:"count"`
 Bytes int64 `json:"bytes"`
 SkippedRecoveryDirs int `json:"skipped_recovery_dirs"`
 Failures int `json:"failures"`
 Interrupted bool `json:"interrupted"`
}

// Absence is not ownership proof. Preserve even empty root .tmp entries,
// opaque event/prebuffer contents and unexpected staging types.
func recoveryArtifactsPresent(dir string) (bool,error) {
 info,err:=os.Lstat(dir)
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 entries,err:=os.ReadDir(dir)
 if err!=nil {return true,err}
 for _,entry:=range entries {if strings.HasSuffix(entry.Name(),".tmp") {return true,nil}}
 for _,name:=range []string{"temp","buffer"} {
  retained,err:=opaqueStagingPresent(filepath.Join(dir,name))
  if retained || err!=nil {return true,err}
 }
 return false,nil
}

func opaqueStagingPresent(path string) (bool,error) {
 info,err:=os.Lstat(path)
 if os.IsNotExist(err) {return false,nil}
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 f,err:=os.Open(path)
 if err!=nil {return true,err}
 defer f.Close()
 names,err:=f.Readdirnames(1)
 if len(names)>0 {return true,nil}
 if err==io.EOF {return false,nil}
 return true,err
}

// Per-call seams exercise failures without global hooks or permission guesses.
// They do not make pathname operations atomic or kernel I/O interruptible.
type orphanCleanupIO struct {
 readDir func(string)([]os.DirEntry,error)
 inspect func(string)(bool,error)
 remove func(string) error
 interrupted func() bool
}
func orphanCleanupIOFor(ctx context.Context) orphanCleanupIO {
 return orphanCleanupIO{os.ReadDir,recoveryArtifactsPresent,os.Remove,func() bool{return ctx.Err()!=nil}}
}
func cleanupOrphanedAt(root string,validDirs map[string]bool) (orphanCleanupResult,error) {
 return cleanupOrphanedWithIO(root,validDirs,orphanCleanupIOFor(context.Background()))
}
func cleanupOrphanedWithIO(root string,validDirs map[string]bool,ops orphanCleanupIO) (orphanCleanupResult,error) {
 var result orphanCleanupResult
 stop:=func() bool {if !ops.interrupted() {return false};result.Interrupted=true;result.Failures++;return true}
 if stop() {return result,nil}
 // A stable symlink root is not an accepted cleanup destination.
 info,err:=os.Lstat(root)
 if err!=nil {return result,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return result,os.ErrInvalid}
 if stop() {return result,nil}
 entries,err:=ops.readDir(root)
 if err!=nil {return result,err}
 for _,entry:=range entries {
  if stop() {return result,nil}
  // Hidden application/private directories are not orphan camera media.
  if !entry.IsDir() || strings.HasPrefix(entry.Name(),".") || validDirs[entry.Name()] {continue}
  dir:=filepath.Join(root,entry.Name())
  preserve:=func() bool {
   retained,err:=ops.inspect(dir)
   if retained || err!=nil {result.SkippedRecoveryDirs++;if err!=nil {result.Failures++};return true}
   return false
  }
  if preserve() {continue}
  if stop() {return result,nil}
  files,err:=ops.readDir(dir)
  if err!=nil {result.Failures++;continue}
  retained:=false
  for _,entry:=range files {
   if stop() {return result,nil}
   if filepath.Ext(entry.Name())!=".mp4" {continue}
   file:=filepath.Join(dir,entry.Name())
   info,err:=os.Lstat(file)
   if err!=nil {if !os.IsNotExist(err) {result.Failures++};continue}
   if !info.Mode().IsRegular() {continue}
   // Observe newly arrived recovery evidence before each removal, not just
   // once per directory. A replacement can still race this last observation.
   if preserve() {retained=true;break}
   if stop() {return result,nil}
   if err:=ops.remove(file);err!=nil {if !os.IsNotExist(err) {result.Failures++};continue}
   result.Count++;result.Bytes+=info.Size()
  }
  if retained {continue}
  if stop() {return result,nil}
  if preserve() {continue}
  if stop() {return result,nil}
  remaining,err:=ops.readDir(dir)
  if err!=nil {result.Failures++;continue}
  if len(remaining)!=0 {continue}
  if stop() {return result,nil}
  // Only an observed-empty directory; never recurse through opaque inputs.
  if err:=ops.remove(dir);err!=nil && !os.IsNotExist(err) {result.Failures++}
 }
 return result,nil
}

func cleanupOrphanedRecordingsHandler(w http.ResponseWriter,r *http.Request) {
 serveOrphanCleanup(w,r,nvr,"/recordings",orphanCleanupIOFor(r.Context()))
}
func serveOrphanCleanup(w http.ResponseWriter,r *http.Request,n *NVR,root string,ops orphanCleanupIO) {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 if n.shuttingDown.Load() {recordingError(w,errRecorderShuttingDown);return}
 n.mu.RLock()
 validDirs:=make(map[string]bool,len(n.cameras))
 for _,cam:=range n.cameras {if cam!=nil {validDirs[filepath.Base(cam.RecordDir)]=true}}
 n.mu.RUnlock()
 interrupted:=ops.interrupted
 ops.interrupted=func() bool {return r.Context().Err()!=nil || n.shuttingDown.Load() || interrupted()}
 result,err:=cleanupOrphanedWithIO(root,validDirs,ops)
 if err!=nil {http.Error(w,"Unable to inspect recordings root",500);return}
 w.Header().Set("Content-Type","application/json")
 json.NewEncoder(w).Encode(map[string]interface{}{
  "success":result.Failures==0,"count":result.Count,"size_mb":float64(result.Bytes)/(1024*1024),
  "skipped_recovery_dirs":result.SkippedRecoveryDirs,"failures":result.Failures,"interrupted":result.Interrupted,
 })
}
