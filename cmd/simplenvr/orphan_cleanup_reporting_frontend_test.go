package main

import "testing"

func TestFrontendOrphanCleanupReporting(t *testing.T) {
 runFrontendRegression(t, "tests/frontend/orphan-cleanup-reporting.cjs", "UTC")
}
