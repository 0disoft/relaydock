//go:build integration

package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	dbassets "github.com/your-org/ai-runtime-gateway/db"
	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/outbox"
	"github.com/your-org/ai-runtime-gateway/internal/persistence/migrate"
	persistencepostgres "github.com/your-org/ai-runtime-gateway/internal/persistence/postgres"
	"github.com/your-org/ai-runtime-gateway/internal/protocol/stream"
	runtimegateway "github.com/your-org/ai-runtime-gateway/internal/runtime"
)

const disposableDatabaseAcknowledgement = "I_UNDERSTAND_THIS_DATABASE_WILL_BE_TRUNCATED"

func TestRuntimeJournalAndOutboxEndToEnd(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("ARG_TEST_POSTGRES_URL"))
	if databaseURL == "" {
		t.Skip("ARG_TEST_POSTGRES_URL is not set")
	}
	if os.Getenv("ARG_TEST_POSTGRES_ALLOW_RESET") != disposableDatabaseAcknowledgement {
		t.Fatalf("ARG_TEST_POSTGRES_ALLOW_RESET must equal %q; this test truncates product tables", disposableDatabaseAcknowledgement)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := persistencepostgres.OpenSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open disposable PostgreSQL: %v", err)
	}

	migrations, err := migrate.Load(dbassets.Migrations, "migrations")
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	runner, err := migrate.New(database, migrations)
	if err != nil {
		t.Fatalf("construct migration runner: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("apply embedded migrations: %v", err)
	}
	resetProductTables(t, ctx, database)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		resetProductTables(t, cleanupCtx, database)
		if err := database.Close(); err != nil {
			t.Logf("close disposable PostgreSQL: %v", err)
		}
	})

	testPostgresRuntimeSnapshotStore(t, ctx, database)

	organizationID, projectID, virtualKeyID := seedControlIdentity(t, ctx, database)
	if organizationID == "" || projectID == "" || virtualKeyID == "" {
		t.Fatal("seeded control identities must not be empty")
	}

	journal, err := persistencepostgres.NewRuntimeJournal(database)
	if err != nil {
		t.Fatalf("construct runtime journal: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Microsecond)
	request := runtimegateway.JournalRequest{
		ID:              "req_integration_001",
		ClientRequestID: "client-integration-001",
		TenantID:        "tenant-integration",
		ProjectID:       projectID,
		VirtualKeyID:    virtualKeyID,
		IngressProtocol: "openai.responses",
		VirtualModel:    "code-deep",
		PriceRevisionID: "price-integration-v1",
		AuthorizationID: "hold-integration-001",
		StartedAt:       base,
	}
	if err := journal.BeginRequest(ctx, request); err != nil {
		t.Fatalf("begin request: %v", err)
	}
	if err := journal.BeginRequest(ctx, request); err != nil {
		t.Fatalf("repeat identical request must be idempotent: %v", err)
	}
	conflictingRequest := request
	conflictingRequest.VirtualModel = "different-model"
	if err := journal.BeginRequest(ctx, conflictingRequest); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("conflicting request rewrite error = %v, want ErrConflict", err)
	}
	conflictingTenant := request
	conflictingTenant.TenantID = "different-tenant"
	if err := journal.BeginRequest(ctx, conflictingTenant); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("conflicting request tenant error = %v, want ErrConflict", err)
	}
	conflictingAuthorization := request
	conflictingAuthorization.AuthorizationID = "different-hold"
	if err := journal.BeginRequest(ctx, conflictingAuthorization); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("conflicting request authorization error = %v, want ErrConflict", err)
	}

	attempt := runtimegateway.JournalAttempt{
		ID:            "attempt_integration_001",
		RequestID:     request.ID,
		Number:        1,
		Provider:      "openai",
		AccountID:     "account-primary",
		UpstreamModel: "gpt-integration",
		Protocol:      "openai.responses",
		StartedAt:     base.Add(10 * time.Millisecond),
	}
	if err := journal.BeginAttempt(ctx, attempt); err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if err := journal.BeginAttempt(ctx, attempt); err != nil {
		t.Fatalf("repeat identical attempt must be idempotent: %v", err)
	}
	conflictingAttempt := attempt
	conflictingAttempt.Protocol = "anthropic.messages"
	if err := journal.BeginAttempt(ctx, conflictingAttempt); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("conflicting attempt protocol error = %v, want ErrConflict", err)
	}
	if err := journal.MarkCommitted(ctx, request.ID, base.Add(20*time.Millisecond)); err != nil {
		t.Fatalf("mark request committed: %v", err)
	}

	usage := stream.Usage{
		InputTokens:      101,
		CacheReadTokens:  17,
		CacheWriteTokens: 5,
		OutputTokens:     43,
		ReasoningTokens:  13,
	}
	attemptResult := runtimegateway.JournalAttemptResult{
		ID:          attempt.ID,
		RequestID:   request.ID,
		State:       runtimegateway.AttemptStateCompleted,
		Committed:   true,
		Usage:       usage,
		CompletedAt: base.Add(2 * time.Second),
	}
	if err := journal.FinishAttempt(ctx, attemptResult); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}
	if err := journal.FinishAttempt(ctx, attemptResult); err != nil {
		t.Fatalf("repeat identical attempt finish must be idempotent: %v", err)
	}

	requestResult := runtimegateway.JournalRequestResult{
		ID:          request.ID,
		State:       runtimegateway.RequestStateCompleted,
		Usage:       usage,
		Attempts:    1,
		CompletedAt: base.Add(3 * time.Second),
	}
	if err := journal.FinishRequest(ctx, requestResult); err != nil {
		t.Fatalf("finish request: %v", err)
	}
	if err := journal.FinishRequest(ctx, requestResult); err != nil {
		t.Fatalf("repeat identical request finish must be idempotent: %v", err)
	}

	assertPersistedRuntimeLifecycle(t, ctx, database, request.ID, attempt.ID, usage)

	repository, err := persistencepostgres.NewOutboxRepository(database)
	if err != nil {
		t.Fatalf("construct outbox repository: %v", err)
	}
	claimAt := base.Add(4 * time.Second)
	claimed, err := repository.Claim(ctx, outbox.ClaimCommand{
		WorkerID: "worker-integration-a",
		Now:      claimAt,
		Limit:    10,
		LeaseTTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("claim outbox events: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed events = %d, want 2: %#v", len(claimed), claimed)
	}

	var attemptEvent outbox.ClaimedEvent
	var requestEvent outbox.ClaimedEvent
	for _, event := range claimed {
		switch event.Topic {
		case "runtime.attempt.finished":
			attemptEvent = event
			var payload runtimegateway.AttemptFinishedEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode attempt outbox payload: %v", err)
			}
			if payload.SchemaVersion != runtimegateway.RuntimeEventSchemaVersion || payload.Request.ID != request.ID || payload.Attempt.ID != attempt.ID || payload.Result.Usage != usage {
				t.Fatalf("attempt outbox payload does not preserve lifecycle identity: %#v", payload)
			}
		case "runtime.request.finished":
			requestEvent = event
			var payload runtimegateway.RequestFinishedEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode request outbox payload: %v", err)
			}
			if payload.SchemaVersion != runtimegateway.RuntimeEventSchemaVersion || payload.Request.ID != request.ID || payload.Result.Usage != usage {
				t.Fatalf("request outbox payload does not preserve lifecycle identity: %#v", payload)
			}
		default:
			t.Fatalf("unexpected outbox topic %q", event.Topic)
		}
	}
	if attemptEvent.ID == "" || requestEvent.ID == "" {
		t.Fatalf("missing expected outbox events: attempt=%#v request=%#v", attemptEvent, requestEvent)
	}

	publishedAt := claimAt.Add(time.Second)
	if err := repository.MarkPublished(ctx, attemptEvent.ID, "worker-integration-a", publishedAt); err != nil {
		t.Fatalf("publish attempt event inside lease: %v", err)
	}
	if err := repository.MarkPublished(ctx, requestEvent.ID, "worker-integration-a", requestEvent.LockExpiresAt); !errors.Is(err, core.ErrLeaseLost) {
		t.Fatalf("completion exactly at lease expiry error = %v, want ErrLeaseLost", err)
	}

	reclaimedAt := requestEvent.LockExpiresAt.Add(time.Microsecond)
	reclaimed, err := repository.Claim(ctx, outbox.ClaimCommand{
		WorkerID: "worker-integration-b",
		Now:      reclaimedAt,
		Limit:    10,
		LeaseTTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != requestEvent.ID {
		t.Fatalf("reclaimed events = %#v, want only %s", reclaimed, requestEvent.ID)
	}
	if err := repository.MarkPublished(ctx, requestEvent.ID, "worker-integration-b", reclaimedAt.Add(time.Second)); err != nil {
		t.Fatalf("publish reclaimed event: %v", err)
	}

	status, err := repository.Status(ctx, reclaimedAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("query outbox status: %v", err)
	}
	if status.Pending != 0 || status.Locked != 0 || status.Dead != 0 || status.PublishedLastHour != 2 {
		t.Fatalf("unexpected final outbox status: %#v", status)
	}
}

func testPostgresRuntimeSnapshotStore(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	store, err := persistencepostgres.NewRuntimeSnapshotStore(database, persistencepostgres.RuntimeSnapshotStoreOptions{PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("construct PostgreSQL snapshot store: %v", err)
	}
	if _, err := store.Current(ctx); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("empty snapshot store current error = %v, want ErrNotFound", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate snapshot signing key: %v", err)
	}
	signer := snapshot.NewEd25519Signer(privateKey, publicKey)
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	updates, err := store.Watch(watchCtx, 0)
	if err != nil {
		t.Fatalf("watch PostgreSQL snapshots: %v", err)
	}
	generatedAt := time.Now().UTC().Add(123456789 * time.Nanosecond)
	first, err := signer.Sign(ctx, snapshot.Snapshot{
		Revision:    1,
		GeneratedAt: generatedAt,
		ExpiresAt:   generatedAt.Add(time.Hour + 987654321*time.Nanosecond),
		Models: []snapshot.ModelRoute{{
			VirtualModel: "code-deep",
			Candidates:   []string{"openai/gpt-integration"},
			PolicyID:     "integration-policy",
		}},
	})
	if err != nil {
		t.Fatalf("sign first snapshot: %v", err)
	}
	if err := store.Publish(ctx, first); err != nil {
		t.Fatalf("publish first PostgreSQL snapshot: %v", err)
	}
	if err := store.Publish(ctx, first); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("repeat snapshot revision error = %v, want ErrConflict", err)
	}
	persisted, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("load first PostgreSQL snapshot: %v", err)
	}
	if persisted.Revision != first.Revision || !persisted.GeneratedAt.Equal(first.GeneratedAt) || !persisted.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("snapshot round trip lost signed timestamp precision: got=%#v want=%#v", persisted, first)
	}
	if err := signer.Verify(ctx, persisted); err != nil {
		t.Fatalf("verify persisted first snapshot: %v", err)
	}
	select {
	case update := <-updates:
		if update.Revision != 1 {
			t.Fatalf("first watched revision = %d, want 1", update.Revision)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first PostgreSQL snapshot watch event")
	}

	second := first
	second.Revision = 2
	second.GeneratedAt = first.GeneratedAt.Add(time.Minute)
	second.ExpiresAt = second.GeneratedAt.Add(time.Hour)
	second.Signature = nil
	second, err = signer.Sign(ctx, second)
	if err != nil {
		t.Fatalf("sign second snapshot: %v", err)
	}
	if err := store.Publish(ctx, second); err != nil {
		t.Fatalf("publish second PostgreSQL snapshot: %v", err)
	}
	select {
	case update := <-updates:
		if update.Revision != 2 {
			t.Fatalf("second watched revision = %d, want 2", update.Revision)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second PostgreSQL snapshot watch event")
	}
}

func seedControlIdentity(t *testing.T, ctx context.Context, database *sql.DB) (string, string, string) {
	t.Helper()
	var organizationID string
	if err := database.QueryRowContext(ctx, `
INSERT INTO control.organizations (slug, name)
VALUES ('integration-org', 'Integration Organization')
RETURNING id::text`).Scan(&organizationID); err != nil {
		t.Fatalf("insert integration organization: %v", err)
	}
	var projectID string
	if err := database.QueryRowContext(ctx, `
INSERT INTO control.projects (organization_id, slug, name)
VALUES ($1::uuid, 'integration-project', 'Integration Project')
RETURNING id::text`, organizationID).Scan(&projectID); err != nil {
		t.Fatalf("insert integration project: %v", err)
	}
	var virtualKeyID string
	if err := database.QueryRowContext(ctx, `
INSERT INTO control.virtual_keys (project_id, public_id, secret_digest, scopes, allowed_models)
VALUES ($1::uuid, 'integration-public-key', decode('00', 'hex'), ARRAY['runtime:invoke'], ARRAY['code-deep'])
RETURNING id::text`, projectID).Scan(&virtualKeyID); err != nil {
		t.Fatalf("insert integration virtual key: %v", err)
	}
	return organizationID, projectID, virtualKeyID
}

func assertPersistedRuntimeLifecycle(t *testing.T, ctx context.Context, database *sql.DB, requestID, attemptID string, usage stream.Usage) {
	t.Helper()
	var (
		requestState string
		attemptCount int
		inputTokens  int64
		outputTokens int64
		reasoning    int64
		completedAt  sql.NullTime
		semanticAt   sql.NullTime
	)
	if err := database.QueryRowContext(ctx, `
SELECT state, attempt_count, input_tokens, output_tokens, reasoning_tokens,
       completed_at, first_semantic_event_at
FROM runtime.requests
WHERE id = $1`, requestID).Scan(
		&requestState,
		&attemptCount,
		&inputTokens,
		&outputTokens,
		&reasoning,
		&completedAt,
		&semanticAt,
	); err != nil {
		t.Fatalf("query persisted request: %v", err)
	}
	if requestState != runtimegateway.RequestStateCompleted || attemptCount != 1 || inputTokens != usage.InputTokens || outputTokens != usage.OutputTokens || reasoning != usage.ReasoningTokens || !completedAt.Valid || !semanticAt.Valid {
		t.Fatalf("unexpected persisted request state: state=%s attempts=%d input=%d output=%d reasoning=%d completed=%v semantic=%v", requestState, attemptCount, inputTokens, outputTokens, reasoning, completedAt, semanticAt)
	}

	var attemptState string
	var committed bool
	if err := database.QueryRowContext(ctx, `
SELECT state, committed
FROM runtime.provider_attempts
WHERE id = $1`, attemptID).Scan(&attemptState, &committed); err != nil {
		t.Fatalf("query persisted attempt: %v", err)
	}
	if attemptState != runtimegateway.AttemptStateCompleted || !committed {
		t.Fatalf("unexpected persisted attempt: state=%s committed=%v", attemptState, committed)
	}

	var usageRows int
	if err := database.QueryRowContext(ctx, `
SELECT count(*)
FROM runtime.usage_events
WHERE request_id = $1 AND attempt_id = $2`, requestID, attemptID).Scan(&usageRows); err != nil {
		t.Fatalf("query persisted usage: %v", err)
	}
	if usageRows != 1 {
		t.Fatalf("usage rows = %d, want 1", usageRows)
	}
}

func resetProductTables(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	if ctx.Err() != nil {
		return
	}
	const statement = `
TRUNCATE TABLE
    outbox.events,
    runtime.usage_events,
    runtime.provider_attempts,
    runtime.requests,
    expert.consultation_results,
    expert.consultations,
    expert.context_packs,
    control.runtime_snapshots,
    control.model_routes,
    control.provider_connections,
    control.virtual_keys,
    control.projects,
    control.organizations
RESTART IDENTITY CASCADE`
	if _, err := database.ExecContext(ctx, statement); err != nil {
		if t.Failed() {
			t.Logf("truncate disposable PostgreSQL after failed test: %v", err)
			return
		}
		t.Fatalf("truncate disposable PostgreSQL: %v", err)
	}
}

func TestDisposableDatabaseAcknowledgementCannotBeAccidental(t *testing.T) {
	if len(disposableDatabaseAcknowledgement) < 32 || !strings.Contains(disposableDatabaseAcknowledgement, "TRUNCATED") {
		t.Fatalf("reset acknowledgement is too weak: %q", disposableDatabaseAcknowledgement)
	}
	if strings.EqualFold(disposableDatabaseAcknowledgement, "true") {
		t.Fatal("reset acknowledgement must not be a generic boolean")
	}
}
