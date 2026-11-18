package main

import "testing"

func TestFrontendCameraRemovalConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/camera-removal-confirmation.cjs", "UTC")
}
