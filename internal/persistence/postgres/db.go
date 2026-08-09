package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, strings.TrimSpace(databaseURL))
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// OpenSQL exposes pgx through database/sql for repositories that intentionally
// avoid depending on pgx-specific types. The returned handle is pinged before
// use so startup fails instead of accepting traffic with a dead database.
func OpenSQL(ctx context.Context, databaseURL string) (*sql.DB, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, fmt.Errorf("PostgreSQL URL is empty")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(8)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return db, nil
}

func (db *DB) Close() {
	if db != nil && db.Pool != nil {
		db.Pool.Close()
	}
}
