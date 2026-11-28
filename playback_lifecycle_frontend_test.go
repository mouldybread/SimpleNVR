package main

import "testing"

func TestFrontendPlaybackLifecycle(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/playback-lifecycle.cjs", "UTC")
}
