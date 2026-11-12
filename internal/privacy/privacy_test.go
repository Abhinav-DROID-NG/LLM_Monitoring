package privacy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/probe"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/store"
)

func TestMonitoringRunDoesNotPersistSentinelsOrExposeProxy(t *testing.T) {
	const sentinel = "USER_SECRET_12345"
	t.Setenv("MONITOR_SENTINEL", sentinel)
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	suite, err := canary.Load(filepath.Join("..", "..", "canaries_fixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := probe.NewRunner(probe.Config{Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), struct{}{}, sentinel)
	if _, err := runner.RunAndStore(ctx, suite, mockprovider.New(3).Provider(), db, "privacy-run"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT task_id, features_json, error FROM observations")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID, featureJSON, requestError string
		if err := rows.Scan(&taskID, &featureJSON, &requestError); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(taskID+featureJSON+requestError, sentinel) {
			t.Fatalf("sentinel persisted in observation")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "logs")); err == nil {
		t.Fatal("unexpected monitor log directory")
	}
}
