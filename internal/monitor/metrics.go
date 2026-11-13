package monitor

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

type Features map[string]float64

func ExtractMetrics(library canary.Library, observations []Observation) Features {
	division := make(map[string]string, len(library.Tasks))
	for _, task := range library.Tasks {
		division[task.ID] = task.Division
	}
	var refusals, ambiguous, providerFilters, jsonValid, checks, passed, errorsCount, truncated float64
	var outputWords, outputTokens, latency float64
	for _, observation := range observations {
		if observation.Error != "" {
			errorsCount++
		}
		if observation.FinishReason == "content_filter" {
			providerFilters++
			refusals++
		}
		if observation.FinishReason == "length" {
			truncated++
		}
		if strings.Contains(strings.ToLower(observation.Response), "i can't") ||
			strings.Contains(strings.ToLower(observation.Response), "i cannot") {
			refusals++
		}
		var value any
		if json.Unmarshal([]byte(strings.TrimSpace(observation.Response)), &value) == nil {
			jsonValid++
		}
		outputWords += float64(len(strings.Fields(observation.Response)))
		outputTokens += float64(observation.OutputTokens)
		latency += float64(observation.LatencyMS)
		for _, check := range observation.Checks {
			if check.Unavailable {
				continue
			}
			checks++
			if check.Passed {
				passed++
			}
		}
		for _, check := range observation.Checks {
			if check.Unavailable {
				ambiguous++
			}
		}
		_ = division[observation.CanaryID]
	}
	n := float64(len(observations))
	return Features{
		"refusal_rate":               divide(refusals, n),
		"ambiguous_rate":             divide(ambiguous, n),
		"provider_filter_rate":       divide(providerFilters, n),
		"json_validity_rate":         divide(jsonValid, n),
		"structure_compliance_rate":  divide(passed, checks),
		"output_words":               divide(outputWords, n),
		"output_tokens":              divide(outputTokens, n),
		"error_rate":                 divide(errorsCount, n),
		"truncation_rate":            divide(truncated, n),
		"rule_compliance_rate":       divide(passed, checks),
		"latency_mean":               divide(latency, n),
		"sentinel_present_rate":      sentinelRate(observations),
		"returned_model_consistency": returnedModelConsistency(observations),
	}
}

func returnedModelConsistency(observations []Observation) float64 {
	if len(observations) == 0 {
		return 0
	}
	counts := make(map[string]int)
	for _, observation := range observations {
		counts[observation.ReturnedModel]++
	}
	values := make([]int, 0, len(counts))
	for _, count := range counts {
		values = append(values, count)
	}
	sort.Ints(values)
	return float64(values[len(values)-1]) / float64(len(observations))
}

func divide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
