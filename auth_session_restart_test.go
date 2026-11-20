package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func restartRequest(h *probeHarness, method string, withCookie bool, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://nvr.test/api/settings/restart", strings.NewReader("{}"))
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

func restartHarness(t *testing.T) (*probeHarness, chan struct{}) {
	t.Helper()
	h := newRenewalHarness(t, 0, 20*time.Minute, renewalDownstream)
	called := make(chan struct{}, 4)
	h.a.restart = func() { called <- struct{}{} }
	return h, called
}

func expectNoRestart(t *testing.T, called chan struct{}) {
	t.Helper()
	select {
	case <-called:
		t.Fatal("restart hook ran")
	case <-time.After(800 * time.Millisecond):
	}
}

func TestRestartOnlyWhenSavedSettingsDiffer(t *testing.T) {
	h, called := restartHarness(t)
	if w := restartRequest(h, "POST", true, settingsJSON); w.Code != http.StatusConflict {
		t.Fatalf("no pending change: got %d (%s), want 409", w.Code, strings.TrimSpace(w.Body.String()))
	}
	expectNoRestart(t, called)
	if w := settingsRequest(h, "POST", `{"idle_minutes":45,"lifetime_minutes":600}`, true, settingsJSON); w.Code != http.StatusOK {
		t.Fatalf("save: %d", w.Code)
	}
	w := restartRequest(h, "POST", true, settingsJSON)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), "restarting") {
		t.Fatalf("got %d (%s), want 202", w.Code, strings.TrimSpace(w.Body.String()))
	}
	select {
	case <-called:
	case <-time.After(3 * time.Second):
		t.Fatal("restart hook did not run")
	}
}

func TestRestartUnavailableWithoutHook(t *testing.T) {
	h := newRenewalHarness(t, 0, 20*time.Minute, renewalDownstream)
	if w := settingsRequest(h, "POST", `{"idle_minutes":45,"lifetime_minutes":600}`, true, settingsJSON); w.Code != http.StatusOK {
		t.Fatalf("save: %d", w.Code)
	}
	if w := restartRequest(h, "POST", true, settingsJSON); w.Code != http.StatusNotImplemented {
		t.Fatalf("got %d, want 501", w.Code)
	}
}

func TestRestartRejectsBadRequests(t *testing.T) {
	h, called := restartHarness(t)
	if w := settingsRequest(h, "POST", `{"idle_minutes":45,"lifetime_minutes":600}`, true, settingsJSON); w.Code != http.StatusOK {
		t.Fatalf("save: %d", w.Code)
	}
	token := map[string]string{"Authorization": "Bearer 0123456789012345678901234567890123456789012", "Content-Type": "application/json"}
	cross := map[string]string{"Content-Type": "application/json", "Origin": "https://evil.invalid"}
	for _, tc := range []struct {
		name, method string
		withCookie   bool
		header       map[string]string
		want         int
	}{
		{"no session", "POST", false, settingsJSON, http.StatusUnauthorized},
		{"event token", "POST", false, token, http.StatusUnauthorized},
		{"get", "GET", true, settingsJSON, http.StatusMethodNotAllowed},
		{"no content type", "POST", true, nil, http.StatusUnsupportedMediaType},
		{"wrong content type", "POST", true, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{"cross-origin", "POST", true, cross, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := restartRequest(h, tc.method, tc.withCookie, tc.header)
			if w.Code != tc.want {
				t.Fatalf("got %d (%s), want %d", w.Code, strings.TrimSpace(w.Body.String()), tc.want)
			}
			if tc.want == http.StatusMethodNotAllowed && w.Header().Get("Allow") != "POST" {
				t.Fatalf("Allow %q", w.Header().Get("Allow"))
			}
		})
	}
	expectNoRestart(t, called)
}
