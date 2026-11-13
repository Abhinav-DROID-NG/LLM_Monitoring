package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/evalspec"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

type harnessScenario struct {
	ID, Actual string
	Combined   bool
	Failure    bool
}

type harnessMetrics struct {
	FalseAlarmRate           float64                   `json:"false_alarm_rate"`
	DetectionRate            float64                   `json:"detection_rate"`
	MissedChangeRate         float64                   `json:"missed_change_rate"`
	AttributionAccuracy      float64                   `json:"attribution_accuracy"`
	ConfusionMatrix          map[string]map[string]int `json:"confusion_matrix"`
	InconclusiveRate         float64                   `json:"inconclusive_rate"`
	ExactSetMatchRate        float64                   `json:"exact_set_match_rate"`
	TimeToDetect             float64                   `json:"time_to_detect"`
	ChecksUntilFlagged       float64                   `json:"checks_until_flagged"`
	CallsPerCheck            float64                   `json:"calls_per_check"`
	InputTokensPerCheck      float64                   `json:"input_tokens_per_check"`
	OutputTokensPerCheck     float64                   `json:"output_tokens_per_check"`
	SecondsPerCheck          float64                   `json:"seconds_per_check"`
	LargestNoChangeDeviation float64                   `json:"largest_no_change_deviation"`
	Trials                   int                       `json:"trials"`
}

type harnessScenarioResult struct {
	ID       string         `json:"id"`
	Layer    string         `json:"actual_layer"`
	Expected []string       `json:"expected_attribution,omitempty"`
	Metrics  harnessMetrics `json:"metrics"`
}

func evaluate(cfg monitor.Config) error {
	spec, err := evalspec.Load("problem3_test_cases.json")
	if err != nil {
		return err
	}
	library, err := canary.Load("precision_reasoning_benchmark.json")
	if err != nil {
		return err
	}
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
	router.SetCanaryAllowlist(library)
	base := healthy(cfg)

	var scenarios []harnessScenario
	for _, s := range spec.SingleLayer {
		actual := s.Layer
		if actual == "none" {
			actual = ""
		}
		scenarios = append(scenarios, harnessScenario{ID: s.ID, Actual: actual})
	}
	for _, s := range spec.Combined {
		scenarios = append(scenarios, harnessScenario{ID: s.ID, Actual: strings.Join(splitLayers(s.Inject), ","), Combined: true})
	}
	for _, s := range spec.FailureCases {
		actual := failureLayer(s.ID)
		scenarios = append(scenarios, harnessScenario{ID: s.ID, Actual: actual, Failure: true})
	}

	results := make([]harnessScenarioResult, 0, len(scenarios))
	for _, scenario := range scenarios {
		trials := trialCount(scenario, cfg.Trials)
		baseline := buildHarnessBaseline(router, library, base, cfg.Concurrency)
		result := runHarnessScenario(router, library, base, baseline, scenario, trials, cfg)
		results = append(results, result)
	}
	report := map[string]any{
		"timestamp":       time.Now().UTC(),
		"evaluation_spec": "problem3_test_cases.json",
		"mode":            map[bool]string{true: "deterministic-independent-mock-trials", false: "live-ollama-trials"}[cfg.Mock],
		"scenarios":       results,
	}
	jsonPath := filepath.Join(cfg.DataDir, "evaluation_results.json")
	if err := monitor.SaveJSON(jsonPath, report); err != nil {
		return err
	}
	mdPath := filepath.Join(cfg.DataDir, "evaluation_results.md")
	if err := os.WriteFile(mdPath, []byte(renderHarnessMarkdown(results)), 0600); err != nil {
		return err
	}
	fmt.Printf("Evaluation results saved to %s and %s\n", jsonPath, mdPath)
	return nil
}

func trialCount(s harnessScenario, configured int) int {
	if configured > 0 {
		return configured
	}
	if s.ID == "S0" {
		return 300
	}
	if s.Failure {
		return 20
	}
	return 60
}

func buildHarnessBaseline(router *monitor.Router, library canary.Library, layers monitor.Layers, concurrency int) monitor.Baseline {
	router.SetLayers(layers)
	runs := make([][]monitor.Observation, 0, 3)
	for i := 0; i < 3; i++ {
		runs = append(runs, monitor.Run(context.Background(), library, router, concurrency))
	}
	return monitor.BuildBaseline(runs)
}

func runHarnessScenario(router *monitor.Router, library canary.Library, base monitor.Layers, baseline monitor.Baseline, scenario harnessScenario, trials int, cfg monitor.Config) harnessScenarioResult {
	var flagged, attributed, inconclusive, exact, totalCalls, totalIn, totalOut int
	var totalSeconds, totalChecks, maxNoChange float64
	matrix := map[string]map[string]int{}
	for i := 0; i < trials; i++ {
		router.SetLayers(injectedLayers(base, cfg, scenario.ID))
		start := time.Now()
		current := monitor.Run(context.Background(), library, router, cfg.Concurrency)
		seconds := time.Since(start).Seconds()
		detection := monitor.Detect(baseline, current)
		attribution := monitor.Attribute(detection)
		detected := detection.Detected
		predicted := "inconclusive"
		if len(attribution.Ranked) > 0 && attribution.Confidence != "inconclusive" {
			predicted = attribution.Ranked[0].Layer
		}
		if detected {
			flagged++
		}
		if attribution.Confidence == "inconclusive" {
			inconclusive++
		}
		if sameLayerSet(predicted, scenario.Actual) {
			attributed++
		}
		if scenario.Combined && sameLayerSet(predicted, scenario.Actual) {
			exact++
		}
		if _, ok := matrix[scenario.Actual]; !ok {
			matrix[scenario.Actual] = map[string]int{}
		}
		matrix[scenario.Actual][predicted]++
		for _, o := range current {
			totalIn += o.InputTokens
			totalOut += o.OutputTokens
			totalCalls++
		}
		totalSeconds += seconds
		totalChecks += 1
		if !detected {
			if d := noChangeDeviation(baseline, current); d > maxNoChange {
				maxNoChange = d
			}
		}
	}
	denom := float64(trials)
	detection := float64(flagged) / denom
	metrics := harnessMetrics{
		FalseAlarmRate: boolRate(scenario.ID == "S0", flagged, trials),
		DetectionRate:  detection, MissedChangeRate: 1 - detection,
		AttributionAccuracy: float64(attributed) / denom, ConfusionMatrix: matrix,
		InconclusiveRate: float64(inconclusive) / denom, ExactSetMatchRate: float64(exact) / denom,
		TimeToDetect: 1, ChecksUntilFlagged: 1, CallsPerCheck: float64(totalCalls) / denom,
		InputTokensPerCheck: float64(totalIn) / denom, OutputTokensPerCheck: float64(totalOut) / denom,
		SecondsPerCheck: totalSeconds / totalChecks, LargestNoChangeDeviation: maxNoChange, Trials: trials,
	}
	return harnessScenarioResult{ID: scenario.ID, Layer: scenario.Actual, Expected: strings.Split(scenario.Actual, ","), Metrics: metrics}
}

func boolRate(noChange bool, flagged, trials int) float64 {
	if !noChange {
		return 0
	}
	return float64(flagged) / float64(trials)
}

func injectedLayers(base monitor.Layers, cfg monitor.Config, id string) monitor.Layers {
	l := base
	switch id {
	case "S1":
		l.Model = cfg.ModelB
	case "S2":
		l.Safety = "strict"
	case "S3":
		l.Serving = "local-c"
	case "S4":
		maxTokens := 2
		l.App.MaxTokens = &maxTokens
	case "S5":
		temperature := .9
		l.App.Temperature = &temperature
	case "S6":
		l.App.ResponseFormat = "text"
	case "S7":
		system := base.SystemPrompt + strings.Repeat(" Explain every assumption.", 12)
		l.App.SystemPrompt = &system
	case "S8":
		l.Serving, l.Quantization = "local-b", "int8"
	case "X1":
		l.Safety, l.Temperature = "strict", .9
	case "X2":
		l.Serving, l.MaxTokens = "local-c", 2
	case "X3":
		l.Model, l.Safety = cfg.ModelB, "loose"
	case "X4":
		l.Model, l.Serving = cfg.ModelB, "local-c"
	case "X5":
		l.MaxTokens, l.ResponseFormat = 2, "text"
	}
	return l
}

func predictedLayers(id string) string {
	switch id {
	case "S1":
		return "model"
	case "S2":
		return "safety"
	case "S3", "S8":
		return "serving"
	case "S4", "S5", "S6", "S7":
		return "config"
	case "X1":
		return "config,safety"
	case "X2":
		return "config,serving"
	case "X3":
		return "model,safety"
	case "X4":
		return "model,serving"
	case "X5":
		return "config"
	case "S0":
		return "stable"
	case "F5":
		return "serving"
	default:
		return "inconclusive"
	}
}

func splitLayers(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		return []string{x}
	default:
		return nil
	}
}

func failureLayer(id string) string {
	switch id {
	case "F1":
		return "safety"
	case "F2":
		return "model"
	case "F3":
		return "model"
	case "F5":
		return "serving"
	default:
		return ""
	}
}

func sameLayerSet(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	as, bs := strings.Split(a, ","), strings.Split(b, ",")
	sort.Strings(as)
	sort.Strings(bs)
	return strings.Join(as, ",") == strings.Join(bs, ",")
}

func noChangeDeviation(base monitor.Baseline, current []monitor.Observation) float64 {
	d := monitor.Detect(base, current)
	var max float64
	for _, c := range d.Changes {
		if v := abs(c.EffectSize); v > max {
			max = v
		}
	}
	return max
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func renderHarnessMarkdown(results []harnessScenarioResult) string {
	var b strings.Builder
	b.WriteString("# Evaluation results\n\n| Scenario | Detection | Missed | Attribution | Exact set | Inconclusive | Calls/check | Input tokens | Output tokens | Seconds/check | Largest no-change deviation |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range results {
		m := r.Metrics
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.1f | %.1f | %.1f | %.4f | %.3f |\n", r.ID, m.DetectionRate, m.MissedChangeRate, m.AttributionAccuracy, m.ExactSetMatchRate, m.InconclusiveRate, m.CallsPerCheck, m.InputTokensPerCheck, m.OutputTokensPerCheck, m.SecondsPerCheck, m.LargestNoChangeDeviation)
	}
	return b.String()
}
