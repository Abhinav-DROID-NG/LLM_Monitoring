package scenario

import (
	"path/filepath"
	"testing"
)

func TestLoadFixedScenarios(t *testing.T) {
	spec, err := Load(filepath.Join("..", "..", "scenarios_fixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Scenarios) < 9 {
		t.Fatalf("scenario count = %d, want at least 9", len(spec.Scenarios))
	}
}
