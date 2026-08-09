package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/stream"
)

func TestMemoryJournalLifecycleAndIdempotency(t *testing.T) {
	journal := NewMemoryJournal()
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	request := JournalRequest{ID: "req_1", ClientRequestID: "client_1", IngressProtocol: "openai.responses", VirtualModel: "code-deep", StartedAt: now}
	if err := journal.BeginRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := journal.BeginRequest(context.Background(), request); err != nil {
		t.Fatalf("idempotent request start: %v", err)
	}
	attempt := JournalAttempt{ID: "att_1", RequestID: request.ID, Number: 1, Provider: "openai", UpstreamModel: "model", StartedAt: now.Add(time.Second)}
	if err := journal.BeginAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if err := journal.MarkCommitted(context.Background(), request.ID, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	attemptResult := JournalAttemptResult{ID: attempt.ID, RequestID: request.ID, State: AttemptStateCompleted, Committed: true, Usage: stream.Usage{InputTokens: 10, OutputTokens: 3}, CompletedAt: now.Add(3 * time.Second)}
	if err := journal.FinishAttempt(context.Background(), attemptResult); err != nil {
		t.Fatal(err)
	}
	result := JournalRequestResult{ID: request.ID, State: RequestStateCompleted, Usage: attemptResult.Usage, Attempts: 1, CompletedAt: now.Add(4 * time.Second)}
	if err := journal.FinishRequest(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if err := journal.FinishRequest(context.Background(), result); err != nil {
		t.Fatalf("idempotent request finish: %v", err)
	}
	requests, attempts := journal.Snapshot()
	if !requests[request.ID].Finished || !attempts[attempt.ID].Finished {
		t.Fatalf("journal snapshot is incomplete: requests=%+v attempts=%+v", requests, attempts)
	}
}

func TestMemoryJournalRejectsRequestFinishWithRunningAttempt(t *testing.T) {
	journal := NewMemoryJournal()
	now := time.Now().UTC()
	_ = journal.BeginRequest(context.Background(), JournalRequest{ID: "req_1", IngressProtocol: "openai.responses", VirtualModel: "model", StartedAt: now})
	_ = journal.BeginAttempt(context.Background(), JournalAttempt{ID: "att_1", RequestID: "req_1", Number: 1, Provider: "openai", UpstreamModel: "model", StartedAt: now})
	err := journal.FinishRequest(context.Background(), JournalRequestResult{ID: "req_1", State: RequestStateFailed, Attempts: 1, CompletedAt: now})
	if !errors.Is(err, core.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}
