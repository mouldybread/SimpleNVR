package main

import (
 "crypto/sha256"
 "encoding/hex"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/alexedwards/argon2id"
)

func authFixture(t *testing.T) (*authManager, authConfig) {
 t.Helper()
 hash, err := argon2id.CreateHash("correct horse battery staple",&argon2id.Params{Memory:19456,Iterations:2,Parallelism:1,SaltLength:16,KeyLength:32})
 if err != nil { t.Fatal(err) }
 cfg := authConfig{Username:"admin",PasswordHash:hash}
 digest := sha256.Sum256([]byte("0123456789012345678901234567890123456789012"))
 cfg.EventTokenHash = hex.EncodeToString(digest[:])
 path := filepath.Join(t.TempDir(),"auth.json")
 if err := writeAuthConfig(path,cfg); err != nil { t.Fatal(err) }
 a, err := newAuth(path)
 if err != nil { t.Fatal(err) }
 return a,cfg
}

func TestAuthProtectsRoutesAndScopesToken(t *testing.T) {
 a,_ := authFixture(t)
 handler := a.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="" || r.Header.Get("Cookie")!="" { t.Error("credentials reached downstream handler") }
  w.WriteHeader(204)
 }))
 for _, path := range []string{"/api/cameras","/api/download","/api/stream/FRONT","/recordings/cameras.json"} {
  w := httptest.NewRecorder()
  handler.ServeHTTP(w,httptest.NewRequest("GET","https://nvr.test"+path,nil))
  if w.Code!=401 { t.Errorf("%s: got %d",path,w.Code) }
 }
 for _, path := range []string{"/api/events/trigger","/api/cameras/delete-all"} {
  r := httptest.NewRequest("POST","https://nvr.test"+path,strings.NewReader("{}"))
  r.Header.Set("Authorization","Bearer 0123456789012345678901234567890123456789012")
  w := httptest.NewRecorder(); handler.ServeHTTP(w,r)
  expected:=401; if path=="/api/events/trigger" { expected=204 }
  if w.Code!=expected { t.Errorf("token scope %s: got %d",path,w.Code) }
 }
}

func TestLoginSessionLogoutAndCSRF(t *testing.T) {
 a,_ := authFixture(t)
 handler := a.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)}))
 form := url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 r := httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()))
 r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 w := httptest.NewRecorder(); handler.ServeHTTP(w,r)
 if w.Code!=303 { t.Fatalf("login: %d %s",w.Code,w.Body.String()) }
 cookies:=w.Result().Cookies()
 if len(cookies)==0 { t.Fatal("no session cookie") }
 cookie:=cookies[0]
 if cookie.Name!="__Host-simplenvr" || !cookie.Secure || !cookie.HttpOnly || cookie.Path!="/" || cookie.Domain!="" || cookie.SameSite!=http.SameSiteLaxMode { t.Fatal("unsafe session cookie") }
 request:=func(method,path,origin string) int {
  r:=httptest.NewRequest(method,"https://nvr.test"+path,nil); r.AddCookie(cookie)
  if origin!="" {r.Header.Set("Origin",origin)}
  w:=httptest.NewRecorder(); handler.ServeHTTP(w,r); return w.Code
 }
 if code:=request("GET","/api/cameras","");code!=204 {t.Fatalf("session rejected: %d",code)}
 if code:=request("GET","/api/cameras/delete-all","");code!=405 {t.Fatalf("GET mutation allowed: %d",code)}
 if code:=request("POST","/api/cameras/delete-all","https://evil.invalid");code!=403 {t.Fatalf("CSRF allowed: %d",code)}
 if code:=request("GET","/recordings/cameras.json","");code!=404 {t.Fatalf("configuration exposed: %d",code)}
 if code:=request("POST","/auth/logout","https://nvr.test");code!=303 {t.Fatalf("logout: %d",code)}
 if code:=request("GET","/api/cameras","");code!=401 {t.Fatalf("session survived logout: %d",code)}
}

func TestCredentialChangeInvalidatesSession(t *testing.T) {
 a,cfg:=authFixture(t)
 handler:=a.Handler(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(204)}))
 form:=url.Values{"username":{"admin"},"password":{"correct horse battery staple"}}
 r:=httptest.NewRequest("POST","https://nvr.test/auth/login",strings.NewReader(form.Encode()))
 r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 w:=httptest.NewRecorder(); handler.ServeHTTP(w,r)
 if w.Code!=303 {t.Fatal(w.Code)}
 cfg.EventTokenHash=""
 if err:=writeAuthConfig(a.path,cfg);err!=nil {t.Fatal(err)}
 r=httptest.NewRequest("GET","https://nvr.test/api/cameras",nil)
 for _,c:=range w.Result().Cookies(){r.AddCookie(c)}
 w=httptest.NewRecorder();handler.ServeHTTP(w,r)
 if w.Code!=401 {t.Fatal("old session accepted after credential change")}
}

func TestAuthFailsClosed(t *testing.T) {
 if _,err:=newAuth(filepath.Join(t.TempDir(),"missing.json"));err==nil {t.Fatal("missing credentials accepted")}
 a,_:=authFixture(t)
 if err:=os.Chmod(a.path,0644);err!=nil {t.Fatal(err)}
 if _,_,err:=readAuthConfig(a.path);err==nil {t.Fatal("world-readable credentials accepted")}
}
