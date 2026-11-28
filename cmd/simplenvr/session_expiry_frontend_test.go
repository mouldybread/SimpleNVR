package main

import "testing"

func TestFrontendSessionExpiry(t *testing.T) {
	runFrontendRegression(t, "tests/frontend/session-expiry.cjs", "UTC")
}
