package expert_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/redaction"
)

func newContextCompiler(t *testing.T) *contextpack.Compiler {
	t.Helper()
	scanner, err := redaction.NewDefaultScanner()
	if err != nil {
		t.Fatalf("create redactor: %v", err)
	}
	return contextpack.NewCompiler(contextpack.NewGitAwareSelector(), scanner)
}

func TestSecretFindingsAreRemovedBeforeUpload(t *testing.T) {
	root := t.TempDir()
	source := `package billing

const openAIKey = "sk-proj-abcdefghijklmnopqrstuvwxyz123456"
const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlMTIzNDU2"
const randomSession = "Q7mF2vK9pR4xT8zN1cL6sW3yH5jB0dAqE9uI2oP7"

// Authorization: Bearer top-secret-session-cookie
`
	privateKey := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----\n"
	if err := os.WriteFile(filepath.Join(root, "ledger.go"), []byte(source+privateKey), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DATABASE_URL=postgres://admin:secret@db/prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := newContextCompiler(t).Build(context.Background(), contextpack.BuildRequest{
		RepositoryRoot: root,
		Objective:      "review billing ledger idempotency",
		CandidatePaths: []string{"ledger.go", ".env"},
	})
	if err != nil {
		t.Fatalf("build ContextPack: %v", err)
	}
	if len(pack.Evidence) != 1 || pack.Evidence[0].Reference != "ledger.go" {
		t.Fatalf("sensitive file was not removed: %+v", pack.Evidence)
	}
	content := pack.Evidence[0].Content
	for _, secret := range []string{"sk-proj-", "eyJhbGci", "top-secret-session-cookie", "BEGIN PRIVATE KEY", "Q7mF2vK9"} {
		if strings.Contains(content, secret) {
			t.Fatalf("secret %q survived redaction: %s", secret, content)
		}
	}
	if pack.RedactionReport.RemovedFiles != 1 {
		t.Fatalf("expected one removed file, got %+v", pack.RedactionReport)
	}
	if pack.RedactionReport.RemovedSegments < 4 {
		t.Fatalf("too few redaction findings: %+v", pack.RedactionReport)
	}
}

func TestContextPackDetectsRepositoryMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ledger.go")
	if err := os.WriteFile(path, []byte("package ledger\n\nfunc Capture() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	compiler := newContextCompiler(t)
	request := contextpack.BuildRequest{
		RepositoryRoot: root,
		Objective:      "review ledger capture",
		CandidatePaths: []string{"ledger.go"},
	}
	before, err := compiler.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	if err := os.WriteFile(path, []byte("package ledger\n\nfunc Capture() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := compiler.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if before.Evidence[0].Digest == after.Evidence[0].Digest {
		t.Fatal("evidence digest did not change after repository mutation")
	}
	if before.ID == after.ID {
		t.Fatal("ContextPack ID did not change after repository mutation")
	}
}

func TestContextPackDoesNotFollowRepositorySymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "outside-secret.go")
	if err := os.WriteFile(secretPath, []byte("package leaked\nconst Secret = \"outside-repository\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.go"), []byte("package safe\nfunc Review() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "linked.go")
	if err := os.Symlink(secretPath, linkPath); err != nil {
		t.Skipf("symbolic links are unavailable in this environment: %v", err)
	}

	pack, err := newContextCompiler(t).Build(context.Background(), contextpack.BuildRequest{
		RepositoryRoot: root,
		Objective:      "review safe repository evidence without following symbolic links",
		CandidatePaths: []string{"safe.go", "linked.go"},
	})
	if err != nil {
		t.Fatalf("build ContextPack: %v", err)
	}
	if len(pack.Evidence) != 1 || pack.Evidence[0].Reference != "safe.go" {
		t.Fatalf("symbolic link escaped repository boundary: %+v", pack.Evidence)
	}
	if strings.Contains(pack.Evidence[0].Content, "outside-repository") {
		t.Fatal("outside repository content entered ContextPack")
	}
}
