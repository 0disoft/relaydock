//go:build !windows

package localipc

import (
	"os"
	"path/filepath"
)

func DefaultEndpoint() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "ai-runtime-gateway.sock")
}
