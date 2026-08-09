package releasepack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const publicModulePath = "github.com/0disoft/relaydock"

type ReadinessReport struct {
	Version string
	Module  string
	Files   int
}

func CheckReleaseReadiness(root, expectedVersion string) (ReadinessReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ReadinessReport{}, err
	}
	expectedVersion = strings.TrimSpace(expectedVersion)
	var blockers []string
	version := readTrimmed(filepath.Join(root, "VERSION"), "VERSION", &blockers)
	if expectedVersion == "" {
		blockers = append(blockers, "expected release version is required")
	} else if version != "" && version != expectedVersion {
		blockers = append(blockers, fmt.Sprintf("VERSION is %q, expected %q", version, expectedVersion))
	}
	module := readTrimmed(filepath.Join(root, "go.mod"), "go.mod", &blockers)
	if module != "" && !strings.HasPrefix(module, "module "+publicModulePath+"\n") && module != "module "+publicModulePath {
		blockers = append(blockers, "go.mod does not declare "+publicModulePath)
	}
	for _, required := range []string{"go.sum", "bun.lock", "LICENSE"} {
		readTrimmed(filepath.Join(root, required), required, &blockers)
	}
	if _, err := os.Stat(filepath.Join(root, "LICENSE-PENDING.md")); err == nil {
		blockers = append(blockers, "LICENSE-PENDING.md must be resolved before release")
	} else if !os.IsNotExist(err) {
		blockers = append(blockers, fmt.Sprintf("inspect LICENSE-PENDING.md: %v", err))
	}
	manifest, manifestErr := Verify(root)
	if manifestErr != nil {
		blockers = append(blockers, "source manifest is not current: "+manifestErr.Error())
	}
	if len(blockers) > 0 {
		return ReadinessReport{}, fmt.Errorf("release readiness failed: %s", strings.Join(blockers, "; "))
	}
	return ReadinessReport{Version: version, Module: publicModulePath, Files: manifest.Files}, nil
}

func readTrimmed(path, label string, blockers *[]string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		*blockers = append(*blockers, fmt.Sprintf("%s is required: %v", label, err))
		return ""
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		*blockers = append(*blockers, label+" must not be empty")
	}
	return value
}
