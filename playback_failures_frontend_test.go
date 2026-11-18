package main

import "testing"

func TestFrontendPlaybackFailures(t *testing.T) {
 runFrontendRegression(t, "tests/playback-failures.cjs", "UTC")
 runFrontendRegression(t, "tests/playback-failures.cjs", "America/Chicago")
}
