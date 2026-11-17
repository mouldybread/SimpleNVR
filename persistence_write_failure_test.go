package main

import (
 "bytes"
 "errors"
 "io"
 "os"
 "path/filepath"
 "reflect"
 "testing"
)

func TestPersistenceAtomicWriteFailureBoundaries(t *testing.T) {
 for _,stage:=range []string{"create","write","short-write","file-sync","file-close","rename","directory-open","directory-sync","success"} {
  t.Run(stage,func(t *testing.T){
   dir:=t.TempDir();path:=filepath.Join(dir,"state.json")
   old,newBytes:=[]byte("old evidence"),[]byte("new evidence")
   if err:=os.WriteFile(path,old,0600);err!=nil {t.Fatal(err)}
   injected:=errors.New("injected "+stage)
   ops:=realPersistenceWriteOps()
   switch stage {
   case "create":ops.create=func(string,string)(*os.File,error){return nil,injected}
   case "write":ops.write=func(f *os.File,b []byte)(int,error){n,err:=f.Write(b[:1]);if err!=nil {return n,err};return n,injected}
   case "short-write":ops.write=func(f *os.File,b []byte)(int,error){return f.Write(b[:1])}
   case "file-sync":ops.fileSync=func(*os.File)error{return injected}
   case "file-close":ops.fileClose=func(f *os.File)error{if err:=f.Close();err!=nil {return err};return injected}
   case "rename":ops.rename=func(string,string)error{return injected}
   case "directory-open":ops.openDir=func(string)(*os.File,error){return nil,injected}
   case "directory-sync":ops.dirSync=func(*os.File)error{return injected}
   }
   err:=atomicWriteWithOps(path,newBytes,ops)
   if stage=="success" {if err!=nil {t.Fatal(err)}} else {
    want:=injected;if stage=="short-write" {want=io.ErrShortWrite}
    if !errors.Is(err,want) {t.Fatalf("failure lost: %v want %v",err,want)}
   }
   want:=old
   if stage=="directory-open" || stage=="directory-sync" || stage=="success" {want=newBytes}
   got,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(got,want) {t.Fatal("incorrect pre/post rename visibility",string(got),err)}
   info,err:=os.Stat(path);if err!=nil || info.Mode().Perm()!=0600 {t.Fatal("permissions changed",err)}
   leftovers,err:=filepath.Glob(filepath.Join(dir,".simplenvr-config-*"));if err!=nil || len(leftovers)!=0 {t.Fatal("temporary file leaked",leftovers,err)}
  })
 }
}

func TestPersistenceSaveBackupAndPrimaryFailureBoundaries(t *testing.T) {
 for _,stage:=range []string{"backup","primary-before-rename","primary-after-rename"} {
  t.Run(stage,func(t *testing.T){
   dir:=t.TempDir();path:=filepath.Join(dir,"cameras.json")
   old:=[]byte("{\"cameras\":{},\"camera_order\":[],\"events\":[]}\n")
   older:=[]byte("older retained backup")
   if err:=os.WriteFile(path,old,0600);err!=nil {t.Fatal(err)}
   if err:=os.WriteFile(path+".bak",older,0600);err!=nil {t.Fatal(err)}
   n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
   before:=deletionSnapshot(n);calls:=0;injected:=errors.New("injected save boundary")
   err:=n.saveAtWithWriter(path,func(target string,raw []byte)error{
    calls++
    wantTarget:=path+".bak";if calls==2 {wantTarget=path}
    if target!=wantTarget {t.Fatal("unexpected write order",target)}
    if stage=="backup" || (stage=="primary-before-rename" && calls==2) {return injected}
    if stage=="primary-after-rename" && calls==2 {
     ops:=realPersistenceWriteOps();ops.dirSync=func(*os.File)error{return injected}
     return atomicWriteWithOps(target,raw,ops)
    }
    return atomicWrite(target,raw)
   })
   if !errors.Is(err,injected) {t.Fatal("save failure lost",err)}
   wantCalls:=2;if stage=="backup" {wantCalls=1}
   if calls!=wantCalls {t.Fatal("primary attempted after backup failure",calls)}
   if !reflect.DeepEqual(before,deletionSnapshot(n)) {t.Fatal("save changed live state")}
   wantBackup:=old;if stage=="backup" {wantBackup=older}
   got,err:=os.ReadFile(path+".bak");if err!=nil || !bytes.Equal(got,wantBackup) {t.Fatal("backup outcome incorrect",err)}
   got,err=os.ReadFile(path);if err!=nil {t.Fatal(err)}
   if stage=="primary-after-rename" {
    if bytes.Equal(got,old) {t.Fatal("post-rename failure falsely rolled back primary")}
    if _,err:=decodeState(got);err!=nil {t.Fatal("visible replacement invalid",err)}
   } else if !bytes.Equal(got,old) {t.Fatal("primary changed before authorized replacement")}
   leftovers,err:=filepath.Glob(filepath.Join(dir,".simplenvr-config-*"));if err!=nil || len(leftovers)!=0 {t.Fatal("temporary file leaked",leftovers,err)}
  })
 }
}
