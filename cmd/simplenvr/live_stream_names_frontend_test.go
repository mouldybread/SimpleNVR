package main

import "testing"

func TestFrontendLiveStreamNames(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/live-stream-names.cjs", "UTC")
}
