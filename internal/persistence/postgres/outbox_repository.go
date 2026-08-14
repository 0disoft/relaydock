package postgres

/* llmnav/1 module
id=relaydock.outbox.postgres
role=Lease PostgreSQL outbox events to workers and fence publish, retry, release, and dead-letter completion by the active lease owner.
owns=outbox lease persistence|outbox completion fencing|dead-letter administration
excludes=event publishing|retry delay policy
search=claim PostgreSQL outbox|fence worker lease|requeue dead letter
invariant=Concurrent claims skip locked rows and preserve queue order among eligible events.
invariant=A stale or expired worker lease cannot complete an event.
stability=contract
*/

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/outbox"
)

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) (*OutboxRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: outbox database", core.ErrInvalidConfiguration)
	}
	return &OutboxRepository{db: db}, nil
}

func (r *OutboxRepository) Claim(ctx context.Context, command outbox.ClaimCommand) ([]outbox.ClaimedEvent, error) {
	command.WorkerID = strings.TrimSpace(command.WorkerID)
	if command.WorkerID == "" || command.Limit <= 0 || command.LeaseTTL <= 0 || command.Now.IsZero() {
		return nil, core.ErrInvalidArgument
	}
	if command.Limit > 1_000 {
		return nil, fmt.Errorf("%w: outbox claim limit exceeds 1000", core.ErrInvalidArgument)
	}
	expiresAt := command.Now.UTC().Add(command.LeaseTTL)
	rows, err := r.db.QueryContext(ctx, `
WITH candidates AS (
    SELECT id
    FROM outbox.events
    WHERE published_at IS NULL
      AND dead_at IS NULL
      AND available_at <= $2
      AND (
          locked_by IS NULL
          OR lock_expires_at IS NULL
          OR lock_expires_at <= $2
      )
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $3
)
UPDATE outbox.events AS event
SET locked_by = $1,
    locked_at = $2,
    lock_expires_at = $4
FROM candidates
WHERE event.id = candidates.id
RETURNING event.id, event.topic, event.aggregate_id, event.payload,
          event.available_at, event.attempts, event.locked_by,
          event.lock_expires_at`,
		command.WorkerID,
		command.Now.UTC(),
		command.Limit,
		expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	claimed := make([]outbox.ClaimedEvent, 0, command.Limit)
	for rows.Next() {
		var event outbox.ClaimedEvent
		var payload []byte
		if err := rows.Scan(
			&event.ID,
			&event.Topic,
			&event.AggregateID,
			&payload,
			&event.AvailableAt,
			&event.Attempts,
			&event.LockedBy,
			&event.LockExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		if !json.Valid(payload) {
			return nil, fmt.Errorf("%w: outbox event %s contains invalid JSON", core.ErrCorruptState, event.ID)
		}
		event.Payload = append(json.RawMessage(nil), payload...)
		claimed = append(claimed, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}
	return claimed, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID, workerID string, publishedAt time.Time) error {
	if invalidOutboxCompletion(eventID, workerID, publishedAt) {
		return core.ErrInvalidArgument
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE outbox.events
SET published_at = $3,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    last_error = ''
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL`, eventID, workerID, publishedAt.UTC())
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	return requireLeaseAffected(result)
}

func (r *OutboxRepository) Retry(ctx context.Context, eventID, workerID string, failedAt, availableAt time.Time, message string) error {
	if invalidOutboxCompletion(eventID, workerID, failedAt) || availableAt.IsZero() {
		return core.ErrInvalidArgument
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE outbox.events
SET attempts = attempts + 1,
    available_at = $4,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    last_error = $5
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL`, eventID, workerID, failedAt.UTC(), availableAt.UTC(), boundedDatabaseError(message))
	if err != nil {
		return fmt.Errorf("retry outbox event: %w", err)
	}
	return requireLeaseAffected(result)
}

func (r *OutboxRepository) DeadLetter(ctx context.Context, eventID, workerID string, deadAt time.Time, message string) error {
	if invalidOutboxCompletion(eventID, workerID, deadAt) {
		return core.ErrInvalidArgument
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE outbox.events
SET attempts = attempts + 1,
    dead_at = $3,
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL,
    last_error = $4
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL`, eventID, workerID, deadAt.UTC(), boundedDatabaseError(message))
	if err != nil {
		return fmt.Errorf("dead-letter outbox event: %w", err)
	}
	return requireLeaseAffected(result)
}

func (r *OutboxRepository) Release(ctx context.Context, eventID, workerID string, releasedAt, availableAt time.Time) error {
	if invalidOutboxCompletion(eventID, workerID, releasedAt) || availableAt.IsZero() {
		return core.ErrInvalidArgument
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE outbox.events
SET available_at = LEAST(available_at, $4),
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL
WHERE id = $1
  AND locked_by = $2
  AND lock_expires_at > $3
  AND published_at IS NULL
  AND dead_at IS NULL`, eventID, workerID, releasedAt.UTC(), availableAt.UTC())
	if err != nil {
		return fmt.Errorf("release outbox event: %w", err)
	}
	return requireLeaseAffected(result)
}

func requireLeaseAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return core.ErrLeaseLost
	}
	return nil
}

func invalidOutboxCompletion(eventID, workerID string, at time.Time) bool {
	return strings.TrimSpace(eventID) == "" || strings.TrimSpace(workerID) == "" || at.IsZero()
}

func boundedDatabaseError(value string) string {
	value = strings.TrimSpace(value)
	const maximum = 4096
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}

func (r *OutboxRepository) Status(ctx context.Context, now time.Time) (outbox.DeliveryStatus, error) {
	if now.IsZero() {
		return outbox.DeliveryStatus{}, core.ErrInvalidArgument
	}
	var status outbox.DeliveryStatus
	var oldest sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT
    (SELECT count(*) FROM outbox.events WHERE published_at IS NULL AND dead_at IS NULL),
    (SELECT count(*) FROM outbox.events WHERE published_at IS NULL AND dead_at IS NULL AND locked_by IS NOT NULL AND lock_expires_at > $1),
    (SELECT count(*) FROM outbox.events WHERE dead_at IS NOT NULL),
    (SELECT count(*) FROM outbox.events WHERE published_at >= $1 - interval '1 hour'),
    (SELECT min(available_at) FROM outbox.events WHERE published_at IS NULL AND dead_at IS NULL)`, now.UTC()).Scan(
		&status.Pending,
		&status.Locked,
		&status.Dead,
		&status.PublishedLastHour,
		&oldest,
	)
	if err != nil {
		return outbox.DeliveryStatus{}, fmt.Errorf("query outbox status: %w", err)
	}
	if oldest.Valid {
		value := oldest.Time.UTC()
		status.OldestPendingAt = &value
	}
	return status, nil
}

func (r *OutboxRepository) ListDeadLetters(ctx context.Context, limit int) ([]outbox.DeadLetterRecord, error) {
	if limit <= 0 || limit > 1_000 {
		return nil, fmt.Errorf("%w: dead-letter limit must be between 1 and 1000", core.ErrInvalidArgument)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, topic, aggregate_id, attempts, last_error, dead_at, created_at
FROM outbox.events
WHERE dead_at IS NOT NULL
ORDER BY dead_at DESC, id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list outbox dead letters: %w", err)
	}
	defer rows.Close()
	records := make([]outbox.DeadLetterRecord, 0, limit)
	for rows.Next() {
		var record outbox.DeadLetterRecord
		if err := rows.Scan(
			&record.ID,
			&record.Topic,
			&record.AggregateID,
			&record.Attempts,
			&record.LastError,
			&record.DeadAt,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox dead letter: %w", err)
		}
		record.DeadAt = record.DeadAt.UTC()
		record.CreatedAt = record.CreatedAt.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox dead letters: %w", err)
	}
	return records, nil
}

func (r *OutboxRepository) RequeueDeadLetter(ctx context.Context, eventID string, availableAt time.Time, resetAttempts bool) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || availableAt.IsZero() {
		return core.ErrInvalidArgument
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE outbox.events
SET dead_at = NULL,
    available_at = $2,
    attempts = CASE WHEN $3 THEN 0 ELSE attempts END,
    last_error = '',
    locked_by = NULL,
    locked_at = NULL,
    lock_expires_at = NULL
WHERE id = $1
  AND dead_at IS NOT NULL
  AND published_at IS NULL`, eventID, availableAt.UTC(), resetAttempts)
	if err != nil {
		return fmt.Errorf("requeue outbox dead letter: %w", err)
	}
	return requireAffected(result)
}

func (r *OutboxRepository) PurgePublishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if before.IsZero() || limit <= 0 || limit > 100_000 {
		return 0, fmt.Errorf("%w: purge requires a timestamp and limit between 1 and 100000", core.ErrInvalidArgument)
	}
	result, err := r.db.ExecContext(ctx, `
WITH candidates AS (
    SELECT id
    FROM outbox.events
    WHERE published_at IS NOT NULL AND published_at < $1
    ORDER BY published_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT $2
)
DELETE FROM outbox.events AS event
USING candidates
WHERE event.id = candidates.id`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("purge published outbox events: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged outbox events: %w", err)
	}
	return count, nil
}
