package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/consultation"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/expert/localstore"
	"github.com/0disoft/relaydock/internal/expert/postgresstore"
	"github.com/0disoft/relaydock/internal/expert/redaction"
	"github.com/0disoft/relaydock/internal/expert/resultcontract"
)

type App struct {
	Consultations *consultation.Service
	Repository    consultation.Repository
	Context       *contextpack.Compiler
	Packs         contextpack.Store
	Results       resultcontract.Store
}

type atomicContextCreator interface {
	CreateWithContext(context.Context, contextpack.Pack, consultation.CreateCommand) (consultation.Consultation, contextpack.Pack, error)
}

type atomicResultCommitter interface {
	CommitResult(context.Context, string, resultcontract.Result) (consultation.Consultation, string, error)
}

type atomicAttestedResultCommitter interface {
	CommitResultAttested(context.Context, string, string, resultcontract.Result) (consultation.Consultation, string, error)
}

type atomicClaimResultCommitter interface {
	CommitClaimResult(context.Context, string, string, resultcontract.Result) (consultation.Consultation, string, error)
}

type atomicAttestedClaimResultCommitter interface {
	CommitClaimResultAttested(context.Context, string, string, string, resultcontract.Result) (consultation.Consultation, string, error)
}

func NewInMemory() (*App, error) {
	repository := consultation.NewMemoryRepository()
	return newApp(repository, contextpack.NewMemoryStore(), resultcontract.NewMemoryStore())
}

// NewLocal opens the durable single-user store used by desktop and headless
// runtimes. All three expert aggregates share one atomic state file so a
// consultation can never reference a context pack or result that was only
// partially persisted.
func NewLocal(path string) (*App, error) {
	store, err := localstore.Open(path)
	if err != nil {
		return nil, err
	}
	return newApp(store, store.ContextPacks(), store.Results())
}

// NewPostgres creates the managed-server expert application. Scope should be
// set for a single-tenant deployment; an empty scope is reserved for trusted
// cluster workers and deliberately disables unscoped List operations.
func NewPostgres(db *sql.DB, tenantID, projectID string) (*App, error) {
	store, err := postgresstore.New(db, postgresstore.Scope{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	return newApp(store, store.ContextPacks(), store.Results())
}

func newApp(repository consultation.Repository, packs contextpack.Store, results resultcontract.Store) (*App, error) {
	if repository == nil || packs == nil || results == nil {
		return nil, fmt.Errorf("%w: expert stores", core.ErrInvalidConfiguration)
	}
	redactor, err := redaction.NewDefaultScanner()
	if err != nil {
		return nil, err
	}
	return &App{
		Consultations: consultation.NewService(repository),
		Repository:    repository,
		Context:       contextpack.NewCompiler(contextpack.NewGitAwareSelector(), redactor),
		Packs:         packs,
		Results:       results,
	}, nil
}

func (a *App) PreviewContext(ctx context.Context, request contextpack.BuildRequest) (contextpack.Pack, error) {
	if err := a.validate(); err != nil {
		return contextpack.Pack{}, err
	}
	return a.Context.Preview(ctx, request)
}

func (a *App) BuildContext(ctx context.Context, request contextpack.BuildRequest) (contextpack.Pack, error) {
	if err := a.validate(); err != nil {
		return contextpack.Pack{}, err
	}
	pack, err := a.Context.Build(ctx, request)
	if err != nil {
		return contextpack.Pack{}, err
	}
	if err := a.Packs.Put(ctx, pack); err != nil {
		return contextpack.Pack{}, err
	}
	return pack, nil
}

func (a *App) CreateWithContext(ctx context.Context, build contextpack.BuildRequest, command consultation.CreateCommand) (consultation.Consultation, contextpack.Pack, error) {
	if build.TenantID == "" {
		build.TenantID = command.TenantID
	}
	if build.ProjectID == "" {
		build.ProjectID = command.ProjectID
	}
	if build.TTL <= 0 {
		build.TTL = command.TTL
	}
	pack, err := a.Context.Build(ctx, build)
	if err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	if creator, ok := a.Repository.(atomicContextCreator); ok {
		return creator.CreateWithContext(ctx, pack, command)
	}
	if err := a.Packs.Put(ctx, pack); err != nil {
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	command.ContextPackID = pack.ID
	created, err := a.Consultations.Create(ctx, command)
	if err != nil {
		_ = a.Packs.Delete(context.Background(), pack.ID)
		return consultation.Consultation{}, contextpack.Pack{}, err
	}
	if created.ContextPackID != pack.ID {
		_ = a.Packs.Delete(context.Background(), pack.ID)
		existing, getErr := a.Packs.Get(ctx, created.ContextPackID)
		if getErr != nil {
			return consultation.Consultation{}, contextpack.Pack{}, getErr
		}
		return created, existing, nil
	}
	return created, pack, nil
}

// StoreClaimResult is the compatibility worker completion path. New workers
// should call StoreClaimResultAttested so the provider/model provenance is
// persisted together with the structured decision.
func (a *App) StoreClaimResult(ctx context.Context, consultationID, workerID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return a.StoreClaimResultAttested(ctx, consultationID, workerID, resultcontract.UnverifiedAttestation, result)
}

// StoreClaimResultAttested is the worker-only completion path. It requires
// proof of the active lease and keeps the result plus attestation in the same
// atomic repository transaction whenever the repository supports it.
func (a *App) StoreClaimResultAttested(ctx context.Context, consultationID, workerID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	if err := a.validate(); err != nil {
		return consultation.Consultation{}, "", err
	}
	record := resultcontract.NormalizeRecord(resultcontract.Record{Result: result, ModelAttestation: modelAttestation})
	if committer, ok := a.Repository.(atomicAttestedClaimResultCommitter); ok {
		return committer.CommitClaimResultAttested(ctx, consultationID, workerID, record.ModelAttestation, record.Result)
	}
	if record.ModelAttestation == resultcontract.UnverifiedAttestation {
		if committer, ok := a.Repository.(atomicClaimResultCommitter); ok {
			return committer.CommitClaimResult(ctx, consultationID, workerID, record.Result)
		}
	}
	attacher, ok := a.Repository.(consultation.ClaimResultAttacher)
	if !ok {
		return consultation.Consultation{}, "", fmt.Errorf("%w: repository cannot fence worker result commits", core.ErrInvalidConfiguration)
	}
	id, err := putResultRecord(ctx, a.Results, record)
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	updated, err := attacher.AttachClaimResult(ctx, consultationID, workerID, id)
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	return updated, id, nil
}

func (a *App) StoreResult(ctx context.Context, consultationID string, result resultcontract.Result) (consultation.Consultation, string, error) {
	return a.StoreResultAttested(ctx, consultationID, resultcontract.UnverifiedAttestation, result)
}

// StoreResultAttested stores a manual or synchronous expert decision together
// with its provenance. Local durable and PostgreSQL repositories implement the
// atomic committer so a completed consultation cannot point at a missing
// result after a process crash.
func (a *App) StoreResultAttested(ctx context.Context, consultationID, modelAttestation string, result resultcontract.Result) (consultation.Consultation, string, error) {
	if err := a.validate(); err != nil {
		return consultation.Consultation{}, "", err
	}
	record := resultcontract.NormalizeRecord(resultcontract.Record{Result: result, ModelAttestation: modelAttestation})
	if committer, ok := a.Repository.(atomicAttestedResultCommitter); ok {
		return committer.CommitResultAttested(ctx, consultationID, record.ModelAttestation, record.Result)
	}
	if record.ModelAttestation == resultcontract.UnverifiedAttestation {
		if committer, ok := a.Repository.(atomicResultCommitter); ok {
			return committer.CommitResult(ctx, consultationID, record.Result)
		}
	}
	current, err := a.Consultations.Get(ctx, consultationID)
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	if current.State == consultation.StateQueued {
		if _, err := a.Consultations.Advance(ctx, consultationID, consultation.EventStarted); err != nil {
			return consultation.Consultation{}, "", err
		}
	}
	id, err := putResultRecord(ctx, a.Results, record)
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	updated, err := a.Consultations.SubmitResult(ctx, consultationID, id)
	if err != nil {
		return consultation.Consultation{}, "", err
	}
	return updated, id, nil
}

func putResultRecord(ctx context.Context, store resultcontract.Store, record resultcontract.Record) (string, error) {
	if records, ok := store.(resultcontract.RecordStore); ok {
		return records.PutRecord(ctx, record)
	}
	if record.ModelAttestation != resultcontract.UnverifiedAttestation {
		return "", fmt.Errorf("%w: result store cannot preserve model attestation", core.ErrInvalidConfiguration)
	}
	return store.Put(ctx, record.Result)
}

func (a *App) validate() error {
	if a == nil || a.Consultations == nil || a.Repository == nil || a.Context == nil || a.Packs == nil || a.Results == nil {
		return fmt.Errorf("%w: expert application", core.ErrInvalidConfiguration)
	}
	return nil
}
