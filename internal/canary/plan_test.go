package canary

import (
	"path/filepath"
	"testing"
)

func TestPlanProfiles(t *testing.T) {
	library, err := Load(filepath.Join("..", "..", "precision_reasoning_benchmark.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(library.Plan("development", 1)); got != 51 {
		t.Fatalf("development cycle = %d, want 51", got)
	}
}
