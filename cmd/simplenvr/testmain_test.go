package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain runs every test from the repository root, so the repository-relative
// paths the tests use (web/, tests/, ...) keep resolving after the Go sources
// moved to cmd/simplenvr. It finds the root by walking up to go.mod. When the
// process is not inside the repository (for example a helper process that a
// test starts in a private fixture directory), the working directory is left
// unchanged.
func TestMain(m *testing.M) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: getwd:", err)
		os.Exit(2)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if err := os.Chdir(dir); err != nil {
				fmt.Fprintln(os.Stderr, "TestMain: chdir:", err)
				os.Exit(2)
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	os.Exit(m.Run())
}
