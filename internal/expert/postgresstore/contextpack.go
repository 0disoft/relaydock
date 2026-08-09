package postgresstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
)

func (s *Store) putPack(ctx context.Context, pack contextpack.Pack) error {
	return s.putPackWithExecutor(ctx, s.db, pack)
}

func (s *Store) putPackWithExecutor(ctx context.Context, executor sqlExecutor, pack contextpack.Pack) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return fmt.Errorf("%w: context pack ID", core.ErrInvalidArgument)
	}
	tenantID, projectID, err := s.applyScope(pack.TenantID, pack.ProjectID)
	if err != nil {
		return err
	}
	pack.TenantID, pack.ProjectID = tenantID, projectID
	if pack.CreatedAt.IsZero() {
		pack.CreatedAt = time.Now().UTC()
	} else {
		pack.CreatedAt = pack.CreatedAt.UTC()
	}
	if pack.ExpiresAt.IsZero() || !pack.ExpiresAt.After(pack.CreatedAt) {
		return fmt.Errorf("%w: context pack expiry", core.ErrInvalidArgument)
	}
	manifest, err := json.Marshal(pack)
	if err != nil {
		return fmt.Errorf("encode context pack: %w", err)
	}
	result, err := executor.ExecContext(ctx, `
INSERT INTO expert.context_packs (
    id, tenant_id, project_id, repository_revision, working_tree_digest,
    manifest, expires_at, created_at
) VALUES ($1, $2, $3::uuid, $4, $5, $6::jsonb, $7, $8)
ON CONFLICT (id) DO UPDATE
SET manifest = EXCLUDED.manifest
WHERE expert.context_packs.project_id = EXCLUDED.project_id
  AND expert.context_packs.tenant_id = EXCLUDED.tenant_id
  AND expert.context_packs.manifest = EXCLUDED.manifest`,
		pack.ID, pack.TenantID, pack.ProjectID, pack.Revision, pack.WorkingTreeHash,
		string(manifest), pack.ExpiresAt, pack.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("persist context pack: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect context pack write: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: context pack ID already contains different data", core.ErrConflict)
	}
	return nil
}

func (s *Store) getPack(ctx context.Context, id string) (contextpack.Pack, error) {
	if err := ctx.Err(); err != nil {
		return contextpack.Pack{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return contextpack.Pack{}, core.ErrInvalidArgument
	}
	query := `SELECT manifest FROM expert.context_packs WHERE id = $1`
	args := []any{id}
	if s.scope.ProjectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, s.scope.ProjectID)
	}
	if s.scope.TenantID != "" {
		query += fmt.Sprintf(` AND tenant_id = $%d`, len(args)+1)
		args = append(args, s.scope.TenantID)
	}
	var raw []byte
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return contextpack.Pack{}, core.ErrNotFound
		}
		return contextpack.Pack{}, fmt.Errorf("read context pack: %w", err)
	}
	var pack contextpack.Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return contextpack.Pack{}, fmt.Errorf("decode context pack %s: %w", id, err)
	}
	return pack, nil
}

func (s *Store) deletePack(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return core.ErrInvalidArgument
	}
	var referenced bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM expert.consultations WHERE context_pack_id = $1)`, id).Scan(&referenced); err != nil {
		return fmt.Errorf("check context pack references: %w", err)
	}
	if referenced {
		return core.ErrConflict
	}
	query := `DELETE FROM expert.context_packs WHERE id = $1`
	args := []any{id}
	if s.scope.ProjectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, s.scope.ProjectID)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete context pack: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return core.ErrNotFound
	}
	return nil
}
