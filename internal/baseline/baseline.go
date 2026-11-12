package baseline

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/probe"
)

type Window struct {
	ID      string              `json:"id"`
	Samples []features.Features `json:"samples"`
	Summary Summary             `json:"summary"`
}

type Summary struct {
	Count              int     `json:"count"`
	AccuracyMean       float64 `json:"accuracy_mean"`
	RefusalRate        float64 `json:"refusal_rate"`
	JSONValidityRate   float64 `json:"json_validity_rate"`
	ResponseLengthMean float64 `json:"response_length_mean"`
	LatencyMean        float64 `json:"latency_mean"`
	InputTokensMean    float64 `json:"input_tokens_mean"`
	OutputTokensMean   float64 `json:"output_tokens_mean"`
	StyleDistanceMean  float64 `json:"style_distance_mean"`
	DeterministicRate  float64 `json:"deterministic_rate"`
}

type NoiseReport struct {
	AccuracyAbsDiff     float64 `json:"accuracy_abs_diff"`
	RefusalRateAbsDiff  float64 `json:"refusal_rate_abs_diff"`
	LengthAbsDiff       float64 `json:"length_abs_diff"`
	LatencyAbsDiff      float64 `json:"latency_abs_diff"`
	OutputTokensAbsDiff float64 `json:"output_tokens_abs_diff"`
	StyleDistance       float64 `json:"style_distance"`
}

func Build(id string, observations []probe.Observation) Window {
	samples := make([]features.Features, 0, len(observations))
	for _, observation := range observations {
		samples = append(samples, observation.Features)
	}
	return Window{ID: id, Samples: samples, Summary: summarize(samples)}
}

func Compare(a, b Window) NoiseReport {
	return NoiseReport{
		AccuracyAbsDiff:     abs(a.Summary.AccuracyMean - b.Summary.AccuracyMean),
		RefusalRateAbsDiff:  abs(a.Summary.RefusalRate - b.Summary.RefusalRate),
		LengthAbsDiff:       abs(a.Summary.ResponseLengthMean - b.Summary.ResponseLengthMean),
		LatencyAbsDiff:      abs(a.Summary.LatencyMean - b.Summary.LatencyMean),
		OutputTokensAbsDiff: abs(a.Summary.OutputTokensMean - b.Summary.OutputTokensMean),
		StyleDistance:       meanStyleDistance(a.Samples, b.Samples),
	}
}

func Marshal(window Window) (string, error) {
	data, err := json.Marshal(window)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func summarize(samples []features.Features) Summary {
	summary := Summary{Count: len(samples)}
	if len(samples) == 0 {
		return summary
	}
	var style, deterministic float64
	for i, sample := range samples {
		summary.AccuracyMean += sample.Accuracy
		if sample.Refused {
			summary.RefusalRate++
		}
		if sample.JSONValid {
			summary.JSONValidityRate++
		}
		summary.ResponseLengthMean += float64(sample.ResponseLength)
		summary.LatencyMean += float64(sample.LatencyMS)
		summary.InputTokensMean += float64(sample.InputTokens)
		summary.OutputTokensMean += float64(sample.OutputTokens)
		style += features.CosineDistance(sample.Vector, features.StyleVector(""))
		if i == 0 || sample.ResponseHash == samples[i-1].ResponseHash {
			deterministic++
		}
	}
	n := float64(len(samples))
	summary.AccuracyMean /= n
	summary.RefusalRate /= n
	summary.JSONValidityRate /= n
	summary.ResponseLengthMean /= n
	summary.LatencyMean /= n
	summary.InputTokensMean /= n
	summary.OutputTokensMean /= n
	summary.StyleDistanceMean = style / n
	summary.DeterministicRate = deterministic / n
	return summary
}

func meanStyleDistance(a, b []features.Features) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 1
	}
	count := min(len(a), len(b))
	distances := make([]float64, count)
	for i := 0; i < count; i++ {
		distances[i] = features.CosineDistance(a[i].Vector, b[i].Vector)
	}
	sort.Float64s(distances)
	return distances[len(distances)/2]
}

func abs(value float64) float64 {
	return math.Abs(value)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
