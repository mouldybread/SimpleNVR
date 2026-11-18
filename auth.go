package main

import (
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
 "unicode"
 "unicode/utf8"

 "github.com/alexedwards/argon2id"
 "github.com/alexedwards/scs/v2"
 "golang.org/x/term"
)

type authConfig struct {
 Username string `json:"username"`
 PasswordHash string `json:"password_hash"`
 EventTokenHash string `json:"event_token_sha256,omitempty"`
}

type authManager struct {
 path string
 sessions *scs.SessionManager
 mu sync.Mutex
 nextLogin time.Time
 hashing chan struct{}
}

func authFile() string {
 if path := os.Getenv("NVR_AUTH_FILE"); path != "" { return path }
 return "/recordings/.auth/credentials.json"
}

func readAuthConfig(path string) (authConfig, string, error) {
 var cfg authConfig
 info, err := os.Lstat(path)
 if err != nil { return cfg, "", err }
 if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 { return cfg, "", errors.New("credential file must be a regular, owner-only file smaller than 16 KiB") }
 raw, err := os.ReadFile(path)
 if err != nil { return cfg, "", err }
 if err = json.Unmarshal(raw, &cfg); err != nil { return cfg, "", err }
 if !validUsername(cfg.Username) { return cfg, "", errors.New("invalid administrator username") }
 p, _, _, err := argon2id.DecodeHash(cfg.PasswordHash)
 if err != nil { return cfg, "", errors.New("invalid administrator password hash") }
 if p.Memory < 19456 || p.Memory > 262144 || p.Iterations < 2 || p.Iterations > 10 || p.Parallelism < 1 || p.Parallelism > 4 || p.SaltLength < 16 || p.SaltLength > 64 || p.KeyLength < 32 || p.KeyLength > 64 { return cfg, "", errors.New("password hash parameters outside supported security limits") }
 if cfg.EventTokenHash != "" {
  b, err := hex.DecodeString(cfg.EventTokenHash)
  if err != nil || len(b) != sha256.Size { return cfg, "", errors.New("invalid event token digest") }
 }
 stamp := sha256.Sum256(raw)
 return cfg, hex.EncodeToString(stamp[:]), nil
}

func validUsername(s string) bool {
 if s == "" || len(s) > 128 || !utf8.ValidString(s) { return false }
 for _, r := range s { if unicode.IsControl(r) { return false } }
 return true
}

func newAuth(path string) (*authManager, error) {
 if _, _, err := readAuthConfig(path); err != nil { return nil, fmt.Errorf("authentication configuration unavailable; initialize with 'auth set-password USERNAME': %w", err) }
 s := scs.New()
 s.Lifetime = 8*time.Hour
 s.IdleTimeout = 20*time.Minute
 s.HashTokenInStore = true
 s.Cookie.Name = "__Host-simplenvr"
 s.Cookie.Path = "/"
 s.Cookie.Domain = ""
 s.Cookie.Secure = true
 s.Cookie.HttpOnly = true
 s.Cookie.SameSite = http.SameSiteLaxMode
 s.Cookie.Persist = false
 return &authManager{path: path, sessions: s, hashing: make(chan struct{}, 2)}, nil
}

var mutationPaths = map[string]bool{
 "/api/cameras/add":true, "/api/cameras/delete":true, "/api/cameras/delete-all":true,
 "/api/cameras/update-retention":true, "/api/cameras/update-retention-all":true,
 "/api/cameras/update-mode":true, "/api/cameras/reorder":true, "/api/go2rtc/scan":true,
 "/api/record/start":true, "/api/record/stop":true, "/api/recordings/delete-all":true,
 "/api/recordings/cleanup-orphaned":true, "/api/events/trigger":true,
 "/api/events/clear-all":true, "/api/events/clear-camera":true,
}

func (a *authManager) Handler(next http.Handler) http.Handler {
 protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("X-Content-Type-Options", "nosniff")
  w.Header().Set("Referrer-Policy", "same-origin")
  w.Header().Set("Cache-Control", "no-store")
  r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
  if r.URL.Path == "/auth/login" { a.login(w,r); return }
  if r.URL.Path == "/auth/logout" { a.logout(w,r); return }
  cfg, stamp, err := readAuthConfig(a.path)
  if err != nil { http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable); return }
  if authorization := r.Header.Get("Authorization"); authorization != "" {
   fields := strings.Fields(authorization)
   if r.URL.Path != "/api/events/trigger" || r.Method != http.MethodPost || len(fields) != 2 || !strings.EqualFold(fields[0],"Bearer") || !eventTokenMatches(fields[1],cfg.EventTokenHash) { http.Error(w,"Unauthorized",http.StatusUnauthorized); return }
  } else if a.sessions.GetString(r.Context(),"user") != cfg.Username || a.sessions.GetString(r.Context(),"credential_version") != stamp {
   if r.URL.Path == "/" && r.Method == http.MethodGet { http.Redirect(w,r,"/auth/login",http.StatusSeeOther) } else { http.Error(w,"Unauthorized",http.StatusUnauthorized) }
   return
  }
  if mutationPaths[r.URL.Path] {
   if r.Method != http.MethodPost { w.Header().Set("Allow","POST"); http.Error(w,"Method not allowed",http.StatusMethodNotAllowed); return }
  } else if r.Method != http.MethodGet && r.Method != http.MethodHead {
   w.Header().Set("Allow","GET, HEAD"); http.Error(w,"Method not allowed",http.StatusMethodNotAllowed); return
  }
  if strings.HasPrefix(r.URL.Path,"/recordings/") { servePrivateRecording(w,r); return }
  clean := r.Clone(r.Context())
  clean.Header.Del("Authorization")
  clean.Header.Del("Cookie")
  next.ServeHTTP(w,clean)
 })
 return http.NewCrossOriginProtection().Handler(a.sessions.LoadAndSave(protected))
}

func eventTokenMatches(token, digest string) bool {
 if len(token) < 32 || len(token) > 256 || digest == "" { return false }
 expected, err := hex.DecodeString(digest)
 if err != nil || len(expected) != sha256.Size { return false }
 got := sha256.Sum256([]byte(token))
 return subtle.ConstantTimeCompare(got[:], expected) == 1
}

const loginHTML = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>SimpleNVR login</title><h1>SimpleNVR</h1><form method="post" action="/auth/login"><label>Username <input name="username" autocomplete="username" maxlength="128" required></label><p><label>Password <input type="password" name="password" autocomplete="current-password" required></label></p><button>Sign in</button></form></html>`

func (a *authManager) login(w http.ResponseWriter, r *http.Request) {
 w.Header().Set("Content-Security-Policy","default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
 if r.Method == http.MethodGet { w.Header().Set("Content-Type","text/html; charset=utf-8"); io.WriteString(w,loginHTML); return }
 if r.Method != http.MethodPost { w.Header().Set("Allow","GET, POST"); http.Error(w,"Method not allowed",405); return }
 a.mu.Lock()
 allowed := !time.Now().Before(a.nextLogin)
 if allowed { a.nextLogin = time.Now().Add(time.Second) }
 a.mu.Unlock()
 if !allowed { w.Header().Set("Retry-After","1"); http.Error(w,"Try again later",429); return }
 select { case a.hashing <- struct{}{}: defer func(){ <-a.hashing }(); default: http.Error(w,"Try again later",429); return }
 r.Body = http.MaxBytesReader(w,r.Body,16384)
 if err := r.ParseForm(); err != nil { http.Error(w,"Invalid login request",400); return }
 username, password := r.PostForm.Get("username"), r.PostForm.Get("password")
 if len(username)>128 || len(password)>1024 { http.Error(w,"Invalid credentials",401); return }
 cfg, stamp, err := readAuthConfig(a.path)
 if err != nil { http.Error(w,"Authentication unavailable",503); return }
 match, err := argon2id.ComparePasswordAndHash(password,cfg.PasswordHash)
 if err != nil || !match || username != cfg.Username { http.Error(w,"Invalid credentials",401); return }
 if err = a.sessions.RenewToken(r.Context()); err != nil { http.Error(w,"Unable to establish session",500); return }
 a.sessions.Put(r.Context(),"user",cfg.Username)
 a.sessions.Put(r.Context(),"credential_version",stamp)
 http.Redirect(w,r,"/",http.StatusSeeOther)
}

func (a *authManager) logout(w http.ResponseWriter, r *http.Request) {
 if r.Method == http.MethodGet {
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  io.WriteString(w,`<!doctype html><title>SimpleNVR logout</title><form method="post" action="/auth/logout"><button>Sign out</button></form>`)
  return
 }
 if r.Method != http.MethodPost { w.Header().Set("Allow","GET, POST"); http.Error(w,"Method not allowed",405); return }
 if err := a.sessions.Destroy(r.Context()); err != nil { http.Error(w,"Unable to end session",500); return }
 http.Redirect(w,r,"/auth/login",http.StatusSeeOther)
}

func servePrivateRecording(w http.ResponseWriter, r *http.Request) {
 servePrivateRecordingAt(w,r,nvr,"/recordings")
}

func authCommand(args []string) error {
 if len(args)==0 { return errors.New("usage: auth set-password USERNAME | auth rotate-event-token") }
 path := authFile()
 cfg, _, err := readAuthConfig(path)
 if err != nil && !os.IsNotExist(err) { return err }
 switch args[0] {
 case "set-password":
  if len(args)!=2 || !validUsername(args[1]) { return errors.New("usage: auth set-password USERNAME") }
  var password []byte
  if term.IsTerminal(int(os.Stdin.Fd())) {
   fmt.Fprint(os.Stderr,"New password: ")
   password, err = term.ReadPassword(int(os.Stdin.Fd()))
   fmt.Fprintln(os.Stderr)
   if err != nil { return err }
   fmt.Fprint(os.Stderr,"Confirm password: ")
   confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
   fmt.Fprintln(os.Stderr)
   if err != nil { return err }
   if string(password)!=string(confirmation) { return errors.New("passwords do not match") }
  } else {
   password, err = io.ReadAll(io.LimitReader(os.Stdin,1027))
   if err != nil { return err }
   password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password),"\n"),"\r"))
  }
  if !utf8.Valid(password) || len(password)>1024 || utf8.RuneCount(password)<15 || strings.ContainsAny(string(password),"\r\n") { return errors.New("password must be 15 or more characters, at most 1024 bytes, on one line") }
  hash, err := argon2id.CreateHash(string(password),&argon2id.Params{Memory:65536,Iterations:3,Parallelism:1,SaltLength:16,KeyLength:32})
  if err != nil { return err }
  cfg.Username, cfg.PasswordHash = args[1], hash
 case "rotate-event-token":
  if len(args)!=1 || cfg.PasswordHash=="" { return errors.New("initialize the administrator first; usage: auth rotate-event-token") }
  raw := make([]byte,32)
  if _, err := rand.Read(raw); err != nil { return err }
  token := base64.RawURLEncoding.EncodeToString(raw)
  digest := sha256.Sum256([]byte(token))
  cfg.EventTokenHash = hex.EncodeToString(digest[:])
  if err := writeAuthConfig(path,cfg); err != nil { return err }
  fmt.Fprintln(os.Stdout,token)
  return nil
 default: return errors.New("usage: auth set-password USERNAME | auth rotate-event-token")
 }
 return writeAuthConfig(path,cfg)
}

func writeAuthConfig(path string, cfg authConfig) error {
 if err := os.MkdirAll(filepath.Dir(path),0700); err != nil { return err }
 raw, err := json.MarshalIndent(cfg,"","  ")
 if err != nil { return err }
 return atomicWrite(path,append(raw,'\n'))
}
