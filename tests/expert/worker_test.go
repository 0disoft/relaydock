package expert_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
	"github.com/your-org/ai-runtime-gateway/internal/expert/worker"
	"github.com/your-org/ai-runtime-gateway/internal/provider"
)

type retryOnceExecutor struct{ calls atomic.Int32 }

func (e *retryOnceExecutor) Execute(context.Context, consultation.Consultation, contextpack.Pack) (resultcontract.Result, error) {
	if e.calls.Add(1) == 1 {
		return resultcontract.Result{}, &provider.UpstreamError{Provider: "test", Class: provider.ErrorClassRateLimit, Message: "retry"}
	}
	return resultcontract.Result{Decision: "keep the durable worker", Confidence: 0.9}, nil
}

func (e *retryOnceExecutor) ModelAttestation(consultation.Consultation) string {
	return "provider=test;model=expert-v1;attestation=test-fixture"
}

func TestWorkerRetriesAndPersistsResult(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	application, err := app.NewLocal(filepath.Join(t.TempDir(), "expert.json"))
	if err != nil {
		t.Fatal(err)
	}
	pack := contextpack.Pack{ID: "ctx_worker", Objective: "test", CreatedAt: time.Now().UTC()}
	if err := application.Packs.Put(ctx, pack); err != nil {
		t.Fatal(err)
	}
	item, err := application.Consultations.Create(ctx, consultation.CreateCommand{
		Objective:     "test worker",
		ContextPackID: pack.ID,
		Route:         consultation.RouteOpenAIAPIPro,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Consultations.Approve(ctx, item.ID); err != nil {
		t.Fatal(err)
	}

	executor := &retryOnceExecutor{}
	runner, err := worker.New(application, executor, worker.Config{
		WorkerID:        "test-worker",
		PollInterval:    5 * time.Millisecond,
		LeaseTTL:        5 * time.Second,
		RenewInterval:   time.Second,
		MaximumAttempts: 3,
		RetryBaseDelay:  5 * time.Millisecond,
		RetryMaxDelay:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(runCtx) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := application.Consultations.Get(ctx, item.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.State == consultation.StateCompleted {
			if current.AttemptCount != 2 {
				t.Fatalf("expected two attempts, got %d", current.AttemptCount)
			}
			result, resultErr := application.Results.Get(ctx, current.ResultID)
			if resultErr != nil {
				t.Fatal(resultErr)
			}
			if result.Decision != "keep the durable worker" {
				t.Fatalf("unexpected result: %#v", result)
			}
			records, ok := application.Results.(resultcontract.RecordStore)
			if !ok {
				t.Fatal("result store does not expose attestation records")
			}
			record, recordErr := records.GetRecord(ctx, current.ResultID)
			if recordErr != nil {
				t.Fatal(recordErr)
			}
			if record.ModelAttestation != "provider=test;model=expert-v1;attestation=test-fixture" {
				t.Fatalf("unexpected model attestation: %q", record.ModelAttestation)
			}
			cancel()
			if runErr := <-done; runErr != nil {
				t.Fatal(runErr)
			}
			return
		}
		if current.State == consultation.StateFailed {
			t.Fatalf("worker failed consultation: %s", current.FailureReason)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatal("worker did not complete consultation")
}

func TestRecoverStaleWorkerLease(t *testing.T) {
	t.Parallel()
	repository := consultation.NewMemoryRepository()
	ctx := context.Background()
	item, err := repository.Create(ctx, consultation.CreateCommand{Objective: "recover", ContextPackID: "ctx", Route: consultation.RouteOpenAIAPIPro})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdateState(ctx, item.ID, consultation.StateApprovalPending, consultation.StateQueued); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed, err := repository.Claim(ctx, consultation.ClaimCommand{Route: consultation.RouteOpenAIAPIPro, WorkerID: "dead-worker", LeaseTTL: 5 * time.Second, ClaimedAt: now, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.State != consultation.StateRunning {
		t.Fatalf("unexpected claim state: %s", claimed.State)
	}
	recovered, err := repository.RecoverStaleClaims(ctx, now.Add(6*time.Second), 3)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("expected one recovered claim, got %d", recovered)
	}
	current, err := repository.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != consultation.StateQueued || current.LockedBy != "" {
		t.Fatalf("unexpected recovered state: %#v", current)
	}
	if _, err := repository.RenewClaim(ctx, item.ID, "dead-worker", 5*time.Second); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("expected stale worker renewal conflict, got %v", err)
	}
}
