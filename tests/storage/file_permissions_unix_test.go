//go:build !windows

package storage_test

import (
	"os"
	"testing"
)

func assertPrivateStateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file is accessible outside owner: %o", info.Mode().Perm())
	}
}
