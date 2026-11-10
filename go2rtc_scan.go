package main

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "time"
 "unicode"
 "unicode/utf8"
)

const scanTimeout = 10*time.Second
const scanRequestLimit = 64<<10
const scanResponseLimit = 1<<20
const scanStreamLimit = 256

var scanHTTPClient = &http.Client{Timeout:scanTimeout,CheckRedirect:func(_ *http.Request,_ []*http.Request) error {return http.ErrUseLastResponse}}

type scannedStream struct {Name,RTSPURL string}

type scanFailure struct {status int; message string}
func (e *scanFailure) Error() string {return e.message}

func readScanURL(w http.ResponseWriter,r *http.Request) (string,error) {
 r.Body=http.MaxBytesReader(w,r.Body,scanRequestLimit)
 mediaType,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
 if err!=nil && r.Header.Get("Content-Type")!="" {return "",err}
 var values map[string]string
 if mediaType=="application/json" {
  dec:=json.NewDecoder(r.Body)
  if err:=dec.Decode(&values);err!=nil {return "",err}
  var extra interface{}
  if err:=dec.Decode(&extra);err!=io.EOF {return "",errors.New("expected one JSON object")}
  if values==nil {return "",errors.New("expected JSON object")}
 } else {
  if mediaType=="multipart/form-data" {
   err=r.ParseMultipartForm(scanRequestLimit)
   if r.MultipartForm!=nil {defer r.MultipartForm.RemoveAll()}
  } else if mediaType=="application/x-www-form-urlencoded" || mediaType=="" {err=r.ParseForm()} else {return "",errors.New("unsupported content type")}
  if err!=nil {return "",err}
  values=make(map[string]string)
  for key:=range r.Form {values[key]=r.Form.Get(key)}
 }
 for _,key:=range []string{"go2rtc-url","go2rtc_url","url","go2rtcURL"} {if value:=strings.TrimSpace(values[key]);value!="" {return value,nil}}
 return "http://go2rtc:1984",nil
}

func fetchScanStreams(ctx context.Context,client *http.Client,base string) ([]scannedStream,error) {
 u,err:=url.Parse(strings.TrimRight(base,"/"))
 if err!=nil || (u.Scheme!="http" && u.Scheme!="https") || u.Hostname()=="" || u.Port()=="" || u.User!=nil || u.RawQuery!="" || u.Fragment!="" || u.Opaque!="" {return nil,&scanFailure{400,"Invalid go2rtc URL; use http(s)://host:port without credentials, query or fragment"}}
 host:=u.Hostname()
 u.Path=strings.TrimRight(u.Path,"/")+"/api/streams";u.RawPath=""
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,u.String(),nil)
 if err!=nil {return nil,&scanFailure{400,"Invalid go2rtc URL"}}
 resp,err:=client.Do(req)
 if err!=nil {return nil,err}
 defer resp.Body.Close()
 if resp.StatusCode<200 || resp.StatusCode>=300 {return nil,&scanFailure{502,"go2rtc returned an unsuccessful HTTP status"}}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,scanResponseLimit+1))
 if err!=nil {return nil,err}
 if len(raw)>scanResponseLimit {return nil,&scanFailure{502,"go2rtc response exceeds 1 MiB"}}
 var streams map[string]json.RawMessage
 if err:=json.Unmarshal(raw,&streams);err!=nil || streams==nil || len(streams)>scanStreamLimit {return nil,&scanFailure{502,"Invalid go2rtc streams object or stream count exceeds 256"}}
 names:=make([]string,0,len(streams))
 for name,entry:=range streams {
  var object map[string]json.RawMessage
  invalid:=name=="" || len(name)>256 || !utf8.ValidString(name) || name=="." || name==".." || strings.ContainsAny(name,"/\\")
  for _,ch:=range name {if unicode.IsControl(ch) {invalid=true}}
  if invalid || json.Unmarshal(entry,&object)!=nil || object==nil {return nil,&scanFailure{502,"Invalid go2rtc stream entry"}}
  names=append(names,name)
 }
 sort.Strings(names)
 result:=make([]scannedStream,0,len(names))
 for _,name:=range names {
  streamURL:=url.URL{Scheme:"rtsp",Host:net.JoinHostPort(host,"8554"),Path:"/"+name}
  result=append(result,scannedStream{Name:name,RTSPURL:streamURL.String()})
 }
 return result,nil
}

// This boundary is shared by the production handler and local-server tests.
func serveGo2rtcScan(w http.ResponseWriter,r *http.Request,client *http.Client,budget time.Duration,apply func(context.Context,[]scannedStream) error) bool {
 if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return false}
 base,err:=readScanURL(w,r)
 if err!=nil {
  status:=400;var tooLarge *http.MaxBytesError
  if errors.As(err,&tooLarge) {status=413}
  http.Error(w,"Invalid or oversized scan request",status);return false
 }
 ctx,cancel:=context.WithTimeout(r.Context(),budget);defer cancel()
 streams,err:=fetchScanStreams(ctx,client,base)
 if err==nil {err=ctx.Err()}
 if err==nil {err=apply(ctx,streams)}
 if err!=nil {
  status,message:=502,"go2rtc scan failed"
  var failure *scanFailure
  if errors.As(err,&failure) {status,message=failure.status,failure.message} else if errors.Is(err,errRecorderShuttingDown) {status,message=503,"Server shutting down"} else if errors.Is(err,context.Canceled) {status,message=408,"Scan request canceled"} else {
   var timeout net.Error
   if errors.Is(err,context.DeadlineExceeded) || (errors.As(err,&timeout) && timeout.Timeout()) {status,message=504,"go2rtc scan timed out"}
  }
  http.Error(w,message,status);return false
 }
 return true
}

func (n *NVR) applyScanStreams(ctx context.Context,streams []scannedStream) error {
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 n.mu.Lock();defer n.mu.Unlock()
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 if err:=ctx.Err();err!=nil {return err}
 existing:=make(map[string]bool,len(n.cameras))
 for _,cam:=range n.cameras {existing[cam.RTSPUrl]=true}
 pending:=make([]*Camera,0,len(streams))
 committed:=false
 defer func(){if !committed {for _,cam:=range pending {os.Remove(cam.RecordDir)}}}()
 now:=time.Now().UTC()
 for _,stream:=range streams {
  if existing[stream.RTSPURL] {continue}
  if err:=ctx.Err();err!=nil {return err}
  id:=generateID(stream.Name,now);dir:=filepath.Join("/recordings",id)
  if err:=os.Mkdir(dir,0755);err!=nil {return &scanFailure{500,"Unable to create recording directory"}}
  pending=append(pending,&Camera{ID:id,Name:stream.Name,RTSPUrl:stream.RTSPURL,RecordDir:dir,RetentionDays:7,RecordingMode:ModeContinuous,AddedAt:now})
  existing[stream.RTSPURL]=true
 }
 if n.shuttingDown.Load() {return errRecorderShuttingDown}
 if err:=ctx.Err();err!=nil {return err}
 for _,cam:=range pending {n.cameras[cam.ID]=cam;n.cameraOrder=append(n.cameraOrder,cam.ID)}
 committed=true
 return nil
}

func scanGo2rtcHandler(w http.ResponseWriter,r *http.Request) {
 if serveGo2rtcScan(w,r,scanHTTPClient,scanTimeout,nvr.applyScanStreams) {finishCameraUpdate(w,r)}
}
