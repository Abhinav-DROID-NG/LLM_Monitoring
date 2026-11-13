package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/evalspec"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	command := "run"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	cfg := monitor.DefaultConfig()
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	mock := flags.Bool("mock", false, "use the offline mock provider")
	profile := flags.String("profile", "monitoring", "execution profile")
	tier := flags.String("tier", "", "comma-separated tiers")
	dataDir := flags.String("data-dir", "", "data directory")
	seed := flags.Int64("seed", 1, "random seed")
	trials := flags.Int("trials", 0, "evaluation trial count")
	concurrency := flags.Int("concurrency", 0, "probe concurrency")
	quiet := flags.Bool("quiet", false, "suppress progress output")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	cfg.Mock, cfg.Profile, cfg.Tier, cfg.Seed, cfg.Trials, cfg.Quiet = *mock, *profile, *tier, *seed, *trials, *quiet
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}
	if *concurrency > 0 {
		cfg.Concurrency = *concurrency
	}
	if err := cfg.Normalize(); err != nil {
		return err
	}
	switch command {
	case "run", "baseline", "start":
		return execute(command, cfg)
	case "serve":
		return serve(cfg)
	case "evaluate":
		return evaluate(cfg)
	case "compare":
		return legacyEvaluate(cfg)
	case "validate":
		_, err := canary.Load("precision_reasoning_benchmark.json")
		if err == nil {
			fmt.Println("canary library is valid")
		}
		return err
	case "privacy-check":
		library, err := canary.Load("precision_reasoning_benchmark.json")
		if err != nil {
			return err
		}
		if err := monitor.PrivacyCheck(library, nil); err != nil {
			return err
		}
		fmt.Println("stored_real_user_content = 0")
		return nil
	default:
		return errors.New("usage: model-monitor [baseline|run|start|evaluate|compare|validate|privacy-check]")
	}
}

func execute(command string, cfg monitor.Config) error {
	library, err := canary.Load("precision_reasoning_benchmark.json")
	if err != nil {
		return err
	}
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
	aliasServer := httptest.NewServer(router.Handler())
	defer aliasServer.Close()
	router.SetCanaryAllowlist(library)
	layers := healthy(cfg)
	var entries []monitor.GroundTruthEntry
	cycle := 0
	run := func() error {
		if command == "start" && cycle > 0 && cycle%4 == 0 {
			previous := layers
			layers = scheduledLayers(cfg, layers, cycle/4)
			router.SetLayers(layers)
			entries = append(entries, monitor.GroundTruthEntry{Timestamp: time.Now().UTC(), Previous: previous, New: layers,
				Layer: scheduledLayer(cycle / 4), Experiment: fmt.Sprintf("scheduled-%d", cycle/4)})
			sealed, err := monitor.NewSealedLog(entries)
			if err != nil {
				return err
			}
			if err := sealed.Save(filepath.Join(cfg.DataDir, "ground-truth.sealed.json")); err != nil {
				return err
			}
		}
		cycle++
		observations := monitor.RunViaAlias(context.Background(), library, aliasServer.URL+"/v1/chat/completions", nil, cfg.Concurrency)
		outPath := filepath.Join(cfg.DataDir, command+"-"+time.Now().UTC().Format("20060102T150405Z")+".json")
		if err := monitor.SaveJSON(outPath, observations); err != nil {
			return err
		}
		fmt.Printf("Successfully saved %d observations to %s\n\n", len(observations), outPath)
		return nil
	}
	if command != "start" {
		return run()
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		if err := run(); err != nil {
			return err
		}
		<-ticker.C
	}
}

func scheduledLayer(epoch int) string {
	return []string{"model", "safety", "request_config", "serving"}[(epoch-1)%4]
}

func scheduledLayers(cfg monitor.Config, layers monitor.Layers, epoch int) monitor.Layers {
	next := layers
	switch scheduledLayer(epoch) {
	case "model":
		next.Model = cfg.ModelB
	case "safety":
		if next.Safety == "strict" {
			next.Safety = "loose"
		} else {
			next.Safety = "strict"
		}
	case "request_config":
		if next.RequestConfig == "default" {
			next.RequestConfig, next.Temperature, next.MaxTokens, next.SystemPrompt = "changed", 0.8, 64, "Be concise and answer in JSON."
		} else {
			next.RequestConfig, next.Temperature, next.MaxTokens, next.SystemPrompt = "default", cfg.Temperature, cfg.MaxTokens, cfg.SystemPrompt
		}
	case "serving":
		if next.Serving == "local-a" {
			next.Serving = "local-c"
		} else {
			next.Serving = "local-a"
		}
	}
	return next
}

func healthy(cfg monitor.Config) monitor.Layers {
	return monitor.Layers{Model: cfg.ModelA, Safety: "normal", RequestConfig: "default", Serving: "local-a", Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, SystemPrompt: cfg.SystemPrompt}
}

func loadLatestBaseline(dataDir string) ([]monitor.Observation, string, error) {
	files, err := filepath.Glob(filepath.Join(dataDir, "baseline-*.json"))
	if err != nil || len(files) == 0 {
		return nil, "", fmt.Errorf("no baseline file found in '%s'; please run 'go run ./cmd/model-monitor baseline' first", dataDir)
	}
	sort.Strings(files)
	latest := files[len(files)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		return nil, "", fmt.Errorf("read baseline %s: %w", latest, err)
	}
	var observations []monitor.Observation
	if err := json.Unmarshal(data, &observations); err != nil {
		return nil, "", fmt.Errorf("decode baseline %s: %w", latest, err)
	}
	return observations, latest, nil
}

type TaskComparison struct {
	CanaryID        string   `json:"canary_id"`
	Status          string   `json:"status"`
	Reasons         []string `json:"reasons,omitempty"`
	BaselineChecks  int      `json:"baseline_checks_passed"`
	EvalChecks      int      `json:"eval_checks_passed"`
	TotalChecks     int      `json:"total_checks"`
	BaselineLatency float64  `json:"baseline_latency_sec"`
	EvalLatency     float64  `json:"eval_latency_sec"`
}

type ModelEvalResult struct {
	ModelName      string
	Observations   []monitor.Observation
	WithBaseline   int
	BelowBaseline  int
	AboveBaseline  int
	BelowDetails   []string
	WithDetails    []string
	Comparisons    []TaskComparison
	Analysis       monitor.Analysis
	TotalDuration  time.Duration
	AvgLatencySec  float64
	ChecksPassRate float64
}

func evaluateModel(modelName string, baseline []monitor.Observation, library canary.Library, router *monitor.Router, endpoint string, cfg monitor.Config) ModelEvalResult {
	layers := healthy(cfg)
	layers.Model = modelName
	router.SetLayers(layers)

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("EVALUATING MODEL: %s (1 round, %d tasks)\n", modelName, len(library.Tasks))
	fmt.Println("--------------------------------------------------------------------------------")

	start := time.Now()
	current := monitor.RunViaAlias(context.Background(), library, endpoint, nil, cfg.Concurrency)
	duration := time.Since(start)

	modelSlug := strings.ReplaceAll(strings.ReplaceAll(modelName, ":", "-"), "/", "-")
	_ = monitor.SaveJSON(filepath.Join(cfg.DataDir, fmt.Sprintf("eval-%s-%s.json", modelSlug, time.Now().UTC().Format("20060102T150405Z"))), current)

	baseMap := make(map[string]monitor.Observation, len(baseline))
	for _, obs := range baseline {
		baseMap[obs.CanaryID] = obs
	}

	res := ModelEvalResult{
		ModelName:     modelName,
		Observations:  current,
		TotalDuration: duration,
	}

	var totalLatency float64
	var totalChecks, totalChecksPassed int

	for _, cur := range current {
		base, exists := baseMap[cur.CanaryID]
		comp := TaskComparison{
			CanaryID:        cur.CanaryID,
			TotalChecks:     len(cur.Checks),
			BaselineLatency: float64(base.LatencyMS) / 1000.0,
			EvalLatency:     float64(cur.LatencyMS) / 1000.0,
		}
		totalLatency += comp.EvalLatency

		if !exists {
			comp.Status = "NEW"
			res.Comparisons = append(res.Comparisons, comp)
			continue
		}

		basePassed, curPassed, comparable, regressions := comparableCheckCounts(base.Checks, cur.Checks)
		comp.TotalChecks = comparable
		comp.BaselineChecks = basePassed
		comp.EvalChecks = curPassed
		totalChecks += comparable
		totalChecksPassed += curPassed

		if cur.Error != "" && base.Error == "" {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Execution error: %s", cur.Error))
		}

		for _, regression := range regressions {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Check '%s' failed", regression))
		}

		if base.FinishReason == "stop" && cur.FinishReason != "stop" && cur.FinishReason != "" {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Finish reason changed from stop to %s", cur.FinishReason))
		}

		if len(comp.Reasons) > 0 || curPassed < basePassed {
			comp.Status = "BELOW_BASELINE"
			res.BelowBaseline++
			reasonStr := strings.Join(comp.Reasons, "; ")
			if reasonStr == "" {
				reasonStr = fmt.Sprintf("Check pass dropped (%d/%d vs base %d/%d)", curPassed, comp.TotalChecks, basePassed, comp.TotalChecks)
			}
			res.BelowDetails = append(res.BelowDetails, fmt.Sprintf("[%s] %s (latency: %.2fs vs %.2fs)", cur.CanaryID, reasonStr, comp.EvalLatency, comp.BaselineLatency))
		} else if curPassed > basePassed {
			comp.Status = "ABOVE_BASELINE"
			res.AboveBaseline++
			res.WithBaseline++
			res.WithDetails = append(res.WithDetails, fmt.Sprintf("[%s] Improved (%d/%d vs base %d/%d, latency: %.2fs vs %.2fs)", cur.CanaryID, curPassed, comp.TotalChecks, basePassed, comp.TotalChecks, comp.EvalLatency, comp.BaselineLatency))
		} else {
			comp.Status = "WITH_BASELINE"
			res.WithBaseline++
			res.WithDetails = append(res.WithDetails, fmt.Sprintf("[%s] Matched baseline (%d/%d checks passed, latency: %.2fs)", cur.CanaryID, curPassed, comp.TotalChecks, comp.EvalLatency))
		}

		res.Comparisons = append(res.Comparisons, comp)
	}

	if len(current) > 0 {
		res.AvgLatencySec = totalLatency / float64(len(current))
	}
	if totalChecks > 0 {
		res.ChecksPassRate = float64(totalChecksPassed) * 100.0 / float64(totalChecks)
	}
	res.Analysis = monitor.Analyze(baseline, current)
	return res
}

func comparableCheckCounts(base, current []monitor.CheckObservation) (basePassed, currentPassed, comparable int, regressions []string) {
	baseMap, baseInvalid := indexChecksForEvaluation(base)
	currentMap, currentInvalid := indexChecksForEvaluation(current)
	if baseInvalid || currentInvalid {
		return 0, 0, 0, nil
	}
	for key, before := range baseMap {
		after, ok := currentMap[key]
		if !ok || before.Type != after.Type || before.Unavailable || after.Unavailable {
			continue
		}
		comparable++
		if before.Passed {
			basePassed++
		}
		if after.Passed {
			currentPassed++
		}
		if before.Passed && !after.Passed {
			regressions = append(regressions, after.Type)
		}
	}
	return
}

func indexChecksForEvaluation(checks []monitor.CheckObservation) (map[string]monitor.CheckObservation, bool) {
	result := make(map[string]monitor.CheckObservation, len(checks))
	for _, check := range checks {
		key := check.CheckID
		if key == "" {
			key = "legacy:" + check.Type + ":" + check.Expected
		}
		if _, exists := result[key]; exists {
			return nil, true
		}
		result[key] = check
	}
	return result, false
}

func legacyEvaluate(cfg monitor.Config) error {
	spec, err := evalspec.Load("problem3_test_cases.json")
	if err != nil {
		return err
	}
	baseline, baselinePath, err := loadLatestBaseline(cfg.DataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Loaded baseline from: %s (%d observations, Model A: %s)\n\n", baselinePath, len(baseline), cfg.ModelA)

	library, err := canary.Load("precision_reasoning_benchmark.json")
	if err != nil {
		return err
	}
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
	aliasServer := httptest.NewServer(router.Handler())
	defer aliasServer.Close()
	router.SetCanaryAllowlist(library)

	// Evaluate Model B and Model C sequentially
	resB := evaluateModel(cfg.ModelB, baseline, library, router, aliasServer.URL+"/v1/chat/completions", cfg)
	resC := evaluateModel(cfg.ModelC, baseline, library, router, aliasServer.URL+"/v1/chat/completions", cfg)

	// Baseline stats
	var baseTotalLatency float64
	var baseTotalChecks, baseTotalChecksPassed int
	for _, b := range baseline {
		baseTotalLatency += float64(b.LatencyMS) / 1000.0
		for _, c := range b.Checks {
			if c.Passed {
				baseTotalChecksPassed++
			}
			baseTotalChecks++
		}
	}
	baseAvgLatency := 0.0
	if len(baseline) > 0 {
		baseAvgLatency = baseTotalLatency / float64(len(baseline))
	}
	basePassRate := 0.0
	if baseTotalChecks > 0 {
		basePassRate = float64(baseTotalChecksPassed) * 100.0 / float64(baseTotalChecks)
	}

	bEvalResults := []map[string]any{
		{"experiment_id": "eval-" + cfg.ModelB, "actual_layer": "model", "predicted_layer": resB.Analysis.PredictedLayer, "detected": resB.Analysis.Detected},
	}
	cEvalResults := []map[string]any{
		{"experiment_id": "eval-" + cfg.ModelC, "actual_layer": "model", "predicted_layer": resC.Analysis.PredictedLayer, "detected": resC.Analysis.Detected},
	}

	report := map[string]any{
		"experiment_id":  fmt.Sprintf("eval-B(%s)-and-C(%s)-vs-A(%s)", cfg.ModelB, cfg.ModelC, cfg.ModelA),
		"timestamp":      time.Now().UTC(),
		"baseline_file":  baselinePath,
		"baseline_model": cfg.ModelA,
		"evaluation_spec": map[string]int{
			"single_layer":  len(spec.SingleLayer),
			"combined":      len(spec.Combined),
			"failure_cases": len(spec.FailureCases),
		},
		"model_b": map[string]any{
			"model":           cfg.ModelB,
			"duration":        resB.TotalDuration.String(),
			"avg_latency_sec": resB.AvgLatencySec,
			"check_pass_rate": resB.ChecksPassRate,
			"with_baseline":   resB.WithBaseline,
			"below_baseline":  resB.BelowBaseline,
			"drift_analysis":  resB.Analysis,
			"metrics":         score(bEvalResults),
			"comparisons":     resB.Comparisons,
		},
		"model_c": map[string]any{
			"model":           cfg.ModelC,
			"duration":        resC.TotalDuration.String(),
			"avg_latency_sec": resC.AvgLatencySec,
			"check_pass_rate": resC.ChecksPassRate,
			"with_baseline":   resC.WithBaseline,
			"below_baseline":  resC.BelowBaseline,
			"drift_analysis":  resC.Analysis,
			"metrics":         score(cEvalResults),
			"comparisons":     resC.Comparisons,
		},
	}

	fmt.Println("\n==========================================================================================")
	fmt.Println("                       MULTI-MODEL EVALUATION REPORT (A vs B vs C)                        ")
	fmt.Println("==========================================================================================")
	fmt.Printf("Baseline File: %s\n\n", filepath.Base(baselinePath))

	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Metric", "Model A (FP16)", "Model B (Q8_0)", "Model C (Q4_K_M)")
	fmt.Println("------------------------------------------------------------------------------------------")
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Model Tag", cfg.ModelA, cfg.ModelB, cfg.ModelC)
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Approx Disk Size", "~6.2 GB", "~3.3 GB", "~1.9 GB")
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Run Duration", "Baseline Reference", resB.TotalDuration.Round(time.Second).String(), resC.TotalDuration.Round(time.Second).String())
	fmt.Printf("%-24s | %-18.2fs | %-18.2fs | %-18.2fs\n", "Average Latency", baseAvgLatency, resB.AvgLatencySec, resC.AvgLatencySec)
	fmt.Printf("%-24s | %-17.1f%% | %-17.1f%% | %-17.1f%%\n", "Check Pass Rate", basePassRate, resB.ChecksPassRate, resC.ChecksPassRate)
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "At/With Baseline", fmt.Sprintf("%d/%d (100%%)", len(baseline), len(baseline)), fmt.Sprintf("%d/%d (%.1f%%)", resB.WithBaseline, len(resB.Observations), float64(resB.WithBaseline)*100.0/float64(len(resB.Observations))), fmt.Sprintf("%d/%d (%.1f%%)", resC.WithBaseline, len(resC.Observations), float64(resC.WithBaseline)*100.0/float64(len(resC.Observations))))
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Below Baseline", "0 (0%)", fmt.Sprintf("%d/%d (%.1f%%)", resB.BelowBaseline, len(resB.Observations), float64(resB.BelowBaseline)*100.0/float64(len(resB.Observations))), fmt.Sprintf("%d/%d (%.1f%%)", resC.BelowBaseline, len(resC.Observations), float64(resC.BelowBaseline)*100.0/float64(len(resC.Observations))))
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Drift Detected", "No (Baseline)", fmt.Sprintf("%v (conf: %.2f)", resB.Analysis.Detected, resB.Analysis.Confidence), fmt.Sprintf("%v (conf: %.2f)", resC.Analysis.Detected, resC.Analysis.Confidence))
	fmt.Printf("%-24s | %-18s | %-18s | %-18s\n", "Attributed Layer", "N/A", resB.Analysis.PredictedLayer, resC.Analysis.PredictedLayer)
	fmt.Println("------------------------------------------------------------------------------------------")

	if len(resB.BelowDetails) > 0 {
		fmt.Printf("\n>>> MODEL B (%s) REGRESSIONS BELOW BASELINE (%d):\n", cfg.ModelB, len(resB.BelowDetails))
		for _, d := range resB.BelowDetails {
			fmt.Printf(" [BELOW] %s\n", d)
		}
	}

	if len(resC.BelowDetails) > 0 {
		fmt.Printf("\n>>> MODEL C (%s) REGRESSIONS BELOW BASELINE (%d):\n", cfg.ModelC, len(resC.BelowDetails))
		for _, d := range resC.BelowDetails {
			fmt.Printf(" [BELOW] %s\n", d)
		}
	}

	evalPath := filepath.Join(cfg.DataDir, "evaluation.json")
	if err := monitor.SaveJSON(evalPath, report); err != nil {
		return err
	}
	fmt.Printf("\nComprehensive multi-model evaluation saved to: %s\n==========================================================================================\n\n", evalPath)
	return nil
}

func dOrSample(list []string, idx int) string {
	if idx < len(list) {
		return list[idx]
	}
	return ""
}

func score(results []map[string]any) map[string]float64 {
	var correct, predictedPositive, actualPositive, truePositive, falsePositive, falseNegative, layerCorrect float64
	for _, result := range results {
		actual := result["actual_layer"].(string)
		predicted := result["predicted_layer"].(string)
		detected := result["detected"].(bool)
		positive := actual != ""
		if positive {
			actualPositive++
		}
		if detected {
			predictedPositive++
		}
		if detected && positive {
			truePositive++
		}
		if !detected && positive {
			falseNegative++
		}
		if detected && !positive {
			falsePositive++
		}
		if detected == positive {
			correct++
		}
		if predicted == actual {
			layerCorrect++
		}
	}
	divide := func(n, d float64) float64 {
		if d == 0 {
			return 0
		}
		return n / d
	}
	return map[string]float64{"detection_accuracy": divide(correct, float64(len(results))),
		"precision": divide(truePositive, predictedPositive), "recall": divide(truePositive, actualPositive),
		"false_positive_rate":        divide(falsePositive, float64(len(results))-actualPositive),
		"false_negative_rate":        divide(falseNegative, actualPositive),
		"layer_attribution_accuracy": divide(layerCorrect, float64(len(results)))}
}

func newRouter(cfg monitor.Config) (*monitor.Router, error) {
	if cfg.Mock {
		return monitor.NewProviderRouter(map[string]monitor.Upstream{
			cfg.ModelA: monitor.NewMockProvider(1), cfg.ModelB: monitor.NewMockProvider(2), cfg.ModelC: monitor.NewMockProvider(3),
			cfg.ModelA + "|local-a": monitor.NewMockProvider(11), cfg.ModelA + "|local-b": monitor.NewMockProvider(12), cfg.ModelA + "|local-c": monitor.NewMockProvider(13),
			cfg.ModelB + "|local-a": monitor.NewMockProvider(21), cfg.ModelB + "|local-b": monitor.NewMockProvider(22), cfg.ModelB + "|local-c": monitor.NewMockProvider(23),
			cfg.ModelC + "|local-a": monitor.NewMockProvider(31), cfg.ModelC + "|local-b": monitor.NewMockProvider(32), cfg.ModelC + "|local-c": monitor.NewMockProvider(33),
		}, healthy(cfg))
	}
	client := &http.Client{Timeout: 60 * time.Second}
	localA := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	localB := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	localC := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	return monitor.NewProviderRouter(map[string]monitor.Upstream{
		cfg.ModelA: localA, cfg.ModelB: localB, cfg.ModelC: localC,
		cfg.ModelA + "|local-a": localA, cfg.ModelA + "|local-c": localC,
		cfg.ModelB + "|local-a": localB, cfg.ModelB + "|local-c": localC,
		cfg.ModelC + "|local-a": localC, cfg.ModelC + "|local-c": localA,
	}, healthy(cfg))
}
