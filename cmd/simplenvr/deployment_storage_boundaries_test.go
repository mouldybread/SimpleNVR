package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "testing"

 "golang.org/x/sys/unix"
)

// This deliberately proves a limitation, not a mount-health guarantee: a
// disappeared nested mount can expose a writable directory with spare bytes.
func TestStorageFailureMountNamespaceDetachedMountFallback(t *testing.T) {
 f:=mountedStorageFailureFixture(t,1<<20,128)
 target:=filepath.Join(f.root,"lost-camera")
 if err:=os.Mkdir(target,0700);err!=nil {t.Fatal(err)}
 fallback:=filepath.Join(target,"underlying-only.txt")
 if err:=os.WriteFile(fallback,[]byte("underlying directory"),0600);err!=nil {t.Fatal(err)}
 if err:=unix.Mount("tmpfs",target,"tmpfs",uintptr(unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC),"size=65536,nr_inodes=32,mode=0700");err!=nil {t.Fatal(err)}
 mounted:=true
 defer func(){if mounted {if err:=unix.Unmount(target,0);err!=nil {t.Error(err)}}}()
 evidence:=[]byte("retained mounted capture input")
 if err:=os.WriteFile(filepath.Join(target,"capture.mp4.tmp"),evidence,0600);err!=nil {t.Fatal(err)}
 held,err:=os.Open(target);if err!=nil {t.Fatal(err)};defer held.Close()
 if err:=checkRecordingStorage(target);err!=nil {t.Fatal("mounted capacity check failed",err)}
 if err:=unix.Unmount(target,unix.MNT_DETACH);err!=nil {t.Fatal(err)}
 mounted=false
 // Capacity remains plausible on the exposed underlying directory. Do not
 // turn this observed hazard into an assertion that admission detects loss.
 if err:=checkRecordingStorage(target);err!=nil {t.Fatal("fixture did not expose the fallback limitation",err)}
 if data,err:=os.ReadFile(fallback);err!=nil || string(data)!="underlying directory" {t.Fatal("underlying fixture changed",err)}
 if _,err:=os.Lstat(filepath.Join(target,"capture.mp4.tmp"));!os.IsNotExist(err) {t.Fatal("detached evidence still visible by pathname",err)}
 pinned:=filepath.Join(fmt.Sprintf("/proc/self/fd/%d",held.Fd()),"capture.mp4.tmp")
 if data,err:=os.ReadFile(pinned);err!=nil || !bytes.Equal(data,evidence) {t.Fatal("detached capture evidence changed",err)}
 f.assertPreserved(t)
 t.Log("DETACHED_MOUNT_FALLBACK_OBSERVED: capacity is not mount identity; live mount replacement is outside accepted deployment scope")
}

func TestMissingRecordingStorageAdmissionIsNotAlwaysEnabled(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"missing")
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","1")
 if err:=checkRecordingStorage(dir);!errors.Is(err,errStorageUnavailable) {t.Fatal("missing camera storage accepted with reserve enabled",err)}
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","0")
 if err:=checkRecordingStorage(dir);err!=nil {t.Fatal("disabled capacity policy changed",err)}
}

// Explicit operator-only compatibility run, using a new synthetic scratch
// directory. It is not a test of existing capture evidence or power loss.
func TestDeploymentFilesystemCompatibility(t *testing.T) {
 root:=os.Getenv("SIMPLENVR_DEPLOYMENT_TEST_ROOT")
 if root=="" {t.Skip("requires an explicitly supplied deployment filesystem scratch parent")}
 info,err:=os.Lstat(root)
 if err!=nil || !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {t.Fatal("deployment scratch parent must be a real directory",err)}
 dir,err:=os.MkdirTemp(root,".simplenvr-compatibility-");if err!=nil {t.Fatal(err)}
 defer func(){if err:=os.RemoveAll(dir);err!=nil {t.Error("synthetic scratch cleanup failed",err)}}()
 source,final:=filepath.Join(dir,"source.tmp"),filepath.Join(dir,"published.mp4")
 data:=[]byte("synthetic publication compatibility input")
 if err:=os.WriteFile(source,data,0600);err!=nil {t.Fatal(err)}
 if err:=renameRecordingNoReplace(source,final);err!=nil {t.Fatal("required no-replace publication unsupported",err)}
 if err:=os.WriteFile(source,[]byte("collision must not overwrite"),0600);err!=nil {t.Fatal(err)}
 if err:=renameRecordingNoReplace(source,final);!errors.Is(err,unix.EEXIST) {t.Fatal("no-replace collision not rejected",err)}
 if got,err:=os.ReadFile(final);err!=nil || !bytes.Equal(got,data) {t.Fatal("published collision target changed",err)}
 fd,err:=unix.Open(final,unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC,0);if err!=nil {t.Fatal(err)};defer unix.Close(fd)
 if err:=os.Rename(final,filepath.Join(dir,"retained-original"));err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(final,[]byte("replacement"),0600);err!=nil {t.Fatal(err)}
 reopened,err:=os.Open(fmt.Sprintf("/proc/self/fd/%d",fd));if err!=nil {t.Fatal("procfs descriptor reopen unavailable",err)}
 got,readErr:=io.ReadAll(reopened);closeErr:=reopened.Close()
 if readErr!=nil || closeErr!=nil || !bytes.Equal(got,data) {t.Fatal("procfs reopen lost original inode",readErr,closeErr)}
 var fs unix.Statfs_t
 if err:=unix.Statfs(dir,&fs);err!=nil {t.Fatal(err)}
 // Exercise the real publication boundary with a synthetic accepting probe;
 // media decode is intentionally outside this filesystem-only test.
 if err:=publishRecordingWithOps(context.Background(),source,filepath.Join(dir,"second.mp4"),acceptPublicationFixture,renameRecordingNoReplace);err!=nil {t.Fatal(err)}
 t.Logf("DEPLOYMENT_FILESYSTEM_COMPATIBILITY: fs_type=%#x no_replace=passed collision=preserved procfs_original_inode=passed; no durability or mount-loss guarantee",fs.Type)
}
