package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 1

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("database path cannot be empty")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	var version int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version >= currentSchemaVersion {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS probe_runs (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			status TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS observations (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES probe_runs(id),
			task_id TEXT NOT NULL,
			returned_model TEXT,
			system_fingerprint TEXT,
			latency_ms INTEGER NOT NULL,
			input_tokens INTEGER,
			output_tokens INTEGER,
			finish_reason TEXT,
			http_status INTEGER NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("apply schema migration %d: %w", currentSchemaVersion, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", currentSchemaVersion); err != nil {
		return fmt.Errorf("record schema migration %d: %w", currentSchemaVersion, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration %d: %w", currentSchemaVersion, err)
	}
	return nil
}
