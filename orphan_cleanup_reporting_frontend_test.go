package main

import "testing"

func TestFrontendOrphanCleanupReporting(t *testing.T) {
 runFrontendRegression(t, "tests/orphan-cleanup-reporting.cjs", "UTC")
}
