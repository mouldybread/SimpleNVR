package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

type storageFailureFixture struct {
 root,stage,config string
 manifest,configuration []byte
 state *NVR
}

func mountedStorageFailureFixture(t *testing.T,size,inodes int) storageFailureFixture {
 t.Helper()
 if os.Getenv("SIMPLENVR_STORAGE_FAILURE_TESTS")!="1" {t.Skip("isolated mount-namespace storage tests run in their dedicated CI step")}
 host:=os.Getenv("SIMPLENVR_STORAGE_HOST_MNT_NS")
 current,err:=os.Readlink("/proc/self/ns/mnt")
 if err!=nil || host=="" || current==host || os.Geteuid()!=0 {t.Fatal("storage tests require root inside a distinct private mount namespace",err)}
 if err:=unix.Mount("","/","",uintptr(unix.MS_REC|unix.MS_PRIVATE),"");err!=nil {t.Fatal("cannot make isolated namespace recursively private",err)}
 root:=t.TempDir()
 flags:=uintptr(unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC)
 options:=fmt.Sprintf("size=%d,nr_inodes=%d,mode=0700",size,inodes)
 if err:=unix.Mount("tmpfs",root,"tmpfs",flags,options);err!=nil {t.Fatal("required isolated tmpfs mount failed",err)}
 t.Cleanup(func(){if err:=unix.Unmount(root,0);err!=nil {t.Error("test tmpfs did not unmount cleanly",err)}})
 var fs unix.Statfs_t
 if err:=unix.Statfs(root,&fs);err!=nil || fs.Type!=unix.TMPFS_MAGIC {t.Fatal("refusing to fill a non-tmpfs filesystem",err)}
 stage:=filepath.Join(root,"temp","event-evidence")
 if err:=os.MkdirAll(stage,0700);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 m:=eventRecoveryManifest{Version:1,Revision:2,CameraID:"a",EventID:"retained",RequestedStart:now,RequestedEnd:now.Add(time.Second),DurationSeconds:1,CreatedAt:now,ObservedAt:now,State:"pinned",Inputs:[]eventRecoveryInput{},CaptureName:"event.mp4"}
 manifest,err:=json.Marshal(m);if err!=nil {t.Fatal(err)}
 if err:=writeEventRecovery(stage,manifest);err!=nil {t.Fatal(err)}
 for path,content:=range map[string]string{filepath.Join(root,"capture.mp4.tmp"):"retained root output",filepath.Join(stage,"event.mp4"):"retained event capture"} {
  if err:=os.WriteFile(path,[]byte(content),0600);err!=nil {t.Fatal(err)}
 }
 state:=&NVR{cameras:map[string]*Camera{},cameraOrder:[]string{},events:[]Event{}}
 config:=filepath.Join(root,"cameras.json");if err:=state.saveAt(config);err!=nil {t.Fatal(err)}
 original,err:=os.ReadFile(config);if err!=nil {t.Fatal(err)}
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","1");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS","0")
 return storageFailureFixture{root:root,stage:stage,config:config,manifest:manifest,configuration:original,state:state}
}

func (f storageFailureFixture) assertPreserved(t *testing.T) {
 t.Helper()
 for path,want:=range map[string][]byte{
  filepath.Join(f.root,"capture.mp4.tmp"):[]byte("retained root output"),
  filepath.Join(f.stage,"event.mp4"):[]byte("retained event capture"),
  filepath.Join(f.stage,"recovery.json"):f.manifest,
  f.config:f.configuration,
 } {
  got,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(got,want) {t.Fatalf("protected artifact changed: %s: %v",filepath.Base(path),err)}
 }
}

func (f storageFailureFixture) failManifestAndPersistence(t *testing.T,want error) {
 t.Helper()
 if err:=writeEventRecovery(f.stage,bytes.Replace(f.manifest,[]byte(`"revision":2`),[]byte(`"revision":3`),1));!errors.Is(err,want) {t.Fatalf("expected real manifest write failure %v: %v",want,err)}
 now:=time.Now().UTC();f.state.mu.Lock();f.state.events=append(f.state.events,Event{ID:"unsaved",CameraName:"a",Label:"test",StartTime:now,Duration:1,CreatedAt:now});f.state.mu.Unlock()
 if err:=f.state.saveAt(f.config);!errors.Is(err,want) {t.Fatalf("expected real persistence failure %v: %v",want,err)}
 f.assertPreserved(t)
}

func TestStorageFailureMountNamespaceByteExhaustion(t *testing.T) {
 f:=mountedStorageFailureFixture(t,1<<20,128)
 output,err:=os.CreateTemp(f.root,"writer-*.tmp");if err!=nil {t.Fatal(err)}
 if err:=os.Remove(output.Name());err!=nil {output.Close();t.Fatal(err)}
 t.Cleanup(func(){if err:=output.Close();err!=nil {t.Error(err)}})
 fill,err:=os.OpenFile(filepath.Join(f.root,"bounded-fill.bin"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil {t.Fatal(err)}
 chunk:=make([]byte,64<<10);total:=0;var full error
 for total<8<<20 {n,err:=fill.Write(chunk);total+=n;if err!=nil {full=err;break};if n==0 {break}}
 if err:=fill.Close();err!=nil {t.Fatal(err)}
 if !errors.Is(full,unix.ENOSPC) {t.Fatal("byte-exhaustion condition not exercised",full,total)}
 capacity,err:=readRecordingStorageCapacity(f.root)
 if err!=nil || capacity.AvailableBytes!=0 {t.Fatal("filesystem did not report actual byte exhaustion",capacity,err)}
 writer:=&trimFileWriter{file:output,limit:maxTrimBytes}
 n,err:=writer.Write([]byte("attempted output"))
 if n!=0 || !errors.Is(err,unix.ENOSPC) || writer.exceeded {t.Fatal("OS write failure confused with application size cap",n,err)}
 f.failManifestAndPersistence(t,unix.ENOSPC)
 report:=inspectRecordingStorage(context.Background(),f.root)
 if !report.Complete || report.FreeReserveMet==nil || *report.FreeReserveMet {t.Fatalf("complete inspection must not imply adequate capacity: %+v",report)}
 // Real media remains on a separate healthy test directory; only the output
 // filesystem is full. No mount or source in /recordings is used.
 clips:=syntheticExportClips(t,t.TempDir());base:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 for i:=range clips {clips[i].CameraID="front";clips[i].Start=base.Add(time.Duration(i)*2*time.Second);clips[i].End=clips[i].Start.Add(2*time.Second)}
 plan,err:=buildExportPlan(clips,timelineWindow{Start:base.Add(500*time.Millisecond),End:base.Add(3500*time.Millisecond)});if err!=nil {t.Fatal(err)}
 pool:=newExportInputPool(1,2);pool.tempDir=t.TempDir()
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 pins,err:=pool.prepare(ctx,plan.clips());if err!=nil {t.Fatal(err)};defer pins.Close()
 rendered,err:=renderTrimmedExport(ctx,pins,plan,f.root,maxTrimBytes)
 if rendered!=nil {rendered.Close();t.Fatal("full filesystem returned a prepared trim")}
 if !errors.Is(err,errTrimRender) {t.Fatal("full-filesystem render not rejected",err)}
 files:=append([]*os.File{pins.manifest},pins.files...);if err:=pins.Close();err!=nil {t.Fatal(err)};assertPinsClosed(t,pool,files)
 f.assertPreserved(t)
}

func TestStorageFailureMountNamespaceInodeExhaustion(t *testing.T) {
 f:=mountedStorageFailureFixture(t,8<<20,32)
 var exhausted error
 for i:=0;i<256;i++ {
  file,err:=os.OpenFile(filepath.Join(f.root,fmt.Sprintf("inode-%03d",i)),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if err!=nil {exhausted=err;break};if err:=file.Close();err!=nil {t.Fatal(err)}
 }
 if !errors.Is(exhausted,unix.ENOSPC) {t.Fatal("inode-exhaustion condition not exercised",exhausted)}
 capacity,err:=readRecordingStorageCapacity(f.root)
 if err!=nil || capacity.AvailableBytes==0 || capacity.FreeInodes==nil || *capacity.FreeInodes!=0 {t.Fatal("expected free bytes with exhausted inodes",capacity,err)}
 f.failManifestAndPersistence(t,unix.ENOSPC)
 cam:=&Camera{ID:"a",Name:"A",RecordDir:f.root,IsRecording:true,RecordingMode:ModeEventBased}
 bm:=newBufferManager(context.Background(),cam);defer bm.cancel();cam.bufferManager=bm
 n:=&NVR{cameras:map[string]*Camera{"a":cam},events:[]Event{}}
 body:=fmt.Sprintf(`{"camera_name":"A","label":"test","start_time":%q,"duration_seconds":1}`,time.Now().UTC().Format(time.RFC3339))
 w:=httptest.NewRecorder();saved:=0
 triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(body)),n,func()error{saved++;return nil})
 if w.Code!=500 || saved!=0 || len(n.events)!=0 || len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatalf("inode failure accepted capture or leaked state: %d %s",w.Code,w.Body.String())}
 f.assertPreserved(t)
}

func TestStorageFailureMountNamespaceReadOnly(t *testing.T) {
 f:=mountedStorageFailureFixture(t,1<<20,128)
 flags:=uintptr(unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC)
 if err:=unix.Mount("tmpfs",f.root,"tmpfs",flags,"");err!=nil {t.Fatal("read-only remount failed",err)}
 f.failManifestAndPersistence(t,unix.EROFS)
 source:=filepath.Join(f.root,"capture.mp4.tmp");final:=filepath.Join(f.root,"capture.mp4")
 err:=publishRecordingWithOps(context.Background(),source,final,acceptPublicationFixture,renameRecordingNoReplace)
 if !errors.Is(err,unix.EROFS) {t.Fatal("read-only publication did not fail closed",err)}
 if _,err:=os.Lstat(final);!os.IsNotExist(err) {t.Fatal("read-only publication created destination",err)}
 report:=inspectEventRecovery(context.Background(),f.root,"a","event-evidence")
 if !report.Complete {t.Fatalf("read-only evidence inspection failed: %+v",report)}
 f.assertPreserved(t)
}
