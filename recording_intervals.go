package main

import (
 "bytes"
 "container/list"
 "context"
 "encoding/json"
 "errors"
 "io"
 "math"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
 "sync"
 "syscall"
 "time"
)

const recordingMetadataBudget=30*time.Second
const maxRecordingScanEntries=8192
var errRecordingMetadata=errors.New("recording metadata unavailable")
var errRecordingChanged=errors.New("recording changed during inspection")
var errRecordingScanLimit=errors.New("recording scan limit exceeded")

type durationProbe func(context.Context,*os.File) (time.Duration,error)
type durationEntry struct {info os.FileInfo;duration time.Duration;element *list.Element}
type durationCache struct {
 mu sync.Mutex
 entries map[string]durationEntry
 order *list.List
 capacity int
 slots chan struct{}
 probe durationProbe
 budget time.Duration
}

func newDurationCache(capacity,parallel int,probe durationProbe) *durationCache {
 if capacity<1 || parallel<1 || probe==nil {panic("invalid duration cache configuration")}
 return &durationCache{entries:make(map[string]durationEntry),order:list.New(),capacity:capacity,slots:make(chan struct{},parallel),probe:probe,budget:5*time.Second}
}
var recordingDurations=newDurationCache(2048,2,probeVideoDuration)

func sameMediaFile(a,b os.FileInfo) bool {
 return a!=nil && b!=nil && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a,b) && a.Size()==b.Size() && a.ModTime().Equal(b.ModTime())
}
func (c *durationCache) lookup(path string,info os.FileInfo) (time.Duration,bool) {
 c.mu.Lock();defer c.mu.Unlock()
 entry,ok:=c.entries[path]
 if !ok {return 0,false}
 if !sameMediaFile(entry.info,info) {delete(c.entries,path);c.order.Remove(entry.element);return 0,false}
 c.order.MoveToFront(entry.element)
 return entry.duration,true
}
func (c *durationCache) store(path string,info os.FileInfo,duration time.Duration) {
 c.mu.Lock();defer c.mu.Unlock()
 if old,ok:=c.entries[path];ok {c.order.Remove(old.element)}
 element:=c.order.PushFront(path)
 c.entries[path]=durationEntry{info,duration,element}
 for len(c.entries)>c.capacity {old:=c.order.Back();delete(c.entries,old.Value.(string));c.order.Remove(old)}
}
func (c *durationCache) duration(parent context.Context,path string) (time.Duration,os.FileInfo,error) {
 ctx,cancel:=context.WithTimeout(parent,c.budget);defer cancel()
 if err:=ctx.Err();err!=nil {return 0,nil,err}
 path,err:=filepath.Abs(path)
 if err!=nil {return 0,nil,errRecordingMetadata}
 // Final symlinks are refused; nonblocking open prevents a raced FIFO open
 // from waiting for a writer. Parent-directory confinement is separate work.
 file,err:=os.OpenFile(path,os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,0)
 if err!=nil {return 0,nil,err}
 defer file.Close()
 info,err:=file.Stat()
 if err!=nil || !info.Mode().IsRegular() || info.Size()==0 {return 0,nil,errRecordingMetadata}
 if duration,ok:=c.lookup(path,info);ok {if err:=ctx.Err();err!=nil {return 0,nil,err};return duration,info,nil}
 select {case c.slots<-struct{}{}:defer func(){<-c.slots}();case <-ctx.Done():return 0,nil,ctx.Err()}
 if err:=ctx.Err();err!=nil {return 0,nil,err}
 fresh,err:=file.Stat()
 if err!=nil || !sameMediaFile(info,fresh) {return 0,nil,errRecordingChanged}
 if duration,ok:=c.lookup(path,info);ok {return duration,info,nil}
 duration,err:=c.probe(ctx,file)
 if ctx.Err()!=nil {return 0,nil,ctx.Err()}
 if err!=nil || duration<=0 {return 0,nil,errRecordingMetadata}
 after,err:=file.Stat()
 if err!=nil || !sameMediaFile(info,after) {return 0,nil,errRecordingChanged}
 named,err:=os.Lstat(path)
 if err!=nil || !sameMediaFile(info,named) {return 0,nil,errRecordingChanged}
 c.store(path,info,duration)
 return duration,info,nil
}

type limitedProbeOutput struct {bytes.Buffer}
func (b *limitedProbeOutput) Write(data []byte) (int,error) {
 if b.Len()+len(data)>65536 {return 0,errRecordingMetadata}
 return b.Buffer.Write(data)
}
func decodeVideoDuration(raw []byte) (time.Duration,error) {
 var data struct {
  Streams []struct {CodecType string `json:"codec_type"`;Duration string `json:"duration"`} `json:"streams"`
  Format struct {Duration string `json:"duration"`} `json:"format"`
 }
 if json.Unmarshal(raw,&data)!=nil || len(data.Streams)!=1 || data.Streams[0].CodecType!="video" {return 0,errRecordingMetadata}
 for _,value:=range []string{data.Streams[0].Duration,data.Format.Duration} {
  seconds,err:=strconv.ParseFloat(value,64)
  if err!=nil || math.IsNaN(seconds) || math.IsInf(seconds,0) || seconds<=0 || seconds>=float64(math.MaxInt64)/float64(time.Second) {continue}
  duration:=time.Duration(seconds*float64(time.Second))
  if duration>0 {return duration,nil}
 }
 return 0,errRecordingMetadata
}
func probeVideoDuration(ctx context.Context,file *os.File) (time.Duration,error) {
 // Linux runtime: probe the already-open descriptor rather than reopen a
 // pathname that could have been replaced. MOV demuxing covers these MP4s.
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-protocol_whitelist","file,pipe","-f","mov","-i","/proc/self/fd/3","-select_streams","v:0","-show_entries","stream=codec_type,duration:format=duration","-of","json")
 cmd.ExtraFiles=[]*os.File{file}
 cmd.WaitDelay=2*time.Second
 var output limitedProbeOutput
 cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil {if ctx.Err()!=nil {return 0,ctx.Err()};return 0,errRecordingMetadata}
 return decodeVideoDuration(output.Bytes())
}

type recordingInterval struct {CameraID,CameraName,Path string;Start,End time.Time;Size int64;identity os.FileInfo}
func intervalsOverlap(start,end,windowStart,windowEnd time.Time) bool {return start.Before(windowEnd) && end.After(windowStart)}
func collectRecordingIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow,cache *durationCache) ([]recordingInterval,error) {
 result:=make([]recordingInterval,0)
 scanned:=0
 for _,cam:=range cameras {
  if err:=ctx.Err();err!=nil {return nil,err}
  dir,err:=os.Open(cam.Dir)
  if os.IsNotExist(err) {continue}
  if err!=nil {return nil,errRecordingMetadata}
  err=func() error {
   defer dir.Close()
   for {
    if err:=ctx.Err();err!=nil {return err}
    entries,readErr:=dir.ReadDir(256)
    for _,entry:=range entries {
     scanned++;if scanned>maxRecordingScanEntries {return errRecordingScanLimit}
     if err:=ctx.Err();err!=nil {return err}
     name:=entry.Name()
     if !strings.HasSuffix(name,".mp4") || len(name)<15 {continue}
     start,err:=time.ParseInLocation("20060102_150405",name[:15],time.UTC)
     if err!=nil || !start.Before(window.End) {continue}
     path:=filepath.Join(cam.Dir,name)
     info,err:=os.Lstat(path)
     if os.IsNotExist(err) {continue}
     if err!=nil {return errRecordingMetadata}
     if !info.Mode().IsRegular() {continue}
     duration,info,err:=cache.duration(ctx,path)
     if os.IsNotExist(err) {continue}
     if err!=nil {return err}
     end:=start.Add(duration)
     if end.Year()>9999 || !end.After(start) {return errRecordingMetadata}
     if intervalsOverlap(start,end,window.Start,window.End) {result=append(result,recordingInterval{cam.ID,cam.Name,path,start,end,info.Size(),info})}
    }
    if readErr==io.EOF {return nil}
    if readErr!=nil {return errRecordingMetadata}
   }
  }()
  if err!=nil {return nil,err}
 }
 sort.Slice(result,func(i,j int) bool {if result[i].Start.Equal(result[j].Start) {return result[i].Path<result[j].Path};return result[i].Start.Before(result[j].Start)})
 return result,nil
}
func writeRecordingIntervalError(w http.ResponseWriter,r *http.Request,err error) {
 if r.Context().Err()!=nil {return}
 status:=http.StatusServiceUnavailable
 if errors.Is(err,context.DeadlineExceeded) {status=http.StatusGatewayTimeout}
 w.Header().Set("X-Recording-Metadata-Status","unavailable")
 http.Error(w,"Recording metadata unavailable; no partial timeline or export was returned. Retry or inspect stored media.",status)
}
