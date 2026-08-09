package postgres

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/control/snapshot"
	"github.com/your-org/ai-runtime-gateway/internal/core"
)

const runtimeSnapshotAdvisoryLockID int64 = 0x415247534e4150 // "ARGSNAP"

// RuntimeSnapshotStore persists every published signed snapshot in PostgreSQL.
// Watch uses monotonic polling because database/sql does not expose pgx LISTEN
// notifications. Snapshots are immutable; readers always select the highest
// committed revision and can safely miss intermediate revisions.
type RuntimeSnapshotStore struct {
	database     *sql.DB
	pollInterval time.Duration
}

type RuntimeSnapshotStoreOptions struct {
	PollInterval time.Duration
}

func NewRuntimeSnapshotStore(database *sql.DB, options RuntimeSnapshotStoreOptions) (*RuntimeSnapshotStore, error) {
	if database == nil {
		return nil, fmt.Errorf("%w: runtime snapshot database", core.ErrInvalidConfiguration)
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 2 * time.Second
	}
	if options.PollInterval < 100*time.Millisecond || options.PollInterval > 5*time.Minute {
		return nil, fmt.Errorf("%w: snapshot poll interval must be between 100ms and 5m", core.ErrInvalidConfiguration)
	}
	return &RuntimeSnapshotStore{database: database, pollInterval: options.PollInterval}, nil
}

func (s *RuntimeSnapshotStore) Current(ctx context.Context) (snapshot.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return snapshot.Snapshot{}, err
	}
	var (
		revision    int64
		payload     []byte
		signature   []byte
		generatedAt time.Time
		expiresAt   time.Time
	)
	err := s.database.QueryRowContext(ctx, `
SELECT revision, payload, signature, generated_at, expires_at
FROM control.runtime_snapshots
ORDER BY revision DESC
LIMIT 1`).Scan(&revision, &payload, &signature, &generatedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot.Snapshot{}, core.ErrNotFound
	}
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("load current runtime snapshot: %w", err)
	}
	return decodePersistedSnapshot(revision, payload, signature, generatedAt, expiresAt)
}

func (s *RuntimeSnapshotStore) Publish(ctx context.Context, value snapshot.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := snapshot.Validate(value, snapshot.ValidationOptions{Now: time.Now().UTC()}); err != nil {
		return err
	}
	if len(value.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: runtime snapshot requires an Ed25519 signature", core.ErrInvalidArgument)
	}
	unsigned := value
	unsigned.Signature = nil
	payload, err := json.Marshal(unsigned)
	if err != nil {
		return fmt.Errorf("encode runtime snapshot payload: %w", err)
	}

	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin runtime snapshot transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, runtimeSnapshotAdvisoryLockID); err != nil {
		return fmt.Errorf("lock runtime snapshot publisher: %w", err)
	}
	var currentRevision int64
	if err := transaction.QueryRowContext(ctx, `SELECT COALESCE(max(revision), 0) FROM control.runtime_snapshots`).Scan(&currentRevision); err != nil {
		return fmt.Errorf("load runtime snapshot revision: %w", err)
	}
	if value.Revision <= currentRevision {
		return fmt.Errorf("%w: runtime snapshot revision %d is not newer than %d", core.ErrConflict, value.Revision, currentRevision)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO control.runtime_snapshots (revision, payload, signature, generated_at, expires_at)
VALUES ($1, $2, $3, $4, $5)`,
		value.Revision,
		payload,
		append([]byte(nil), value.Signature...),
		value.GeneratedAt.UTC(),
		value.ExpiresAt.UTC(),
	); err != nil {
		return fmt.Errorf("persist runtime snapshot revision %d: %w", value.Revision, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit runtime snapshot revision %d: %w", value.Revision, err)
	}
	return nil
}

func (s *RuntimeSnapshotStore) Watch(ctx context.Context, after int64) (<-chan snapshot.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if after < 0 {
		return nil, fmt.Errorf("%w: snapshot watch revision must not be negative", core.ErrInvalidArgument)
	}
	updates := make(chan snapshot.Snapshot, 1)
	go s.poll(ctx, after, updates)
	return updates, nil
}

func (s *RuntimeSnapshotStore) poll(ctx context.Context, after int64, updates chan<- snapshot.Snapshot) {
	defer close(updates)
	lastRevision := after
	poll := func() bool {
		current, err := s.Current(ctx)
		switch {
		case err == nil && current.Revision > lastRevision:
			select {
			case updates <- current:
				lastRevision = current.Revision
				return true
			case <-ctx.Done():
				return false
			}
		case err == nil, errors.Is(err, core.ErrNotFound):
			return true
		case ctx.Err() != nil:
			return false
		default:
			// A transient database failure must not terminate every connected
			// watcher. Readiness probes still expose the database failure while
			// this loop retries on the next bounded interval.
			return true
		}
	}
	if !poll() {
		return
	}
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !poll() {
				return
			}
		}
	}
}

func decodePersistedSnapshot(revision int64, payload, signature []byte, generatedAt, expiresAt time.Time) (snapshot.Snapshot, error) {
	if revision <= 0 || len(payload) == 0 || len(signature) != ed25519.SignatureSize || generatedAt.IsZero() || expiresAt.IsZero() {
		return snapshot.Snapshot{}, fmt.Errorf("%w: incomplete persisted runtime snapshot", core.ErrCorruptState)
	}
	var value snapshot.Snapshot
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("decode persisted runtime snapshot payload: %w", err)
	}
	if err := ensureSnapshotJSONEOF(decoder); err != nil {
		return snapshot.Snapshot{}, err
	}
	if len(value.Signature) != 0 || value.Revision != revision ||
		!samePostgresTimestamp(value.GeneratedAt, generatedAt) ||
		!samePostgresTimestamp(value.ExpiresAt, expiresAt) {
		return snapshot.Snapshot{}, fmt.Errorf("%w: runtime snapshot columns do not match payload", core.ErrCorruptState)
	}
	value.GeneratedAt = value.GeneratedAt.UTC()
	value.ExpiresAt = value.ExpiresAt.UTC()
	value.Signature = append([]byte(nil), signature...)
	if err := snapshot.Validate(value, snapshot.ValidationOptions{Now: time.Now().UTC()}); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("validate persisted runtime snapshot: %w", err)
	}
	return value, nil
}

func ensureSnapshotJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: multiple JSON values in runtime snapshot", core.ErrCorruptState)
		}
		return fmt.Errorf("decode persisted runtime snapshot trailing data: %w", err)
	}
	return nil
}

func samePostgresTimestamp(left, right time.Time) bool {
	return left.UTC().Truncate(time.Microsecond).Equal(right.UTC().Truncate(time.Microsecond))
}
