package releasepack

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/updater"
)

func TestUpdaterManifestSigningUsesCanonicalVerifierPayload(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := updater.Manifest{
		Version:      " 0.5.1 ",
		ReleaseNotes: "signed updater test",
		ArtifactURL:  " https://updates.example/relaydock-0.5.1-windows-amd64.zip ",
		SHA256:       " ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789 ",
		Size:         4096,
	}
	signed, err := updater.SignManifest(manifest, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if signed.Version != "0.5.1" || signed.ArtifactURL != "https://updates.example/relaydock-0.5.1-windows-amd64.zip" || signed.SHA256 != strings.ToLower(strings.TrimSpace(manifest.SHA256)) {
		t.Fatalf("manifest was not normalized before signing: %+v", signed)
	}
	if err := updater.VerifyManifestSignature(signed, publicKey); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "update-manifest.json")
	if err := updater.WriteManifestFile(path, signed); err != nil {
		t.Fatal(err)
	}
	loaded, err := updater.ReadManifestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := updater.VerifyManifestSignature(loaded, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := updater.WriteManifestFile(path, signed); err == nil {
		t.Fatal("expected signed updater manifest overwrite rejection")
	}
	loaded.Size++
	if err := updater.VerifyManifestSignature(loaded, publicKey); err == nil {
		t.Fatal("expected updater manifest tamper rejection")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updater.DecodeManifest(append(raw, []byte("{}\n")...)); err == nil {
		t.Fatal("expected trailing updater manifest JSON rejection")
	}
}

func TestReleaseChecksumSignatureRoundTripAndTamperRejection(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	checksums := filepath.Join(t.TempDir(), "SHA256SUMS")
	mustWrite(t, checksums, strings.Repeat("a", 64)+"  relaydock.zip\n")
	envelope, err := SignReleaseChecksums(checksums, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != releaseSignatureSchema || envelope.Algorithm != "Ed25519" || envelope.KeyID == "" {
		t.Fatalf("unexpected release signature envelope: %+v", envelope)
	}
	signaturePath := filepath.Join(t.TempDir(), "SHA256SUMS.sig.json")
	if err := WriteReleaseSignature(signaturePath, envelope); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReleaseSignature(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReleaseChecksums(checksums, loaded, publicKey); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, checksums, strings.Repeat("b", 64)+"  relaydock.zip\n")
	if err := VerifyReleaseChecksums(checksums, loaded, publicKey); err == nil {
		t.Fatal("expected changed checksum file rejection")
	}
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReleaseChecksums(checksums, loaded, otherPublic); err == nil {
		t.Fatal("expected wrong release key rejection")
	}
}

func TestReleaseSigningKeysAndEnvelopeFailClosed(t *testing.T) {
	t.Parallel()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := privateKey.Seed()
	decoded, err := DecodeReleasePrivateKey(base64.RawURLEncoding.EncodeToString(seed))
	if err != nil || !privateKey.Equal(decoded) {
		t.Fatalf("decode release seed: %v", err)
	}
	if _, err := DecodeReleasePrivateKey(base64.RawURLEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected short release key rejection")
	}
	path := filepath.Join(t.TempDir(), "signature.json")
	mustWrite(t, path, `{"schemaVersion":"relaydock.release-checksums-signature/v1","algorithm":"Ed25519","keyId":"x","subjectSha256":"x","signature":"x","unknown":true}`)
	if _, err := ReadReleaseSignature(path); err == nil {
		t.Fatal("expected unknown release signature field rejection")
	}
	mustWrite(t, path, `{"schemaVersion":"relaydock.release-checksums-signature/v1","algorithm":"Ed25519","keyId":"x","subjectSha256":"x","signature":"x"} {}`)
	if _, err := ReadReleaseSignature(path); err == nil {
		t.Fatal("expected trailing release signature JSON rejection")
	}
}

func TestBuildCreatesChunkedManifestAndDeterministicArchive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.0.0-test\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	for index := 0; index < 180; index++ {
		name := filepath.Join(root, "internal", "pkg", formatIndex(index)+".go")
		mustWrite(t, name, "package pkg\n\n// "+strings.Repeat("x", 180)+"\n")
	}
	generatedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	first := filepath.Join(t.TempDir(), "first.zip")
	second := filepath.Join(t.TempDir(), "second.zip")
	firstReport, err := Build(BuildOptions{Root: root, OutputZIP: first, GeneratedAt: generatedAt, ManifestChunkBytes: 4 << 10})
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.ManifestChunks < 2 {
		t.Fatalf("expected multiple manifest chunks, got %d", firstReport.ManifestChunks)
	}
	if _, err := Verify(root); err != nil {
		t.Fatal(err)
	}
	secondReport, err := Build(BuildOptions{Root: root, OutputZIP: second, GeneratedAt: generatedAt, ManifestChunkBytes: 4 << 10})
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.ArchiveSHA256 != secondReport.ArchiveSHA256 {
		t.Fatalf("archive is not deterministic: %s != %s", firstReport.ArchiveSHA256, secondReport.ArchiveSHA256)
	}
	reader, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) == 0 || !strings.HasPrefix(reader.File[0].Name, defaultArchivePrefix+"/") {
		t.Fatalf("unexpected archive prefix: %#v", reader.File)
	}
}

func TestDefaultArchivePrefixUsesRelayDockIdentity(t *testing.T) {
	t.Parallel()
	if defaultArchivePrefix != "relaydock" {
		t.Fatalf("default archive prefix = %q, want relaydock", defaultArchivePrefix)
	}
}

func TestReleaseReadinessFailsClosedAndAcceptsCompleteInputs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.5.1-dev\n")
	mustWrite(t, filepath.Join(root, "go.mod"), "module github.com/0disoft/relaydock\n\ngo 1.26\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	if _, err := CheckReleaseReadiness(root, "0.5.1-dev"); err == nil || !strings.Contains(err.Error(), "go.sum") || !strings.Contains(err.Error(), "bun.lock") || !strings.Contains(err.Error(), "LICENSE") || !strings.Contains(err.Error(), "NOTICE") {
		t.Fatalf("expected missing release input blockers, got %v", err)
	}
	mustWrite(t, filepath.Join(root, "go.sum"), "example.invalid/module v1.0.0 h1:test\n")
	mustWrite(t, filepath.Join(root, "bun.lock"), "{\n  \"lockfileVersion\": 1\n}\n")
	mustWrite(t, filepath.Join(root, "LICENSE"), "test license\n")
	mustWrite(t, filepath.Join(root, "NOTICE"), "test notice\n")
	if _, err := Build(BuildOptions{Root: root, OutputZIP: filepath.Join(t.TempDir(), "repo.zip"), GeneratedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	report, err := CheckReleaseReadiness(root, "0.5.1-dev")
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "0.5.1-dev" || report.Module != publicModulePath || report.Files == 0 {
		t.Fatalf("unexpected readiness report: %+v", report)
	}
}

func TestCanonicalFileModeForOS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		path     string
		mode     os.FileMode
		goos     string
		expected os.FileMode
	}{
		{name: "Windows regular file", path: "README.md", mode: 0o666, goos: "windows", expected: 0o644},
		{name: "Windows shell source", path: "scripts/check.sh", mode: 0o666, goos: "windows", expected: 0o755},
		{name: "POSIX executable", path: "scripts/tool", mode: 0o755, goos: "linux", expected: 0o755},
		{name: "POSIX regular file", path: "config.json", mode: 0o600, goos: "linux", expected: 0o644},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if actual := canonicalFileModeForOS(test.path, test.mode, test.goos); actual != test.expected {
				t.Fatalf("mode = %04o, want %04o", actual, test.expected)
			}
		})
	}
}

func TestAuditRejectsOversizedFileWithoutDocumentedException(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":128,"exceptions":[]}`)
	mustWrite(t, filepath.Join(root, "large.txt"), strings.Repeat("x", 129))
	if _, err := Audit(root, 128); err == nil {
		t.Fatal("expected oversized file rejection")
	}
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":128,"exceptions":[{"path":"large.txt","reason":"binary fixture cannot be split"}]}`)
	if _, err := Audit(root, 128); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDetectsChangedFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.0.0-test\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	path := filepath.Join(root, "README.md")
	mustWrite(t, path, "before\n")
	if _, err := Build(BuildOptions{Root: root, OutputZIP: filepath.Join(t.TempDir(), "repo.zip"), GeneratedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, "after\n")
	if _, err := Verify(root); err == nil {
		t.Fatal("expected manifest mismatch")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func formatIndex(value int) string {
	const digits = "0123456789"
	return string([]byte{digits[(value/100)%10], digits[(value/10)%10], digits[value%10]})
}

func TestVerifyRejectsChangedAggregateHash(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.0.0-test\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	mustWrite(t, filepath.Join(root, "README.md"), "content\n")
	if _, err := Build(BuildOptions{Root: root, OutputZIP: filepath.Join(t.TempDir(), "repo.zip"), GeneratedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, rootManifestPath)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(raw), `"aggregateSha256": "`, `"aggregateSha256": "0`, 1)
	if err := os.WriteFile(manifestPath, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil {
		t.Fatal("expected aggregate hash rejection")
	}
}

func TestBuildReplacesExistingArchiveAndRejectsOutputInsideRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.0.0-test\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	mustWrite(t, filepath.Join(root, "README.md"), "content\n")
	output := filepath.Join(t.TempDir(), "repo.zip")
	mustWrite(t, output, "old archive")
	if _, err := Build(BuildOptions{Root: root, OutputZIP: output, GeneratedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := zip.OpenReader(output); err != nil {
		t.Fatalf("replacement is not a ZIP: %v", err)
	}
	if _, err := Build(BuildOptions{Root: root, OutputZIP: filepath.Join(root, "repo.zip"), GeneratedAt: time.Unix(0, 0).UTC()}); err == nil {
		t.Fatal("expected output-inside-root rejection")
	}
}

func TestAuditRejectsSymlinkInsteadOfSilentlyDroppingIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	mustWrite(t, filepath.Join(root, "target.txt"), "target\n")
	if err := os.Symlink("target.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, err := Audit(root, DefaultMaximumFileBytes); err == nil || !strings.Contains(err.Error(), "unsupported non-regular file") {
		t.Fatalf("expected explicit symlink rejection, got %v", err)
	}
}

func TestPolicyAndManifestRejectTrailingJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]} {}`)
	if _, err := Audit(root, DefaultMaximumFileBytes); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("expected policy trailing JSON rejection, got %v", err)
	}

	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "VERSION"), "0.0.0-test\n")
	mustWrite(t, filepath.Join(root, fileSizeExceptionConfig), `{"version":1,"maxBytes":40960,"exceptions":[]}`)
	if _, err := Build(BuildOptions{Root: root, OutputZIP: filepath.Join(t.TempDir(), "repo.zip"), GeneratedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, rootManifestPath)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(manifest, []byte("{}\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("expected manifest trailing JSON rejection, got %v", err)
	}
}
