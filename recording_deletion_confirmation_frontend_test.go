package main

import "testing"

func TestFrontendRecordingDeletionConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/recording-deletion-confirmation.cjs", "UTC")
}
