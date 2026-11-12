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
