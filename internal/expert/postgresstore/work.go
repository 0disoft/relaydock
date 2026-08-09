package postgresstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
)

func (s *Store) Claim(ctx context.Context, command consultation.ClaimCommand) (consultation.Consultation, error) {
	normalized, err := consultation.NormalizeClaimCommand(command)
	if err != nil {
		return consultation.Consultation{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("begin consultation claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `SELECT ` + consultationColumns + `
FROM expert.consultations
WHERE state = 'queued'
  AND available_at <= $1
  AND expires_at > $1
  AND attempt_count < $2`
	args := []any{normalized.ClaimedAt, normalized.MaxAttempts}
	if normalized.Route != "" {
		query += fmt.Sprintf(` AND route = $%d`, len(args)+1)
		args = append(args, string(normalized.Route))
	}
	query, args = s.appendScope(query, args)
	query += ` ORDER BY available_at, created_at, id FOR UPDATE SKIP LOCKED LIMIT 1`
	item, err := scanConsultation(tx.QueryRowContext(ctx, query, args...))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrNotFound
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("select consultation claim: %w", err)
	}
	if err := consultation.ApplyClaim(&item, normalized); err != nil {
		return consultation.Consultation{}, err
	}
	updated, err := scanConsultation(tx.QueryRowContext(ctx, `
UPDATE expert.consultations
SET state = 'running', attempt_count = $2, available_at = $3,
    failure_reason = '', locked_by = $4, locked_at = $5,
    lock_expires_at = $6, updated_at = $5
WHERE id = $1 AND state = 'queued'
RETURNING `+consultationColumns,
		item.ID, item.AttemptCount, normalized.ClaimedAt, item.LockedBy,
		item.UpdatedAt, item.LockExpiresAt,
	))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrConflict
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("claim consultation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return consultation.Consultation{}, fmt.Errorf("commit consultation claim: %w", err)
	}
	return updated, nil
}

func (s *Store) RenewClaim(ctx context.Context, id, workerID string, ttl time.Duration) (consultation.Consultation, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || ttl < 5*time.Second || ttl > 30*time.Minute {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	now := time.Now().UTC()
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET lock_expires_at = $3, updated_at = $2
WHERE id = $1 AND state = 'running' AND locked_by = $4
  AND lock_expires_at > $2 AND expires_at > $2
RETURNING `+consultationColumns, id, now, now.Add(ttl), workerID))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrLeaseLost
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("renew consultation claim: %w", err)
	}
	return item, nil
}

func (s *Store) RetryClaim(ctx context.Context, id, workerID string, availableAt time.Time, reason string, maximumAttempts int) (consultation.Consultation, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	if maximumAttempts <= 0 {
		maximumAttempts = 3
	}
	now := time.Now().UTC()
	if availableAt.Before(now) {
		availableAt = now
	}
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET state = CASE WHEN attempt_count >= $5 THEN 'failed' ELSE 'queued' END,
    failure_reason = left($4, 2048),
    available_at = CASE WHEN attempt_count >= $5 THEN $2 ELSE $3 END,
    locked_by = NULL, locked_at = NULL, lock_expires_at = NULL,
    updated_at = $2
WHERE id = $1 AND state = 'running' AND locked_by = $6
  AND lock_expires_at > $2
RETURNING `+consultationColumns,
		id, now, availableAt.UTC(), strings.TrimSpace(reason), maximumAttempts, workerID,
	))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrLeaseLost
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("retry consultation claim: %w", err)
	}
	return item, nil
}

func (s *Store) RecoverStaleClaims(ctx context.Context, now time.Time, maximumAttempts int) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if maximumAttempts <= 0 {
		maximumAttempts = 3
	}
	query := `
UPDATE expert.consultations
SET state = CASE WHEN attempt_count >= $2 THEN 'failed' ELSE 'queued' END,
    failure_reason = 'worker lease expired before completion',
    available_at = $1,
    locked_by = NULL, locked_at = NULL, lock_expires_at = NULL,
    updated_at = $1
WHERE state = 'running' AND lock_expires_at <= $1`
	args := []any{now, maximumAttempts}
	query, args = s.appendScope(query, args)
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("recover stale consultation claims: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (s *Store) AttachClaimResult(ctx context.Context, id, workerID, resultID string) (consultation.Consultation, error) {
	now := time.Now().UTC()
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET result_id = $3, state = 'completed', locked_by = NULL, locked_at = NULL,
    lock_expires_at = NULL, updated_at = $4
WHERE id = $1 AND state = 'running' AND locked_by = $2
  AND lock_expires_at > $4
RETURNING `+consultationColumns,
		id, strings.TrimSpace(workerID), strings.TrimSpace(resultID), now,
	))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrLeaseLost
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("attach claimed result: %w", err)
	}
	return item, nil
}
