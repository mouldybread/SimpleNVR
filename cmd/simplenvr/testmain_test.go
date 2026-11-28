package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain runs every test from the repository root, so the repository-relative
// paths the tests use (web/, tests/, ...) keep resolving after the Go sources
// moved to cmd/simplenvr. It finds the root by walking up to go.mod, so it also
// works when a compiled test binary is started from the root or elsewhere.
func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: getwd:", err)
		os.Exit(2)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "TestMain: go.mod not found above the test directory")
			os.Exit(2)
		}
		dir = parent
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: chdir:", err)
		os.Exit(2)
	}
	os.Exit(m.Run())
}
