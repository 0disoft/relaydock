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

	if count := strings.Count(workflow, "uses: actions/attest@v4"); count != 12 {
		t.Fatalf("release workflow has %d provenance and SBOM attestation steps, want 12", count)
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

func TestReleaseArtifactsRequirePinnedSBOMs(t *testing.T) {
	t.Parallel()
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)

	for token, want := range map[string]int{
		"uses: anchore/sbom-action@v0.24.0": 6,
		"syft-version: v1.50.0":             6,
		"format: spdx-json":                 6,
		"upload-artifact: false":            6,
		"upload-release-assets: false":      6,
		"sbom-path:":                        6,
	} {
		if count := strings.Count(workflow, token); count != want {
			t.Errorf("release workflow contains %q %d times, want %d", token, count, want)
		}
	}
	for _, required := range []string{
		"dist/generated-contracts-SBOM.spdx.json",
		"dist/source-SBOM.spdx.json",
		"dist/server-${{ matrix.suffix }}-SBOM.spdx.json",
		"dist/web-SBOM.spdx.json",
		"dist/container-${{ matrix.component }}-SBOM.spdx.json",
		"dist/windows-desktop-SBOM.spdx.json",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow does not contain %q", required)
		}
	}
}

func TestReleaseContainerCandidatesStayUnpublished(t *testing.T) {
	t.Parallel()
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)

	for _, required := range []string{
		"uses: docker/setup-buildx-action@v4.1.0",
		"uses: docker/build-push-action@v7.2.0",
		"version: v0.34.1",
		"driver-opts: image=moby/buildkit:v0.30.0",
		"platforms: linux/amd64",
		"push: false",
		"provenance: false",
		"sbom: false",
		"outputs: type=oci,dest=",
		"subject-checksums: dist/SHA256SUMS-container",
		"^sha256:[0-9a-f]{64}$",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow does not contain %q", required)
		}
	}
	if strings.Contains(workflow, "docker/login-action") {
		t.Error("release-candidate workflow must not log in to a container registry")
	}
	for _, dockerfile := range []string{
		"deploy/docker/Dockerfile.gateway",
		"deploy/docker/Dockerfile.control",
		"deploy/docker/Dockerfile.expert",
		"deploy/docker/Dockerfile.outbox",
		"deploy/docker/Dockerfile.ops",
		"deploy/docker/Dockerfile.webhook-sink",
	} {
		if !strings.Contains(workflow, "dockerfile: "+dockerfile) {
			t.Errorf("release workflow does not build %s", dockerfile)
		}
	}
}

func TestReleaseSigningKeyIsIsolatedToFinalJob(t *testing.T) {
	t.Parallel()
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)

	for _, required := range []string{
		"sign-release-checksums:",
		"needs: [validate, contracts, source-archive, server-binaries, web-assets, container-images, windows-desktop]",
		`pattern: "!*.dockerbuild"`,
		"RELAYDOCK_RELEASE_SIGNING_PUBLIC_KEY: ${{ vars.RELAYDOCK_RELEASE_SIGNING_PUBLIC_KEY }}",
		`if [[ "${#checksum_files[@]}" -ne 14 ]]`,
		"releasepack sign-checksums --input",
		"releasepack verify-checksums --input",
		"name: release-signatures",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow does not contain %q", required)
		}
	}
	secretReference := "RELAYDOCK_RELEASE_SIGNING_PRIVATE_KEY: ${{ secrets.RELAYDOCK_RELEASE_SIGNING_PRIVATE_KEY }}"
	if count := strings.Count(workflow, secretReference); count != 1 {
		t.Errorf("release signing private key is exposed to %d workflow locations, want 1", count)
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

func TestContainerBuildsRequireGoChecksums(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	dockerfiles, err := filepath.Glob(filepath.Join(root, "deploy", "docker", "Dockerfile.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dockerfiles) != 6 {
		t.Fatalf("found %d service Dockerfiles, want 6", len(dockerfiles))
	}
	for _, dockerfile := range dockerfiles {
		dockerfile := dockerfile
		t.Run(filepath.Base(dockerfile), func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(dockerfile)
			if err != nil {
				t.Fatal(err)
			}
			content := string(raw)
			for _, required := range []string{
				"ENV GOFLAGS=-mod=readonly",
				"COPY go.mod go.sum ./",
				"internal/buildinfo.Version=${VERSION}",
				"internal/buildinfo.Commit=${COMMIT}",
				"internal/buildinfo.BuildTime=${BUILD_TIME}",
				"org.opencontainers.image.source=\"https://github.com/0disoft/relaydock\"",
				"org.opencontainers.image.version=\"${VERSION}\"",
				"org.opencontainers.image.revision=\"${COMMIT}\"",
				"org.opencontainers.image.created=\"${BUILD_TIME}\"",
				"USER nonroot:nonroot",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("%s does not contain %q", filepath.Base(dockerfile), required)
				}
			}
		})
	}
}
