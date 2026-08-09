package postgresstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
)

func (s *Store) Create(ctx context.Context, command consultation.CreateCommand) (consultation.Consultation, error) {
	if err := ctx.Err(); err != nil {
		return consultation.Consultation{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("begin consultation create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	item, err := s.createInTransaction(ctx, tx, command)
	if err != nil {
		return consultation.Consultation{}, err
	}
	if err := tx.Commit(); err != nil {
		return consultation.Consultation{}, fmt.Errorf("commit consultation create: %w", err)
	}
	return item, nil
}

// CreateWithContext writes the ContextPack and consultation in one PostgreSQL
// transaction. It closes the crash window between a standalone pack insert and
// consultation creation while retaining immutable pack and idempotency rules.
func (s *Store) CreateWithContext(ctx context.Context, pack contextpack.Pack, command consultation.CreateCommand) (consultation.Consultation, contextpack.Pack, error) {
	if err := ctx.Err(); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return consultation.Consultation{}, contextpack.Pack{}, core.ErrInvalidArgument
	}
	command.ContextPackID = pack.ID
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, fmt.Errorf("begin context-bound consultation create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.putPackWithExecutor(ctx, tx, pack); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	item, err := s.createInTransaction(ctx, tx, command)
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	var selectedRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT manifest FROM expert.context_packs WHERE id = $1 AND project_id = $2::uuid`, item.ContextPackID, item.ProjectID).Scan(&selectedRaw); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, fmt.Errorf("read selected context pack: %w", err)
	}
	var selected contextpack.Pack
	if err := json.Unmarshal(selectedRaw, &selected); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, fmt.Errorf("decode selected context pack: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, fmt.Errorf("commit context-bound consultation create: %w", err)
	}
	return item, selected, nil
}

func (s *Store) createInTransaction(ctx context.Context, tx *sql.Tx, command consultation.CreateCommand) (consultation.Consultation, error) {
	tenantID, projectID, err := s.applyScope(command.TenantID, command.ProjectID)
	if err != nil {
		return consultation.Consultation{}, err
	}
	command.TenantID, command.ProjectID = tenantID, projectID
	command.ContextPackID = strings.TrimSpace(command.ContextPackID)
	if command.ContextPackID == "" {
		return consultation.Consultation{}, fmt.Errorf("%w: context pack ID", core.ErrInvalidArgument)
	}
	var packProject, packTenant string
	if err := tx.QueryRowContext(ctx, `SELECT project_id::text, tenant_id FROM expert.context_packs WHERE id = $1`, command.ContextPackID).Scan(&packProject, &packTenant); err != nil {
		if err == sql.ErrNoRows {
			return consultation.Consultation{}, core.ErrNotFound
		}
		return consultation.Consultation{}, fmt.Errorf("read context pack scope: %w", err)
	}
	if packProject != projectID || (tenantID != "" && packTenant != "" && packTenant != tenantID) {
		return consultation.Consultation{}, core.ErrForbidden
	}
	fingerprintText := consultation.RequestFingerprint(command)
	fingerprint, decodeErr := hex.DecodeString(fingerprintText)
	if decodeErr != nil {
		return consultation.Consultation{}, fmt.Errorf("decode consultation fingerprint: %w", decodeErr)
	}
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if idempotencyKey != "" {
		existing, existingFingerprint, found, findErr := findByIdempotency(ctx, tx, projectID, idempotencyKey)
		if findErr != nil {
			return consultation.Consultation{}, findErr
		}
		if found {
			if !bytes.Equal(existingFingerprint, fingerprint) {
				return consultation.Consultation{}, core.ErrConflict
			}
			return existing, nil
		}
	}
	now := time.Now().UTC()
	ttl := command.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	id := idgen.New("con")
	if idempotencyKey == "" {
		idempotencyKey = "__auto__:" + id
	}
	item, err := scanConsultation(tx.QueryRowContext(ctx, `
INSERT INTO expert.consultations (
    id, tenant_id, project_id, context_pack_id, objective, task_type, route,
    state, maximum_cost_minor, idempotency_key, request_fingerprint,
    available_at, expires_at, created_at, updated_at
) VALUES (
    $1, $2, $3::uuid, $4, $5, $6, $7, 'approval_pending', $8, $9, $10,
    $11, $12, $11, $11
)
ON CONFLICT (project_id, idempotency_key) DO NOTHING
RETURNING `+consultationColumns,
		id, tenantID, projectID, command.ContextPackID, strings.TrimSpace(command.Objective),
		strings.TrimSpace(command.TaskType), string(command.Route), command.MaximumCostMinor,
		idempotencyKey, fingerprint, now, now.Add(ttl),
	))
	if err == sql.ErrNoRows {
		existing, existingFingerprint, found, findErr := findByIdempotency(ctx, tx, projectID, idempotencyKey)
		if findErr != nil {
			return consultation.Consultation{}, findErr
		}
		if !found || !bytes.Equal(existingFingerprint, fingerprint) {
			return consultation.Consultation{}, core.ErrConflict
		}
		return existing, nil
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("insert consultation: %w", err)
	}
	return item, nil
}

func findByIdempotency(ctx context.Context, tx *sql.Tx, projectID, idempotencyKey string) (consultation.Consultation, []byte, bool, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+consultationColumns+`, request_fingerprint
FROM expert.consultations
WHERE project_id = $1::uuid AND idempotency_key = $2`, projectID, idempotencyKey)
	var item consultation.Consultation
	var route, state string
	var lockExpires sql.NullTime
	var fingerprint []byte
	err := row.Scan(
		&item.ID, &item.TenantID, &item.ProjectID, &item.Objective, &item.TaskType,
		&route, &state, &item.ContextPackID, &item.MaximumCostMinor, &item.ResultID,
		&item.FailureReason, &item.AttemptCount, &item.AvailableAt, &item.LockedBy,
		&lockExpires, &item.CreatedAt, &item.UpdatedAt, &item.ExpiresAt, &fingerprint,
	)
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, nil, false, nil
	}
	if err != nil {
		return consultation.Consultation{}, nil, false, fmt.Errorf("read idempotent consultation: %w", err)
	}
	item.Route = consultation.Route(route)
	item.State = consultation.State(state)
	if lockExpires.Valid {
		item.LockExpiresAt = lockExpires.Time.UTC()
	}
	return item, fingerprint, true, nil
}
