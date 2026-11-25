package main

import "testing"

func TestFrontendTimelineCameraFilter(t *testing.T) {
 runFrontendRegression(t, "tests/timeline-camera-filter.cjs", "UTC")
}
