package main

import (
 "crypto/sha256"
 "encoding/base64"
 "io"
 "net/http"
)

const themedLoginCSS = `
* { box-sizing: border-box; }
html { color-scheme: dark; }
body { margin: 0; min-height: 100vh; background: #1a1a1a; color: #fff; font-family: sans-serif; display: flex; flex-direction: column; padding: 48px 20px; }
main { width: 100%; max-width: 400px; margin: auto; }
h1 { margin: 0 0 28px; text-align: center; font-size: 32px; }
.login-card { background: #2a2a2a; border: 1px solid #444; border-radius: 12px; padding: 32px; box-shadow: 0 4px 20px rgba(0,0,0,.3); }
h2 { margin: 0 0 24px; text-align: center; font-size: 20px; }
label { display: block; text-align: center; color: #ccc; margin-bottom: 8px; font-size: 14px; }
input { display: block; width: 100%; min-width: 0; margin: 0 0 20px; padding: 12px; background: #333; color: #fff; border: 1px solid #555; border-radius: 6px; font: inherit; text-align: center; }
button { width: 100%; padding: 12px; background: #4CAF50; color: #fff; border: 0; border-radius: 6px; font: inherit; font-weight: 600; cursor: pointer; }
button:hover { background: #45a049; }
input:focus-visible, button:focus-visible { outline: 2px solid #4CAF50; outline-offset: 3px; }
@media (max-width: 420px) { body { padding: 24px 16px; } .login-card { padding: 24px; } }
`

const themedLoginHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>SimpleNVR login</title>
<style>` + themedLoginCSS + `</style>
</head>
<body>
<main>
<h1><span aria-hidden="true">🔴</span>SimpleNVR</h1>
<section class="login-card" aria-labelledby="login-title">
<h2 id="login-title">Sign in</h2>
<form method="post" action="/auth/login">
<label for="username">Username</label>
<input id="username" name="username" type="text" autocomplete="username" maxlength="128" autocapitalize="none" spellcheck="false" required>
<label for="password">Password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required>
<button type="submit">Sign in</button>
</form>
</section>
</main>
</body>
</html>`

// Only replace the public login document. Credential submission and every
// other request continue through the existing authentication middleware.
func themedLoginPage(next http.Handler) http.Handler {
 digest := sha256.Sum256([]byte(themedLoginCSS))
 policy := "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"
 return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodGet || r.URL.Path != "/auth/login" { next.ServeHTTP(w,r); return }
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  w.Header().Set("Content-Security-Policy",policy)
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","same-origin")
  io.WriteString(w,themedLoginHTML)
 })
}
