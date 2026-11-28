package main

import "testing"

func TestFrontendPersistenceResponses(t *testing.T) {
	runFrontendRegression(t, "tests/frontend/persistence-responses.cjs", "UTC")
}
