package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/identifier"
	"github.com/0disoft/relaydock/internal/protocol/stream"
	runtimegateway "github.com/0disoft/relaydock/internal/runtime"
)

type RuntimeJournal struct {
	db *sql.DB
}

func NewRuntimeJournal(db *sql.DB) (*RuntimeJournal, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: runtime journal database", core.ErrInvalidConfiguration)
	}
	return &RuntimeJournal{db: db}, nil
}

func (j *RuntimeJournal) BeginRequest(ctx context.Context, value runtimegateway.JournalRequest) error {
	if err := validatePersistentRequest(value); err != nil {
		return err
	}
	var id string
	err := j.db.QueryRowContext(ctx, `
INSERT INTO runtime.requests (
    id, project_id, virtual_key_id, ingress_protocol, virtual_model,
    price_revision_id, authorization_id, state, client_request_id,
    tenant_id, created_at
) VALUES (
    $1, $2::uuid, $3::uuid, $4, $5, $6, NULLIF($7, ''), 'running', $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
WHERE runtime.requests.project_id = EXCLUDED.project_id
  AND runtime.requests.virtual_key_id = EXCLUDED.virtual_key_id
  AND runtime.requests.ingress_protocol = EXCLUDED.ingress_protocol
  AND runtime.requests.virtual_model = EXCLUDED.virtual_model
  AND runtime.requests.price_revision_id = EXCLUDED.price_revision_id
  AND runtime.requests.client_request_id = EXCLUDED.client_request_id
  AND runtime.requests.tenant_id = EXCLUDED.tenant_id
  AND runtime.requests.authorization_id IS NOT DISTINCT FROM EXCLUDED.authorization_id
  AND runtime.requests.created_at = EXCLUDED.created_at
RETURNING id`,
		value.ID, value.ProjectID, value.VirtualKeyID, value.IngressProtocol,
		value.VirtualModel, value.PriceRevisionID, value.AuthorizationID,
		value.ClientRequestID, value.TenantID, value.StartedAt.UTC(),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("begin runtime request: %w", err)
	}
	return nil
}

func (j *RuntimeJournal) MarkCommitted(ctx context.Context, requestID string, committedAt time.Time) error {
	if strings.TrimSpace(requestID) == "" || committedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	result, err := j.db.ExecContext(ctx, `
UPDATE runtime.requests
SET first_semantic_event_at = COALESCE(first_semantic_event_at, $2),
    state = 'running'
WHERE id = $1 AND state = 'running'`, requestID, committedAt.UTC())
	if err != nil {
		return fmt.Errorf("mark runtime request committed: %w", err)
	}
	return requireAffected(result)
}

func (j *RuntimeJournal) BeginAttempt(ctx context.Context, value runtimegateway.JournalAttempt) error {
	if value.ID == "" || value.RequestID == "" || value.Number <= 0 || value.Provider == "" || value.UpstreamModel == "" || value.StartedAt.IsZero() {
		return core.ErrInvalidArgument
	}
	var id string
	err := j.db.QueryRowContext(ctx, `
INSERT INTO runtime.provider_attempts (
    id, request_id, provider_connection_id, upstream_model, attempt_number,
    state, started_at, provider, account_id, protocol
) VALUES ($1, $2, NULL, $3, $4, 'running', $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
WHERE runtime.provider_attempts.request_id = EXCLUDED.request_id
  AND runtime.provider_attempts.attempt_number = EXCLUDED.attempt_number
  AND runtime.provider_attempts.provider = EXCLUDED.provider
  AND runtime.provider_attempts.account_id = EXCLUDED.account_id
  AND runtime.provider_attempts.upstream_model = EXCLUDED.upstream_model
  AND runtime.provider_attempts.protocol = EXCLUDED.protocol
  AND runtime.provider_attempts.started_at = EXCLUDED.started_at
RETURNING id`,
		value.ID, value.RequestID, value.UpstreamModel, value.Number,
		value.StartedAt.UTC(), value.Provider, value.AccountID, value.Protocol,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("begin provider attempt: %w", err)
	}
	return nil
}

func (j *RuntimeJournal) FinishAttempt(ctx context.Context, value runtimegateway.JournalAttemptResult) error {
	if err := validateAttemptFinish(value); err != nil {
		return err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider attempt transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
UPDATE runtime.provider_attempts
SET state = $3,
    committed = $4,
    error_code = $5,
    error_message = $6,
    completed_at = $7
WHERE id = $1 AND request_id = $2
  AND (
      state = 'running'
      OR (state = $3 AND committed = $4 AND error_code = $5 AND error_message = $6 AND completed_at = $7)
  )`,
		value.ID, value.RequestID, value.State, value.Committed,
		value.ErrorCode, value.Error, value.CompletedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("finish provider attempt: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}

	usageID := "usage_" + value.ID
	var persistedUsage string
	err = tx.QueryRowContext(ctx, `
INSERT INTO runtime.usage_events (
    id, request_id, attempt_id, input_tokens, cache_read_tokens,
    cache_write_tokens, output_tokens, reasoning_tokens, price_revision_id
)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, request.price_revision_id
FROM runtime.requests AS request
WHERE request.id = $2
ON CONFLICT (request_id, attempt_id) DO UPDATE SET id = runtime.usage_events.id
WHERE runtime.usage_events.input_tokens = EXCLUDED.input_tokens
  AND runtime.usage_events.cache_read_tokens = EXCLUDED.cache_read_tokens
  AND runtime.usage_events.cache_write_tokens = EXCLUDED.cache_write_tokens
  AND runtime.usage_events.output_tokens = EXCLUDED.output_tokens
  AND runtime.usage_events.reasoning_tokens = EXCLUDED.reasoning_tokens
RETURNING id`, usageID, value.RequestID, value.ID,
		value.Usage.InputTokens, value.Usage.CacheReadTokens, value.Usage.CacheWriteTokens,
		value.Usage.OutputTokens, value.Usage.ReasoningTokens,
	).Scan(&persistedUsage)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("persist provider usage: %w", err)
	}
	request, err := loadRuntimeRequest(ctx, tx, value.RequestID)
	if err != nil {
		return err
	}
	attempt, err := loadRuntimeAttempt(ctx, tx, value.ID)
	if err != nil {
		return err
	}
	event := runtimegateway.AttemptFinishedEvent{
		SchemaVersion: runtimegateway.RuntimeEventSchemaVersion,
		Request:       request,
		Attempt:       attempt,
		Result:        value,
	}
	if err := enqueueRuntimeEvent(ctx, tx, "runtime.attempt.finished", value.ID, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider attempt: %w", err)
	}
	return nil
}

func (j *RuntimeJournal) FinishRequest(ctx context.Context, value runtimegateway.JournalRequestResult) error {
	if err := validateRequestFinish(value); err != nil {
		return err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime request transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE runtime.requests AS request
SET state = $2,
    attempt_count = $3,
    input_tokens = $4,
    cache_read_tokens = $5,
    cache_write_tokens = $6,
    output_tokens = $7,
    reasoning_tokens = $8,
    error_code = $9,
    error_message = $10,
    completed_at = $11
WHERE request.id = $1
  AND NOT EXISTS (
      SELECT 1 FROM runtime.provider_attempts AS attempt
      WHERE attempt.request_id = request.id AND attempt.state IN ('reserved', 'running')
  )
  AND (
      request.state = 'running'
      OR (
          request.state = $2 AND request.attempt_count = $3
          AND request.input_tokens = $4 AND request.cache_read_tokens = $5
          AND request.cache_write_tokens = $6 AND request.output_tokens = $7
          AND request.reasoning_tokens = $8 AND request.error_code = $9
          AND request.error_message = $10 AND request.completed_at = $11
      )
  )`,
		value.ID, value.State, value.Attempts,
		value.Usage.InputTokens, value.Usage.CacheReadTokens, value.Usage.CacheWriteTokens,
		value.Usage.OutputTokens, value.Usage.ReasoningTokens,
		value.ErrorCode, value.Error, value.CompletedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("finish runtime request: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	request, err := loadRuntimeRequest(ctx, tx, value.ID)
	if err != nil {
		return err
	}
	event := runtimegateway.RequestFinishedEvent{
		SchemaVersion: runtimegateway.RuntimeEventSchemaVersion,
		Request:       request,
		Result:        value,
	}
	if err := enqueueRuntimeEvent(ctx, tx, "runtime.request.finished", value.ID, event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit runtime request: %w", err)
	}
	return nil
}

func enqueueRuntimeEvent(ctx context.Context, tx *sql.Tx, topic, aggregateID string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode runtime outbox event: %w", err)
	}
	eventID := "evt_" + strings.ReplaceAll(topic, ".", "_") + "_" + aggregateID
	var persistedID string
	err = tx.QueryRowContext(ctx, `
INSERT INTO outbox.events (id, topic, aggregate_id, payload)
VALUES ($1, $2, $3, $4::jsonb)
ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
WHERE outbox.events.topic = EXCLUDED.topic
  AND outbox.events.aggregate_id = EXCLUDED.aggregate_id
  AND outbox.events.payload = EXCLUDED.payload
RETURNING id`, eventID, topic, aggregateID, raw).Scan(&persistedID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: outbox event %s has conflicting content", core.ErrConflict, eventID)
	}
	if err != nil {
		return fmt.Errorf("enqueue runtime outbox event: %w", err)
	}
	return nil
}

func loadRuntimeRequest(ctx context.Context, tx *sql.Tx, requestID string) (runtimegateway.JournalRequest, error) {
	var value runtimegateway.JournalRequest
	var authorizationID sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT id, client_request_id, tenant_id, project_id::text, virtual_key_id::text,
       ingress_protocol, virtual_model, price_revision_id, authorization_id, created_at
FROM runtime.requests
WHERE id = $1`, requestID).Scan(
		&value.ID,
		&value.ClientRequestID,
		&value.TenantID,
		&value.ProjectID,
		&value.VirtualKeyID,
		&value.IngressProtocol,
		&value.VirtualModel,
		&value.PriceRevisionID,
		&authorizationID,
		&value.StartedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimegateway.JournalRequest{}, core.ErrNotFound
	}
	if err != nil {
		return runtimegateway.JournalRequest{}, fmt.Errorf("load runtime request for outbox: %w", err)
	}
	if authorizationID.Valid {
		value.AuthorizationID = authorizationID.String
	}
	value.StartedAt = value.StartedAt.UTC()
	return value, nil
}

func loadRuntimeAttempt(ctx context.Context, tx *sql.Tx, attemptID string) (runtimegateway.JournalAttempt, error) {
	var value runtimegateway.JournalAttempt
	err := tx.QueryRowContext(ctx, `
SELECT id, request_id, attempt_number, provider, account_id, upstream_model, protocol, started_at
FROM runtime.provider_attempts
WHERE id = $1`, attemptID).Scan(
		&value.ID,
		&value.RequestID,
		&value.Number,
		&value.Provider,
		&value.AccountID,
		&value.UpstreamModel,
		&value.Protocol,
		&value.StartedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimegateway.JournalAttempt{}, core.ErrNotFound
	}
	if err != nil {
		return runtimegateway.JournalAttempt{}, fmt.Errorf("load runtime attempt for outbox: %w", err)
	}
	value.StartedAt = value.StartedAt.UTC()
	return value, nil
}

func validatePersistentRequest(value runtimegateway.JournalRequest) error {
	if value.ID == "" || !identifier.IsUUID(value.ProjectID) || !identifier.IsUUID(value.VirtualKeyID) || value.IngressProtocol == "" || value.VirtualModel == "" || value.PriceRevisionID == "" || value.StartedAt.IsZero() {
		return fmt.Errorf("%w: persistent runtime request requires UUID project/key IDs and complete metadata", core.ErrInvalidArgument)
	}
	return nil
}

func validateAttemptFinish(value runtimegateway.JournalAttemptResult) error {
	if value.ID == "" || value.RequestID == "" || value.CompletedAt.IsZero() || !validAttemptState(value.State) || !validUsage(value.Usage) {
		return core.ErrInvalidArgument
	}
	return nil
}

func validateRequestFinish(value runtimegateway.JournalRequestResult) error {
	if value.ID == "" || value.CompletedAt.IsZero() || value.Attempts < 0 || !validRequestState(value.State) || !validUsage(value.Usage) {
		return core.ErrInvalidArgument
	}
	return nil
}

func validAttemptState(value string) bool {
	return value == runtimegateway.AttemptStateCompleted || value == runtimegateway.AttemptStateFailed || value == runtimegateway.AttemptStateCancelled
}

func validRequestState(value string) bool {
	return value == runtimegateway.RequestStateCompleted || value == runtimegateway.RequestStateFailed || value == runtimegateway.RequestStateCancelled
}

func validUsage(value stream.Usage) bool {
	return value.InputTokens >= 0 && value.CacheReadTokens >= 0 && value.CacheWriteTokens >= 0 && value.OutputTokens >= 0 && value.ReasoningTokens >= 0
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return core.ErrConflict
	}
	return nil
}
