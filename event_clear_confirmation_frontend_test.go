package main

import "testing"

func TestFrontendEventClearConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/event-clear-confirmation.cjs", "UTC")
}
