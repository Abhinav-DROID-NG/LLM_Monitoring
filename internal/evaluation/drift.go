package evaluation

import "math"

type DriftStatus string

const (
	Normal            DriftStatus = "NORMAL"
	PotentialDrift    DriftStatus = "POTENTIAL_DRIFT"
	ConfirmedDrift    DriftStatus = "CONFIRMED_DRIFT"
	InconclusiveDrift DriftStatus = "INCONCLUSIVE"
	Recovered         DriftStatus = "RECOVERED"
)

type MetricBaseline struct {
	Mean float64 `json:"mean"`
	SD   float64 `json:"sd"`
	N    int     `json:"n"`
}
type Baseline struct {
	SchemaVersion string                    `json:"schema_version"`
	BaselineID    string                    `json:"baseline_id"`
	Objective     Objective                 `json:"objective"`
	Dataset       string                    `json:"dataset"`
	Metadata      Metadata                  `json:"metadata"`
	Config        Config                    `json:"config"`
	Metrics       map[string]MetricBaseline `json:"metrics"`
	Observations  []Trial                   `json:"observations"`
}
type MetricDrift struct {
	Metric     string      `json:"metric"`
	Status     DriftStatus `json:"status"`
	Baseline   float64     `json:"baseline"`
	Current    float64     `json:"current"`
	EffectSize float64     `json:"effect_size"`
	Direction  string      `json:"direction"`
	Reason     string      `json:"reason,omitempty"`
}
type DriftReport struct {
	Objective Objective     `json:"objective"`
	Status    DriftStatus   `json:"status"`
	Metrics   []MetricDrift `json:"metrics"`
}

func BuildBaseline(evaluations []Evaluation) Baseline {
	base := Baseline{SchemaVersion: SchemaVersion, Objective: Behavioural, Metrics: map[string]MetricBaseline{}}
	if len(evaluations) == 0 {
		return base
	}
	base.Objective, base.Dataset, base.Metadata, base.Config = evaluations[0].Objective, evaluations[0].Dataset, evaluations[0].Metadata, evaluations[0].Config
	values := map[string][]float64{}
	for _, evaluation := range evaluations {
		for key, value := range evaluation.Metrics.Values {
			values[key] = append(values[key], value)
		}
		base.Observations = append(base.Observations, evaluation.Trials...)
	}
	for key, samples := range values {
		mean := 0.0
		for _, value := range samples {
			mean += value
		}
		mean /= float64(len(samples))
		variance := 0.0
		for _, value := range samples {
			variance += (value - mean) * (value - mean)
		}
		if len(samples) > 1 {
			variance /= float64(len(samples) - 1)
		}
		base.Metrics[key] = MetricBaseline{Mean: mean, SD: math.Sqrt(variance), N: len(samples)}
	}
	return base
}

func Compare(base Baseline, current Evaluation, minEffect map[string]float64) DriftReport {
	report := DriftReport{Objective: current.Objective, Status: Normal, Metrics: []MetricDrift{}}
	if base.Objective != current.Objective || base.Dataset != current.Dataset {
		report.Status = InconclusiveDrift
		return report
	}
	for key, stats := range base.Metrics {
		value, ok := current.Metrics.Values[key]
		if !ok || stats.N < 2 {
			report.Metrics = append(report.Metrics, MetricDrift{Metric: key, Status: InconclusiveDrift, Baseline: stats.Mean, Reason: "missing metric or insufficient baseline samples"})
			continue
		}
		floor := math.Max(stats.SD, math.Abs(stats.Mean)*.05)
		effect := (value - stats.Mean) / math.Max(floor, 1e-9)
		threshold := minEffect[key]
		if threshold == 0 {
			threshold = math.Abs(stats.Mean) * .1
			if threshold == 0 {
				threshold = .1
			}
		}
		status := Normal
		if math.Abs(value-stats.Mean) >= threshold {
			status = PotentialDrift
		}
		if math.Abs(value-stats.Mean) >= threshold*2 && math.Abs(effect) >= 2 {
			status = ConfirmedDrift
		}
		direction := "unchanged"
		if value > stats.Mean {
			direction = "increased"
		} else if value < stats.Mean {
			direction = "decreased"
		}
		report.Metrics = append(report.Metrics, MetricDrift{Metric: key, Status: status, Baseline: stats.Mean, Current: value, EffectSize: effect, Direction: direction})
		if status == ConfirmedDrift {
			report.Status = ConfirmedDrift
		} else if status == PotentialDrift && report.Status == Normal {
			report.Status = PotentialDrift
		}
	}
	return report
}
