package main

import "testing"

func TestFrontendToastText(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/toast-text.cjs", "UTC")
}
