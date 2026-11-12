package main

import (
 "fmt"
 "log"
 "os"
 "path/filepath"
)

// A retained directory may be reused, but a stable symlink or special entry
// must not redirect a new event recorder. Parent paths remain operator-trusted.
func ensureEventStorageDirectory(dir string) error {
 if err:=os.Mkdir(dir,0700);err!=nil && !os.IsExist(err) {return err}
 info,err:=os.Lstat(dir)
 if err!=nil {return err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return fmt.Errorf("event storage is not a real directory")}
 return nil
}

// Presence is not evidence of corruption, completion, or permission to delete.
// Lstat does not follow stable links; no recursive inspection or media probe
// is needed to preserve an entry. Inspection failures are reported explicitly.
func logPreservedEventStorage(cam *Camera) {
 for _,name:=range []string{"buffer","temp"} {
  _,err:=os.Lstat(filepath.Join(cam.RecordDir,name))
  if os.IsNotExist(err) {continue}
  if err!=nil {
   log.Printf("Unable to inspect %s staging for camera %s; left unchanged",name,cam.ID)
   continue
  }
  log.Printf("Preserved %s staging for camera %s; manual inspection/recovery required",name,cam.ID)
 }
}
