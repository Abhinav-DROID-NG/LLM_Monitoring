package evaluation

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type fakeClient struct{ calls int }

func (f *fakeClient) Complete(_ context.Context, request Request) (Response, error) {
	f.calls++
	if request.Prompt == "error" {
		return Response{}, errors.New("provider unavailable")
	}
	return Response{Text: request.Prompt, OutputTokens: 2, Latency: 10 * time.Millisecond}, nil
}

func TestDatasetValidationAndEvaluatorOutcomes(t *testing.T) {
	dataset := Dataset{Version: "test", Behavioural: []TestCase{{ID: "b1", Category: "instruction", Prompt: "ok", Expected: "ok", Evaluator: EvaluatorSpec{Type: "exact_match"}}}}
	if err := dataset.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := Evaluate(dataset.Behavioural[0].Evaluator, "ok", "ok"); got.Status != Pass {
		t.Fatalf("got %+v", got)
	}
	if got := Evaluate(dataset.Behavioural[0].Evaluator, "ok", "no"); got.Status != Fail {
		t.Fatalf("got %+v", got)
	}
	if got := Evaluate(EvaluatorSpec{Type: "semantic_similarity"}, "", "text"); got.Status != Inconclusive {
		t.Fatalf("got %+v", got)
	}
}

func TestRunnerRepeatsAndPreservesErrors(t *testing.T) {
	client := &fakeClient{}
	runner := Runner{Client: client}
	dataset := Dataset{Version: "test", Behavioural: []TestCase{
		{ID: "b1", Category: "instruction", Prompt: "ok", Expected: "ok", Evaluator: EvaluatorSpec{Type: "exact_match"}, Repetitions: 3},
		{ID: "b2", Category: "instruction", Prompt: "error", Expected: "ok", Evaluator: EvaluatorSpec{Type: "exact_match"}, Repetitions: 1},
	}}
	result, err := runner.Run(context.Background(), dataset, Behavioural, Metadata{Model: "mock", Provider: "test"}, DefaultConfig("development"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trials) != 4 || client.calls != 5 {
		t.Fatalf("trials=%d calls=%d", len(result.Trials), client.calls)
	}
	if result.Metrics.Values["pass_rate"] != .75 {
		t.Fatalf("metrics=%v", result.Metrics.Values)
	}
}

func TestAggregatePercentiles(t *testing.T) {
	metrics := Aggregate([]Trial{{Status: Pass, Score: ptr(1), LatencyMS: 1}, {Status: Pass, Score: ptr(1), LatencyMS: 2}, {Status: Fail, Score: ptr(0), LatencyMS: 3}, {Status: Inconclusive, LatencyMS: 4}})
	if metrics.Values["latency_p95"] != 4 || metrics.Coverage != .75 {
		t.Fatalf("metrics=%v coverage=%v", metrics.Values, metrics.Coverage)
	}
}

func TestCompareRejectsIncompatibleBaseline(t *testing.T) {
	base := Baseline{Objective: Behavioural, Dataset: "v1", Metrics: map[string]MetricBaseline{"pass_rate": {Mean: .9, SD: .02, N: 3}}}
	current := Evaluation{Objective: Performance, Dataset: "v1", Metrics: Metrics{Values: map[string]float64{"pass_rate": .1}}}
	if report := Compare(base, current, nil); report.Status != InconclusiveDrift {
		t.Fatalf("report=%+v", report)
	}
}

func TestBaselineStorageIsExplicitAndNonDestructive(t *testing.T) {
	dir := t.TempDir()
	base := Baseline{SchemaVersion: SchemaVersion, BaselineID: "b1", Objective: Behavioural, Dataset: "v1"}
	if _, err := SaveBaseline(dir, base); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveBaseline(dir, base); err == nil {
		t.Fatal("expected duplicate baseline error")
	}
	if _, err := os.Stat(dir + "/evaluation-baselines/b1.json"); err != nil {
		t.Fatal(err)
	}
}
func ptr(v float64) *float64 { return &v }
