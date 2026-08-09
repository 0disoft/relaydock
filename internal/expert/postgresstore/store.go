package postgresstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/your-org/ai-runtime-gateway/internal/core"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
)

// Scope limits administrative listing and worker claims to one tenant/project.
// Empty scope is accepted for a trusted cluster-wide worker, but List rejects
// an unscoped call so an HTTP handler cannot accidentally enumerate tenants.
type Scope struct {
	TenantID  string
	ProjectID string
}

type Store struct {
	db    *sql.DB
	scope Scope
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func New(db *sql.DB, scope Scope) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: PostgreSQL database", core.ErrInvalidConfiguration)
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ProjectID = strings.TrimSpace(scope.ProjectID)
	return &Store{db: db, scope: scope}, nil
}

func (s *Store) ContextPacks() contextpack.Store { return contextPackStore{store: s} }
func (s *Store) Results() resultcontract.Store   { return resultStore{store: s} }

type contextPackStore struct{ store *Store }
type resultStore struct{ store *Store }

func (a contextPackStore) Put(ctx context.Context, pack contextpack.Pack) error {
	return a.store.putPack(ctx, pack)
}

func (a contextPackStore) Get(ctx context.Context, id string) (contextpack.Pack, error) {
	return a.store.getPack(ctx, id)
}

func (a contextPackStore) Delete(ctx context.Context, id string) error {
	return a.store.deletePack(ctx, id)
}

func (a resultStore) Put(ctx context.Context, result resultcontract.Result) (string, error) {
	return a.store.putResultRecord(ctx, resultcontract.Record{
		Result:           result,
		ModelAttestation: resultcontract.UnverifiedAttestation,
	})
}

func (a resultStore) Get(ctx context.Context, id string) (resultcontract.Result, error) {
	record, err := a.store.getResultRecord(ctx, id)
	return record.Result, err
}

func (a resultStore) PutRecord(ctx context.Context, record resultcontract.Record) (string, error) {
	return a.store.putResultRecord(ctx, record)
}

func (a resultStore) GetRecord(ctx context.Context, id string) (resultcontract.Record, error) {
	return a.store.getResultRecord(ctx, id)
}

func (s *Store) applyScope(tenantID, projectID string) (string, string, error) {
	tenantID = strings.TrimSpace(tenantID)
	projectID = strings.TrimSpace(projectID)
	if s.scope.TenantID != "" {
		if tenantID != "" && tenantID != s.scope.TenantID {
			return "", "", core.ErrForbidden
		}
		tenantID = s.scope.TenantID
	}
	if s.scope.ProjectID != "" {
		if projectID != "" && projectID != s.scope.ProjectID {
			return "", "", core.ErrForbidden
		}
		projectID = s.scope.ProjectID
	}
	if projectID == "" {
		return "", "", fmt.Errorf("%w: project ID is required for PostgreSQL expert storage", core.ErrInvalidArgument)
	}
	return tenantID, projectID, nil
}
