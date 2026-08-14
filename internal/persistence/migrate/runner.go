package migrate

/* llmnav/1 module
id=relaydock.persistence.migrations
role=Apply or roll back embedded PostgreSQL migrations in version order under one advisory lock with checksum validation.
owns=migration serialization|migration checksum ledger|transactional migration steps
excludes=migration SQL definitions|database backup policy
search=run PostgreSQL migrations|validate migration checksum|rollback migration steps
invariant=Only one runner mutates the schema while the advisory lock is held.
invariant=Each migration step and its ledger update commit in the same transaction.
stability=contract
*/

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

const advisoryLockID int64 = 0x4152474d494752 // "ARGMIGR"

type Applied struct {
	Version   int64
	Name      string
	Checksum  string
	AppliedAt time.Time
}

type Status struct {
	Migration Migration
	Applied   *Applied
}

type Runner struct {
	database   *sql.DB
	migrations []Migration
}

func New(database *sql.DB, migrations []Migration) (*Runner, error) {
	if database == nil {
		return nil, fmt.Errorf("migration database is required")
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("at least one migration is required")
	}
	copyOfMigrations := append([]Migration(nil), migrations...)
	sort.Slice(copyOfMigrations, func(i, j int) bool { return copyOfMigrations[i].Version < copyOfMigrations[j].Version })
	return &Runner{database: database, migrations: copyOfMigrations}, nil
}

func (r *Runner) Up(ctx context.Context) ([]Applied, error) {
	connection, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	appliedByVersion, err := loadApplied(ctx, connection)
	if err != nil {
		return nil, err
	}
	if err := validateApplied(r.migrations, appliedByVersion); err != nil {
		return nil, err
	}
	appliedNow := make([]Applied, 0)
	for _, migration := range r.migrations {
		if _, exists := appliedByVersion[migration.Version]; exists {
			continue
		}
		transaction, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return appliedNow, fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}
		if _, err := transaction.ExecContext(ctx, migration.UpSQL); err != nil {
			_ = transaction.Rollback()
			return appliedNow, fmt.Errorf("apply migration %d_%s: %w", migration.Version, migration.Name, err)
		}
		appliedAt := time.Now().UTC()
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO public.schema_migrations (version, name, checksum, applied_at)
VALUES ($1, $2, $3, $4)
`, migration.Version, migration.Name, migration.UpChecksum, appliedAt); err != nil {
			_ = transaction.Rollback()
			return appliedNow, fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
		if err := transaction.Commit(); err != nil {
			return appliedNow, fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
		appliedNow = append(appliedNow, Applied{Version: migration.Version, Name: migration.Name, Checksum: migration.UpChecksum, AppliedAt: appliedAt})
	}
	return appliedNow, nil
}

func (r *Runner) Down(ctx context.Context, steps int) ([]Applied, error) {
	if steps <= 0 {
		return nil, fmt.Errorf("rollback steps must be positive")
	}
	connection, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	appliedByVersion, err := loadApplied(ctx, connection)
	if err != nil {
		return nil, err
	}
	if err := validateApplied(r.migrations, appliedByVersion); err != nil {
		return nil, err
	}
	migrationByVersion := make(map[int64]Migration, len(r.migrations))
	versions := make([]int64, 0, len(appliedByVersion))
	for _, migration := range r.migrations {
		migrationByVersion[migration.Version] = migration
	}
	for version := range appliedByVersion {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })
	if steps > len(versions) {
		steps = len(versions)
	}
	rolledBack := make([]Applied, 0, steps)
	for _, version := range versions[:steps] {
		migration := migrationByVersion[version]
		if migration.DownSQL == "" {
			return rolledBack, fmt.Errorf("migration %d_%s has no rollback SQL", migration.Version, migration.Name)
		}
		transaction, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return rolledBack, fmt.Errorf("begin rollback %d: %w", migration.Version, err)
		}
		if _, err := transaction.ExecContext(ctx, migration.DownSQL); err != nil {
			_ = transaction.Rollback()
			return rolledBack, fmt.Errorf("rollback migration %d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := transaction.ExecContext(ctx, `DELETE FROM public.schema_migrations WHERE version = $1`, migration.Version); err != nil {
			_ = transaction.Rollback()
			return rolledBack, fmt.Errorf("remove migration record %d: %w", migration.Version, err)
		}
		if err := transaction.Commit(); err != nil {
			return rolledBack, fmt.Errorf("commit rollback %d: %w", migration.Version, err)
		}
		rolledBack = append(rolledBack, appliedByVersion[version])
	}
	return rolledBack, nil
}

func (r *Runner) Status(ctx context.Context) ([]Status, error) {
	connection, release, err := r.lockedConnection(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	appliedByVersion, err := loadApplied(ctx, connection)
	if err != nil {
		return nil, err
	}
	if err := validateApplied(r.migrations, appliedByVersion); err != nil {
		return nil, err
	}
	statuses := make([]Status, 0, len(r.migrations))
	for _, migration := range r.migrations {
		status := Status{Migration: migration}
		if applied, exists := appliedByVersion[migration.Version]; exists {
			copyOfApplied := applied
			status.Applied = &copyOfApplied
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (r *Runner) lockedConnection(ctx context.Context) (*sql.Conn, func(), error) {
	connection, err := r.database.Conn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reserve migration connection: %w", err)
	}
	release := func() { _ = connection.Close() }
	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		release()
		return nil, nil, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	unlockAndRelease := func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.ExecContext(unlockContext, `SELECT pg_advisory_unlock($1)`, advisoryLockID)
		release()
	}
	if err := ensureMigrationTable(ctx, connection); err != nil {
		unlockAndRelease()
		return nil, nil, err
	}
	return connection, unlockAndRelease, nil
}

func ensureMigrationTable(ctx context.Context, connection *sql.Conn) error {
	_, err := connection.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    checksum text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
)
`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	return nil
}

func loadApplied(ctx context.Context, connection *sql.Conn) (map[int64]Applied, error) {
	rows, err := connection.QueryContext(ctx, `
SELECT version, name, checksum, applied_at
FROM public.schema_migrations
ORDER BY version
`)
	if err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[int64]Applied)
	for rows.Next() {
		var value Applied
		if err := rows.Scan(&value.Version, &value.Name, &value.Checksum, &value.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[value.Version] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func validateApplied(migrations []Migration, applied map[int64]Applied) error {
	known := make(map[int64]Migration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}
	for version, value := range applied {
		migration, exists := known[version]
		if !exists {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
		if value.Name != migration.Name {
			return fmt.Errorf("migration %d name changed from %q to %q", version, value.Name, migration.Name)
		}
		if value.Checksum != migration.UpChecksum {
			return fmt.Errorf("migration %d_%s checksum changed after application", version, migration.Name)
		}
	}
	return nil
}
