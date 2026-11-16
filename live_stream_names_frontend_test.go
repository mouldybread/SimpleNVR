package main

import "testing"

func TestFrontendLiveStreamNames(t *testing.T) {
 runFrontendRegression(t, "tests/live-stream-names.cjs", "UTC")
}
