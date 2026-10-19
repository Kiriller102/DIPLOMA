package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jmoiron/sqlx"
)

//go:embed V*.sql demo.sql
var files embed.FS

// Apply runs pending migrations atomically; a database lock also serializes starts.
func Apply(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(20261019)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "V*.sql")
	if err != nil {
		return err
	}
	for _, name := range names {
		var exists bool
		if err = tx.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SeedDemo is explicitly enabled by DEMO_MODE for local demonstrations.
func SeedDemo(ctx context.Context, db *sqlx.DB) error {
	body, err := files.ReadFile("demo.sql")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(body))
	return err
}
