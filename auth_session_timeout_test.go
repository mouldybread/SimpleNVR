package main

import (
	"strings"
	"testing"
	"time"
)

func TestSessionTimeoutsDefaultsAndOverrides(t *testing.T) {
	t.Setenv("NVR_SESSION_IDLE_TIMEOUT", "")
	t.Setenv("NVR_SESSION_LIFETIME", "  ")
	idle, lifetime, err := sessionTimeouts()
	if err != nil || idle != 20*time.Minute || lifetime != 8*time.Hour {
		t.Fatalf("defaults: idle=%s lifetime=%s err=%v", idle, lifetime, err)
	}
	t.Setenv("NVR_SESSION_IDLE_TIMEOUT", "45m")
	t.Setenv("NVR_SESSION_LIFETIME", "12h")
	idle, lifetime, err = sessionTimeouts()
	if err != nil || idle != 45*time.Minute || lifetime != 12*time.Hour {
		t.Fatalf("overrides: idle=%s lifetime=%s err=%v", idle, lifetime, err)
	}
	t.Setenv("NVR_SESSION_IDLE_TIMEOUT", "1m")
	t.Setenv("NVR_SESSION_LIFETIME", "720h")
	if _, _, err = sessionTimeouts(); err != nil {
		t.Fatalf("range limits must be accepted: %v", err)
	}
}

func TestSessionTimeoutsRejectInvalidValues(t *testing.T) {
	for _, tc := range []struct{ name, idle, lifetime, mention string }{
		{"not a duration", "soon", "", "NVR_SESSION_IDLE_TIMEOUT"},
		{"bare number", "20", "", "NVR_SESSION_IDLE_TIMEOUT"},
		{"idle too short", "30s", "", "NVR_SESSION_IDLE_TIMEOUT"},
		{"idle too long", "25h", "", "NVR_SESSION_IDLE_TIMEOUT"},
		{"negative idle", "-5m", "", "NVR_SESSION_IDLE_TIMEOUT"},
		{"lifetime too short", "", "4m", "NVR_SESSION_LIFETIME"},
		{"lifetime too long", "", "721h", "NVR_SESSION_LIFETIME"},
		{"idle exceeds lifetime", "2h", "1h", "must not exceed"},
		{"default idle exceeds short lifetime", "", "10m", "must not exceed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NVR_SESSION_IDLE_TIMEOUT", tc.idle)
			t.Setenv("NVR_SESSION_LIFETIME", tc.lifetime)
			_, _, err := sessionTimeouts()
			if err == nil || !strings.Contains(err.Error(), tc.mention) {
				t.Fatalf("got %v, want an error mentioning %q", err, tc.mention)
			}
		})
	}
}

func TestNewAuthAppliesConfiguredSessionTimeouts(t *testing.T) {
	t.Setenv("NVR_SESSION_IDLE_TIMEOUT", "45m")
	t.Setenv("NVR_SESSION_LIFETIME", "12h")
	a, _ := authFixture(t)
	if a.sessions.IdleTimeout != 45*time.Minute || a.sessions.Lifetime != 12*time.Hour {
		t.Fatalf("idle=%s lifetime=%s", a.sessions.IdleTimeout, a.sessions.Lifetime)
	}
	t.Setenv("NVR_SESSION_IDLE_TIMEOUT", "forever")
	if _, err := newAuth(a.path); err == nil || !strings.Contains(err.Error(), "NVR_SESSION_IDLE_TIMEOUT") {
		t.Fatalf("invalid timeout accepted by newAuth: %v", err)
	}
}
