package monitor

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCycleRoundTrip(t *testing.T) {
	root := t.TempDir()
	cycle := Cycle{RunID: "run-1", Profile: "monitoring", StartedAt: time.Unix(1, 0).UTC(),
		Config: RequestConfig{ResponseFormat: "json_object"}, Layers: Layers{Model: "v1"},
		Observations: []Observation{{CanaryID: "C1", RunID: "run-1", OutputTokens: 2}},
		Cost:         Cost{Calls: 1, OutputTokens: 2}}
	if err := SaveCycle(root, cycle); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCycle(filepath.Join(root, "cycles", "run-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != cycle.RunID || got.Cost.OutputTokens != 2 || got.Layers.Model != "v1" {
		t.Fatalf("round-trip mismatch: %#v", got)
	}
}
