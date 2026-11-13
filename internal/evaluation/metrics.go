package evaluation

import (
	"math"
	"sort"
)

func Aggregate(trials []Trial) Metrics {
	values := map[string]float64{}
	if len(trials) == 0 {
		return Metrics{Values: values}
	}
	var pass, scored, errors, timeouts float64
	latencies, ttfts, tps := []float64{}, []float64{}, []float64{}
	for _, trial := range trials {
		if trial.Status == Pass {
			pass++
		}
		if trial.Score != nil {
			scored++
		}
		if trial.Status == EvaluationError {
			errors++
		}
		if trial.Error == "context deadline exceeded" {
			timeouts++
		}
		latencies = append(latencies, trial.LatencyMS)
		if trial.TTFTMS != nil {
			ttfts = append(ttfts, *trial.TTFTMS)
		}
		if trial.LatencyMS > 0 && trial.OutputTokens > 0 {
			tps = append(tps, float64(trial.OutputTokens)/(trial.LatencyMS/1000))
		}
	}
	values["pass_rate"] = pass / float64(len(trials))
	values["evaluation_coverage"] = scored / float64(len(trials))
	values["error_rate"] = errors / float64(len(trials))
	values["timeout_rate"] = timeouts / float64(len(trials))
	for name, data := range map[string][]float64{"latency": latencies, "ttft": ttfts, "tokens_per_second": tps} {
		if len(data) == 0 {
			continue
		}
		values[name+"_mean"] = mean(data)
		values[name+"_p50"] = percentile(data, .50)
		values[name+"_p95"] = percentile(data, .95)
		values[name+"_p99"] = percentile(data, .99)
	}
	return Metrics{Values: values, Coverage: values["evaluation_coverage"]}
}

func mean(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func percentile(values []float64, p float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(p * float64(len(sorted)-1)))
	return sorted[index]
}
