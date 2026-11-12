package attribution

import (
	"context"
	"sort"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/stats"
)

const (
	Model         = "model"
	Safety        = "safety"
	RequestConfig = "request_config"
	Serving       = "serving"
	Inconclusive  = "INCONCLUSIVE"
)

type Signature struct {
	Positive map[string]float64
	Stable   map[string]float64
	Primary  []string
}

type Config struct {
	TauMin     float64
	Delta      float64
	Weights    map[string]float64
	Signatures map[string]Signature
}

type Candidate struct {
	Layer    string  `json:"layer"`
	Score    float64 `json:"score"`
	Coverage float64 `json:"coverage"`
}

type FollowUp struct {
	Kind       string           `json:"kind"`
	Reason     string           `json:"reason"`
	TaskIDs    []string         `json:"task_ids"`
	Parameters provider.Request `json:"parameters"`
}

type FollowUpOutcome struct {
	FollowUp  FollowUp `json:"follow_up"`
	Supported bool     `json:"supported"`
	Evidence  string   `json:"evidence"`
}

type Report struct {
	Verdict      string            `json:"verdict"`
	Candidates   []Candidate       `json:"candidates"`
	Attributions []string          `json:"attributions"`
	FollowUps    []FollowUp        `json:"follow_ups"`
	Outcomes     []FollowUpOutcome `json:"outcomes"`
}

func DefaultConfig() Config {
	return Config{
		TauMin: 0.55,
		Delta:  0.15,
		Weights: map[string]float64{
			"accuracy": 1, "refusal": 1, "json_valid": 1, "response_length": 0.8,
			"latency": 1, "style": 0.8, "truncation": 1, "output_tokens": 1,
		},
		Signatures: defaultSignatures(),
	}
}

func Analyze(changes []stats.Change, config Config) Report {
	config = normalizedConfig(config)
	candidates := make([]Candidate, 0, len(config.Signatures))
	for layer, signature := range config.Signatures {
		var positive, penalty, changedPrimary float64
		for feature, expected := range signature.Positive {
			change, ok := findChange(changes, feature)
			if !ok {
				continue
			}
			weight := config.Weights[feature]
			if weight == 0 {
				weight = 1
			}
			if change.Changed {
				changedPrimary++
				positive += weight * absEffect(change.Effect)
			}
			if expected < 0 && change.Changed {
				penalty += weight * absEffect(change.Effect)
			}
		}
		for feature, weight := range signature.Stable {
			if change, ok := findChange(changes, feature); ok && change.Changed {
				penalty += weight * absEffect(change.Effect)
			}
		}
		raw := positive - penalty
		score := clamp(raw / maxFloat(1, positive+penalty))
		coverage := 0.0
		if len(signature.Primary) > 0 {
			for _, feature := range signature.Primary {
				if change, ok := findChange(changes, feature); ok && change.Changed {
					coverage++
				}
			}
			coverage /= float64(len(signature.Primary))
		}
		candidates = append(candidates, Candidate{Layer: layer, Score: score, Coverage: coverage})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	report := Report{Candidates: candidates}
	if len(candidates) == 0 {
		report.Verdict = Inconclusive
		return report
	}
	ambiguous := len(candidates) > 1 && candidates[0].Score-candidates[1].Score < config.Delta
	if candidates[0].Score < config.TauMin || ambiguous {
		report.Verdict = Inconclusive
		report.FollowUps = selectFollowUps(candidates, changes)
		return report
	}
	report.Verdict = candidates[0].Layer
	report.Attributions = []string{candidates[0].Layer}
	for _, candidate := range candidates[1:] {
		if candidate.Score >= config.TauMin && candidate.Score >= candidates[0].Score-config.Delta {
			report.Attributions = append(report.Attributions, candidate.Layer)
		}
	}
	return report
}

func ExecuteFollowUps(ctx context.Context, suite canary.Suite, p provider.Provider, followUps []FollowUp) []FollowUpOutcome {
	outcomes := make([]FollowUpOutcome, 0, len(followUps))
	for _, followUp := range followUps {
		supported := false
		evidence := "follow-up could not establish support"
		for _, taskID := range followUp.TaskIDs {
			result, err := suite.Execute(ctx, p, taskID)
			if err != nil {
				evidence = err.Error()
				continue
			}
			if len(result.Checks) > 0 && result.Checks[0].Passed {
				supported = true
				evidence = "follow-up check passed"
			}
		}
		outcomes = append(outcomes, FollowUpOutcome{FollowUp: followUp, Supported: supported, Evidence: evidence})
	}
	return outcomes
}

func normalizedConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.TauMin <= 0 {
		config.TauMin = defaults.TauMin
	}
	if config.Delta <= 0 {
		config.Delta = defaults.Delta
	}
	if config.Weights == nil {
		config.Weights = defaults.Weights
	}
	if config.Signatures == nil {
		config.Signatures = defaults.Signatures
	}
	return config
}

func defaultSignatures() map[string]Signature {
	return map[string]Signature{
		Model: {
			Positive: map[string]float64{"accuracy": 1, "refusal": 0.5, "json_valid": 1, "response_length": 0.5, "style": 1},
			Stable:   map[string]float64{"latency": 0.5, "truncation": 0.5},
			Primary:  []string{"accuracy", "json_valid", "style"},
		},
		Safety: {
			Positive: map[string]float64{"refusal": 1},
			Stable:   map[string]float64{"accuracy": 1, "latency": 0.5, "response_length": 0.5},
			Primary:  []string{"refusal"},
		},
		RequestConfig: {
			Positive: map[string]float64{"json_valid": 1, "response_length": 1, "truncation": 1, "style": 0.5},
			Stable:   map[string]float64{"refusal": 0.5, "latency": 0.5},
			Primary:  []string{"json_valid", "response_length", "truncation"},
		},
		Serving: {
			Positive: map[string]float64{"latency": 1, "output_tokens": 1},
			Stable:   map[string]float64{"accuracy": 1, "refusal": 1, "json_valid": 1},
			Primary:  []string{"latency", "output_tokens"},
		},
	}
}

func selectFollowUps(candidates []Candidate, changes []stats.Change) []FollowUp {
	if len(candidates) == 0 {
		return nil
	}
	top := candidates[0].Layer
	followUp := FollowUp{
		Kind:       "pin_request_configuration",
		Reason:     "pin parameters to distinguish request defaults from model and serving changes",
		Parameters: provider.Request{Temperature: floatPtr(0.2), MaxTokens: intPtr(256), ResponseFormat: "text"},
	}
	if top == Serving || (len(candidates) > 1 && candidates[1].Layer == Serving) {
		followUp = FollowUp{
			Kind:   "repeat_across_time",
			Reason: "repeat unchanged synthetic probes to test serving-stack variability",
		}
	}
	if top == Safety {
		followUp.Kind = "repeat_refusal_canaries"
		followUp.Reason = "repeat existing refusal canaries to isolate refusal behavior"
	}
	return []FollowUp{followUp}
}

func findChange(changes []stats.Change, feature string) (stats.Change, bool) {
	for _, change := range changes {
		if change.Feature == feature {
			return change, true
		}
	}
	if feature == "output_tokens" {
		for _, change := range changes {
			if change.Feature == "response_length" {
				return change, true
			}
		}
	}
	return stats.Change{}, false
}

func absEffect(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func floatPtr(value float64) *float64 { return &value }
func intPtr(value int) *int           { return &value }
