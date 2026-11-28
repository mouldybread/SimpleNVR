package main

import "testing"

func TestFrontendTimelineCameraFilter(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/timeline-camera-filter.cjs", "UTC")
}
