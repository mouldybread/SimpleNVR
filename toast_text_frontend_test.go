package main

import "testing"

func TestFrontendToastText(t *testing.T) {
 runFrontendRegression(t, "tests/toast-text.cjs", "UTC")
}
