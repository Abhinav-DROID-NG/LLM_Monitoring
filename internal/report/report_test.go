package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/attribution"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/stats"
)

func TestMarkdownSeparatesObservedAndInferredEvidence(t *testing.T) {
	value := Report{
		RunID: "run-1", TimeWindow: "window", Baseline: "base", ProbeCount: 2,
		Changes: []stats.Change{{Feature: "refusal", Baseline: 0, Current: 1, Changed: true}},
		Attribution: attribution.Report{
			Verdict:    attribution.Safety,
			Candidates: []attribution.Candidate{{Layer: attribution.Safety, Score: 0.9, Coverage: 1}},
		},
	}
	output := Markdown(value)
	if !strings.Contains(output, "## OBSERVED (facts)") || !strings.Contains(output, "## LIKELY CAUSE (inference)") {
		t.Fatal("report does not separate evidence from inference")
	}
	if !strings.Contains(output, "run-1") || !strings.Contains(output, "refusal") {
		t.Fatalf("report missing metadata: %s", output)
	}
}

func TestCSVContainsChangeRows(t *testing.T) {
	var output bytes.Buffer
	err := CSV(&output, Report{Changes: []stats.Change{{Feature: "latency", Baseline: 1, Current: 2, Changed: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "feature,baseline,current") || !strings.Contains(output.String(), "latency") {
		t.Fatalf("unexpected CSV: %s", output.String())
	}
}
