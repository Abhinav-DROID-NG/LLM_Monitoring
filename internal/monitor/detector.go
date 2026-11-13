package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"sort"
	"strings"
)

type FeatureStats struct {
	Mean float64 `json:"mean"`
	SD   float64 `json:"sd"`
	N    int     `json:"n"`
}

type Baseline struct {
	Features     map[string]FeatureStats `json:"features"`
	Observations []Observation           `json:"observations,omitempty"`
}

type FeatureChange struct {
	Feature    string  `json:"feature"`
	Direction  string  `json:"direction"`
	EffectSize float64 `json:"effect_size"`
	PValue     float64 `json:"adjusted_p_value"`
	Baseline   float64 `json:"baseline"`
	Current    float64 `json:"current"`
}

type Detection struct {
	Detected bool            `json:"detected"`
	Changes  []FeatureChange `json:"changes"`
}

func DetectWithThresholds(base Baseline, current []Observation, thresholds Thresholds) Detection {
	now := observationFeatures(current)
	pValues := make([]float64, 0, len(base.Features))
	keys := make([]string, 0, len(base.Features))
	effects := make([]float64, 0, len(base.Features))
	for key, stats := range base.Features {
		value, ok := now[key]
		if !ok {
			continue
		}
		floor := noiseFloor(key, stats.Mean)
		effect := value - stats.Mean
		z := effect / math.Max(stats.SD, floor)
		keys = append(keys, key)
		effects = append(effects, z)
		pValues = append(pValues, Normal2SidedP(z))
	}
	adjusted, rejected := BenjaminiHochberg(pValues, thresholds.FDRQ)
	changes := make([]FeatureChange, 0)
	for i, key := range keys {
		stats := base.Features[key]
		value := now[key]
		minEffect := thresholds.MinEffect["proportion_abs"]
		if strings.Contains(key, "latency") {
			minEffect = thresholds.MinEffect["latency_relative"]
		}
		if minEffect == 0 {
			minEffect = 0.1
		}
		effect := value - stats.Mean
		if rejected[i] && math.Abs(effect) >= minEffect {
			direction := "increased"
			if effect < 0 {
				direction = "decreased"
			}
			changes = append(changes, FeatureChange{Feature: key, Direction: direction,
				EffectSize: effects[i], PValue: adjusted[i], Baseline: stats.Mean, Current: value})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return math.Abs(changes[i].EffectSize) > math.Abs(changes[j].EffectSize)
	})
	return Detection{Detected: len(changes) > 0, Changes: changes}
}

func BuildBaseline(runs [][]Observation) Baseline {
	values := make(map[string][]float64)
	for _, run := range runs {
		for key, value := range observationFeatures(run) {
			values[key] = append(values[key], value)
		}
	}
	stats := make(map[string]FeatureStats, len(values))
	for key, sample := range values {
		mean, sd := meanSD(sample)
		stats[key] = FeatureStats{Mean: mean, SD: sd, N: len(sample)}
	}
	return Baseline{Features: stats}
}

func Detect(base Baseline, current []Observation) Detection {
	now := observationFeatures(current)
	changes := make([]FeatureChange, 0)
	for key, stats := range base.Features {
		value, ok := now[key]
		if !ok {
			continue
		}
		floor := noiseFloor(key, stats.Mean)
		z := (value - stats.Mean) / math.Max(stats.SD, floor)
		if math.Abs(z) < 4 {
			continue
		}
		direction := "increased"
		if z < 0 {
			direction = "decreased"
		}
		changes = append(changes, FeatureChange{Feature: key, Direction: direction,
			EffectSize: z, PValue: math.Erfc(math.Abs(z) / math.Sqrt2),
			Baseline: stats.Mean, Current: value})
	}
	sort.Slice(changes, func(i, j int) bool { return math.Abs(changes[i].EffectSize) > math.Abs(changes[j].EffectSize) })
	return Detection{Detected: len(changes) > 0, Changes: changes}
}

func observationFeatures(observations []Observation) map[string]float64 {
	features := map[string]float64{}
	if len(observations) == 0 {
		return features
	}
	var refusals, validJSON, passed, checks, lengths, inputTokens float64
	latencies := make([]float64, 0, len(observations))
	for _, observation := range observations {
		if isRefusal(strings.ToLower(observation.Response)) || observation.FinishReason == "content_filter" {
			refusals++
		}
		var decoded any
		if strings.TrimSpace(observation.Response) != "" && json.Unmarshal([]byte(observation.Response), &decoded) == nil {
			validJSON++
		}
		for _, check := range observation.Checks {
			if check.Unavailable {
				continue
			}
			checks++
			if check.Passed {
				passed++
			}
		}
		lengths += float64(observation.OutputTokens)
		inputTokens += float64(observation.InputTokens)
		latencies = append(latencies, float64(observation.LatencyMS))
	}
	n := float64(len(observations))
	features["refusal_rate"] = refusals / n
	features["json_validity_rate"] = validJSON / n
	features["structure_compliance"] = safeDivide(passed, checks)
	features["output_token_count"] = lengths / n
	features["input_token_count"] = inputTokens / n
	features["returned_model"] = returnedModelCode(observations)
	features["latency_p50"] = percentile(latencies, 0.50)
	features["latency_p95"] = percentile(latencies, 0.95)
	features["finish_reason_length_rate"] = finishRate(observations, "length")
	features["sentinel_present_rate"] = sentinelRate(observations)
	return features
}

func returnedModelCode(observations []Observation) float64 {
	if len(observations) == 0 {
		return 0
	}
	model := observations[0].ReturnedModel
	sum := sha256.Sum256([]byte(model))
	var value uint64
	for _, byteValue := range sum[:8] {
		value = value<<8 | uint64(byteValue)
	}
	return float64(value%1000000) / 1000000
}

func noiseFloor(name string, mean float64) float64 {
	switch {
	case strings.Contains(name, "rate"), strings.Contains(name, "compliance"):
		return 0.04
	case strings.Contains(name, "latency"):
		return math.Max(1, math.Abs(mean)*0.08)
	case strings.Contains(name, "token"), strings.Contains(name, "length"):
		return math.Max(1, math.Abs(mean)*0.05)
	default:
		return 0.01
	}
}

func meanSD(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	if len(values) < 2 {
		return mean, 0
	}
	var variance float64
	for _, value := range values {
		variance += (value - mean) * (value - mean)
	}
	return mean, math.Sqrt(variance / float64(len(values)-1))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	index := int(math.Round(p * float64(len(values)-1)))
	return values[index]
}

func safeDivide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
func finishRate(items []Observation, reason string) float64 {
	var count float64
	for _, item := range items {
		if item.FinishReason == reason {
			count++
		}
	}
	return safeDivide(count, float64(len(items)))
}
func sentinelRate(items []Observation) float64 {
	var count float64
	for _, item := range items {
		if strings.Contains(item.Response, "<<END>>") {
			count++
		}
	}
	return safeDivide(count, float64(len(items)))
}
