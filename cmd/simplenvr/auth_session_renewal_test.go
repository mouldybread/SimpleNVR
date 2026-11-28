package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	renewalIdle = 1200 * time.Millisecond
	renewalPoll = 250 * time.Millisecond
)

type dashboardRoute struct {
	label  string
	method string
	path   string
}

// Requests the dashboard issues, taken from web/static/js/*.js and the routes
// registered in main.go. /auth/session is covered by the probe tests.
var dashboardRoutes = []dashboardRoute{
	{"index", "GET", "/"},
	{"static_asset", "GET", "/static/js/core.js"},
	{"camera_list", "GET", "/api/cameras"},
	{"event_list", "GET", "/api/events/list"},
	{"timeline", "GET", "/api/timeline/recordings?start_time=2026-01-01T00%3A00%3A00.000Z&end_time=2026-01-02T00%3A00%3A00.000Z"},
	{"live_stream", "GET", "/api/stream/FRONT"},
	{"export_download", "GET", "/api/download?camera_id=id_x&start_time=2026-01-01T00%3A00%3A00.000Z&end_time=2026-01-01T00%3A10%3A00.000Z"},
	{"recording_file", "GET", "/recordings/cameras.json"},
	{"record_start", "POST", "/api/record/start"},
}

// The downstream handler modifies the session only on /touch, which is the
// positive control for a request that really reaches the session-saving path.
func renewalDownstream(a *authManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/touch" {
			a.sessions.Put(r.Context(), "touch", time.Now().String())
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// newRenewalHarness logs in with the given absolute lifetime (0 keeps the
// default) and idle timeout, both applied before the session is created.
func newRenewalHarness(t *testing.T, lifetime, idle time.Duration, downstream func(a *authManager) http.Handler) *probeHarness {
	t.Helper()
	a, cfg := authFixture(t)
	if lifetime > 0 {
		a.sessions.Lifetime = lifetime
	}
	a.sessions.IdleTimeout = idle
	handler := a.Handler(downstream(a))
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

// probeUntilExpired probes /auth/session until shortly after deadline and
// requires 204 before it, 401 after it, and never a Set-Cookie.
func probeUntilExpired(t *testing.T, h *probeHarness, deadline time.Time) {
	t.Helper()
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
				t.Fatalf("after deadline: got %d, want 401", w.Code)
			}
			expired++
		}
		time.Sleep(100 * time.Millisecond)
	}
	if valid == 0 || expired == 0 {
		t.Fatalf("test did not observe both states: valid=%d expired=%d", valid, expired)
	}
}

// For every request the dashboard makes, the Set-Cookie signal used by the
// other probe tests must agree with real expiry: a route emits Set-Cookie if
// and only if polling it keeps the session alive past the original idle
// deadline. The test also requires every route to behave the same way, so a
// change in session-library or middleware behaviour fails the build instead of
// silently changing what keeps a dashboard logged in.
func TestSessionRenewalMatchesExpiryForDashboardRoutes(t *testing.T) {
	var mu sync.Mutex
	renews := map[string]bool{}
	t.Run("routes", func(t *testing.T) {
		for _, route := range dashboardRoutes {
			t.Run(route.label, func(t *testing.T) {
				t.Parallel()
				h := newRenewalHarness(t, 0, renewalIdle, renewalDownstream)
				deadline := h.start.Add(renewalIdle)
				sawCookie := false
				for time.Now().Before(deadline.Add(2 * probeGrace)) {
					now := time.Now()
					w := h.do(route.method, route.path, h.cookie)
					if now.Before(deadline.Add(-probeGrace)) {
						if w.Code == http.StatusUnauthorized {
							t.Fatalf("%s %s rejected a valid session: %d", route.method, route.path, w.Code)
						}
						if len(setCookies(w)) > 0 {
							sawCookie = true
						}
					}
					time.Sleep(renewalPoll)
				}
				survived := h.get("/auth/session").Code == http.StatusNoContent
				if sawCookie != survived {
					t.Fatalf("%s %s: Set-Cookie seen=%t but session survived past the idle deadline=%t", route.method, route.path, sawCookie, survived)
				}
				mu.Lock()
				renews[route.label] = sawCookie
				mu.Unlock()
			})
		}
	})
	if len(renews) != len(dashboardRoutes) {
		t.Fatalf("only %d of %d routes completed", len(renews), len(dashboardRoutes))
	}
	var want, seen bool
	for label, got := range renews {
		if !seen {
			want, seen = got, true
			continue
		}
		if got != want {
			t.Errorf("route %s renews=%t but other routes renew=%t; renewal is no longer uniform: %v", label, got, want, renews)
		}
	}
	t.Logf("authenticated dashboard requests renew the idle deadline: %t (%v)", want, renews)
}

// A live view is one long-lived request. The session is saved when its response
// starts, so it must not keep the session alive afterwards. This documents that
// a dashboard left on a live feed is sent to the login screen once the idle
// timeout passes, even though the stream itself is still open.
func TestLongLivedRequestDoesNotExtendIdleDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(release) }) }
	t.Cleanup(stop)
	h := newRenewalHarness(t, 0, renewalIdle, func(a *authManager) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/stream/") {
				w.WriteHeader(http.StatusOK)
				close(started)
				<-release
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
	r := httptest.NewRequest("GET", "https://nvr.test/api/stream/FRONT", nil)
	r.AddCookie(h.cookie)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.handler.ServeHTTP(httptest.NewRecorder(), r)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream request never reached the downstream handler")
	}
	probeUntilExpired(t, h, time.Now().Add(renewalIdle))
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream request did not finish")
	}
}

// The 8-hour absolute lifetime must end a session even when requests keep
// renewing the idle deadline, and the probe must report it.
func TestSessionProbeHonoursAbsoluteLifetime(t *testing.T) {
	const lifetime = 1500 * time.Millisecond
	h := newRenewalHarness(t, lifetime, time.Minute, renewalDownstream)
	deadline := h.start.Add(lifetime)
	valid, expired := 0, 0
	for time.Now().Before(deadline.Add(2 * probeGrace)) {
		now := time.Now()
		touch := h.get("/touch")
		probe := h.get("/auth/session")
		if c := setCookies(probe); len(c) != 0 {
			t.Fatalf("probe wrote Set-Cookie: %v", c)
		}
		switch {
		case now.Before(deadline.Add(-probeGrace)):
			if touch.Code != http.StatusNoContent || probe.Code != http.StatusNoContent {
				t.Fatalf("before lifetime deadline: touch=%d probe=%d, want 204", touch.Code, probe.Code)
			}
			valid++
		case now.After(deadline.Add(probeGrace)):
			if touch.Code != http.StatusUnauthorized || probe.Code != http.StatusUnauthorized {
				t.Fatalf("after lifetime deadline: touch=%d probe=%d, want 401 (idle renewal outlived the absolute lifetime)", touch.Code, probe.Code)
			}
			expired++
		}
		time.Sleep(200 * time.Millisecond)
	}
	if valid == 0 || expired == 0 {
		t.Fatalf("test did not observe both states: valid=%d expired=%d", valid, expired)
	}
}
