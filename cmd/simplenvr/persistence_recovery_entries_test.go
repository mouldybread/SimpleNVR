package main

import (
 "os"
 "path/filepath"
 "reflect"
 "testing"
)

func TestLoadMissingConfigurationPreservesExistingRecoveryEntries(t *testing.T) {
 for _,kind:=range []string{"dangling-primary","dangling-backup","regular-backup","directory-backup","symlink-backup"} {
  t.Run(kind,func(t *testing.T){
   dir:=t.TempDir();path:=filepath.Join(dir,"cameras.json");entry:=path+".bak"
   if kind=="dangling-primary" {entry=path}
   target:=filepath.Join(dir,"missing-target")
   switch kind {
   case "dangling-primary","dangling-backup":
    if err:=os.Symlink(target,entry);err!=nil {t.Fatal(err)}
   case "regular-backup":
    if err:=os.WriteFile(entry,[]byte("retained recovery evidence"),0600);err!=nil {t.Fatal(err)}
   case "directory-backup":
    if err:=os.Mkdir(entry,0700);err!=nil {t.Fatal(err)}
   case "symlink-backup":
    if err:=os.WriteFile(target,[]byte("retained recovery evidence"),0600);err!=nil {t.Fatal(err)}
    if err:=os.Symlink(target,entry);err!=nil {t.Fatal(err)}
   }
   before,err:=os.Lstat(entry);if err!=nil {t.Fatal(err)}
   n:=testNVR();snapshot:=deletionSnapshot(n)
   if err:=n.loadAt(path);err==nil {t.Fatal("existing recovery entry treated as fresh installation")}
   if !reflect.DeepEqual(snapshot,deletionSnapshot(n)) {t.Fatal("refused load changed live state")}
   after,err:=os.Lstat(entry);if err!=nil {t.Fatal(err)}
   if !os.SameFile(before,after) || before.Mode()!=after.Mode() {t.Fatal("recovery entry changed")}
   if before.Mode()&os.ModeSymlink!=0 {
    got,err:=os.Readlink(entry);if err!=nil || got!=target {t.Fatalf("symlink changed: %q %v",got,err)}
   }
   if kind=="regular-backup" {raw,err:=os.ReadFile(entry);if err!=nil || string(raw)!="retained recovery evidence" {t.Fatal("backup bytes changed",err)}}
   if kind=="symlink-backup" {raw,err:=os.ReadFile(target);if err!=nil || string(raw)!="retained recovery evidence" {t.Fatal("backup target bytes changed",err)}}
   if kind=="dangling-primary" || kind=="dangling-backup" {if _,err:=os.Lstat(target);!os.IsNotExist(err) {t.Fatal("missing symlink target created",err)}}
   if kind!="dangling-primary" {if _,err:=os.Lstat(path);!os.IsNotExist(err) {t.Fatal("primary configuration created",err)}}
  })
 }
}

func TestLoadGenuinelyAbsentConfigurationStillAllowsFirstRun(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"cameras.json")
 n:=testNVR();snapshot:=deletionSnapshot(n)
 if err:=n.loadAt(path);err!=nil {t.Fatal(err)}
 if !reflect.DeepEqual(snapshot,deletionSnapshot(n)) {t.Fatal("first-run load changed live state")}
 for _,entry:=range []string{path,path+".bak"} {if _,err:=os.Lstat(entry);!os.IsNotExist(err) {t.Fatal("first-run load created an entry",entry,err)}}
}
