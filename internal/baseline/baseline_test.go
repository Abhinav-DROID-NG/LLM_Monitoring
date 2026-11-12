package baseline

import (
	"strings"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/probe"
)

func TestBuildAndCompareBaselineWindows(t *testing.T) {
	vector := features.StyleVector("stable synthetic answer")
	observations := []probe.Observation{
		{Features: features.Features{
			Accuracy: 1, JSONValid: true, ResponseLength: 20, LatencyMS: 5,
			InputTokens: 4, OutputTokens: 3, ResponseHash: "same", Vector: vector,
		}},
		{Features: features.Features{
			Accuracy: 1, JSONValid: true, ResponseLength: 20, LatencyMS: 5,
			InputTokens: 4, OutputTokens: 3, ResponseHash: "same", Vector: vector,
		}},
	}
	window := Build("baseline-a", observations)
	if window.Summary.Count != 2 || window.Summary.AccuracyMean != 1 ||
		window.Summary.DeterministicRate != 1 {
		t.Fatalf("unexpected summary: %+v", window.Summary)
	}
	noise := Compare(window, window)
	if noise.AccuracyAbsDiff != 0 || noise.StyleDistance != 0 {
		t.Fatalf("same baseline should have no noise: %+v", noise)
	}
	encoded, err := Marshal(window)
	if err != nil || !strings.Contains(encoded, `"baseline-a"`) {
		t.Fatalf("marshal window: %v %s", err, encoded)
	}
}

func TestBaselineNoiseCapturesWindowDifferences(t *testing.T) {
	a := Build("a", []probe.Observation{{Features: features.Features{
		Accuracy: 1, ResponseLength: 10, LatencyMS: 5, Vector: features.StyleVector("short"),
	}}})
	b := Build("b", []probe.Observation{{Features: features.Features{
		Accuracy: 0, ResponseLength: 30, LatencyMS: 15, Vector: features.StyleVector("long structured response"),
	}}})
	noise := Compare(a, b)
	if noise.AccuracyAbsDiff != 1 || noise.LengthAbsDiff != 20 || noise.LatencyAbsDiff != 10 ||
		noise.StyleDistance <= 0 {
		t.Fatalf("unexpected baseline noise: %+v", noise)
	}
}
