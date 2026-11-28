package main

import "testing"

func TestFrontendRecordingDeletionConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/recording-deletion-confirmation.cjs", "UTC")
}
