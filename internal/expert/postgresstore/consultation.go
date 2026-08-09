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

func (s *Store) Get(ctx context.Context, id string) (consultation.Consultation, error) {
	if err := ctx.Err(); err != nil {
		return consultation.Consultation{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return consultation.Consultation{}, core.ErrInvalidArgument
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE expert.consultations
SET state = 'expired', failure_reason = 'consultation expired', locked_by = NULL,
    locked_at = NULL, lock_expires_at = NULL, updated_at = now()
WHERE id = $1 AND expires_at <= now()
  AND state NOT IN ('completed', 'failed', 'cancelled', 'expired')`, id); err != nil {
		return consultation.Consultation{}, fmt.Errorf("expire consultation: %w", err)
	}
	query := `SELECT ` + consultationColumns + ` FROM expert.consultations WHERE id = $1`
	args := []any{id}
	query, args = s.appendScope(query, args)
	item, err := scanConsultation(s.db.QueryRowContext(ctx, query, args...))
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrNotFound
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("read consultation: %w", err)
	}
	return item, nil
}

func (s *Store) List(ctx context.Context, limit int) ([]consultation.Consultation, error) {
	if s.scope.ProjectID == "" {
		return nil, fmt.Errorf("%w: unscoped PostgreSQL consultation listing", core.ErrForbidden)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE expert.consultations
SET state = 'expired', failure_reason = 'consultation expired', locked_by = NULL,
    locked_at = NULL, lock_expires_at = NULL, updated_at = now()
WHERE project_id = $1::uuid AND expires_at <= now()
  AND state NOT IN ('completed', 'failed', 'cancelled', 'expired')`, s.scope.ProjectID); err != nil {
		return nil, fmt.Errorf("expire consultations: %w", err)
	}
	query := `SELECT ` + consultationColumns + ` FROM expert.consultations WHERE project_id = $1::uuid`
	args := []any{s.scope.ProjectID}
	if s.scope.TenantID != "" {
		query += ` AND tenant_id = $2`
		args = append(args, s.scope.TenantID)
	}
	query += fmt.Sprintf(` ORDER BY updated_at DESC, id DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list consultations: %w", err)
	}
	defer rows.Close()
	items := make([]consultation.Consultation, 0, limit)
	for rows.Next() {
		item, scanErr := scanConsultation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan consultation list: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate consultation list: %w", err)
	}
	return items, nil
}

func (s *Store) UpdateState(ctx context.Context, id string, expected, next consultation.State) (consultation.Consultation, error) {
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET state = $3,
    locked_by = CASE WHEN $3 = 'running' THEN locked_by ELSE NULL END,
    locked_at = CASE WHEN $3 = 'running' THEN locked_at ELSE NULL END,
    lock_expires_at = CASE WHEN $3 = 'running' THEN lock_expires_at ELSE NULL END,
    updated_at = now()
WHERE id = $1 AND state = $2 AND expires_at > now()
RETURNING `+consultationColumns, id, string(expected), string(next)))
	return mapConsultationWrite("update consultation state", item, err)
}

func (s *Store) AttachContext(ctx context.Context, id, packID string, expected, next consultation.State) (consultation.Consultation, error) {
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET context_pack_id = $2, state = $4, updated_at = now()
WHERE id = $1 AND state = $3 AND expires_at > now()
RETURNING `+consultationColumns, id, strings.TrimSpace(packID), string(expected), string(next)))
	return mapConsultationWrite("attach consultation context", item, err)
}

func (s *Store) ClaimNext(ctx context.Context, route string) (consultation.Consultation, error) {
	return s.Claim(ctx, consultation.ClaimCommand{
		Route:    consultation.Route(route),
		WorkerID: "legacy",
		LeaseTTL: 2 * time.Minute,
	})
}

func (s *Store) AttachResult(ctx context.Context, id, resultID string) (consultation.Consultation, error) {
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET result_id = $2, state = 'result_pending', locked_by = NULL, locked_at = NULL,
    lock_expires_at = NULL, updated_at = now()
WHERE id = $1 AND state IN ('running', 'result_pending') AND expires_at > now()
RETURNING `+consultationColumns, id, strings.TrimSpace(resultID)))
	return mapConsultationWrite("attach consultation result", item, err)
}

func (s *Store) SetFailure(ctx context.Context, id, reason string) (consultation.Consultation, error) {
	item, err := scanConsultation(s.db.QueryRowContext(ctx, `
UPDATE expert.consultations
SET state = 'failed', failure_reason = left($2, 2048), locked_by = NULL,
    locked_at = NULL, lock_expires_at = NULL, available_at = now(), updated_at = now()
WHERE id = $1 AND state NOT IN ('completed', 'failed', 'cancelled', 'expired')
RETURNING `+consultationColumns, id, strings.TrimSpace(reason)))
	return mapConsultationWrite("fail consultation", item, err)
}

func (s *Store) appendScope(query string, args []any) (string, []any) {
	if s.scope.ProjectID != "" {
		query += fmt.Sprintf(` AND project_id = $%d::uuid`, len(args)+1)
		args = append(args, s.scope.ProjectID)
	}
	if s.scope.TenantID != "" {
		query += fmt.Sprintf(` AND tenant_id = $%d`, len(args)+1)
		args = append(args, s.scope.TenantID)
	}
	return query, args
}

func mapConsultationWrite(operation string, item consultation.Consultation, err error) (consultation.Consultation, error) {
	if err == sql.ErrNoRows {
		return consultation.Consultation{}, core.ErrConflict
	}
	if err != nil {
		return consultation.Consultation{}, fmt.Errorf("%s: %w", operation, err)
	}
	return item, nil
}
