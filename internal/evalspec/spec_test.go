package evalspec

import (
	"path/filepath"
	"testing"
)

func TestLoadAndNormalizeEvaluationSpec(t *testing.T) {
	spec, err := Load(filepath.Join("..", "..", "problem3_test_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	spec.NormalizeLayers()
	if len(spec.SingleLayer) != 9 || len(spec.Combined) != 5 || len(spec.FailureCases) != 7 {
		t.Fatalf("unexpected scenario counts: %d/%d/%d", len(spec.SingleLayer), len(spec.Combined), len(spec.FailureCases))
	}
	for _, scenario := range spec.SingleLayer {
		if scenario.Layer == "config" {
			t.Fatalf("config layer was not normalized")
		}
	}
}
