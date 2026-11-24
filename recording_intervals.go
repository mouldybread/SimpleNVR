package main

import (
 "container/list"
 "context"
 "errors"
 "net/http"
 "os"
 "path/filepath"
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

type recordingInterval struct {CameraID,CameraName,Path string;Start,End time.Time;Size int64;identity os.FileInfo}
func intervalsOverlap(start,end,windowStart,windowEnd time.Time) bool {return start.Before(windowEnd) && end.After(windowStart)}

// Exports select recordings by stamped name only (collectStampedIntervals): no
// probing, and the duration cache argument is kept for existing callers but unused.
func collectRecordingIntervals(ctx context.Context,cameras []timelineCamera,window timelineWindow,_ *durationCache) ([]recordingInterval,error) {
 scan,err:=collectStampedIntervals(ctx,cameras,window);if err!=nil {return nil,err}
 return scan.Intervals,nil
}
func writeRecordingIntervalError(w http.ResponseWriter,r *http.Request,err error) {
 if r.Context().Err()!=nil {return}
 status:=http.StatusServiceUnavailable
 if errors.Is(err,context.DeadlineExceeded) {status=http.StatusGatewayTimeout}
 w.Header().Set("X-Recording-Metadata-Status","unavailable")
 http.Error(w,"Recording metadata unavailable; no partial timeline or export was returned. Retry or inspect stored media.",status)
}
