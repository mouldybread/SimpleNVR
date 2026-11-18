package main

import (
 "errors"
 "io"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

type liveBoundaryTransport func(*http.Request) (*http.Response,error)
func (transport liveBoundaryTransport) RoundTrip(r *http.Request) (*http.Response,error) {return transport(r)}

func TestLiveProxyRejectsAmbiguousNamesAndInvalidSourcesBeforeNetwork(t *testing.T) {
 cases:=[]struct{name,source string;duplicate bool;want int}{
  {name:"missing",want:404},
  {name:"front",source:"rtsp://camera/live",duplicate:true,want:409},
  {name:"front",source:"http://admin:camera-secret@camera/live",want:503},
  {name:"front",source:"rtsp:camera",want:503},
  {name:"front",source:"rtsp:///live",want:503},
  {name:"front",source:"rtsp://[not-an-ip]/live",want:503},
  {name:"front",source:"rtsp://camera:0/live",want:503},
  {name:"front",source:"rtsp://camera:65536/live",want:503},
  {name:"front",source:"rtsp://camera:/live",want:503},
  {name:"front",source:"rtsp://camera/live#fragment",want:503},
 }
 for _,tc:=range cases {t.Run(tc.name+"/"+tc.source,func(t *testing.T){
  n:=&NVR{cameras:map[string]*Camera{"nil":nil}}
  if tc.name!="missing" {n.cameras["one"]=&Camera{ID:"one",Name:tc.name,RTSPUrl:tc.source}}
  if tc.duplicate {n.cameras["two"]=&Camera{ID:"two",Name:tc.name,RTSPUrl:"rtsp://different/live"}}
  calls:=0
  client:=&http.Client{Transport:liveBoundaryTransport(func(*http.Request)(*http.Response,error){calls++;return nil,errors.New("unexpected network")})}
  for i:=0;i<16;i++ {
   w:=httptest.NewRecorder();serveLiveProxy(w,httptest.NewRequest("GET","/api/stream/"+tc.name,nil),n,client)
   if w.Code!=tc.want || strings.Contains(w.Body.String(),"camera-secret") {t.Fatal("unexpected proxy selection failure",w.Code,w.Body.String())}
  }
  if calls!=0 {t.Fatal("invalid or ambiguous target reached outbound transport")}
 })}
}

func TestLiveProxyTargetUsesOnlySnapshotHostAndEncodedName(t *testing.T) {
 name:="front & back/second"
 for _,source:=range []string{"rtsps://admin:camera-secret@camera.example:8554/private?token=hidden","rtsp://[2001:db8::1]:554/private","rtsp://[fe80::1%25eth0]/private"} {
  n:=&NVR{cameras:map[string]*Camera{"one":{ID:"one",Name:name,RTSPUrl:source}}}
  parsedSource,err:=url.Parse(source);if err!=nil {t.Fatal(err)}
  calls:=0
  client:=&http.Client{Transport:liveBoundaryTransport(func(r *http.Request)(*http.Response,error){
   calls++
   if r.URL.Scheme!="http" || r.URL.Hostname()!=parsedSource.Hostname() || r.URL.Port()!="1984" || r.URL.Path!="/api/stream.mp4" || r.URL.User!=nil || len(r.URL.Query())!=1 || r.URL.Query().Get("src")!=name {t.Error("outbound target escaped its fixed authority/path/query policy",r.URL)}
   for _,key:=range []string{"Cookie","Authorization","Proxy-Authorization","Connection","X-Forwarded-Host"} {if r.Header.Get(key)!="" {t.Error("request header escaped allowlist",key)}}
   if r.Header.Get("Accept")!="video/mp4" || r.Header.Get("Range")!="bytes=0-3" {t.Error("supported request headers lost")}
   return &http.Response{StatusCode:200,Header:http.Header{"Content-Type":[]string{"video/mp4"}},Body:io.NopCloser(strings.NewReader("data")),Request:r},nil
  })}
  request:=httptest.NewRequest("GET","/api/stream/"+url.PathEscape(name)+"?target=http://unrelated/",nil)
  for _,key:=range []string{"Cookie","Authorization","Proxy-Authorization","Connection","X-Forwarded-Host"} {request.Header.Set(key,"private-value")}
  request.Header.Set("Accept","video/mp4");request.Header.Set("Range","bytes=0-3")
  w:=httptest.NewRecorder();serveLiveProxy(w,request,n,client)
  if w.Code!=200 || w.Body.String()!="data" || calls!=1 {t.Fatal("valid proxy target failed",w.Code,w.Body.String(),calls)}
 }
}

func TestLiveProxyMethodAndRouteRejection(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{}}
 for _,tc:=range []struct{method,path string;want int}{{"POST","/api/stream/front",405},{"GET","/different",400},{"GET","/api/stream/",400}} {
  w:=httptest.NewRecorder();serveLiveProxy(w,httptest.NewRequest(tc.method,tc.path,nil),n,nil)
  if w.Code!=tc.want {t.Fatal("invalid stream route/method accepted",tc,w.Code)}
  if tc.want==405 && w.Header().Get("Allow")!="GET, HEAD" {t.Fatal("missing allowed methods")}
 }
}

func TestLiveProxyRejectsActiveUpstreamContentAndDoesNotFollowRedirects(t *testing.T) {
 var redirects atomic.Int32
 destination:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){redirects.Add(1);w.Header().Set("Content-Type","video/mp4");io.WriteString(w,"redirected")}));defer destination.Close()
 cases:=[]struct{name,method,kind,contentRange string;upstream,want int}{
  {name:"video",method:"GET",kind:"video/mp4",upstream:200,want:200},
  {name:"parameters",method:"GET",kind:"video/mp4; codecs=avc1",upstream:200,want:200},
  {name:"head",method:"HEAD",kind:"video/mp4",upstream:200,want:200},
  {name:"range",method:"GET",kind:"video/mp4",contentRange:"bytes 0-3/4",upstream:206,want:206},
  {name:"html",method:"GET",kind:"text/html",upstream:200,want:502},
  {name:"missing type",method:"GET",kind:"",upstream:200,want:502},
  {name:"malformed type",method:"GET",kind:"video/mp4; broken",upstream:200,want:502},
  {name:"redirect",method:"GET",kind:"text/html",upstream:302,want:502},
  {name:"upstream error",method:"GET",kind:"text/html",upstream:500,want:502},
  {name:"safe unsatisfied range",method:"GET",kind:"text/html",contentRange:"bytes */4",upstream:416,want:416},
  {name:"invalid unsatisfied range",method:"GET",kind:"text/html",contentRange:"bytes */-1",upstream:416,want:502},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   w.Header().Set("Content-Type",tc.kind);w.Header().Set("Set-Cookie","upstream=secret");w.Header().Set("Content-Disposition","attachment; filename=untrusted.html")
   if tc.contentRange!="" {w.Header().Set("Content-Range",tc.contentRange)}
   if tc.upstream==302 {w.Header().Set("Location",destination.URL)}
   w.WriteHeader(tc.upstream)
   if tc.want==200 || tc.want==206 {io.WriteString(w,"data")} else {io.WriteString(w,"<script>upstream-active-marker</script>")}
  }));defer upstream.Close()
  client:=*liveProxyClient;client.Transport=upstream.Client().Transport;client.Timeout=3*time.Second
  w:=httptest.NewRecorder();relayLiveStream(w,httptest.NewRequest(tc.method,"/api/stream/front",nil),upstream.URL,&client)
  if w.Code!=tc.want {t.Fatal("unexpected sanitized upstream status",w.Code,w.Body.String())}
  if w.Header().Get("Cache-Control")!="no-store" || w.Header().Get("X-Content-Type-Options")!="nosniff" || w.Header().Get("Set-Cookie")!="" || w.Header().Get("Location")!="" || w.Header().Get("Content-Disposition")!="" {t.Fatal("unsafe proxy response headers",w.Header())}
  if tc.want==200 || tc.want==206 {
   if w.Header().Get("Content-Type")!="video/mp4" {t.Fatal("media type not normalized")}
   expected:="data";if tc.method=="HEAD" {expected=""};if w.Body.String()!=expected {t.Fatal("unexpected live body",w.Body.String())}
   if tc.upstream==206 && w.Header().Get("Content-Range")!=tc.contentRange {t.Fatal("range header lost")}
  } else {
   if strings.Contains(w.Body.String(),"upstream-active-marker") || strings.HasPrefix(w.Header().Get("Content-Type"),"video/") {t.Fatal("unsafe upstream body relayed")}
   if tc.want==416 && w.Header().Get("Content-Range")!="bytes */4" {t.Fatal("safe range failure lost")}
  }
 })}
 if redirects.Load()!=0 {t.Fatal("proxy followed upstream redirect")}
}

func TestUnsatisfiedStreamRangeRejectsMalformedAndOversizeValues(t *testing.T) {
 for _,value:=range []string{"bytes */","bytes */-1","bytes */+1","bytes */1 2","bytes */9223372036854775808","items */1","bytes */1\r\nX-Test: value"} {if _,ok:=safeUnsatisfiedStreamRange(value);ok {t.Fatal("invalid Content-Range accepted",value)}}
 for _,value:=range []string{"bytes */0","bytes */16"} {if got,ok:=safeUnsatisfiedStreamRange(value);!ok || got!=value {t.Fatal("valid unsatisfied range rejected",value)}}
}

func TestLiveProxyRequiresSessionAndKeepsEventTokenOutOfMediaScope(t *testing.T) {
 auth,_:=authFixture(t)
 n:=&NVR{cameras:map[string]*Camera{"one":{ID:"one",Name:"front",RTSPUrl:"rtsp://camera/live"}}}
 calls:=0
 client:=&http.Client{Transport:liveBoundaryTransport(func(r *http.Request)(*http.Response,error){
  calls++
  if r.Header.Get("Cookie")!="" || r.Header.Get("Authorization")!="" {t.Error("authentication credentials reached live upstream")}
  return &http.Response{StatusCode:200,Header:http.Header{"Content-Type":[]string{"video/mp4"}},Body:io.NopCloser(strings.NewReader("data")),Request:r},nil
 })}
 handler:=auth.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){serveLiveProxy(w,r,n,client)}))
 for _,method:=range []string{"GET","HEAD"} {for _,bearer:=range []string{"","Bearer 0123456789012345678901234567890123456789012"} {
  request:=httptest.NewRequest(method,"https://nvr.test/api/stream/front",nil);request.Header.Set("Authorization",bearer)
  w:=httptest.NewRecorder();handler.ServeHTTP(w,request)
  if w.Code!=401 {t.Fatal("sessionless/event-token live request admitted",method,w.Code)}
 }}
 if calls!=0 {t.Fatal("unauthorized media reached transport")}
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 login:=httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()));login.Header.Set("Content-Type","application/x-www-form-urlencoded")
 logged:=httptest.NewRecorder();handler.ServeHTTP(logged,login)
 if logged.Code!=303 || len(logged.Result().Cookies())==0 {t.Fatal("test session login failed",logged.Code)}
 request:=httptest.NewRequest("GET","https://nvr.test/api/stream/front",nil)
 for _,cookie:=range logged.Result().Cookies() {request.AddCookie(cookie)}
 w:=httptest.NewRecorder();handler.ServeHTTP(w,request)
 if w.Code!=200 || w.Body.String()!="data" || calls!=1 {t.Fatal("authenticated live request rejected",w.Code,calls)}
}
