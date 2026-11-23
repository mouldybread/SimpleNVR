package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "time"
)

// Recovery owns only direct timestamp-prefixed .mp4.tmp files in this camera.
// Unrecognised names, non-regular files and spans over 24h remain untouched.
func recoverStampedTemporary(ctx context.Context,cameraDir,path string,validate func(context.Context,string) error) (string,error) {
 if !filepath.IsAbs(cameraDir) || filepath.Clean(cameraDir)!=cameraDir || filepath.Dir(path)!=cameraDir || filepath.Clean(path)!=path {return "",errStampedName}
 name:=filepath.Base(path)
 if !strings.HasSuffix(name,".mp4.tmp") || len(name)<25 || name[15]!='_' {return "",errStampedName}
 start,err:=time.ParseInLocation(stampTimeLayout,name[:15],time.UTC);if err!=nil {return "",errStampedName}
 label:=strings.TrimSuffix(name[16:],".mp4.tmp")
 if stamp,err:=parseStampedName(strings.TrimSuffix(name,".tmp"));err==nil {start=stamp.Start;label=strings.TrimSuffix(strings.TrimSuffix(name,".tmp")[33:],".mp4")}
 label=strings.TrimSuffix(label,"_partial")
 if !stampedLabelValid(label) {return "",errStampedName}
 lease,err:=recordingMediaOwnership.acquire(ctx);if err!=nil {return "",err}
 defer lease.release()
 info,err:=os.Lstat(path);if err!=nil {return "",err}
 if !info.Mode().IsRegular() || info.Size()==0 {return "",errRecordingMetadata}
 end:=info.ModTime().UTC()
 if !end.After(start) {end=start.Add(time.Second)}
 return publishStampedLeasedWith(ctx,lease,path,cameraDir,start,end,label,true,validate)
}

func recoverStampedCameraTemps(ctx context.Context,cameraDir string,validate func(context.Context,string) error) error {
 files,err:=filepath.Glob(filepath.Join(cameraDir,"*.tmp"));if err!=nil {return err}
 var failures error
 for _,path:=range files {
  if err:=ctx.Err();err!=nil {return errors.Join(failures,err)}
  fileCtx,cancel:=context.WithTimeout(ctx,recordingFinalizeGrace)
  _,err:=recoverStampedTemporary(fileCtx,cameraDir,path,validate);cancel()
  if err!=nil {failures=errors.Join(failures,err)}
 }
 return failures
}
