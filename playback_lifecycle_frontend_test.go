package main

import "testing"

func TestFrontendPlaybackLifecycle(t *testing.T) {
 runFrontendRegression(t, "tests/playback-lifecycle.cjs", "UTC")
}
