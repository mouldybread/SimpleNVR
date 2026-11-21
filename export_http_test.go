package main

import (
 "context"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "strings"
 "testing"
 "time"
)

func exportHTTPLogin(t *testing.T,a *authManager,h http.Handler)*http.Cookie {
 t.Helper();a.mu.Lock();a.nextLogin=time.Time{};a.mu.Unlock()
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 r:=httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()));r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 w:=httptest.NewRecorder();h.ServeHTTP(w,r)
 if w.Code!=303 || len(w.Result().Cookies())==0 {t.Fatalf("login %d %s",w.Code,w.Body.String())}
 return w.Result().Cookies()[0]
}
func exportHTTPRequest(h http.Handler,cookie *http.Cookie,method,path,body string)*httptest.ResponseRecorder {
 r:=httptest.NewRequest(method,"https://nvr.test"+path,strings.NewReader(body));if cookie!=nil{r.AddCookie(cookie)}
 if method=="POST"{r.Header.Set("Content-Type","application/json")}
 w:=httptest.NewRecorder();h.ServeHTTP(w,r);return w
}
func exportHTTPFixture(t *testing.T)(*authManager,*exportJobHTTP,http.Handler,*http.Cookie) {
 t.Helper();a,_:=authFixture(t);store:=testExportJobs(t,1024,4096)
 service:=newExportJobHTTP(a,store,func(context.Context,exportCreateRequest)(exportJobRenderer,error){return func(ctx context.Context,w io.Writer)error{_,err:=io.WriteString(w,"0123456789");return err},nil})
 t.Cleanup(func(){ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel();if err:=service.Close(ctx);err!=nil{t.Error(err)}})
 h:=a.Handler(service.Handler(http.NotFoundHandler()));return a,service,h,exportHTTPLogin(t,a,h)
}
const exportHTTPCreateBody=`{"camera_id":"one","start_time":"2026-11-21T00:00:00Z","end_time":"2026-11-21T00:00:10Z"}`
func createHTTPExport(t *testing.T,h http.Handler,cookie *http.Cookie)exportHTTPView {
 t.Helper();w:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody)
 if w.Code!=202{t.Fatalf("create %d %s",w.Code,w.Body.String())}
 var view exportHTTPView;if err:=json.Unmarshal(w.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
 deadline:=time.Now().Add(3*time.Second)
 for time.Now().Before(deadline){
  w=exportHTTPRequest(h,cookie,"GET",exportStatusPath+"?id="+view.ID,"");if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
  if err:=json.Unmarshal(w.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
  if view.State=="ready"{return view};time.Sleep(time.Millisecond)
 }
 t.Fatal("job did not become ready");return view
}

func TestExportHTTPAuthenticationMethodsAndCSRF(t *testing.T) {
 _,_,h,cookie:=exportHTTPFixture(t)
 for _,tc:=range []struct{method,path string}{
  {"POST",exportCreatePath},{"POST",exportCancelPath},{"GET",exportStatusPath},{"GET",exportDownloadPath},{"HEAD",exportDownloadPath},
 }{
  if got:=exportHTTPRequest(h,nil,tc.method,tc.path,"{}").Code;got!=401{t.Fatalf("anonymous %s: %d",tc.path,got)}
  r:=httptest.NewRequest(tc.method,"https://nvr.test"+tc.path,strings.NewReader("{}"));r.Header.Set("Authorization","Bearer 0123456789012345678901234567890123456789012")
  w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=401{t.Fatalf("event token accepted: %s %d",tc.path,w.Code)}
 }
 for _,tc:=range []struct{method,path string}{{"GET",exportCreatePath},{"GET",exportCancelPath},{"POST",exportStatusPath},{"POST",exportDownloadPath}}{if got:=exportHTTPRequest(h,cookie,tc.method,tc.path,"{}").Code;got!=405{t.Fatalf("wrong method %s: %d",tc.path,got)}}
 r:=httptest.NewRequest("POST","https://nvr.test"+exportCreatePath,strings.NewReader(exportHTTPCreateBody));r.AddCookie(cookie);r.Header.Set("Content-Type","application/json");r.Header.Set("Origin","https://evil.invalid")
 w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=403{t.Fatal("cross-origin creation accepted",w.Code)}
 for _,body:=range []string{"{}",exportHTTPCreateBody+" {}",strings.Replace(exportHTTPCreateBody,`"camera_id":"one"`,`"camera_id":"one","owner":"forged"`,1),strings.Replace(exportHTTPCreateBody,"00:00:10Z","00:00:00Z",1)}{if got:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,body).Code;got!=400{t.Fatalf("accepted invalid JSON/range: %d",got)}}
}

func TestExportHTTPDownloadRangeAndSessionIsolation(t *testing.T) {
 a,_,h,cookie:=exportHTTPFixture(t);view:=createHTTPExport(t,h,cookie)
 if view.DownloadURL!=exportDownloadPath+"?id="+view.ID{t.Fatal(view)}
 r:=httptest.NewRequest("GET","https://nvr.test"+view.DownloadURL,nil);r.AddCookie(cookie);r.Header.Set("Range","bytes=2-5")
 w:=httptest.NewRecorder();h.ServeHTTP(w,r)
 if w.Code!=206 || w.Body.String()!="2345" || w.Header().Get("Content-Range")!="bytes 2-5/10" || w.Header().Get("Content-Length")!="4"{t.Fatalf("range %d %v %q",w.Code,w.Header(),w.Body.String())}
 if w.Header().Get("Cache-Control")!="private, no-store" || !strings.HasPrefix(w.Header().Get("Content-Disposition"),"attachment;"){t.Fatal("unsafe download headers")}
 head:=exportHTTPRequest(h,cookie,"HEAD",view.DownloadURL,"");if head.Code!=200 || head.Body.Len()!=0 || head.Header().Get("Content-Length")!="10"{t.Fatal("HEAD body or length wrong")}
 second:=exportHTTPLogin(t,a,h)
 for _,path:=range []string{exportStatusPath+"?id="+view.ID,view.DownloadURL}{if got:=exportHTTPRequest(h,second,"GET",path,"").Code;got!=404{t.Fatalf("second session inherited export: %d",got)}}
 if got:=exportHTTPRequest(h,second,"POST",exportCancelPath,`{"id":"`+view.ID+`"}`).Code;got!=404{t.Fatal("cross-session cancel accepted")}
 if got:=exportHTTPRequest(h,cookie,"POST",exportCancelPath,`{"id":"`+view.ID+`"}`).Code;got!=204{t.Fatal(got)}
 if got:=exportHTTPRequest(h,cookie,"GET",view.DownloadURL,"").Code;got!=404{t.Fatal("canceled download available")}
}

func TestExportHTTPCredentialChangeAndLogout(t *testing.T) {
 a,_,h,cookie:=exportHTTPFixture(t);view:=createHTTPExport(t,h,cookie)
 if got:=exportHTTPRequest(h,cookie,"POST","/auth/logout","").Code;got!=303{t.Fatal(got)}
 if got:=exportHTTPRequest(h,cookie,"GET",view.DownloadURL,"").Code;got!=401{t.Fatal("logged-out download allowed")}
 cookie=exportHTTPLogin(t,a,h);view=createHTTPExport(t,h,cookie)
 cfg,_,err:=readAuthConfig(a.path);if err!=nil{t.Fatal(err)};cfg.EventTokenHash="";if err:=writeAuthConfig(a.path,cfg);err!=nil{t.Fatal(err)}
 if got:=exportHTTPRequest(h,cookie,"GET",view.DownloadURL,"").Code;got!=401{t.Fatal("stale credential session accepted")}
 cookie=exportHTTPLogin(t,a,h)
 if got:=exportHTTPRequest(h,cookie,"GET",view.DownloadURL,"").Code;got!=404{t.Fatal("new credentials inherited job")}
}

func TestExportHTTPShutdownAndConfiguration(t *testing.T) {
 _,service,h,cookie:=exportHTTPFixture(t)
 started:=make(chan struct{});joined:=make(chan struct{})
 service.plan=func(context.Context,exportCreateRequest)(exportJobRenderer,error){return func(ctx context.Context,w io.Writer)error{close(started);<-ctx.Done();close(joined);return ctx.Err()},nil}
 if got:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody).Code;got!=202{t.Fatal(got)}
 select{case <-started:case <-time.After(3*time.Second):t.Fatal("preparation not started")}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel();if err:=service.Close(ctx);err!=nil{t.Fatal(err)}
 select{case <-joined:default:t.Fatal("shutdown did not join preparation")}
 if got:=exportHTTPRequest(h,cookie,"POST",exportCreatePath,exportHTTPCreateBody).Code;got!=503{t.Fatal("shutdown admitted creation")}
 if _,err:=os.Stat(service.store.directory);!os.IsNotExist(err){t.Fatal("scratch storage not released")}
 t.Setenv("NVR_EXPORT_SCRATCH_DIR","/recordings/.exports")
 if _,err:=configuredExportJobHTTP(service.auth,&NVR{});err==nil{t.Fatal("recording directory accepted as scratch")}
}
