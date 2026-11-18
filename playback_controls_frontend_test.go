package main

import "testing"

func TestFrontendPlaybackControls(t *testing.T) {
 runFrontendRegression(t, "tests/playback-controls.cjs", "UTC")
 runFrontendRegression(t, "tests/playback-controls.cjs", "America/Chicago")
}
