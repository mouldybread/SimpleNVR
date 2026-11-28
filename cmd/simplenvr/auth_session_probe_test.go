package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	probeIdleTimeout = 1500 * time.Millisecond
	probeGrace       = 300 * time.Millisecond
)

type probeHarness struct {
	a       *authManager
	cfg     authConfig
	handler http.Handler
	cookie  *http.Cookie
	start   time.Time
}

// newProbeHarness logs in against a short idle timeout. The downstream handler
// modifies the session only on /touch, which is the positive control for
// "a request that really reaches the session-saving path".
func newProbeHarness(t *testing.T) *probeHarness {
	t.Helper()
	a, cfg := authFixture(t)
	a.sessions.IdleTimeout = probeIdleTimeout
	handler := a.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/touch" {
			a.sessions.Put(r.Context(), "touch", time.Now().String())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	form := url.Values{"username": {cfg.Username}, "password": {"correct horse battery staple"}}
	r := httptest.NewRequest("POST", "https://nvr.test/auth/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	return &probeHarness{a: a, cfg: cfg, handler: handler, cookie: cookies[0], start: time.Now()}
}

func (h *probeHarness) do(method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://nvr.test"+path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func (h *probeHarness) get(path string) *httptest.ResponseRecorder {
	return h.do("GET", path, h.cookie)
}

func setCookies(w *httptest.ResponseRecorder) []string {
	return w.Result().Header.Values("Set-Cookie")
}

// Repeated probes must not move the idle deadline: 204 before it, 401 after it,
// and never a Set-Cookie.
func TestSessionProbeNeverRenewsIdleDeadline(t *testing.T) {
	h := newProbeHarness(t)
	deadline := h.start.Add(probeIdleTimeout)
	valid, expired := 0, 0
	for time.Now().Before(deadline.Add(2 * probeGrace)) {
		now := time.Now()
		w := h.get("/auth/session")
		if c := setCookies(w); len(c) != 0 {
			t.Fatalf("probe wrote Set-Cookie: %v", c)
		}
		switch {
		case now.Before(deadline.Add(-probeGrace)):
			if w.Code != http.StatusNoContent {
				t.Fatalf("before deadline: got %d, want 204", w.Code)
			}
			valid++
		case now.After(deadline.Add(probeGrace)):
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("after deadline: got %d, want 401 (probe renewed the session)", w.Code)
			}
			expired++
		}
		time.Sleep(100 * time.Millisecond)
	}
	if valid == 0 || expired == 0 {
		t.Fatalf("test did not observe both states: valid=%d expired=%d", valid, expired)
	}
}

// Positive control: a request that modifies the session must keep it alive past
// the original deadline, otherwise the test above could pass vacuously.
func TestSessionProbeHarnessDetectsRenewal(t *testing.T) {
	h := newProbeHarness(t)
	deadline := h.start.Add(probeIdleTimeout)
	for time.Now().Before(deadline.Add(2 * probeGrace)) {
		w := h.get("/touch")
		if w.Code != http.StatusNoContent {
			t.Fatalf("touch: got %d, session expired despite renewals", w.Code)
		}
		if len(setCookies(w)) == 0 {
			t.Fatal("modifying request did not reach the session-saving path")
		}
		time.Sleep(300 * time.Millisecond)
	}
	if w := h.get("/auth/session"); w.Code != http.StatusNoContent {
		t.Fatalf("probe after renewals: got %d, want 204", w.Code)
	}
}

func TestSessionProbeStatusMatrix(t *testing.T) {
	t.Run("valid session", func(t *testing.T) {
		h := newProbeHarness(t)
		w := h.get("/auth/session")
		if w.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control: %q", w.Header().Get("Cache-Control"))
		}
		if len(setCookies(w)) != 0 {
			t.Fatal("unexpected Set-Cookie")
		}
	})
	t.Run("no cookie", func(t *testing.T) {
		h := newProbeHarness(t)
		w := h.do("GET", "/auth/session", nil)
		if w.Code != http.StatusUnauthorized || len(setCookies(w)) != 0 {
			t.Fatalf("got %d cookies=%v", w.Code, setCookies(w))
		}
	})
	t.Run("unknown token", func(t *testing.T) {
		h := newProbeHarness(t)
		w := h.do("GET", "/auth/session", &http.Cookie{Name: h.cookie.Name, Value: "not-a-session"})
		if w.Code != http.StatusUnauthorized || len(setCookies(w)) != 0 {
			t.Fatalf("got %d cookies=%v", w.Code, setCookies(w))
		}
	})
	t.Run("wrong method", func(t *testing.T) {
		h := newProbeHarness(t)
		w := h.do("POST", "/auth/session", h.cookie)
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
			t.Fatalf("got %d Allow=%q", w.Code, w.Header().Get("Allow"))
		}
	})
	t.Run("credential change", func(t *testing.T) {
		h := newProbeHarness(t)
		h.cfg.EventTokenHash = ""
		if err := writeAuthConfig(h.a.path, h.cfg); err != nil {
			t.Fatal(err)
		}
		if w := h.get("/auth/session"); w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})
	t.Run("after logout", func(t *testing.T) {
		h := newProbeHarness(t)
		r := httptest.NewRequest("POST", "https://nvr.test/auth/logout", nil)
		r.AddCookie(h.cookie)
		r.Header.Set("Origin", "https://nvr.test")
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != http.StatusSeeOther {
			t.Fatalf("logout: %d", w.Code)
		}
		if w := h.get("/auth/session"); w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", w.Code)
		}
	})
	t.Run("authentication unavailable", func(t *testing.T) {
		h := newProbeHarness(t)
		if err := os.Chmod(h.a.path, 0644); err != nil {
			t.Fatal(err)
		}
		if w := h.get("/auth/session"); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", w.Code)
		}
	})
}

// Observation only (no assertions): logs which routes reach the session-saving
// path, i.e. emit Set-Cookie, when sent through the real middleware with a
// stub downstream handler. Real handlers cannot touch the session unless they
// are given the session manager, so this documents the middleware behaviour.
func TestSessionRenewalInventoryLog(t *testing.T) {
	h := newProbeHarness(t)
	for _, p := range []string{"/", "/api/cameras", "/api/stream/FRONT", "/recordings/cameras.json", "/auth/session"} {
		w := h.get(p)
		t.Logf("%-28s status=%d renews_session=%t", p, w.Code, len(setCookies(w)) > 0)
	}
}
