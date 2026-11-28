package main

import (
 "crypto/sha256"
 "encoding/base64"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestThemedLoginDocumentAndPolicy(t *testing.T) {
 handler := themedLoginPage(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) { t.Fatal("login GET reached downstream") }))
 w := httptest.NewRecorder()
 handler.ServeHTTP(w,httptest.NewRequest("GET","https://nvr.test/auth/login",nil))
 if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" { t.Fatalf("unexpected response: %d %v",w.Code,w.Header()) }
 body := w.Body.String()
 for _, fragment := range []string{"SimpleNVR", `class="login-card"`, `method="post" action="/auth/login"`, `for="username"`, `for="password"`, `name="username"`, `name="password"`, `autocomplete="username"`, `autocomplete="current-password"`, `type="password"`, `type="submit"`, "#1a1a1a", "#2a2a2a", "#4CAF50"} {
  if !strings.Contains(body,fragment) { t.Errorf("missing %q",fragment) }
 }
 if strings.Contains(body,"<script") || strings.Contains(body,"/static/") { t.Fatal("login requires external assets or scripts") }
 digest := sha256.Sum256([]byte(themedLoginCSS))
 policy := w.Header().Get("Content-Security-Policy")
 for _, rule := range []string{"default-src 'none'", "style-src 'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'", "form-action 'self'", "base-uri 'none'", "frame-ancestors 'none'"} {
  if !strings.Contains(policy,rule) { t.Errorf("missing policy rule %q",rule) }
 }
 if strings.Contains(policy,"unsafe-inline") { t.Fatal("inline styles must use a hash") }
}

func TestThemedLoginPassesOtherRequestsThrough(t *testing.T) {
 for _, tc := range []struct{method,path string}{{"POST","/auth/login"},{"PUT","/auth/login"},{"GET","/auth/logout"},{"GET","/"},{"GET","/api/cameras"},{"GET","/auth/session"}} {
  t.Run(tc.method+tc.path,func(t *testing.T) {
   called := false
   handler := themedLoginPage(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) { called=true; w.WriteHeader(418) }))
   w := httptest.NewRecorder()
   handler.ServeHTTP(w,httptest.NewRequest(tc.method,"https://nvr.test"+tc.path,nil))
   if !called || w.Code != 418 { t.Fatal("request did not reach authentication middleware") }
  })
 }
}
