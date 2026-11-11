package main

import (
 "errors"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "unicode"
 "unicode/utf8"

 "golang.org/x/sys/unix"
)

func privateMediaComponent(value string) bool {
 if value=="" || value=="." || value==".." || len(value)>255 || !utf8.ValidString(value) || strings.ContainsAny(value,"/\\") {return false}
 for _,r:=range value {if unicode.IsControl(r) {return false}}
 return true
}

// O_PATH observes type without opening a FIFO/device for data. The verified
// regular inode is reopened through procfs, not a potentially replaced name.
func openPrivateMediaAt(root,camera,name string) (*os.File,error) {
 if !privateMediaComponent(camera) || !privateMediaComponent(name) || filepath.Ext(name)!=".mp4" {return nil,errors.New("invalid private media path")}
 flags:=unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC
 rootFD,err:=unix.Open(root,flags,0);if err!=nil {return nil,err};defer unix.Close(rootFD)
 cameraFD,err:=unix.Openat(rootFD,camera,flags,0);if err!=nil {return nil,err};defer unix.Close(cameraFD)
 identityFD,err:=unix.Openat(cameraFD,name,unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC,0);if err!=nil {return nil,err};defer unix.Close(identityFD)
 var before unix.Stat_t
 if err:=unix.Fstat(identityFD,&before);err!=nil {return nil,err}
 if before.Mode&unix.S_IFMT!=unix.S_IFREG {return nil,errors.New("private media is not regular")}
 fd,err:=unix.Open(fmt.Sprintf("/proc/self/fd/%d",identityFD),unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC,0)
 if err!=nil {return nil,err}
 var after unix.Stat_t
 if err:=unix.Fstat(fd,&after);err!=nil {unix.Close(fd);return nil,err}
 if after.Mode&unix.S_IFMT!=unix.S_IFREG || before.Dev!=after.Dev || before.Ino!=after.Ino {unix.Close(fd);return nil,errors.New("private media identity changed")}
 return os.NewFile(uintptr(fd),name),nil
}

// Per-call state/root parameters keep tests entirely outside /recordings.
func servePrivateRecordingAt(w http.ResponseWriter,r *http.Request,n *NVR,root string) {
 w.Header().Set("Cache-Control","private, no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {w.Header().Set("Allow","GET, HEAD");http.Error(w,"Method not allowed",405);return}
 if r.Context().Err()!=nil {return}
 if !strings.HasPrefix(r.URL.Path,"/recordings/") {http.NotFound(w,r);return}
 parts:=strings.Split(strings.TrimPrefix(r.URL.Path,"/recordings/"),"/")
 if len(parts)!=2 || !privateMediaComponent(parts[0]) || !privateMediaComponent(parts[1]) || filepath.Ext(parts[1])!=".mp4" {http.NotFound(w,r);return}
 n.mu.RLock()
 cam:=n.cameras[parts[0]]
 valid:=cam!=nil && cam.ID==parts[0] && cam.RecordDir==filepath.Join(root,parts[0])
 n.mu.RUnlock()
 if !valid {http.NotFound(w,r);return}
 file,err:=openPrivateMediaAt(root,parts[0],parts[1]);if err!=nil {http.NotFound(w,r);return};defer file.Close()
 info,err:=file.Stat();if err!=nil || !info.Mode().IsRegular() {http.NotFound(w,r);return}
 if r.Context().Err()!=nil {return}
 w.Header().Set("Content-Type","video/mp4")
 http.ServeContent(w,r,info.Name(),info.ModTime(),file)
}
