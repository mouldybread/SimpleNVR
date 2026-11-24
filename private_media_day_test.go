package main

import (
 "io"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

const privateDayFile="20261110_120000_E20261110_120008_fixture.mp4"

func privateDayFixture(t *testing.T) (string,*NVR,string) {
 t.Helper();root,n:=privateMediaFixture(t)
 day:=filepath.Join(root,"a","20261110")
 if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
 if err:=os.Rename(filepath.Join(root,"a","clip.mp4"),filepath.Join(day,privateDayFile));err!=nil {t.Fatal(err)}
 return root,n,"/recordings/a/20261110/"+privateDayFile
}

func TestPrivateDayMediaGetHeadAndRange(t *testing.T) {
 root,n,path:=privateDayFixture(t)
 for _,method:=range []string{"GET","HEAD"} {for _,ranged:=range []bool{false,true} {
  request:=httptest.NewRequest(method,path,nil)
  code,length,body:=200,"16","0123456789abcdef"
  if ranged {request.Header.Set("Range","bytes=2-5");code,length,body=206,"4","2345"}
  if method=="HEAD" {body=""}
  w:=httptest.NewRecorder();servePrivateRecordingAt(w,request,n,root)
  if w.Code!=code || w.Header().Get("Content-Length")!=length || w.Header().Get("Content-Type")!="video/mp4" || w.Body.String()!=body {t.Fatal(method,ranged,w.Code,w.Header(),w.Body.String())}
  if ranged && w.Header().Get("Content-Range")!="bytes 2-5/16" {t.Fatal(w.Header())}
  if w.Header().Get("Cache-Control")!="private, no-store" || w.Header().Get("X-Content-Type-Options")!="nosniff" {t.Fatal("private headers lost",w.Header())}
 }}
}

func TestPrivateDayMediaRejectsOtherDirectoriesAndTraversal(t *testing.T) {
 root,n,_:=privateDayFixture(t)
 for _,dir:=range []string{"buffer","temp","20260230","2026111","20261110/nested"} {
  target:=filepath.Join(root,"a",dir,privateDayFile)
  if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil {t.Fatal(err)}
  if err:=os.WriteFile(target,[]byte("must not serve"),0600);err!=nil {t.Fatal(err)}
  w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/"+dir+"/"+privateDayFile,nil),n,root)
  if w.Code!=404 {t.Fatal("non-day or nested directory exposed",dir,w.Code)}
 }
 for _,path:=range []string{
  "/recordings/a/20261110/../"+privateDayFile,
  "/recordings/a/20261110/..%2F"+privateDayFile,
  "/recordings/a/20261110%5cnested/"+privateDayFile,
  "/recordings/a/20261110/"+privateDayFile+".tmp",
  "/recordings/a/20261110/"+privateDayFile+"/",
  "/recordings/a//"+privateDayFile,
  "/recordings/missing/20261110/"+privateDayFile,
 } {
  w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET",path,nil),n,root)
  if w.Code!=404 {t.Fatal("unsafe day path exposed",path,w.Code)}
 }
 n.cameras["a"].RecordDir=t.TempDir()
 w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/20261110/"+privateDayFile,nil),n,root)
 if w.Code!=404 {t.Fatal("registered path mismatch accepted",w.Code)}
}

func TestPrivateDayMediaRejectsSymlinksAndSpecialFiles(t *testing.T) {
 root,n,_:=privateDayFixture(t);day:=filepath.Join(root,"a","20261110")
 if err:=os.Symlink("20261110",filepath.Join(root,"a","20261111"));err!=nil {t.Fatal(err)}
 w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/20261111/"+privateDayFile,nil),n,root)
 if w.Code!=404 {t.Fatal("day symlink followed",w.Code)}
 if err:=os.Symlink(privateDayFile,filepath.Join(day,"link.mp4"));err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(day,"directory.mp4"),0700);err!=nil {t.Fatal(err)}
 fifo:=filepath.Join(day,"fifo.mp4");if err:=unix.Mkfifo(fifo,0600);err!=nil {t.Fatal(err)}
 for _,name:=range []string{"link.mp4","directory.mp4","fifo.mp4"} {
  done:=make(chan int,1)
  go func(){w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/20261110/"+name,nil),n,root);done<-w.Code}()
  select {case code:=<-done:if code!=404 {t.Fatal("special day member served",name,code)};case <-time.After(time.Second):
   if fd,err:=unix.Open(fifo,unix.O_WRONLY|unix.O_NONBLOCK,0);err==nil {unix.Close(fd)}
   select {case <-done:case <-time.After(time.Second):t.Error("day special-entry worker did not join")}
   t.Fatal("day special-entry open blocked",name)
  }
 }
}

func TestPrivateDayMediaPinsOpenedInodeAcrossDirectoryReplacement(t *testing.T) {
 root,_,_:=privateDayFixture(t)
 file,err:=openPrivateMediaAt(root,"a","20261110",privateDayFile);if err!=nil {t.Fatal(err)};defer file.Close()
 day:=filepath.Join(root,"a","20261110")
 if err:=os.Rename(day,day+"-retained");err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(day,privateDayFile),[]byte("replacement"),0600);err!=nil {t.Fatal(err)}
 raw,err:=io.ReadAll(file)
 if err!=nil || string(raw)!="0123456789abcdef" {t.Fatal("descriptor switched to replacement day",string(raw),err)}
}
