package hygiene

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicIdentityUsesRelayDock(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	expectations := map[string][]string{
		"go.mod":                                      {"module github.com/0disoft/relaydock"},
		"package.json":                                {`"name": "relaydock"`},
		"frontend/package.json":                       {`"name": "@0disoft/relaydock-desktop"`},
		"web/control-console/package.json":            {`"name": "@0disoft/relaydock-control-console"`},
		"build/config.yml":                            {`productName: "RelayDock"`, `productIdentifier: "com.0disoft.relaydock"`},
		"Taskfile.yml":                                {"APP_NAME: relaydock", "../relaydock-source.zip"},
		".github/workflows/ci.yml":                    {"bin/relaydock.exe"},
		".github/workflows/release.yml":               {"relaydock-$VERSION-source.zip", "bin/relaydock.exe", "dist/relaydock-$env:VERSION-windows-amd64"},
		"frontend/index.html":                         {"<title>RelayDock</title>"},
		"internal/localipc/endpoint_unix.go":          {"relaydock.sock"},
		"internal/localipc/endpoint_windows.go":       {`\\.\pipe\relaydock`},
		"web/control-console/src/routes/+page.svelte": {"RelayDock control plane"},
	}
	for relative, required := range expectations {
		relative, required := relative, required
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatal(err)
			}
			content := string(raw)
			for _, expected := range required {
				if !strings.Contains(content, expected) {
					t.Errorf("%s does not contain %q", relative, expected)
				}
			}
		})
	}
}
