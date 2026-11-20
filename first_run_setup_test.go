package main

import (
 "context"
 "net/http"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/alexedwards/argon2id"
)

func setupFixture(t *testing.T) *firstRunSetup {
 t.Helper()
 s,err:=newFirstRunSetup(filepath.Join(t.TempDir(),"auth","credentials.json"))
 if err!=nil {t.Fatal(err)}
 return s
}

func setupRequest(s *firstRunSetup,method,path string,form url.Values,origin string) *httptest.ResponseRecorder {
 r:=httptest.NewRequest(method,"https://nvr.test"+path,strings.NewReader(form.Encode()))
 r.Header.Set("Content-Type","application/x-www-form-urlencoded")
 if origin!="" {r.Header.Set("Origin",origin)}
 w:=httptest.NewRecorder()
 http.NewCrossOriginProtection().Handler(s).ServeHTTP(w,r)
 return w
}

func setupForm(s *firstRunSetup) url.Values {
 return url.Values{"setup_code":{s.code},"username":{"admin"},"password":{"correct horse battery staple"},"confirmation":{"correct horse battery staple"}}
}

func TestFirstRunSetupLocksRoutes(t *testing.T) {
 s:=setupFixture(t)
 for _,path:=range []string{"/api/cameras","/api/stream/FRONT","/recordings/cameras.json","/api/events/trigger","/static/style.css","/auth/session"} {
  if w:=setupRequest(s,"GET",path,nil,""); w.Code!=503 {t.Errorf("%s: %d",path,w.Code)}
 }
 if w:=setupRequest(s,"POST","/api/events/trigger",nil,"");w.Code!=503 {t.Fatal(w.Code)}
 w:=setupRequest(s,"GET","/auth/setup",nil,"")
 if w.Code!=200 || w.Header().Get("Cache-Control")!="no-store" || !strings.Contains(w.Body.String(),`autocomplete="new-password"`) || strings.Contains(w.Body.String(),s.code) {t.Fatal("unsafe setup document")}
 if w:=setupRequest(s,"GET","/",nil,"");w.Code!=303 || w.Header().Get("Location")!="/auth/setup" {t.Fatal("missing setup redirect")}
}

func TestFirstRunSetupCreatesOnlyOnce(t *testing.T) {
 s:=setupFixture(t)
 form:=setupForm(s)
 w:=setupRequest(s,"POST","/auth/setup",form,"https://nvr.test")
 if w.Code!=200 {t.Fatalf("%d: %s",w.Code,w.Body.String())}
 select {case <-s.ready:default:t.Fatal("setup did not finish")}
 cfg,_,err:=readAuthConfig(s.path)
 if err!=nil || cfg.Username!="admin" {t.Fatalf("%+v %v",cfg,err)}
 match,err:=argon2id.ComparePasswordAndHash(form.Get("password"),cfg.PasswordHash)
 if err!=nil || !match {t.Fatal("password hash mismatch")}
 info,err:=os.Stat(s.path)
 if err!=nil || info.Mode().Perm()!=0600 {t.Fatal("unsafe credential permissions")}
 if s.code!="" {t.Fatal("setup code retained")}
 for _,method:=range []string{"GET","POST"} {if w:=setupRequest(s,method,"/auth/setup",form,"");w.Code!=410 {t.Fatal("setup reused")}}
 if _,err:=newAuth(s.path);err!=nil {t.Fatal(err)}
}

func TestFirstRunSetupRejectsInvalidAttempts(t *testing.T) {
 for _,kind:=range []string{"code","short","confirmation","username","newline","long","cross-origin"} {
  t.Run(kind,func(t *testing.T){
   s:=setupFixture(t);form:=setupForm(s);origin:="https://nvr.test";want:=400
   switch kind {
   case "code":form.Set("setup_code","wrong");want=401
   case "short":form.Set("password","short")
   case "confirmation":form.Set("confirmation","different")
   case "username":form.Set("username","")
   case "newline":form.Set("password","long password with newline\n")
   case "long":form.Set("password",strings.Repeat("a",1025))
   case "cross-origin":origin="https://evil.invalid";want=403
   }
   if w:=setupRequest(s,"POST","/auth/setup",form,origin);w.Code!=want {t.Fatalf("got %d want %d",w.Code,want)}
   if _,err:=os.Lstat(s.path);!os.IsNotExist(err) {t.Fatalf("credentials created: %v",err)}
  })
 }
 s:=setupFixture(t);form:=setupForm(s);form.Set("setup_code","wrong")
 setupRequest(s,"POST","/auth/setup",form,"")
 if w:=setupRequest(s,"POST","/auth/setup",form,"");w.Code!=429 || w.Header().Get("Retry-After")!="1" {t.Fatal("rate limit missing")}
}

func TestFirstRunSetupNeverOverwritesCredentials(t *testing.T) {
 s:=setupFixture(t)
 if err:=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(s.path,[]byte("existing-invalid-file"),0600);err!=nil {t.Fatal(err)}
 if err:=createInitialCredentials(s.path,authConfig{Username:"replacement"});err==nil {t.Fatal("overwrote existing file")}
 if w:=setupRequest(s,"POST","/auth/setup",setupForm(s),"");w.Code!=409 {t.Fatal(w.Code)}
 raw,_:=os.ReadFile(s.path)
 if string(raw)!="existing-invalid-file" {t.Fatal("existing contents changed")}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Second);defer cancel()
 if err:=ensureBrowserSetup(ctx,s.path);err!=nil {t.Fatal(err)}
 if _,err:=newAuth(s.path);err==nil {t.Fatal("invalid existing credentials accepted")}
}
