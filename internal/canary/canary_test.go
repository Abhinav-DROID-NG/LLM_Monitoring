package canary

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	lib, err := Load(filepath.Join("..", "..", "precision_reasoning_benchmark.json"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(lib.Tasks) != 51 {
		t.Fatalf("expected 51 tasks, got %d", len(lib.Tasks))
	}
	if len(lib.Divisions) != 5 {
		t.Fatalf("expected 5 divisions, got %d", len(lib.Divisions))
	}
}

func TestCheckPreservesNestedParameters(t *testing.T) {
	var check Check
	if err := json.Unmarshal([]byte(`{"type":"json_types","lengths":{"items":4},"word_range":{"summary":[15,25]},"strict":true}`), &check); err != nil {
		t.Fatal(err)
	}
	if _, ok := check.Params["lengths"]; !ok {
		t.Fatal("lengths parameter was dropped")
	}
	if got, ok := check.IntPair("word_range.summary"); !ok || got != [2]int{15, 25} {
		t.Fatalf("word_range = %v, %v", got, ok)
	}
	if !check.Bool("strict") {
		t.Fatal("strict parameter was not preserved")
	}
}

func TestFilterTierAndRequestIdentity(t *testing.T) {
	lib, err := Load(filepath.Join("..", "..", "precision_reasoning_benchmark.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(lib.FilterTier("easy")); got != 7 {
		t.Fatalf("easy task count = %d, want 7", got)
	}
	task := lib.Tasks[0]
	first := RequestIdentity(task, "model-a", 0.2, 128, "system", "")
	if first != RequestIdentity(task, "model-a", 0.2, 128, "system", "") {
		t.Fatal("request identity is not stable")
	}
	if first == RequestIdentity(task, "model-b", 0.2, 128, "system", "") {
		t.Fatal("request identity did not change with model")
	}
}
