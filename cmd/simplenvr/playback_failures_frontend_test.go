package main

import "testing"

func TestFrontendPlaybackFailures(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/playback-failures.cjs", "UTC")
 runFrontendRegression(t, "tests/frontend/playback-failures.cjs", "America/Chicago")
}
