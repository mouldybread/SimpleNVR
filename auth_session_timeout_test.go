package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionSettingsDefaultsWhenFileMissing(t *testing.T) {
	got, err := loadSessionSettings(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil || got != (sessionSettings{IdleMinutes: 20, LifetimeMinutes: 480}) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSessionSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := sessionSettings{IdleMinutes: 45, LifetimeMinutes: 600}
	if err := writeSessionSettings(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadSessionSettings(path)
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	idle, lifetime := got.durations()
	if idle != 45*time.Minute || lifetime != 10*time.Hour {
		t.Fatalf("idle=%s lifetime=%s", idle, lifetime)
	}
}

func TestSessionSettingsValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   sessionSettings
		want string
	}{
		{"idle zero", sessionSettings{0, 480}, "idle timeout must be between"},
		{"idle too long", sessionSettings{1441, 43200}, "idle timeout must be between"},
		{"lifetime too short", sessionSettings{1, 4}, "maximum session length must be between"},
		{"lifetime too long", sessionSettings{20, 43201}, "maximum session length must be between"},
		{"idle exceeds lifetime", sessionSettings{600, 480}, "cannot exceed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if writeSessionSettings(filepath.Join(t.TempDir(), "settings.json"), tc.in) == nil {
				t.Fatal("invalid settings were written")
			}
		})
	}
	for _, ok := range []sessionSettings{{1, 5}, {1440, 43200}, {5, 5}} {
		if err := ok.validate(); err != nil {
			t.Fatalf("%+v rejected: %v", ok, err)
		}
	}
}

func TestSessionSettingsRejectsDamagedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"empty":         "",
		"truncated":     `{"idle_minutes":20`,
		"unknown field": `{"idle_minutes":20,"lifetime_minutes":480,"extra":1}`,
		"out of range":  `{"idle_minutes":0,"lifetime_minutes":480}`,
		"trailing data": `{"idle_minutes":20,"lifetime_minutes":480} {}`,
		"wrong type":    `{"idle_minutes":"20","lifetime_minutes":480}`,
		"too large":     `{"idle_minutes":20,"lifetime_minutes":480}` + strings.Repeat(" ", 5000),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadSessionSettings(path); err == nil {
				t.Fatal("damaged settings file accepted")
			}
		})
	}
	dir := filepath.Join(t.TempDir(), "settings.json")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSessionSettings(dir); err == nil {
		t.Fatal("directory accepted as settings file")
	}
}

func TestNewAuthUsesSavedSessionSettings(t *testing.T) {
	a, _ := authFixture(t)
	if a.sessions.IdleTimeout != 20*time.Minute || a.sessions.Lifetime != 8*time.Hour {
		t.Fatalf("defaults: idle=%s lifetime=%s", a.sessions.IdleTimeout, a.sessions.Lifetime)
	}
	file := sessionSettingsPath(a.path)
	if err := writeSessionSettings(file, sessionSettings{IdleMinutes: 45, LifetimeMinutes: 600}); err != nil {
		t.Fatal(err)
	}
	b, err := newAuth(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if b.sessions.IdleTimeout != 45*time.Minute || b.sessions.Lifetime != 10*time.Hour {
		t.Fatalf("saved: idle=%s lifetime=%s", b.sessions.IdleTimeout, b.sessions.Lifetime)
	}
	if err := os.WriteFile(file, []byte(`{"idle_minutes":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newAuth(a.path); err == nil || !strings.Contains(err.Error(), "session settings") {
		t.Fatalf("damaged settings accepted by newAuth: %v", err)
	}
}

func settingsRequest(h *probeHarness, method, body string, withCookie bool, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://nvr.test/api/settings/session", strings.NewReader(body))
	if withCookie {
		r.AddCookie(h.cookie)
	}
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func settingsView(t *testing.T, w *httptest.ResponseRecorder) sessionSettingsView {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type %q", w.Header().Get("Content-Type"))
	}
	var view sessionSettingsView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

var settingsJSON = map[string]string{"Content-Type": "application/json"}

func TestSessionSettingsAPIRequiresDashboardSession(t *testing.T) {
	h := newRenewalHarness(t, 0, 20*time.Minute, renewalDownstream)
	if w := settingsRequest(h, "GET", "", false, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", w.Code)
	}
	token := map[string]string{"Authorization": "Bearer 0123456789012345678901234567890123456789012", "Content-Type": "application/json"}
	for _, method := range []string{"GET", "POST"} {
		if w := settingsRequest(h, method, `{"idle_minutes":30,"lifetime_minutes":480}`, false, token); w.Code != http.StatusUnauthorized {
			t.Fatalf("event token %s: %d", method, w.Code)
		}
	}
	if _, err := os.Stat(sessionSettingsPath(h.a.path)); !os.IsNotExist(err) {
		t.Fatalf("settings file written without a session: %v", err)
	}
}

func TestSessionSettingsAPIGetAndSave(t *testing.T) {
	h := newRenewalHarness(t, 0, 20*time.Minute, renewalDownstream)
	if got := settingsView(t, settingsRequest(h, "GET", "", true, nil)); got != (sessionSettingsView{IdleMinutes: 20, LifetimeMinutes: 480, ActiveIdleMinutes: 20, ActiveLifetimeMinutes: 480}) {
		t.Fatalf("initial view %+v", got)
	}
	got := settingsView(t, settingsRequest(h, "POST", `{"idle_minutes":45,"lifetime_minutes":600}`, true, settingsJSON))
	if got != (sessionSettingsView{IdleMinutes: 45, LifetimeMinutes: 600, ActiveIdleMinutes: 20, ActiveLifetimeMinutes: 480, RestartRequired: true}) {
		t.Fatalf("saved view %+v", got)
	}
	saved, err := loadSessionSettings(sessionSettingsPath(h.a.path))
	if err != nil || saved != (sessionSettings{IdleMinutes: 45, LifetimeMinutes: 600}) {
		t.Fatalf("file holds %+v, %v", saved, err)
	}
	if h.a.sessions.IdleTimeout != 20*time.Minute || h.a.sessions.Lifetime != 8*time.Hour {
		t.Fatal("running session manager changed before restart")
	}
	if again := settingsView(t, settingsRequest(h, "GET", "", true, nil)); again != got {
		t.Fatalf("GET after save %+v, want %+v", again, got)
	}
	if back := settingsView(t, settingsRequest(h, "POST", `{"idle_minutes":20,"lifetime_minutes":480}`, true, settingsJSON)); back.RestartRequired {
		t.Fatalf("restart flagged when saved values match running values: %+v", back)
	}
}

func TestSessionSettingsAPIRejectsBadRequests(t *testing.T) {
	h := newRenewalHarness(t, 0, 20*time.Minute, renewalDownstream)
	cross := map[string]string{"Content-Type": "application/json", "Origin": "https://evil.invalid"}
	for _, tc := range []struct {
		name, method, body string
		header             map[string]string
		want               int
	}{
		{"idle zero", "POST", `{"idle_minutes":0,"lifetime_minutes":480}`, settingsJSON, http.StatusBadRequest},
		{"idle exceeds lifetime", "POST", `{"idle_minutes":600,"lifetime_minutes":480}`, settingsJSON, http.StatusBadRequest},
		{"missing lifetime", "POST", `{"idle_minutes":20}`, settingsJSON, http.StatusBadRequest},
		{"unknown field", "POST", `{"idle_minutes":20,"lifetime_minutes":480,"x":1}`, settingsJSON, http.StatusBadRequest},
		{"malformed", "POST", `{`, settingsJSON, http.StatusBadRequest},
		{"trailing data", "POST", `{"idle_minutes":20,"lifetime_minutes":480} {}`, settingsJSON, http.StatusBadRequest},
		{"oversized", "POST", strings.Repeat(" ", 5000) + `{"idle_minutes":20,"lifetime_minutes":480}`, settingsJSON, http.StatusBadRequest},
		{"wrong content type", "POST", `{"idle_minutes":20,"lifetime_minutes":480}`, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{"no content type", "POST", `{"idle_minutes":20,"lifetime_minutes":480}`, nil, http.StatusUnsupportedMediaType},
		{"cross-origin", "POST", `{"idle_minutes":20,"lifetime_minutes":480}`, cross, http.StatusForbidden},
		{"put", "PUT", `{"idle_minutes":20,"lifetime_minutes":480}`, settingsJSON, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := settingsRequest(h, tc.method, tc.body, true, tc.header)
			if w.Code != tc.want {
				t.Fatalf("got %d (%s), want %d", w.Code, strings.TrimSpace(w.Body.String()), tc.want)
			}
			if tc.want == http.StatusMethodNotAllowed && w.Header().Get("Allow") != "GET, POST" {
				t.Fatalf("Allow %q", w.Header().Get("Allow"))
			}
		})
	}
	if _, err := os.Stat(sessionSettingsPath(h.a.path)); !os.IsNotExist(err) {
		t.Fatalf("a rejected request wrote the settings file: %v", err)
	}
}
