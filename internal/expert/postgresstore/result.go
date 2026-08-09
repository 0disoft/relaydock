package postgresstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
	"github.com/0disoft/relaydock/internal/idgen"
)

func (s *Store) putResultRecord(ctx context.Context, record resultcontract.Record) (string, error) {
	record = resultcontract.NormalizeRecord(record)
	if err := resultcontract.Validate(record.Result); err != nil {
		return "", err
	}
	raw, err := json.Marshal(record.Result)
	if err != nil {
		return "", fmt.Errorf("encode expert result: %w", err)
	}
	id := idgen.New("result")
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO expert.consultation_results (
    id, consultation_id, structured_result, model_attestation, created_at
) VALUES ($1, NULL, $2::jsonb, $3, $4)`,
		id, string(raw), record.ModelAttestation, record.CreatedAt,
	); err != nil {
		return "", fmt.Errorf("persist expert result: %w", err)
	}
	return id, nil
}

func (s *Store) getResultRecord(ctx context.Context, id string) (resultcontract.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return resultcontract.Record{}, core.ErrInvalidArgument
	}
	return scanResultRecord(s.db.QueryRowContext(ctx, `
SELECT structured_result, model_attestation, created_at
FROM expert.consultation_results
WHERE id = $1`, id))
}

func scanResultRecord(row rowScanner) (resultcontract.Record, error) {
	var raw []byte
	var record resultcontract.Record
	if err := row.Scan(&raw, &record.ModelAttestation, &record.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return resultcontract.Record{}, core.ErrNotFound
		}
		return resultcontract.Record{}, fmt.Errorf("read expert result: %w", err)
	}
	if err := json.Unmarshal(raw, &record.Result); err != nil {
		return resultcontract.Record{}, fmt.Errorf("decode expert result: %w", err)
	}
	record = resultcontract.NormalizeRecord(record)
	if err := resultcontract.Validate(record.Result); err != nil {
		return resultcontract.Record{}, fmt.Errorf("invalid persisted expert result: %w", err)
	}
	return record, nil
}

func (s *Store) CommitResult(ctx context.Context, consultationID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.CommitResultAttested(ctx, consultationID, resultcontract.UnverifiedAttestation, result)
}

func (s *Store) CommitResultAttested(ctx context.Context, consultationID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.commitResult(ctx, consultationID, "", modelAttestation, result, false)
}

func (s *Store) CommitClaimResult(ctx context.Context, consultationID, workerID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.CommitClaimResultAttested(ctx, consultationID, workerID, resultcontract.UnverifiedAttestation, result)
}

func (s *Store) CommitClaimResultAttested(ctx context.Context, consultationID, workerID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return s.commitResult(ctx, consultationID, workerID, modelAttestation, result, true)
}

func (s *Store) commitResult(ctx context.Context, consultationID, workerID, modelAttestation string, result resultcontract.Result, fenced bool) (consultation.Consultation, string, error) {
	record := resultcontract.NormalizeRecord(resultcontract.Record{Result: result, ModelAttestation: modelAttestation})
	if err := resultcontract.Validate(record.Result); err != nil {
		return consultation.Consultation{}, "", err
	}
	raw, err := json.Marshal(record.Result)
	if err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("encode expert result: %w", err)
	}
	consultationID = strings.TrimSpace(consultationID)
	workerID = strings.TrimSpace(workerID)
	if consultationID == "" || (fenced && workerID == "") {
		return consultation.Consultation{}, "", core.ErrInvalidArgument
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("begin expert result commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := scanConsultation(tx.QueryRowContext(ctx, `
SELECT `+consultationColumns+`
FROM expert.consultations
WHERE id = $1
FOR UPDATE`, consultationID))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, "", core.ErrNotFound
	}
	if err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("lock consultation for result: %w", err)
	}

	if item.State == consultation.StateCompleted && item.ResultID != "" {
		existing, existingErr := scanResultRecord(tx.QueryRowContext(ctx, `
SELECT structured_result, model_attestation, created_at
FROM expert.consultation_results WHERE id = $1`, item.ResultID))
		if existingErr != nil {
			return consultation.Consultation{}, "", existingErr
		}
		existingRaw, _ := json.Marshal(existing.Result)
		if bytes.Equal(existingRaw, raw) && existing.ModelAttestation == record.ModelAttestation {
			if err := tx.Commit(); err != nil {
				return consultation.Consultation{}, "", err
			}
			return item, item.ResultID, nil
		}
		return consultation.Consultation{}, "", core.ErrConflict
	}

	now := time.Now().UTC()
	if item.ExpiresAt.Before(now) || item.ExpiresAt.Equal(now) {
		return consultation.Consultation{}, "", core.ErrConflict
	}
	if fenced {
		if err := consultation.ValidateClaimOwner(item, workerID, now); err != nil {
			return consultation.Consultation{}, "", err
		}
	} else {
		if item.State == consultation.StateQueued {
			item.State = consultation.StateRunning
		}
		if item.State != consultation.StateRunning && item.State != consultation.StateResultPending {
			return consultation.Consultation{}, "", core.ErrConflict
		}
	}

	resultID := idgen.New("result")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO expert.consultation_results (
    id, consultation_id, structured_result, model_attestation, created_at
) VALUES ($1, $2, $3::jsonb, $4, $5)`,
		resultID, consultationID, string(raw), record.ModelAttestation, record.CreatedAt,
	); err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("insert consultation result: %w", err)
	}
	updated, err := scanConsultation(tx.QueryRowContext(ctx, `
UPDATE expert.consultations
SET result_id = $2, state = 'completed', locked_by = NULL, locked_at = NULL,
    lock_expires_at = NULL, updated_at = $3
WHERE id = $1
RETURNING `+consultationColumns, consultationID, resultID, now))
	if err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("complete consultation result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return consultation.Consultation{}, "", fmt.Errorf("commit expert result: %w", err)
	}
	return updated, resultID, nil
}
