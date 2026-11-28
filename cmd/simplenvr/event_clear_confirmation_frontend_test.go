package main

import "testing"

func TestFrontendEventClearConfirmation(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/event-clear-confirmation.cjs", "UTC")
}
