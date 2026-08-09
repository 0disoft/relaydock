package expert_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
)

func TestStaleWorkerCannotCommitAfterLeaseRecovery(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	application, err := expertapp.NewLocal(filepath.Join(t.TempDir(), "expert.json"))
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := application.CreateWithContext(ctx, contextpack.BuildRequest{
		RepositoryRoot: root, Objective: "review main function", CandidatePaths: []string{"main.go"},
	}, consultation.CreateCommand{Objective: "review main function", Route: consultation.RouteOpenAIAPIPro})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = application.Consultations.Approve(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	work := application.Repository.(consultation.WorkRepository)
	oldClaim, err := work.Claim(ctx, consultation.ClaimCommand{
		Route: consultation.RouteOpenAIAPIPro, WorkerID: "worker-old", LeaseTTL: 5 * time.Second,
		ClaimedAt: time.Now().UTC().Add(-10 * time.Second), MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = work.RecoverStaleClaims(ctx, time.Now().UTC(), 3); err != nil {
		t.Fatal(err)
	}
	newClaim, err := work.Claim(ctx, consultation.ClaimCommand{
		Route: consultation.RouteOpenAIAPIPro, WorkerID: "worker-new", LeaseTTL: time.Minute, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if oldClaim.ID != newClaim.ID || newClaim.LockedBy != "worker-new" {
		t.Fatalf("unexpected recovered claim: old=%+v new=%+v", oldClaim, newClaim)
	}
	result := resultcontract.Result{Decision: "new worker owns completion", Confidence: 0.9}
	if _, _, err = application.StoreClaimResult(ctx, created.ID, "worker-old", result); !errors.Is(err, core.ErrLeaseLost) {
		t.Fatalf("stale worker commit must be fenced, got %v", err)
	}
	completed, _, err := application.StoreClaimResult(ctx, created.ID, "worker-new", result)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != consultation.StateCompleted {
		t.Fatalf("state=%s", completed.State)
	}
}
