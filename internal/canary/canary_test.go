package canary

import (
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	lib, err := Load(filepath.Join("..", "..", "canaries_fixed.json"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(lib.Tasks) != 81 {
		t.Fatalf("expected 81 tasks, got %d", len(lib.Tasks))
	}
	if len(lib.Divisions) != 9 {
		t.Fatalf("expected 9 divisions, got %d", len(lib.Divisions))
	}
}
