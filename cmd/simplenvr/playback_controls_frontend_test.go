package main

import "testing"

func TestFrontendPlaybackControls(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/playback-controls.cjs", "UTC")
 runFrontendRegression(t, "tests/frontend/playback-controls.cjs", "America/Chicago")
}
