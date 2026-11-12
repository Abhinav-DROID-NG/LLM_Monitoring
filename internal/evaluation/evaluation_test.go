package evaluation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/attribution"
)

func TestHarnessHasRequiredTrialsAndSealedTruth(t *testing.T) {
	harness := New(1000)
	if len(harness.Scenarios) != 220 || !harness.TruthSealed {
		t.Fatalf("harness = %+v", harness)
	}
	if _, err := harness.Truth(); err == nil {
		t.Fatal("sealed truth was available")
	}
	commitment, err := harness.Commit()
	if err != nil || len(commitment) != 64 {
		t.Fatalf("commitment=%q error=%v", commitment, err)
	}
	if _, err := harness.Unseal("wrong"); err == nil {
		t.Fatal("wrong commitment was accepted")
	}
	truth, err := harness.Unseal(commitment)
	if err != nil || len(truth) != 220 {
		t.Fatalf("unseal error=%v truth=%d", err, len(truth))
	}
}

func TestScoreMetricsAndConfusionMatrix(t *testing.T) {
	harness := New(10)
	commitment, err := harness.Commit()
	if err != nil {
		t.Fatal(err)
	}
	truth, err := harness.Unseal(commitment)
	if err != nil {
		t.Fatal(err)
	}
	trials := make([]Trial, len(truth))
	for i, scenario := range truth {
		predicted := attribution.Inconclusive
		detected := scenario.Truth != "NONE"
		if scenario.Truth != "NONE" {
			predicted = scenario.Truth
		}
		trials[i] = Trial{Scenario: scenario, Predicted: predicted, Detected: detected, Requests: 3, PromptTokens: 10, CompletionTokens: 5, LatencyMS: 2}
	}
	metrics := Score(trials, truth)
	if metrics.Trials != 220 || metrics.DetectionRate != 1 || metrics.FalseAlarmRate != 0 ||
		metrics.SingleLayerAttributionAccuracy != 1 || metrics.Confusion["NONE"][attribution.Inconclusive] != 60 {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestWilsonAndResultFiles(t *testing.T) {
	interval := Wilson(5, 10)
	if interval[0] <= 0 || interval[1] >= 1 || interval[0] >= interval[1] {
		t.Fatalf("invalid Wilson interval: %v", interval)
	}
	dir := t.TempDir()
	err := WriteResults(dir, Metrics{Trials: 220, Confusion: map[string]map[string]int{
		"NONE": {attribution.Inconclusive: 60},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"evaluation.md", "evaluation.csv"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing result %s: %v", name, err)
		}
	}
}
