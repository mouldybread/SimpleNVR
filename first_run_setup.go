package main

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
 "unicode/utf8"

 "github.com/alexedwards/argon2id"
)

const firstRunHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>SimpleNVR setup</title><style>` + themedLoginCSS + `</style></head><body><main><h1>SimpleNVR</h1><section class="login-card" aria-labelledby="setup-title"><h2 id="setup-title">Create administrator</h2><p>Enter the one-time setup code from the container logs. Use a password of at least 15 characters.</p><form method="post" action="/auth/setup"><label for="setup-code">Setup code</label><input id="setup-code" name="setup_code" autocomplete="off" autocapitalize="none" spellcheck="false" maxlength="43" required><label for="username">Username</label><input id="username" name="username" autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="128" required><label for="password">Password</label><input id="password" type="password" name="password" autocomplete="new-password" minlength="15" required><label for="confirmation">Confirm password</label><input id="confirmation" type="password" name="confirmation" autocomplete="new-password" minlength="15" required><button type="submit">Create administrator</button></form></section></main></body></html>`

type firstRunSetup struct {
 path string
 code string
 mu sync.Mutex
 nextAttempt time.Time
 completed bool
 ready chan struct{}
}

func newFirstRunSetup(path string) (*firstRunSetup,error) {
 raw := make([]byte,32)
 if _,err := rand.Read(raw); err != nil { return nil,err }
 return &firstRunSetup{path:path,code:base64.RawURLEncoding.EncodeToString(raw),ready:make(chan struct{})},nil
}

func (s *firstRunSetup) ServeHTTP(w http.ResponseWriter,r *http.Request) {
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Referrer-Policy","no-referrer")
 digest := sha256.Sum256([]byte(themedLoginCSS))
 w.Header().Set("Content-Security-Policy","default-src 'none'; style-src 'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.completed { http.Error(w,"Setup complete; sign in when startup finishes",http.StatusGone); return }
 // Any directory entry, including a symlink or invalid credentials file,
 // disables setup. It must never be treated as an account-reset mechanism.
 if _,err := os.Lstat(s.path); err == nil { http.Error(w,"Credentials already exist; restart SimpleNVR",http.StatusConflict); return } else if !errors.Is(err,os.ErrNotExist) { http.Error(w,"Authentication configuration unavailable",http.StatusServiceUnavailable); return }
 if r.URL.Path == "/" || r.URL.Path == "/auth/login" {
  if r.Method == http.MethodGet { http.Redirect(w,r,"/auth/setup",http.StatusSeeOther); return }
 }
 if r.URL.Path != "/auth/setup" { http.Error(w,"Initial setup required",http.StatusServiceUnavailable); return }
 if r.Method == http.MethodGet { w.Header().Set("Content-Type","text/html; charset=utf-8"); io.WriteString(w,firstRunHTML); return }
 if r.Method != http.MethodPost { w.Header().Set("Allow","GET, POST"); http.Error(w,"Method not allowed",http.StatusMethodNotAllowed); return }
 if time.Now().Before(s.nextAttempt) { w.Header().Set("Retry-After","1"); http.Error(w,"Try again later",http.StatusTooManyRequests); return }
 s.nextAttempt = time.Now().Add(time.Second)
 r.Body = http.MaxBytesReader(w,r.Body,16384)
 if err := r.ParseForm(); err != nil { http.Error(w,"Invalid setup request",http.StatusBadRequest); return }
 supplied := sha256.Sum256([]byte(r.PostForm.Get("setup_code")))
 expected := sha256.Sum256([]byte(s.code))
 if subtle.ConstantTimeCompare(supplied[:],expected[:]) != 1 { http.Error(w,"Invalid setup code",http.StatusUnauthorized); return }
 username,password := r.PostForm.Get("username"),r.PostForm.Get("password")
 if !validUsername(username) { http.Error(w,"Invalid administrator username",http.StatusBadRequest); return }
 if !utf8.ValidString(password) || len(password)>1024 || utf8.RuneCountInString(password)<15 || strings.ContainsAny(password,"\r\n") { http.Error(w,"Password must be at least 15 characters, at most 1024 bytes, on one line",http.StatusBadRequest); return }
 if password != r.PostForm.Get("confirmation") { http.Error(w,"Passwords do not match",http.StatusBadRequest); return }
 hash,err := argon2id.CreateHash(password,&argon2id.Params{Memory:65536,Iterations:3,Parallelism:1,SaltLength:16,KeyLength:32})
 if err != nil { http.Error(w,"Unable to create administrator",http.StatusInternalServerError); return }
 if err = createInitialCredentials(s.path,authConfig{Username:username,PasswordHash:hash}); err != nil { http.Error(w,"Unable to create credentials; check server storage and restart if credentials now exist",http.StatusInternalServerError); return }
 s.completed = true
 s.code = ""
 w.Header().Set("Content-Type","text/html; charset=utf-8")
 io.WriteString(w,`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Setup complete</title><h1>Administrator created</h1><p>SimpleNVR is starting. <a href="/auth/login">Continue to sign in</a>. If startup is still finishing, wait a moment and try again.</p></html>`)
 close(s.ready)
}

// Publish a fully written, owner-only credential file without ever replacing
// an existing entry. A concurrent CLI initializer therefore wins safely.
func createInitialCredentials(path string,cfg authConfig) error {
 dir := filepath.Dir(path)
 if err := os.MkdirAll(dir,0700); err != nil { return err }
 raw,err := json.MarshalIndent(cfg,"","  ")
 if err != nil { return err }
 file,err := os.CreateTemp(dir,".setup-*")
 if err != nil { return err }
 defer os.Remove(file.Name())
 defer file.Close()
 if err = file.Chmod(0600); err != nil { return err }
 if _,err = file.Write(append(raw,'\n')); err != nil { return err }
 if err = file.Sync(); err != nil { return err }
 if err = file.Close(); err != nil { return err }
 if err = os.Link(file.Name(),path); err != nil { return err }
 directory,err := os.Open(dir)
 if err != nil { return err }
 defer directory.Close()
 return directory.Sync()
}

// The setup server runs before camera loading, recovery, recorders or the
// ordinary HTTP router. After setup it drains before normal startup binds 443.
func ensureBrowserSetup(ctx context.Context,path string) error {
 if _,err := os.Lstat(path); err == nil { return nil } else if !errors.Is(err,os.ErrNotExist) { return err }
 setup,err := newFirstRunSetup(path)
 if err != nil { return err }
 if err = os.MkdirAll("/certs",0755); err != nil { return err }
 cert,key := filepath.Join("/certs","cert.pem"),filepath.Join("/certs","key.pem")
 if err = ensureTLSCertificates(cert,key); err != nil { return err }
 server := &http.Server{Addr:":443",Handler:http.NewCrossOriginProtection().Handler(setup),ReadHeaderTimeout:10*time.Second,ReadTimeout:30*time.Second,WriteTimeout:30*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:32768}
 log.Printf("First-run setup: open /auth/setup over HTTPS. One-time setup code: %s",setup.code)
 done := make(chan error,1)
 go func(){ done <- server.ListenAndServeTLS(cert,key) }()
 select {
 case err = <-done:
  if err == nil { return errors.New("setup server stopped before setup completed") }
  return fmt.Errorf("setup server: %w",err)
 case <-ctx.Done():
 case <-setup.ready:
 }
 cleanup,cancel := context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 if err = server.Shutdown(cleanup); err != nil { server.Close(); <-done; return fmt.Errorf("setup shutdown: %w",err) }
 if err = <-done; err != nil && !errors.Is(err,http.ErrServerClosed) { return err }
 if ctx.Err()!=nil { return ctx.Err() }
 return nil
}
