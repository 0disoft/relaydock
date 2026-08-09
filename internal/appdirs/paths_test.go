package appdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDirFromBasePrefersRelayDockAndPreservesLegacyState(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	current := filepath.Join(base, applicationDirectory)
	legacy := filepath.Join(base, legacyApplicationDirectory)

	if actual := configDirFromBase(base); actual != current {
		t.Fatalf("new install path = %q, want %q", actual, current)
	}
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if actual := configDirFromBase(base); actual != legacy {
		t.Fatalf("legacy state path = %q, want %q", actual, legacy)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if actual := configDirFromBase(base); actual != current {
		t.Fatalf("current state path = %q, want %q", actual, current)
	}
}
