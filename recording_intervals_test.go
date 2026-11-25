package main

import (
 "context"
 "os"
 "path/filepath"
 "testing"
 "time"
 "net/http"
 "net/http/httptest"
)
func intervalFixture(t *testing.T,dir,name string) string {t.Helper();path:=filepath.Join(dir,name);if err:=os.WriteFile(path,[]byte("fixture"),0600);err!=nil {t.Fatal(err)};return path}

// Export output validation still uses these decoding and bounded-output helpers.
func TestDurationDecodingAndOutputLimit(t *testing.T) {
 for _,raw:=range []string{`{}`,`{"streams":[{"codec_type":"audio","duration":"2"}]}`,`{"streams":[{"codec_type":"video","duration":"NaN"}],"format":{"duration":"Infinity"}}`,`{"streams":[{"codec_type":"video","duration":"0"}]}`} {if _,err:=decodeVideoDuration([]byte(raw));err==nil {t.Fatal("invalid duration accepted",raw)}}
 duration,err:=decodeVideoDuration([]byte(`{"streams":[{"codec_type":"video","duration":"N/A"}],"format":{"duration":"2.5"}}`))
 if err!=nil || duration!=2500*time.Millisecond {t.Fatal(duration,err)}
 var output limitedProbeOutput;if _,err:=output.Write(make([]byte,65537));err==nil || output.Len()!=0 {t.Fatal("unbounded probe output")}
}

func TestRecordingIntervalsTrustNamesAndNeverProbe(t *testing.T) {
 dir:=t.TempDir();start:=time.Date(2026,11,10,0,0,0,0,time.UTC)
 scanFixture(t,dir,start.Add(-2*time.Second),start.Add(2*time.Second),"partial")
 scanFixture(t,dir,start.Add(5*time.Second),start.Add(8*time.Second),"event")
 scanFixture(t,dir,start.Add(10*time.Second),start.Add(12*time.Second),"after")
 intervalFixture(t,dir,"20261110_000001_corrupt.mp4")
 // Selection succeeds on synthetic non-media bytes with no media tools in PATH.
 t.Setenv("PATH",t.TempDir())
 clips,err:=collectRecordingIntervals(context.Background(),[]timelineCamera{{"front","Front",dir}},timelineWindow{Start:start,End:start.Add(6*time.Second)})
 if err!=nil || len(clips)!=2 {t.Fatal(clips,err)}
 if !clips[0].Start.Equal(start.Add(-2*time.Second)) || !clips[0].End.Equal(start.Add(2*time.Second)) || !clips[1].End.Equal(start.Add(8*time.Second)) {t.Fatal("interval did not come from the stamped name",clips)}
}

func TestTimelineShutdownIsJoined(t *testing.T) {
 entered:=make(chan struct{});returned:=make(chan struct{})
 gate:=newShutdownGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-r.Context().Done();close(returned)}))
 go gate.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/api/timeline/recordings",nil))
 select {case <-entered:case <-time.After(3*time.Second):t.Fatal("timeline not admitted")}
 drained:=gate.beginShutdown()
 select {case <-drained:case <-time.After(3*time.Second):t.Fatal("timeline not joined")}
 select {case <-returned:default:t.Fatal("drained before timeline returned")}
}
