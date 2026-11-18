package main

import (
 "bytes"
 "os"
 "path/filepath"
 "reflect"
 "testing"
)

func TestSaveMissingPrimaryPreservesRecoveryEntries(t *testing.T) {
 for _,kind:=range []string{"dangling-primary","valid-backup","invalid-backup","directory-backup","dangling-backup","symlink-backup"} {
  t.Run(kind,func(t *testing.T){
   dir:=t.TempDir();path:=filepath.Join(dir,"cameras.json");entry:=path+".bak"
   if kind=="dangling-primary" {entry=path}
   target:=filepath.Join(dir,"target.json")
   evidence:=[]byte("retained invalid recovery evidence")
   switch kind {
   case "dangling-primary","dangling-backup":
    if err:=os.Symlink(target,entry);err!=nil {t.Fatal(err)}
   case "valid-backup":
    evidence=[]byte("{\"cameras\":{},\"camera_order\":[],\"events\":[]}\n")
    if _,err:=decodeState(evidence);err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(entry,evidence,0600);err!=nil {t.Fatal(err)}
   case "invalid-backup":
    if err:=os.WriteFile(entry,evidence,0600);err!=nil {t.Fatal(err)}
   case "directory-backup":
    if err:=os.Mkdir(entry,0700);err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(filepath.Join(entry,"keep"),evidence,0600);err!=nil {t.Fatal(err)}
   case "symlink-backup":
    if err:=os.WriteFile(target,evidence,0600);err!=nil {t.Fatal(err)}
    if err:=os.Symlink(target,entry);err!=nil {t.Fatal(err)}
   }
   before,err:=os.Lstat(entry);if err!=nil {t.Fatal(err)}
   n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
   snapshot:=deletionSnapshot(n)
   if err:=n.saveAt(path);err==nil {t.Fatal("save replaced missing-config recovery situation")}
   if !reflect.DeepEqual(snapshot,deletionSnapshot(n)) {t.Fatal("refused save changed live state")}
   after,err:=os.Lstat(entry);if err!=nil {t.Fatal(err)}
   if !os.SameFile(before,after) || before.Mode()!=after.Mode() {t.Fatal("recovery entry replaced")}
   switch kind {
   case "dangling-primary","dangling-backup","symlink-backup":
    got,err:=os.Readlink(entry);if err!=nil || got!=target {t.Fatalf("link changed: %q %v",got,err)}
    if kind=="symlink-backup" {raw,err:=os.ReadFile(target);if err!=nil || !bytes.Equal(raw,evidence) {t.Fatal("target bytes changed",err)}} else if _,err:=os.Lstat(target);!os.IsNotExist(err) {t.Fatal("dangling target created",err)}
   case "valid-backup","invalid-backup":
    raw,err:=os.ReadFile(entry);if err!=nil || !bytes.Equal(raw,evidence) {t.Fatal("backup bytes changed",err)}
   case "directory-backup":
    raw,err:=os.ReadFile(filepath.Join(entry,"keep"));if err!=nil || !bytes.Equal(raw,evidence) {t.Fatal("directory evidence changed",err)}
   }
   if kind!="dangling-primary" {if _,err:=os.Lstat(path);!os.IsNotExist(err) {t.Fatal("new primary created",err)}} else if _,err:=os.Lstat(path+".bak");!os.IsNotExist(err) {t.Fatal("backup unexpectedly created",err)}
   leftovers,err:=filepath.Glob(filepath.Join(dir,".simplenvr-config-*"));if err!=nil || len(leftovers)!=0 {t.Fatal("temporary config created or leaked",leftovers,err)}
  })
 }
}

func TestSavePristineConfigurationStillSucceeds(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"cameras.json")
 n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
 if err:=n.saveAt(path);err!=nil {t.Fatal(err)}
 raw,err:=os.ReadFile(path);if err!=nil {t.Fatal(err)}
 if _,err:=decodeState(raw);err!=nil {t.Fatal(err)}
 if _,err:=os.Lstat(path+".bak");!os.IsNotExist(err) {t.Fatal("first save created backup",err)}
 info,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
 if info.Mode().Perm()!=0600 {t.Fatal("insecure primary permissions",info.Mode())}
}

func TestSaveReadablePrimaryStillRotatesExactPreviousBytes(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"cameras.json")
 old:=[]byte("{\"cameras\":{},\"camera_order\":[],\"events\":[]}\n")
 if err:=os.WriteFile(path,old,0600);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path+".bak",[]byte("older backup"),0600);err!=nil {t.Fatal(err)}
 n:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
 if err:=n.saveAt(path);err!=nil {t.Fatal(err)}
 backup,err:=os.ReadFile(path+".bak");if err!=nil || !bytes.Equal(backup,old) {t.Fatal("previous primary not backed up exactly",err)}
 for _,entry:=range []string{path,path+".bak"} {
  raw,err:=os.ReadFile(entry);if err!=nil {t.Fatal(err)}
  if _,err:=decodeState(raw);err!=nil {t.Fatal(err)}
  info,err:=os.Stat(entry);if err!=nil {t.Fatal(err)}
  if info.Mode().Perm()!=0600 {t.Fatal("insecure config permissions",entry,info.Mode())}
 }
}
