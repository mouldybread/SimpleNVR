package main

import "testing"

func TestFrontendCameraRemovalConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/camera-removal-confirmation.cjs", "UTC")
}
