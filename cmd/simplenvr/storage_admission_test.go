package main

import (
 "context"
 "errors"
 "math"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
)

func TestStorageReserveBlockComparison(t *testing.T) {
 cases:=[]struct{blocks uint64;size int64;reserve uint64;fits bool;err error}{
  {0,0,0,true,nil},{0,4096,1,false,nil},{1,4096,4096,true,nil},{1,4096,4097,false,nil},
  {2,4096,4097,true,nil},{math.MaxUint64,4096,math.MaxUint64,true,nil},
  {1,math.MaxInt64,math.MaxUint64,false,nil},{3,math.MaxInt64,math.MaxUint64,true,nil},
  {1,0,1,false,errStorageUnavailable},{1,-1,1,false,errStorageUnavailable},
 }
 for _,item:=range cases {fits,err:=storageReserveFits(item.blocks,item.size,item.reserve);if fits!=item.fits || !errors.Is(err,item.err) {t.Fatalf("comparison %+v: %v %v",item,fits,err)}}
}

func TestRecordingStorageReserveConfiguration(t *testing.T) {
 for _,raw:=range []string{"","0","1","268435456","18446744073709551615"} {
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES",raw);if _,err:=recordingStorageReserve();err!=nil {t.Fatalf("valid policy %q rejected: %v",raw,err)}
 }
 for _,raw:=range []string{"-1","1.5","256MiB","  ","18446744073709551616"} {
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES",raw);if _,err:=recordingStorageReserve();!errors.Is(err,errStoragePolicy) {t.Fatalf("invalid policy %q accepted: %v",raw,err)}
 }
}

func TestStorageAdmissionRejectsBeforeWorkerOrIntentMutation(t *testing.T) {
 for _,item:=range []struct{name,value string;missing bool;want error}{
  {"invalid","bad",false,errStoragePolicy},{"low","18446744073709551615",false,errStorageReserve},{"missing mount","1",true,errStorageUnavailable},
 } {t.Run(item.name,func(t *testing.T){
  t.Setenv("SIMPLENVR_MIN_FREE_BYTES",item.value)
  dir:=t.TempDir();if item.missing {dir=filepath.Join(dir,"missing")}
  cam:=&Camera{ID:"a",RecordDir:dir,RecordingMode:ModeContinuous}
  n:=&NVR{cameras:map[string]*Camera{"a":cam}}
  var calls atomic.Int32
  worker:=func(context.Context,*Camera,RecordingMode){calls.Add(1)}
  if err:=n.transitionRecording("a","start","",worker);!errors.Is(err,item.want) {t.Fatalf("unexpected admission result: %v",err)}
  if calls.Load()!=0 || cam.IsRecording || cam.recordCancel!=nil {t.Fatal("rejected start changed worker state")}
  cam.IsRecording=true
  if err:=n.resumeRecordings(worker);!errors.Is(err,item.want) {t.Fatalf("restart admission: %v",err)}
  if calls.Load()!=0 || !cam.IsRecording || cam.recordCancel!=nil {t.Fatal("failed restart lost persisted intent or allocated a worker")}
 })}
}

func TestStorageGuardDoesNotDisruptExistingWorkerOrBlockStop(t *testing.T) {
 t.Setenv("SIMPLENVR_MIN_FREE_BYTES","bad")
 var canceled atomic.Bool
 cam:=&Camera{ID:"a",RecordingMode:ModeContinuous,IsRecording:true,recordCancel:func(){canceled.Store(true)}}
 n:=&NVR{cameras:map[string]*Camera{"a":cam}}
 worker:=func(context.Context,*Camera,RecordingMode){t.Error("unexpected replacement worker")}
 if err:=n.transitionRecording("a","start","",worker);err!=nil {t.Fatal(err)}
 if err:=n.transitionRecording("a","mode",ModeEventBased,worker);!errors.Is(err,errStoragePolicy) {t.Fatal(err)}
 if canceled.Load() || cam.RecordingMode!=ModeContinuous || !cam.IsRecording {t.Fatal("preflight rejection disrupted the old mode")}
 if err:=n.transitionRecording("a","stop","",worker);err!=nil {t.Fatal(err)}
 if !canceled.Load() || cam.IsRecording || cam.recordCancel!=nil {t.Fatal("storage guard blocked explicit stop")}
}

func TestDisabledStorageGuardDoesNotInspectMissingDirectory(t *testing.T) {
 for _,value:=range []string{"","0"} {t.Setenv("SIMPLENVR_MIN_FREE_BYTES",value);if err:=checkRecordingStorage(filepath.Join(t.TempDir(),"missing"));err!=nil {t.Fatal(err)}}
}

func TestStorageAdmissionHTTPStatus(t *testing.T) {
 for _,item:=range []struct{err error;status int}{{errStorageReserve,507},{errStorageUnavailable,503},{errStoragePolicy,503}} {
  w:=httptest.NewRecorder();recordingError(w,item.err)
  if w.Code!=item.status {t.Fatalf("status %d != %d",w.Code,item.status)}
  if strings.Contains(w.Body.String(),"/recordings") {t.Fatal("storage error exposed a filesystem path")}
 }
}
