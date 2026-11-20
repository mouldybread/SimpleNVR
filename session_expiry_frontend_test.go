package main

import "testing"

func TestFrontendSessionExpiry(t *testing.T) {
	runFrontendRegression(t, "tests/session-expiry.cjs", "UTC")
}
