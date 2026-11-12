package attribution

import (
	"context"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/stats"
)

func TestIsolatedLayerSignaturesRankExpectedLayer(t *testing.T) {
	tests := []struct {
		name    string
		changes []stats.Change
		want    string
	}{
		{"model", []stats.Change{
			changed("accuracy", 1), changed("json_valid", 1), changed("style", 0.8),
		}, Model},
		{"safety", []stats.Change{changed("refusal", 1)}, Safety},
		{"request config", []stats.Change{
			changed("json_valid", 1), changed("response_length", 1), changed("truncation", 1),
		}, RequestConfig},
		{"serving", []stats.Change{
			changed("latency", 1), changed("output_tokens", 1),
		}, Serving},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := Analyze(test.changes, DefaultConfig())
			if report.Verdict != test.want || len(report.Attributions) != 1 {
				t.Fatalf("report = %+v, want %s", report, test.want)
			}
			if report.Candidates[0].Layer != test.want {
				t.Fatalf("top candidate = %+v", report.Candidates[0])
			}
		})
	}
}

func TestAmbiguousAttributionReturnsInconclusiveAndSelectsFollowUp(t *testing.T) {
	report := Analyze([]stats.Change{
		changed("response_length", 0.5),
	}, DefaultConfig())
	if report.Verdict != Inconclusive || len(report.FollowUps) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if report.FollowUps[0].Kind != "pin_request_configuration" {
		t.Fatalf("follow-up = %+v", report.FollowUps[0])
	}
	if report.FollowUps[0].Parameters.Temperature == nil || report.FollowUps[0].Parameters.MaxTokens == nil {
		t.Fatalf("follow-up did not pin request parameters: %+v", report.FollowUps[0])
	}
}

func TestCombinedChangesCanReturnMultipleLayers(t *testing.T) {
	config := DefaultConfig()
	config.Delta = 0.01
	report := Analyze([]stats.Change{
		changed("refusal", 1),
		changed("latency", 1),
		changed("output_tokens", 1),
	}, config)
	if report.Verdict == Inconclusive || len(report.Attributions) < 2 {
		t.Fatalf("combined report = %+v", report)
	}
}

func TestFollowUpOutcomesDoNotUseGroundTruth(t *testing.T) {
	suite := canary.Suite{
		CheckTypes: map[string]string{"output_equals": "check"},
		Schema:     canary.SchemaContract{AllowedTiers: []string{"easy"}, AllowedDivisions: []string{"test"}},
		Tasks: []canary.Task{{
			ID: "synthetic-1", Division: "test", Tier: "easy", Prompt: "synthetic",
			MaxTokens: 10, Checks: []canary.Check{{Type: "output_equals", Value: []byte(`"ok"`)}},
		}},
	}
	p := fixedProvider{}
	outcomes := ExecuteFollowUps(context.Background(), suite, p, []FollowUp{{
		Kind: "pin_request_configuration", TaskIDs: []string{"synthetic-1"},
	}})
	if len(outcomes) != 1 || !outcomes[0].Supported || outcomes[0].Evidence == "" {
		t.Fatalf("outcomes = %+v", outcomes)
	}
}

func changed(feature string, effect float64) stats.Change {
	return stats.Change{Feature: feature, Effect: effect, Changed: true, RawP: 0.001, AdjustedP: 0.001}
}

type fixedProvider struct{}

func (fixedProvider) ChatCompletion(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Choices: []provider.Choice{{
		Message: provider.Message{Content: "ok"}, FinishReason: "stop",
	}}}, nil
}
