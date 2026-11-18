package main

import (
 "bytes"
 "os"
 "path/filepath"
 "reflect"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func TestPersistenceRefusesNonRegularPrimaryWithoutBlocking(t *testing.T) {
 for _,kind:=range []string{"directory","fifo","symlink-fifo"} {
  for _,operation:=range []string{"load","save"} {
   t.Run(kind+"/"+operation,func(t *testing.T){
    dir:=t.TempDir();path:=filepath.Join(dir,"cameras.json");target:=path
    if kind=="symlink-fifo" {target=filepath.Join(dir,"fifo-target");if err:=os.Symlink(target,path);err!=nil {t.Fatal(err)}}
    if kind=="directory" {if err:=os.Mkdir(path,0700);err!=nil {t.Fatal(err)}} else if err:=unix.Mkfifo(target,0600);err!=nil {t.Fatal(err)}
    info,err:=os.Lstat(path);if err!=nil {t.Fatal(err)}
    backup:=[]byte("preserved backup evidence")
    if err:=os.WriteFile(path+".bak",backup,0600);err!=nil {t.Fatal(err)}
    n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
    before:=deletionSnapshot(n)
    done:=make(chan error,1)
    go func(){if operation=="load" {done<-n.loadAt(path)} else {done<-n.saveAt(path)}}()
    select {
    case err:=<-done:if err==nil {t.Fatal("nonregular primary accepted")}
    case <-time.After(time.Second):
     // Rescue a regression that opened the FIFO for a blocking read.
     if kind!="directory" {if fd,err:=unix.Open(target,unix.O_WRONLY|unix.O_NONBLOCK,0);err==nil {unix.Close(fd)}}
     select {case <-done:case <-time.After(time.Second):t.Error("persistence worker did not join")}
     t.Fatal("nonregular primary blocked persistence")
    }
    after,err:=os.Lstat(path);if err!=nil || !os.SameFile(info,after) || info.Mode()!=after.Mode() {t.Fatal("primary evidence changed",err)}
    if !reflect.DeepEqual(before,deletionSnapshot(n)) {t.Fatal("refusal changed live state")}
    raw,err:=os.ReadFile(path+".bak");if err!=nil || !bytes.Equal(raw,backup) {t.Fatal("backup changed",err)}
    if kind=="symlink-fifo" {got,err:=os.Readlink(path);if err!=nil || got!=target {t.Fatal("symlink changed",err)}}
    if kind!="directory" {got,err:=os.Lstat(target);if err!=nil || got.Mode()&os.ModeNamedPipe==0 {t.Fatal("FIFO target changed",err)}}
    leftovers,err:=filepath.Glob(filepath.Join(dir,".simplenvr-config-*"));if err!=nil || len(leftovers)!=0 {t.Fatal("temporary config leaked",leftovers,err)}
   })
  }
 }
}

func TestPersistencePrimaryReaderPreservesRegularFileReads(t *testing.T) {
 for _,symlink:=range []bool{false,true} {
  name:="regular";if symlink {name="symlink-to-regular"}
  t.Run(name,func(t *testing.T){
   dir:=t.TempDir();target:=filepath.Join(dir,"target.json");path:=target
   want:=[]byte("{\"cameras\":{},\"camera_order\":[],\"events\":[]}\n")
   if err:=os.WriteFile(target,want,0600);err!=nil {t.Fatal(err)}
   if symlink {path=filepath.Join(dir,"cameras.json");if err:=os.Symlink(target,path);err!=nil {t.Fatal(err)}}
   got,err:=readPersistencePrimary(path);if err!=nil || !bytes.Equal(got,want) {t.Fatal("regular bytes not preserved",err)}
   n:=&NVR{};if err:=n.loadAt(path);err!=nil {t.Fatal(err)}
   if n.cameras==nil {t.Fatal("regular primary did not load")}
  })
 }
}
