package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrations(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, currentSchemaVersion)
	}
	for _, table := range []string{"probe_runs", "observations"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q was not created", table)
		}
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

func TestProbePersistence(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateRun(ctx, db, "run-1", "running"); err != nil {
		t.Fatal(err)
	}
	if err := SaveObservation(ctx, db, Observation{
		ID: "obs-1", RunID: "run-1", TaskID: "task-1", LatencyMS: 4,
		HTTPStatus: 200, FeaturesJSON: `{"accuracy":1}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateRunStatus(ctx, db, "run-1", "complete"); err != nil {
		t.Fatal(err)
	}
	if err := SaveBaselineWindow(ctx, db, "window-1", `{"count":1}`); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM probe_runs WHERE id = 'run-1'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "complete" {
		t.Fatalf("run status = %q", status)
	}
}
