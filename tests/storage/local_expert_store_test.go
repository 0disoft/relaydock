package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/localstore"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
)

func TestLocalExpertStorePersistsCompleteLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "ledger.go")
	if err := os.WriteFile(source, []byte("package ledger\n\nfunc Capture() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state", "expert.json")
	application, err := expertapp.NewLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	created, pack, err := application.CreateWithContext(ctx, contextpack.BuildRequest{
		RepositoryRoot: root,
		Objective:      "Review ledger capture idempotency",
		CandidatePaths: []string{"ledger.go"},
	}, consultation.CreateCommand{
		Objective:      "Review ledger capture idempotency",
		Route:          consultation.RouteOpenAIAPIPro,
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ContextPackID != pack.ID || created.State != consultation.StateApprovalPending {
		t.Fatalf("unexpected consultation: %#v", created)
	}
	if _, err := application.Consultations.Approve(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	result := resultcontract.Result{Decision: "Use an append-only ledger", Confidence: 0.91}
	completed, resultID, err := application.StoreResultAttested(ctx, created.ID, "user_declared:chatgpt_web_handoff", result)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != consultation.StateCompleted || completed.ResultID != resultID {
		t.Fatalf("unexpected completed consultation: %#v", completed)
	}

	reopened, err := expertapp.NewLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reopened.Consultations.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.State != consultation.StateCompleted || persisted.ResultID != resultID || persisted.ContextPackID != pack.ID {
		t.Fatalf("unexpected persisted consultation: %#v", persisted)
	}
	persistedPack, err := reopened.Packs.Get(ctx, pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persistedPack.Evidence) != 1 || persistedPack.Evidence[0].Reference != "ledger.go" {
		t.Fatalf("unexpected persisted pack: %#v", persistedPack)
	}
	persistedResult, err := reopened.Results.Get(ctx, resultID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedResult.Decision != result.Decision {
		t.Fatalf("unexpected persisted result: %#v", persistedResult)
	}
	records, ok := reopened.Results.(resultcontract.RecordStore)
	if !ok {
		t.Fatal("reopened result store does not expose records")
	}
	record, err := records.GetRecord(ctx, resultID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ModelAttestation != "user_declared:chatgpt_web_handoff" {
		t.Fatalf("unexpected persisted attestation: %q", record.ModelAttestation)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file is accessible outside owner: %o", info.Mode().Perm())
	}
}

func TestLocalExpertStoreRejectsIdempotencyKeyReuseWithDifferentPayload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := localstore.Open(filepath.Join(t.TempDir(), "expert.json"))
	if err != nil {
		t.Fatal(err)
	}
	pack := contextpack.Pack{ID: "ctx_1", Objective: "one", CreatedAt: time.Now().UTC()}
	if err := store.ContextPacks().Put(ctx, pack); err != nil {
		t.Fatal(err)
	}
	first := consultation.CreateCommand{TenantID: "tenant", ProjectID: "project", Objective: "one", ContextPackID: pack.ID, IdempotencyKey: "same"}
	if _, err := store.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Objective = "different"
	if _, err := store.Create(ctx, second); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestLocalExpertStorePreservesReferencedContextPack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := localstore.Open(filepath.Join(t.TempDir(), "expert.json"))
	if err != nil {
		t.Fatal(err)
	}
	pack := contextpack.Pack{ID: "ctx_1", Objective: "one", CreatedAt: time.Now().UTC()}
	if err := store.ContextPacks().Put(ctx, pack); err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(ctx, consultation.CreateCommand{Objective: "one", ContextPackID: pack.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateState(ctx, created.ID, consultation.StateApprovalPending, consultation.StateCancelled); err != nil {
		t.Fatal(err)
	}
	if err := store.ContextPacks().Delete(ctx, pack.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected referenced-pack conflict, got %v", err)
	}
}

func TestLocalExpertStoreRefusesCorruptState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "expert.json")
	corrupt := map[string]any{
		"version":       1,
		"consultations": map[string]any{"wrong-map-key": map[string]any{"id": "con_1"}},
		"idempotency":   map[string]any{},
		"contextPacks":  map[string]any{},
		"results":       map[string]any{},
	}
	raw, _ := json.Marshal(corrupt)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localstore.Open(path); !errors.Is(err, core.ErrInvalidConfiguration) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
}
