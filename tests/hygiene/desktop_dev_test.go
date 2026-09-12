package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Wails task dry-run separately validates YAML and template expansion.
func TestDesktopDevConfigHasRunnableStages(t *testing.T) {
	root := filepath.Join("..", "..")
	for path, required := range map[string][]string{
		"build/config.yml": {
			"root_path: .", "watched_extension:", "git_ignore: true",
			"cmd: wails3 task dev:build\n      type: blocking",
			"cmd: wails3 task dev:frontend\n      type: background",
			"cmd: wails3 task dev:run\n      type: primary",
		},
		"Taskfile.yml": {
			"  dev:build:", "  dev:frontend:", "  dev:run:",
			"go build -buildvcs=false -o bin/relaydock-dev",
			"--host 127.0.0.1 --port {{.WAILS_VITE_PORT",
			"./bin/relaydock-dev",
		},
	} {
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		for _, value := range required {
			if !strings.Contains(text, value) {
				t.Errorf("%s is missing %q", path, value)
			}
		}
	}
}
