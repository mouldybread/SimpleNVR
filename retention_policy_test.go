package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestCameraRetentionConversionIsBounded(t *testing.T) {
 for _,item:=range []struct{days,want int}{{0,7},{1,1},{7,7},{365,365}} {
  duration,err:=cameraRetentionDuration(item.days)
  if err!=nil || duration!=time.Duration(item.want)*24*time.Hour || duration<=0 {t.Fatalf("retention %d: %v %v",item.days,duration,err)}
 }
 for _,days:=range []int{-1,366,106752,200000,int(^uint(0)>>1)} {
  duration,err:=cameraRetentionDuration(days)
  if err==nil || duration!=0 {t.Fatalf("unsafe retention accepted: %d => %v %v",days,duration,err)}
 }
}

func TestRetentionAPIRejectsInvalidValuesBeforeMutation(t *testing.T) {
 old:=nvr
 nvr=&NVR{cameras:map[string]*Camera{"a":{ID:"a",RetentionDays:7},"b":{ID:"b",RetentionDays:14}}}
 t.Cleanup(func(){nvr=old})
 for _,days:=range []int{-1,0,366,106752,int(^uint(0)>>1)} {
  raw,err:=json.Marshal(map[string]interface{}{"id":"a","retention_days":days});if err!=nil {t.Fatal(err)}
  for _,handler:=range []http.HandlerFunc{updateRetentionHandler,updateRetentionAllHandler} {
   w:=httptest.NewRecorder();handler(w,httptest.NewRequest("POST","/retention",bytes.NewReader(raw)))
   if w.Code!=400 {t.Fatalf("invalid retention %d: status %d",days,w.Code)}
   if nvr.cameras["a"].RetentionDays!=7 || nvr.cameras["b"].RetentionDays!=14 {t.Fatal("rejected retention update mutated camera state")}
  }
 }
}

func retentionStateFixture(t *testing.T,days int) ([]byte,*Camera) {
 t.Helper()
 id:=generateID("retention-test",time.Now())
 cam:=&Camera{ID:id,Name:"A",RecordDir:filepath.Join("/recordings",id),RecordingMode:ModeContinuous,RetentionDays:days}
 raw,err:=json.Marshal(persistedState{Cameras:map[string]*Camera{id:cam},CameraOrder:[]string{id},Events:[]Event{}})
 if err!=nil {t.Fatal(err)}
 return raw,cam
}

func TestPersistedRetentionValidationAndFailedLoadPreserveState(t *testing.T) {
 for _,days:=range []int{0,1,365} {raw,_:=retentionStateFixture(t,days);if _,err:=decodeState(raw);err!=nil {t.Fatalf("valid persisted retention %d: %v",days,err)}}
 for _,days:=range []int{-1,366,106752,int(^uint(0)>>1)} {
  raw,_:=retentionStateFixture(t,days)
  if _,err:=decodeState(raw);err==nil {t.Fatalf("invalid persisted retention accepted: %d",days)}
  path:=filepath.Join(t.TempDir(),"cameras.json");if err:=os.WriteFile(path,raw,0600);err!=nil {t.Fatal(err)}
  original:=&Camera{ID:"original",RetentionDays:7}
  n:=&NVR{cameras:map[string]*Camera{"original":original},cameraOrder:[]string{"original"}}
  if err:=n.loadAt(path);err==nil {t.Fatal("invalid configuration loaded")}
  if len(n.cameras)!=1 || n.cameras["original"]!=original {t.Fatal("failed load replaced live state")}
  got,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(got,raw) {t.Fatal("failed load rewrote invalid configuration")}
 }
}

func TestInvalidRetentionSaveDoesNotOverwriteConfiguration(t *testing.T) {
 raw,cam:=retentionStateFixture(t,7);path:=filepath.Join(t.TempDir(),"cameras.json")
 if err:=os.WriteFile(path,raw,0600);err!=nil {t.Fatal(err)}
 cam.RetentionDays=366
 n:=&NVR{cameras:map[string]*Camera{cam.ID:cam},cameraOrder:[]string{cam.ID}}
 if err:=n.saveAt(path);err==nil {t.Fatal("invalid retention saved")}
 got,err:=os.ReadFile(path);if err!=nil || !bytes.Equal(raw,got) {t.Fatal("rejected save overwrote configuration")}
}

func TestInvalidRuntimeRetentionPreservesEventsMediaAndStaging(t *testing.T) {
 for _,days:=range []int{-1,366,106752,int(^uint(0)>>1)} {
  dir:=t.TempDir();media:=filepath.Join(dir,"20000101_000000_old.mp4");staging:=filepath.Join(dir,"old.mp4.tmp")
  putCleanupFixture(t,media,"published");putCleanupFixture(t,staging,"evidence")
  event:=Event{ID:"keep",CameraName:"A",StartTime:time.Date(2000,1,1,0,0,0,0,time.UTC)}
  n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",Name:"A",RecordDir:dir,RetentionDays:days}},events:[]Event{event}}
  ticks:=make(chan time.Time,1);ticks<-time.Now();close(ticks);runCleanupTicks(context.Background(),n,ticks)
  if len(n.events)!=1 || n.events[0].ID!=event.ID {t.Fatal("invalid policy deleted event metadata")}
  for _,path:=range []string{media,staging} {if _,err:=os.Stat(path);err!=nil {t.Fatalf("invalid policy removed %s: %v",path,err)}}
 }
}

func TestNonpositiveRetentionDurationCannotDeletePublishedMedia(t *testing.T) {
 dir:=t.TempDir();path:=filepath.Join(dir,"20000101_000000_old.mp4");putCleanupFixture(t,path,"published")
 for _,duration:=range []time.Duration{0,-time.Hour} {cleanupOldRecordingsContext(context.Background(),dir,duration);if _,err:=os.Stat(path);err!=nil {t.Fatal("invalid duration removed media")}}
}
