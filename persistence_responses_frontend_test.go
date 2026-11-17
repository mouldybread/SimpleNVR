package main

import "testing"

func TestFrontendPersistenceResponses(t *testing.T) {
	runFrontendRegression(t, "tests/persistence-responses.cjs", "UTC")
}
