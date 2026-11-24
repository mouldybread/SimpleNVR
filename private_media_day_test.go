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

func assertPrivateDayMediaGetHeadAndRange(t *testing.T,root string,n *NVR,path string) {
 t.Helper()
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

func TestPrivateDayMediaGetHeadAndRange(t *testing.T) {
 root,n,path:=privateDayFixture(t)
 assertPrivateDayMediaGetHeadAndRange(t,root,n,path)
}

func TestPrivateDayMediaCrossMidnightAndPublishedPartial(t *testing.T) {
 for _,name:=range []string{
  "20261110_235958_E20261111_000006_fixture.mp4",
  "20261110_120000_E20261110_120008_fixture_partial.mp4",
  "20261110_235958_E20261111_000006_fixture_partial.mp4",
 } {t.Run(name,func(t *testing.T){
  root,n,_:=privateDayFixture(t)
  if err:=os.WriteFile(filepath.Join(root,"a","20261110",name),[]byte("0123456789abcdef"),0600);err!=nil {t.Fatal(err)}
  assertPrivateDayMediaGetHeadAndRange(t,root,n,"/recordings/a/20261110/"+name)
 })}
}

func TestPrivateDayMediaRejectsRealFilesInWrongDay(t *testing.T) {
 for _,tc:=range []struct{day,name string}{
  {"20261111",privateDayFile},
  {"20261111","20261110_235958_E20261111_000006_fixture.mp4"},
  {"20261111","20261110_235958_E20261111_000006_fixture_partial.mp4"},
 } {t.Run(tc.name,func(t *testing.T){
  root,n,_:=privateDayFixture(t)
  stamp,err:=parseStampedName(tc.name)
  if err!=nil || stamp.inDay(tc.day) {t.Fatal("wrong-day fixture must have a valid stamp and mismatching day",err)}
  day:=filepath.Join(root,"a",tc.day)
  if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
  target:=filepath.Join(day,tc.name)
  if err:=os.WriteFile(target,[]byte("must not serve"),0600);err!=nil {t.Fatal(err)}
  if info,err:=os.Stat(target);err!=nil || !info.Mode().IsRegular() {t.Fatal("wrong-day fixture is not a real regular file",err)}
  for _,method:=range []string{"GET","HEAD"} {for _,ranged:=range []bool{false,true} {
   request:=httptest.NewRequest(method,"/recordings/a/"+tc.day+"/"+tc.name,nil)
   if ranged {request.Header.Set("Range","bytes=2-5")}
   w:=httptest.NewRecorder();servePrivateRecordingAt(w,request,n,root)
   if w.Code!=404 {t.Fatal("wrong-day real file served",method,ranged,w.Code)}
   if w.Header().Get("Cache-Control")!="private, no-store" || w.Header().Get("X-Content-Type-Options")!="nosniff" {t.Fatal("private error headers lost",w.Header())}
  }}
 })}
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
 // The requested day and name must validate so rejection exercises O_NOFOLLOW.
 symlinkName:="20261111_120000_E20261111_120008_fixture.mp4"
 if !privateMediaParts([]string{"20261111",symlinkName}) {t.Fatal("day symlink fixture fails path validation")}
 if err:=os.WriteFile(filepath.Join(day,symlinkName),[]byte("must not serve"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Symlink("20261110",filepath.Join(root,"a","20261111"));err!=nil {t.Fatal(err)}
 w:=httptest.NewRecorder();servePrivateRecordingAt(w,httptest.NewRequest("GET","/recordings/a/20261111/"+symlinkName,nil),n,root)
 if w.Code!=404 {t.Fatal("day symlink followed",w.Code)}
 linkName:="20261110_120000_E20261110_120008_link.mp4"
 directoryName:="20261110_120000_E20261110_120008_directory.mp4"
 fifoName:="20261110_120000_E20261110_120008_fifo.mp4"
 if err:=os.Symlink(privateDayFile,filepath.Join(day,linkName));err!=nil {t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(day,directoryName),0700);err!=nil {t.Fatal(err)}
 fifo:=filepath.Join(day,fifoName);if err:=unix.Mkfifo(fifo,0600);err!=nil {t.Fatal(err)}
 for _,name:=range []string{linkName,directoryName,fifoName} {
  if !privateMediaParts([]string{"20261110",name}) {t.Fatal("special-file fixture fails path validation",name)}
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
