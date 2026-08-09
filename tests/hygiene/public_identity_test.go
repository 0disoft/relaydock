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

func TestReleaseArtifactsRequireBuildProvenance(t *testing.T) {
	t.Parallel()
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)

	if count := strings.Count(workflow, "uses: actions/attest@v4"); count != 5 {
		t.Fatalf("release workflow has %d provenance steps, want 5", count)
	}
	for _, required := range []string{
		"id-token: write",
		"attestations: write",
		"artifact-metadata: write",
		"subject-checksums: dist/generated-contracts-SHA256SUMS",
		"subject-checksums: dist/SHA256SUMS-source",
		"subject-checksums: dist/SHA256SUMS",
		"subject-checksums: dist/SHA256SUMS-windows-desktop",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow does not contain %q", required)
		}
	}
}

func TestCIUsesFrozenDependencyResolution(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, relative := range []string{
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
	} {
		relative := relative
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatal(err)
			}
			workflow := string(raw)
			if !strings.Contains(workflow, "GOFLAGS: -mod=readonly") {
				t.Error("workflow does not enforce read-only Go module resolution")
			}
			installCount := 0
			for _, line := range strings.Split(workflow, "\n") {
				if !strings.Contains(line, "bun install") {
					continue
				}
				installCount++
				if !strings.Contains(line, "bun install --frozen-lockfile") {
					t.Errorf("non-frozen Bun install: %s", strings.TrimSpace(line))
				}
				if strings.Contains(line, "--cwd") {
					t.Errorf("workspace install must run from repository root: %s", strings.TrimSpace(line))
				}
			}
			if installCount == 0 {
				t.Error("workflow has no Bun install step")
			}
		})
	}
}
