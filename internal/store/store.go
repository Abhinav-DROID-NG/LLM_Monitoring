package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 3

type Observation struct {
	ID                string
	RunID             string
	TaskID            string
	ReturnedModel     string
	SystemFingerprint string
	LatencyMS         int64
	InputTokens       int
	OutputTokens      int
	FinishReason      string
	HTTPStatus        int
	Retries           int
	FeaturesJSON      string
	Error             string
}

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
	if version < 1 {
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
			http_status INTEGER NOT NULL,
			retries INTEGER NOT NULL DEFAULT 0,
			features_json TEXT NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT ''
		);
		`); err != nil {
			return fmt.Errorf("apply schema migration 1: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", 1); err != nil {
			return fmt.Errorf("record schema migration 1: %w", err)
		}
	}
	if version < 2 {
		for _, statement := range []string{
			"ALTER TABLE observations ADD COLUMN retries INTEGER NOT NULL DEFAULT 0",
			"ALTER TABLE observations ADD COLUMN features_json TEXT NOT NULL DEFAULT '{}'",
			"ALTER TABLE observations ADD COLUMN error TEXT NOT NULL DEFAULT ''",
		} {
			if version == 1 {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return fmt.Errorf("apply observation migration: %w", err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS baseline_windows (
				id TEXT PRIMARY KEY,
				created_at TEXT NOT NULL,
				features_json TEXT NOT NULL
			);
		`); err != nil {
			return fmt.Errorf("apply schema migration 2: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", 2); err != nil {
			return fmt.Errorf("record schema migration 2: %w", err)
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS reports (
				id TEXT PRIMARY KEY,
				created_at TEXT NOT NULL,
				markdown TEXT NOT NULL,
				changes_json TEXT NOT NULL,
				attribution_json TEXT NOT NULL
			);
		`); err != nil {
			return fmt.Errorf("apply schema migration 3: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", 3); err != nil {
			return fmt.Errorf("record schema migration 3: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}
	return nil
}

func CreateRun(ctx context.Context, db *sql.DB, id, status string) error {
	if id == "" || status == "" {
		return errors.New("run ID and status are required")
	}
	_, err := db.ExecContext(ctx, "INSERT INTO probe_runs(id, created_at, status) VALUES (?, CURRENT_TIMESTAMP, ?)", id, status)
	if err != nil {
		return fmt.Errorf("create probe run: %w", err)
	}
	return nil
}

func UpdateRunStatus(ctx context.Context, db *sql.DB, id, status string) error {
	if id == "" || status == "" {
		return errors.New("run ID and status are required")
	}
	result, err := db.ExecContext(ctx, "UPDATE probe_runs SET status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("update probe run %q: %w", id, err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect probe run update %q: %w", id, err)
	} else if count != 1 {
		return fmt.Errorf("probe run %q does not exist", id)
	}
	return nil
}

func SaveObservation(ctx context.Context, db *sql.DB, observation Observation) error {
	if observation.ID == "" || observation.RunID == "" || observation.TaskID == "" {
		return errors.New("observation ID, run ID, and task ID are required")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO observations (
			id, run_id, task_id, returned_model, system_fingerprint, latency_ms,
			input_tokens, output_tokens, finish_reason, http_status, retries, features_json, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, observation.ID, observation.RunID, observation.TaskID, observation.ReturnedModel,
		observation.SystemFingerprint, observation.LatencyMS, observation.InputTokens,
		observation.OutputTokens, observation.FinishReason, observation.HTTPStatus,
		observation.Retries, observation.FeaturesJSON, observation.Error)
	if err != nil {
		return fmt.Errorf("save observation %q: %w", observation.ID, err)
	}
	return nil
}

func SaveBaselineWindow(ctx context.Context, db *sql.DB, id, featuresJSON string) error {
	if id == "" || featuresJSON == "" {
		return errors.New("baseline window ID and features are required")
	}
	_, err := db.ExecContext(ctx, "INSERT INTO baseline_windows(id, created_at, features_json) VALUES (?, CURRENT_TIMESTAMP, ?)", id, featuresJSON)
	if err != nil {
		return fmt.Errorf("save baseline window %q: %w", id, err)
	}
	return nil
}

func LatestBaselineWindow(ctx context.Context, db *sql.DB) (string, error) {
	var id, encoded string
	err := db.QueryRowContext(ctx, `
		SELECT id, features_json FROM baseline_windows
		ORDER BY created_at DESC, rowid DESC LIMIT 1
	`).Scan(&id, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("read latest baseline window: %w", err)
	}
	return encoded, nil
}

func SaveReport(ctx context.Context, db *sql.DB, id, markdown, changesJSON, attributionJSON string) error {
	if id == "" || markdown == "" {
		return errors.New("report ID and markdown are required")
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO reports(id, created_at, markdown, changes_json, attribution_json)
		VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?)
	`, id, markdown, changesJSON, attributionJSON)
	if err != nil {
		return fmt.Errorf("save report %q: %w", id, err)
	}
	return nil
}

func LatestReport(ctx context.Context, db *sql.DB) (string, error) {
	var markdown string
	err := db.QueryRowContext(ctx, `
		SELECT markdown FROM reports ORDER BY created_at DESC, rowid DESC LIMIT 1
	`).Scan(&markdown)
	if errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("read latest report: %w", err)
	}
	return markdown, nil
}
