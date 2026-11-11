package main

import (
 "errors"
 "mime"
 "net"
 "net/http"
 "net/netip"
 "net/url"
 "strconv"
 "strings"
)

var errLiveProxyAmbiguous=errors.New("camera name is ambiguous")
var errLiveProxySource=errors.New("camera stream source is invalid")

// Snapshot only immutable strings under the registry lock. No network or
// filesystem operation runs under it. This is not a camera-deletion lease.
func resolveLiveProxyTarget(n *NVR,name string) (string,error) {
 n.mu.RLock();count:=0;raw:=""
 for _,cam:=range n.cameras {if cam!=nil && cam.Name==name {count++;raw=cam.RTSPUrl}}
 n.mu.RUnlock()
 if count==0 {return "",errCameraMissing}
 if count!=1 {return "",errLiveProxyAmbiguous}
 parsed,err:=url.Parse(raw)
 if err!=nil || (parsed.Scheme!="rtsp" && parsed.Scheme!="rtsps") || parsed.Opaque!="" || parsed.Fragment!="" || parsed.Hostname()=="" {return "",errLiveProxySource}
 host:=parsed.Hostname()
 if strings.HasPrefix(parsed.Host,"[") || strings.Contains(host,":") {if _,err:=netip.ParseAddr(host);err!=nil {return "",errLiveProxySource}}
 if strings.HasSuffix(parsed.Host,":") {return "",errLiveProxySource}
 if port:=parsed.Port();port!="" {value,err:=strconv.ParseUint(port,10,16);if err!=nil || value==0 {return "",errLiveProxySource}}
 // Credentials, source port/path/query and request query never become HTTP
 // credentials or a selectable outbound URL. Private addresses are intended.
 target:=url.URL{Scheme:"http",Host:net.JoinHostPort(host,"1984"),Path:"/api/stream.mp4",RawQuery:url.Values{"src":{name}}.Encode()}
 return target.String(),nil
}

func serveLiveProxy(w http.ResponseWriter,r *http.Request,n *NVR,client *http.Client) {
 w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff")
 if r.Method!=http.MethodGet && r.Method!=http.MethodHead {w.Header().Set("Allow","GET, HEAD");http.Error(w,"Method not allowed",405);return}
 if !strings.HasPrefix(r.URL.Path,"/api/stream/") {http.Error(w,"Invalid stream route",400);return}
 name:=strings.TrimPrefix(r.URL.Path,"/api/stream/")
 if name=="" {http.Error(w,"Camera name required",400);return}
 if r.Context().Err()!=nil {return}
 target,err:=resolveLiveProxyTarget(n,name)
 if err!=nil {
  switch {case errors.Is(err,errCameraMissing):http.Error(w,"Camera not found",404)
  case errors.Is(err,errLiveProxyAmbiguous):http.Error(w,"Camera name is ambiguous; use unique names for live playback",409)
  default:http.Error(w,"Camera stream source is invalid; review camera configuration",503)}
  return
 }
 relayLiveStream(w,r,target,client)
}

func liveStreamMediaType(value string) bool {kind,_,err:=mime.ParseMediaType(value);return err==nil && kind=="video/mp4"}
func safeUnsatisfiedStreamRange(value string) (string,bool) {
 const prefix="bytes */"
 if !strings.HasPrefix(value,prefix) {return "",false}
 digits:=strings.TrimPrefix(value,prefix)
 if digits=="" {return "",false}
 for _,ch:=range digits {if ch<'0' || ch>'9' {return "",false}}
 if _,err:=strconv.ParseUint(digits,10,63);err!=nil {return "",false}
 return value,true
}
