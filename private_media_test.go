package main

import (
 "context"
 "io"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func privateMediaFixture(t *testing.T) (string,*NVR) {
 t.Helper();root:=t.TempDir();dir:=filepath.Join(root,"a")
 if err:=os.Mkdir(dir,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"clip.mp4"),[]byte("0123456789abcdef"),0600);err!=nil {t.Fatal(err)}
 return root,&NVR{cameras:map[string]*Camera{"a":{ID:"a",RecordDir:dir}}}
}

func TestPrivateMediaRangeHeadAndPathRestrictions(t *testing.T) {
 root,n:=privateMediaFixture(t)
 for _,method:=range []string{"GET","HEAD"} {
  request:=httptest.NewRequest(method,"/recordings/a/clip.mp4",nil);request.Header.Set("Range","bytes=2-5")
  w:=httptest.NewRecorder();servePrivateRecordingAt(w,request,n,root)
  if w.Code!=206 || w.Header().Get("Content-Range")!="bytes 2-5/16" || w.Header().Get("Content-Length")!="4" || w.Header().Get("Content-Type")!="video/mp4" || w.Header().Get("X-Content-Type-Options")!="nosniff" || w.Header().Get("Cache-Control")!="private, no-store" {t.Fatal("private range response changed",method,w.Code,w.Header())}
  want:="2345";if method=="HEAD" {want=""};if w.Body.String()!=want {t.Fatal("range/HEAD body incorrect",method,w.Body.String())}
 }
 for _,path:=range []string{"/recordings/cameras.json","/recordings/.auth/credentials.json","/recordings/a/clip.mp4.tmp","/recordings/a/temp/event-a/event.mp4","/recordings/a/buffer/clip.mp4","/recordings/missing/clip.mp4","/recordings/../clip.mp4","/recordings/a/..%2Fclip.mp4","/recordings/a/clip%5cmp4","/recordings/a/clip%00.mp4","/recordings/a/clip.mp4/","/other/a/clip.mp4"} {
  w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET",path,nil),n,root)
  if w.Code!=404 {t.Fatal("unsafe media route accepted",path,w.Code)}
 }
 w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("POST","/recordings/a/clip.mp4",nil),n,root)
 if w.Code!=405 || w.Header().Get("Allow")!="GET, HEAD" {t.Fatal("media method restriction missing")}
 n.cameras["a"].RecordDir=t.TempDir()
 w=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/clip.mp4",nil),n,root)
 if w.Code!=404 {t.Fatal("registered path mismatch accepted")}
}

func TestPrivateMediaRejectsSpecialEntriesAndSymlinkDirectories(t *testing.T) {
 root,n:=privateMediaFixture(t);dir:=filepath.Join(root,"a")
 if err:=os.Symlink("clip.mp4",filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(dir,"directory.mp4"),0700);err!=nil {t.Fatal(err)}
 fifo:=filepath.Join(dir,"fifo.mp4");if err:=unix.Mkfifo(fifo,0600);err!=nil {t.Fatal(err)}
 for _,name:=range []string{"link.mp4","directory.mp4","fifo.mp4"} {
  done:=make(chan int,1)
  go func(){w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/"+name,nil),n,root);done<-w.Code}()
  select {case code:=<-done:if code!=404 {t.Fatal("special entry accepted",name,code)};case <-time.After(time.Second):
   // Rescue a regressed blocking FIFO open before reporting failure.
   if fd,err:=unix.Open(fifo,unix.O_WRONLY|unix.O_NONBLOCK,0);err==nil {unix.Close(fd)}
   select {case <-done:case <-time.After(time.Second):t.Error("special-entry test worker did not join")}
   t.Fatal("special media entry blocked",name)
  }
 }
 if err:=os.Rename(dir,filepath.Join(root,"saved-a"));err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(root,"b"),0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"b","clip.mp4"),[]byte("other camera bytes"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Symlink("b",dir);err!=nil {t.Fatal(err)}
 if f,err:=openPrivateMediaAt(root,"a","clip.mp4");err==nil {f.Close();t.Fatal("camera symlink followed")}
 alias:=filepath.Join(t.TempDir(),"root-link");if err:=os.Symlink(root,alias);err!=nil {t.Fatal(err)}
 if f,err:=openPrivateMediaAt(alias,"b","clip.mp4");err==nil {f.Close();t.Fatal("root symlink followed")}
}

func TestPrivateMediaKeepsOpenedInodeAcrossNameReplacement(t *testing.T) {
 root,_:=privateMediaFixture(t)
 file,err:=openPrivateMediaAt(root,"a","clip.mp4");if err!=nil {t.Fatal(err)};defer file.Close()
 path:=filepath.Join(root,"a","clip.mp4")
 if err:=os.Rename(path,path+".retained");err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("replacement"),0600);err!=nil {t.Fatal(err)}
 data,err:=io.ReadAll(file);if err!=nil || string(data)!="0123456789abcdef" {t.Fatal("descriptor switched to replacement",string(data),err)}
}

func TestPrivateMediaRequestsAreCanceledAndJoinedByShutdownGate(t *testing.T) {
 entered,canceled,release,returned:=make(chan struct{}),make(chan struct{}),make(chan struct{}),make(chan struct{})
 defer func(){select {case <-release:default:close(release)}}()
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 t.Cleanup(func(){select {case <-returned:case <-time.After(3*time.Second):t.Error("media gate test worker did not join")}})
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-r.Context().Done();close(canceled);<-release}))
 go func(){defer close(returned);gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/recordings/a/clip.mp4",nil).WithContext(ctx))}()
 awaitShutdownStep(t,entered,"media request not admitted")
 drained:=gate.beginShutdown()
 awaitShutdownStep(t,canceled,"media request context not canceled")
 select {case <-drained:t.Fatal("media request not joined before drain");default:}
 close(release);awaitShutdownStep(t,returned,"media request did not return");awaitShutdownStep(t,drained,"media gate did not drain")
}
