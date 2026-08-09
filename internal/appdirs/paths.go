package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

const applicationDirectory = "ai-runtime-gateway"

// ConfigDir returns the per-user directory for durable local runtime state.
// An explicit root is useful for portable installs, tests, and isolated CI.
func ConfigDir(explicitRoot string) (string, error) {
	if value := strings.TrimSpace(explicitRoot); value != "" {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("resolve application data directory: %w", err)
		}
		return absolute, nil
	}
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("%w: user configuration directory: %v", core.ErrInvalidConfiguration, err)
	}
	return filepath.Join(base, applicationDirectory), nil
}

func SettingsPath(explicitRoot string) (string, error) {
	directory, err := ConfigDir(explicitRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "settings.json"), nil
}

func ExpertStatePath(explicitRoot string) (string, error) {
	directory, err := ConfigDir(explicitRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "state", "expert.json"), nil
}

func RuntimeStatePath(explicitRoot string) (string, error) {
	directory, err := ConfigDir(explicitRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "state", "runtime.json"), nil
}
