package main

import (
 "context"
 "errors"
 "net/http/httptest"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"

 "golang.org/x/sys/unix"
)

func putStagingAdmissionFile(t *testing.T,path,data string) {
 t.Helper();if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte(data),0600);err!=nil {t.Fatal(err)}
}

func TestEventStagingScanScopeCountsAndReadOnlyBehavior(t *testing.T) {
 root:=t.TempDir()
 putStagingAdmissionFile(t,filepath.Join(root,"capture.mp4.tmp"),"12345")
 putStagingAdmissionFile(t,filepath.Join(root,"temp","event-old","pre.mp4"),"1234")
 putStagingAdmissionFile(t,filepath.Join(root,"temp","event-old","concat.txt"),"123")
 if err:=os.MkdirAll(filepath.Join(root,"temp","event-empty"),0700);err!=nil {t.Fatal(err)}
 putStagingAdmissionFile(t,filepath.Join(root,"published.mp4"),strings.Repeat("p",1000))
 putStagingAdmissionFile(t,filepath.Join(root,"buffer","live.mp4"),strings.Repeat("b",1000))
 before:=eventStorageTree(t,root)
 usage,err:=scanEventStaging(context.Background(),root,eventStagingScanEntries,eventStagingScanDepth)
 if err!=nil || usage.bytes!=12 || usage.dirs!=2 {t.Fatalf("usage %+v: %v",usage,err)}
 if !reflect.DeepEqual(before,eventStorageTree(t,root)) {t.Fatal("scanner changed artifacts")}
 if _,err:=scanEventStaging(context.Background(),root,1,eventStagingScanDepth);err==nil {t.Fatal("entry limit ignored")}
 putStagingAdmissionFile(t,filepath.Join(root,"temp","event-old","one","two","three","four","deep"),"x")
 if _,err:=scanEventStaging(context.Background(),root,eventStagingScanEntries,eventStagingScanDepth);err==nil {t.Fatal("depth limit ignored")}
}

func TestEventStagingScanCountsHardLinksConservatively(t *testing.T) {
 root:=t.TempDir();first:=filepath.Join(root,"temp","event-a","one.mp4")
 putStagingAdmissionFile(t,first,"12345678")
 if err:=os.Link(first,filepath.Join(filepath.Dir(first),"two.mp4"));err!=nil {t.Fatal(err)}
 usage,err:=scanEventStaging(context.Background(),root,100,4)
 if err!=nil || usage.bytes!=16 {t.Fatalf("hard-link accounting %+v %v",usage,err)}
}

func TestEventStagingScanRejectsSymlinksAndSpecialEntries(t *testing.T) {
 for _,kind:=range []string{"camera","temp","file","directory","fifo"} {t.Run(kind,func(t *testing.T){
  root,outside:=t.TempDir(),t.TempDir();target:=filepath.Join(outside,"evidence.mp4");putStagingAdmissionFile(t,target,"outside")
  dir:=filepath.Join(root,"temp","event-a");if err:=os.MkdirAll(dir,0700);err!=nil {t.Fatal(err)}
  scanRoot:=root
  switch kind {
  case "camera":scanRoot=filepath.Join(t.TempDir(),"link");if err:=os.Symlink(root,scanRoot);err!=nil {t.Fatal(err)}
  case "temp":if err:=os.RemoveAll(filepath.Join(root,"temp"));err!=nil {t.Fatal(err)};if err:=os.Symlink(outside,filepath.Join(root,"temp"));err!=nil {t.Fatal(err)}
  case "file":if err:=os.Symlink(target,filepath.Join(dir,"link.mp4"));err!=nil {t.Fatal(err)}
  case "directory":if err:=os.Symlink(outside,filepath.Join(dir,"nested"));err!=nil {t.Fatal(err)}
  case "fifo":if err:=unix.Mkfifo(filepath.Join(dir,"pipe"),0600);err!=nil {t.Fatal(err)}
  }
  if _,err:=scanEventStaging(context.Background(),scanRoot,100,4);err==nil {t.Fatal("unsafe staging entry accepted")}
  raw,err:=os.ReadFile(target);if err!=nil || string(raw)!="outside" {t.Fatal("outside evidence changed")}
 })}
}

func TestEventStagingCancellationOverflowAndDisabledPolicy(t *testing.T) {
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=scanEventStaging(ctx,t.TempDir(),100,4);!errors.Is(err,context.Canceled) {t.Fatal(err)}
 usage:=eventStagingUsage{bytes:^uint64(0)-1}
 if err:=addEventStagingSize(&usage,2);err==nil || usage.bytes!=^uint64(0)-1 {t.Fatal("byte overflow accepted")}
 if err:=addEventStagingSize(&usage,-1);err==nil {t.Fatal("negative file size accepted")}
 for _,value:=range []string{"","0"} {
  t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES",value);t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS",value)
  if err:=checkEventStagingAdmission(context.Background(),filepath.Join(t.TempDir(),"missing"));err!=nil {t.Fatal("disabled guard inspected filesystem",err)}
 }
 for _,name:=range []string{"SIMPLENVR_MAX_EVENT_STAGING_BYTES","SIMPLENVR_MAX_EVENT_STAGING_DIRS"} {
  t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS","0")
  t.Setenv(name,"-1");if _,err:=configuredEventStagingPolicy();!errors.Is(err,errEventStagingPolicy) {t.Fatal("invalid policy accepted")}
 }
}

func TestEventStagingHTTPAdmissionBatch(t *testing.T) {
 for _,item:=range []struct{name,bytes,dirs string;missing bool;status int}{
  {"bytes at limit","5","0",false,507},{"dirs at limit","0","1",false,507},
  {"invalid","bad","0",false,503},{"missing camera directory","1","0",true,503},
  {"disabled","0","0",false,200},{"below both limits","6","2",false,200},
 } {t.Run(item.name,func(t *testing.T){
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES",item.bytes);t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS",item.dirs)
  root:=t.TempDir();putStagingAdmissionFile(t,filepath.Join(root,"retained.mp4.tmp"),"12");putStagingAdmissionFile(t,filepath.Join(root,"temp","event-old","pre.mp4"),"345")
  dir:=root;if item.missing {dir=filepath.Join(root,"missing")}
  cam:=&Camera{ID:"a",Name:"Front",RecordDir:dir,IsRecording:true,RecordingMode:ModeEventBased}
  bm:=newBufferManager(context.Background(),cam);defer bm.cancel();cam.bufferManager=bm
  n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}};saved:=0;before:=eventStorageTree(t,root)
  w:=httptest.NewRecorder();triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(eventStorageRequest())),n,func() error {saved++;return nil})
  var job *bufferEvent
  released:=false
  if len(bm.eventChan)>0 {job=<-bm.eventChan}
  defer func(){if job!=nil && !released {bm.releaseJob(job)}}()
  if w.Code!=item.status {t.Fatalf("status %d: %s",w.Code,w.Body.String())}
  want:=make(map[string]string,len(before)+2)
  for path,data:=range before {want[path]=data}
  if item.status==200 {
   if saved!=1 || len(n.events)!=1 || job==nil || len(bm.slots)!=1 {t.Fatal("admitted capture metadata/job/capacity missing")}
   if job.inventory==nil || !job.keepStaging {t.Fatal("admitted job lacks retained ownership")}
   if filepath.Dir(job.dir)!=filepath.Join(root,"temp") || !strings.HasPrefix(filepath.Base(job.dir),"event-") {t.Fatal("unexpected job directory")}
   pinned:=readRecoveryTest(t,job.dir)
   if pinned.State!="pinned" || pinned.Revision!=2 || pinned.CameraID!=cam.ID || pinned.EventID!=n.events[0].ID || pinned.PinnedCount!=0 || len(pinned.Inputs)!=0 {t.Fatal("bad admitted checkpoint",pinned)}
   inventory:=job.inventory
   bm.releaseJob(job);released=true
   if !job.keepStaging || job.inventory!=nil {t.Fatal("unprocessed job not retained or inventory leaked")}
   assertJobInventoryClosed(t,inventory)
   retained:=readRecoveryTest(t,job.dir)
   wantRetained:=pinned;wantRetained.Revision++;wantRetained.ObservedAt=retained.ObservedAt
   if !reflect.DeepEqual(wantRetained,retained) {t.Fatal("retention changed processing evidence",retained)}
   raw,err:=os.ReadFile(filepath.Join(job.dir,"recovery.json"));if err!=nil {t.Fatal(err)}
   relative,err:=filepath.Rel(root,job.dir);if err!=nil {t.Fatal(err)}
   want[relative+"/"]=""
   want[filepath.Join(relative,"recovery.json")]=string(raw)
  } else if saved!=0 || len(n.events)!=0 || job!=nil {t.Fatal("rejected capture changed metadata or queued work")}
  if len(bm.slots)!=0 || len(bm.eventChan)!=0 {t.Fatal("admission test leaked capacity")}
  // Rejection permits no changes. Admission permits exactly the retained job
  // directory/checkpoint, with every earlier artifact still byte-identical.
  if !reflect.DeepEqual(want,eventStorageTree(t,root)) {t.Fatal("staging evidence changed or unexpected admission artifacts")}
 })}
}

func TestMetadataOnlyLoggingBypassesEventStagingPolicy(t *testing.T) {
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","0");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_BYTES","bad");t.Setenv("SIMPLENVR_MAX_EVENT_STAGING_DIRS","bad")
 cam:=&Camera{ID:"a",Name:"Front",RecordDir:filepath.Join(t.TempDir(),"missing"),RecordingMode:ModeNoRecording}
 n:=&NVR{cameras:map[string]*Camera{cam.ID:cam}};saved:=0
 w:=httptest.NewRecorder();triggerEventHandlerWithState(w,httptest.NewRequest("POST","/api/events/trigger",strings.NewReader(eventStorageRequest())),n,func() error {saved++;return nil})
 if w.Code!=200 || saved!=1 || len(n.events)!=1 {t.Fatalf("metadata-only logging rejected: %d",w.Code)}
}
