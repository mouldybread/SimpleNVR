package main

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
 "time"
 "net/http"
 "net/http/httptest"
)
func intervalFixture(t *testing.T,dir,name string) string {t.Helper();path:=filepath.Join(dir,name);if err:=os.WriteFile(path,[]byte("fixture"),0600);err!=nil {t.Fatal(err)};return path}
func TestDurationDecodingAndOutputLimit(t *testing.T) {
 for _,raw:=range []string{`{}`,`{"streams":[{"codec_type":"audio","duration":"2"}]}`,`{"streams":[{"codec_type":"video","duration":"NaN"}],"format":{"duration":"Infinity"}}`,`{"streams":[{"codec_type":"video","duration":"0"}]}`} {if _,err:=decodeVideoDuration([]byte(raw));err==nil {t.Fatal("invalid duration accepted",raw)}}
 duration,err:=decodeVideoDuration([]byte(`{"streams":[{"codec_type":"video","duration":"N/A"}],"format":{"duration":"2.5"}}`))
 if err!=nil || duration!=2500*time.Millisecond {t.Fatal(duration,err)}
 var output limitedProbeOutput;if _,err:=output.Write(make([]byte,65537));err==nil || output.Len()!=0 {t.Fatal("unbounded probe output")}
}
func TestDurationCacheIdentityInvalidationAndCapacity(t *testing.T) {
 dir:=t.TempDir();path:=intervalFixture(t,dir,"clip.mp4");calls:=0
 cache:=newDurationCache(2,1,func(context.Context,*os.File)(time.Duration,error){calls++;return 2*time.Second,nil})
 read:=func(path string){t.Helper();if _,_,err:=cache.duration(context.Background(),path);err!=nil {t.Fatal(err)}}
 read(path);read(path);if calls!=1 {t.Fatal(calls)}
 if err:=os.WriteFile(path,[]byte("different-size"),0600);err!=nil {t.Fatal(err)};read(path)
 info,_:=os.Stat(path);stamp:=info.ModTime().Add(time.Hour);if err:=os.Chtimes(path,stamp,stamp);err!=nil {t.Fatal(err)};read(path)
 info,_=os.Stat(path);replacement:=intervalFixture(t,dir,"replacement");if err:=os.WriteFile(replacement,[]byte("different-size"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Chtimes(replacement,info.ModTime(),info.ModTime());err!=nil {t.Fatal(err)}
 if err:=os.Rename(replacement,path);err!=nil {t.Fatal(err)};read(path)
 if calls!=4 {t.Fatal("identity/size/mtime invalidation failed",calls)}
 read(intervalFixture(t,dir,"b.mp4"));read(intervalFixture(t,dir,"c.mp4"));read(path)
 if calls!=7 || len(cache.entries)!=2 {t.Fatal("cache bound/eviction failed",calls,len(cache.entries))}
 badCalls:=0;bad:=newDurationCache(1,1,func(context.Context,*os.File)(time.Duration,error){badCalls++;return 0,errRecordingMetadata})
 for i:=0;i<2;i++ {if _,_,err:=bad.duration(context.Background(),path);err==nil {t.Fatal("failed probe succeeded")}}
 if badCalls!=2 || len(bad.entries)!=0 {t.Fatal("failure was cached")}
}
func TestDurationProbeDetectsConcurrentChange(t *testing.T) {
 path:=intervalFixture(t,t.TempDir(),"clip.mp4")
 cache:=newDurationCache(1,1,func(context.Context,*os.File)(time.Duration,error){if err:=os.WriteFile(path,[]byte("changed bytes"),0600);err!=nil {t.Fatal(err)};return time.Second,nil})
 if _,_,err:=cache.duration(context.Background(),path);!errors.Is(err,errRecordingChanged) {t.Fatal(err)}
 if len(cache.entries)!=0 {t.Fatal("changed file cached")}
}
func TestDurationConcurrencyAndCancellation(t *testing.T) {
 dir:=t.TempDir();entered:=make(chan struct{},4);var active,peak atomic.Int32
 cache:=newDurationCache(4,2,func(ctx context.Context,_ *os.File)(time.Duration,error){
  count:=active.Add(1);defer active.Add(-1)
  for old:=peak.Load();count>old && !peak.CompareAndSwap(old,count);old=peak.Load() {}
  entered<-struct{}{};<-ctx.Done();return 0,ctx.Err()
 })
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 var workers sync.WaitGroup
 for _,name:=range []string{"a","b","c","d"} {path:=intervalFixture(t,dir,name);workers.Add(1);go func(){defer workers.Done();_,_,err:=cache.duration(ctx,path);if !errors.Is(err,context.Canceled) {t.Errorf("unexpected cancellation: %v",err)}}()}
 for i:=0;i<2;i++ {select {case <-entered:case <-time.After(3*time.Second):t.Fatal("probes not admitted")}}
 if peak.Load()!=2 {t.Fatal("concurrency limit failed")};cancel()
 done:=make(chan struct{});go func(){workers.Wait();close(done)}()
 select {case <-done:case <-time.After(3*time.Second):t.Fatal("canceled probes did not drain")}
 if active.Load()!=0 || peak.Load()>2 {t.Fatal("probe leak")}
 deadline:=newDurationCache(1,1,func(ctx context.Context,_ *os.File)(time.Duration,error){<-ctx.Done();return 0,ctx.Err()});deadline.budget=20*time.Millisecond
 if _,_,err:=deadline.duration(context.Background(),filepath.Join(dir,"a"));!errors.Is(err,context.DeadlineExceeded) {t.Fatal(err)}
}
func TestSyntheticRecordingIntervalsAndCorruptMedia(t *testing.T) {
 requireFFmpeg(t);dir:=t.TempDir();path:=filepath.Join(dir,"20261109_235959_partial.mp4")
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,"ffmpeg","-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-t","2","-c:v","mpeg4","-an","-y",path)
 if output,err:=cmd.CombinedOutput();err!=nil {t.Fatal(err,string(output))}
 if err:=os.Link(path,filepath.Join(dir,"20261110_000005_event.mp4"));err!=nil {t.Fatal(err)}
 cache:=newDurationCache(8,2,probeVideoDuration)
 start:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 clips,err:=collectRecordingIntervals(ctx,[]timelineCamera{{"front","Front",dir}},timelineWindow{Start:start,End:start.Add(6*time.Second)},cache)
 if err!=nil || len(clips)!=2 {t.Fatal(clips,err)}
 if !clips[0].Start.Before(start) || clips[0].End.Sub(clips[0].Start)<1900*time.Millisecond || clips[0].End.Sub(clips[0].Start)>2100*time.Millisecond {t.Fatal("wrong probed partial interval",clips[0])}
 intervalFixture(t,dir,"20261110_000001_corrupt.mp4")
 if _,err:=collectRecordingIntervals(ctx,[]timelineCamera{{"front","Front",dir}},timelineWindow{Start:start,End:start.Add(time.Minute)},cache);err==nil {t.Fatal("corrupt media silently accepted")}
}
func TestTimelineProbeShutdownIsJoined(t *testing.T) {
 entered:=make(chan struct{});returned:=make(chan struct{})
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-r.Context().Done();close(returned)}))
 go gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/timeline/recordings",nil))
 select {case <-entered:case <-time.After(3*time.Second):t.Fatal("timeline not admitted")}
 drained:=gate.beginShutdown()
 select {case <-drained:case <-time.After(3*time.Second):t.Fatal("timeline not joined")}
 select {case <-returned:default:t.Fatal("drained before timeline returned")}
}
