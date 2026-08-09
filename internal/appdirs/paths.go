package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/relaydock/internal/core"
)

const (
	applicationDirectory       = "relaydock"
	legacyApplicationDirectory = "ai-runtime-gateway"
)

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
	return configDirFromBase(base), nil
}

func configDirFromBase(base string) string {
	current := filepath.Join(base, applicationDirectory)
	if info, err := os.Stat(current); err == nil && info.IsDir() {
		return current
	}
	legacy := filepath.Join(base, legacyApplicationDirectory)
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		return legacy
	}
	return current
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
