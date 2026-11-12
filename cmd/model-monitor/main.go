package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
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
	if err := cfg.Normalize(); err != nil {
		return err
	}
	switch command {
	case "run", "baseline", "start":
		return execute(command, cfg)
	case "evaluate":
		return evaluate(cfg)
	default:
		return errors.New("usage: model-monitor [baseline|run|start|evaluate]")
	}
}

func execute(command string, cfg monitor.Config) error {
	library, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
	aliasServer := httptest.NewServer(router.Handler())
	defer aliasServer.Close()
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
	return monitor.Layers{Model: cfg.ModelA, Safety: "strict", RequestConfig: "default", Serving: "local-a", Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, SystemPrompt: cfg.SystemPrompt}
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

func evaluate(cfg monitor.Config) error {
	baseline, baselinePath, err := loadLatestBaseline(cfg.DataDir)
	if err != nil {
		return err
	}
	fmt.Printf("Loaded baseline from: %s (%d observations)\n\n", baselinePath, len(baseline))

	library, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
	aliasServer := httptest.NewServer(router.Handler())
	defer aliasServer.Close()

	// Switch the active router layers to lower model (Model B)
	bLayers := healthy(cfg)
	bLayers.Model = cfg.ModelB
	router.SetLayers(bLayers)

	fmt.Printf("Running 1 evaluation round for lower model (Model B: %s)...\n\n", cfg.ModelB)
	current := monitor.RunViaAlias(context.Background(), library, aliasServer.URL+"/v1/chat/completions", nil, cfg.Concurrency)

	// Cross-check against baseline
	baseMap := make(map[string]monitor.Observation, len(baseline))
	for _, obs := range baseline {
		baseMap[obs.CanaryID] = obs
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

	var withBaseline, belowBaseline, aboveBaseline int
	var comparisons []TaskComparison
	var belowDetails []string
	var withDetails []string

	for _, cur := range current {
		base, exists := baseMap[cur.CanaryID]
		comp := TaskComparison{
			CanaryID:        cur.CanaryID,
			TotalChecks:     len(cur.Checks),
			BaselineLatency: float64(base.LatencyMS) / 1000.0,
			EvalLatency:     float64(cur.LatencyMS) / 1000.0,
		}

		basePassed := 0
		for _, c := range base.Checks {
			if c.Passed {
				basePassed++
			}
		}
		curPassed := 0
		for _, c := range cur.Checks {
			if c.Passed {
				curPassed++
			}
		}
		comp.BaselineChecks = basePassed
		comp.EvalChecks = curPassed

		if !exists {
			comp.Status = "NEW"
			comparisons = append(comparisons, comp)
			continue
		}

		if cur.Error != "" && base.Error == "" {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Execution error: %s", cur.Error))
		}

		// Check for check regressions
		for i, curChk := range cur.Checks {
			if i < len(base.Checks) {
				if base.Checks[i].Passed && !curChk.Passed {
					comp.Reasons = append(comp.Reasons, fmt.Sprintf("Check '%s' failed", curChk.Type))
				}
			}
		}

		// Finish reason regression
		if base.FinishReason == "stop" && cur.FinishReason != "stop" && cur.FinishReason != "" {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Finish reason changed from stop to %s", cur.FinishReason))
		}

		// Significant latency degradation (> 2.5x slower)
		if base.LatencyMS > 0 && cur.LatencyMS > base.LatencyMS*5/2 {
			comp.Reasons = append(comp.Reasons, fmt.Sprintf("Latency degraded (%.2fs vs base %.2fs)", comp.EvalLatency, comp.BaselineLatency))
		}

		if len(comp.Reasons) > 0 || curPassed < basePassed {
			comp.Status = "BELOW_BASELINE"
			belowBaseline++
			reasonStr := strings.Join(comp.Reasons, "; ")
			if reasonStr == "" {
				reasonStr = fmt.Sprintf("Check pass dropped (%d/%d vs base %d/%d)", curPassed, comp.TotalChecks, basePassed, comp.TotalChecks)
			}
			belowDetails = append(belowDetails, fmt.Sprintf("[%s] %s (latency: %.2fs vs %.2fs)", cur.CanaryID, reasonStr, comp.EvalLatency, comp.BaselineLatency))
		} else if curPassed > basePassed {
			comp.Status = "ABOVE_BASELINE"
			aboveBaseline++
			withBaseline++
			withDetails = append(withDetails, fmt.Sprintf("[%s] Improved (%d/%d vs base %d/%d, latency: %.2fs vs %.2fs)", cur.CanaryID, curPassed, comp.TotalChecks, basePassed, comp.TotalChecks, comp.EvalLatency, comp.BaselineLatency))
		} else {
			comp.Status = "WITH_BASELINE"
			withBaseline++
			withDetails = append(withDetails, fmt.Sprintf("[%s] Matched baseline (%d/%d checks passed, latency: %.2fs)", cur.CanaryID, curPassed, comp.TotalChecks, comp.EvalLatency))
		}

		comparisons = append(comparisons, comp)
	}

	analysis := monitor.Analyze(baseline, current)
	control := monitor.Analyze(baseline, baseline)

	evalResults := []map[string]any{
		{"experiment_id": "control-baseline", "actual_layer": "", "predicted_layer": control.PredictedLayer, "detected": control.Detected},
		{"experiment_id": "eval-" + cfg.ModelB, "actual_layer": "model", "predicted_layer": analysis.PredictedLayer, "detected": analysis.Detected},
	}

	report := map[string]any{
		"experiment_id":    fmt.Sprintf("eval-%s-vs-%s", cfg.ModelB, cfg.ModelA),
		"baseline_file":    baselinePath,
		"baseline_model":   cfg.ModelA,
		"evaluated_model":  cfg.ModelB,
		"total_tasks":      len(current),
		"with_baseline":    withBaseline,
		"below_baseline":   belowBaseline,
		"above_baseline":   aboveBaseline,
		"drift_detected":   analysis.Detected,
		"drift_confidence": analysis.Confidence,
		"predicted_layer":  analysis.PredictedLayer,
		"drift_evidence":   analysis.Evidence,
		"metrics":          score(evalResults),
		"task_comparisons": comparisons,
	}

	fmt.Println("\n================================================================================")
	fmt.Println("                       MODEL EVALUATION REPORT                                 ")
	fmt.Println("================================================================================")
	fmt.Printf("Baseline (Model A):  %s (%s)\n", cfg.ModelA, filepath.Base(baselinePath))
	fmt.Printf("Evaluated (Model B): %s (1 round, %d tasks)\n\n", cfg.ModelB, len(current))

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("SUMMARY COMPARISON WITH BASELINE:")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("Total Tasks Evaluated:  %d\n", len(current))
	fmt.Printf("At / With Baseline:     %d (%.1f%%)\n", withBaseline, float64(withBaseline)*100.0/float64(len(current)))
	fmt.Printf("Below Baseline:         %d (%.1f%%)\n", belowBaseline, float64(belowBaseline)*100.0/float64(len(current)))
	fmt.Printf("Drift Detected:         %v (Confidence: %.2f)\n", analysis.Detected, analysis.Confidence)
	fmt.Printf("Predicted Shift Layer:  %s\n\n", analysis.PredictedLayer)

	if len(belowDetails) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf("TASKS BELOW BASELINE (%d Regressions):\n", len(belowDetails))
		fmt.Println("--------------------------------------------------------------------------------")
		for _, d := range belowDetails {
			fmt.Printf(" [BELOW] %s\n", d)
		}
		fmt.Println()
	}

	if len(withDetails) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf("TASKS AT OR ABOVE BASELINE (%d Parity/Maintained):\n", len(withDetails))
		fmt.Println("--------------------------------------------------------------------------------")
		sampleCount := 5
		if len(withDetails) < sampleCount {
			sampleCount = len(withDetails)
		}
		for i := 0; i < sampleCount; i++ {
			fmt.Printf(" [OK]    %s\n", dOrSample(withDetails, i))
		}
		if len(withDetails) > sampleCount {
			fmt.Printf(" ... and %d more tasks matched baseline\n", len(withDetails)-sampleCount)
		}
		fmt.Println()
	}

	evalPath := filepath.Join(cfg.DataDir, "evaluation.json")
	if err := monitor.SaveJSON(evalPath, report); err != nil {
		return err
	}
	fmt.Printf("Detailed evaluation saved to: %s\n================================================================================\n\n", evalPath)
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
	client := &http.Client{Timeout: 60 * time.Second}
	localA := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	localB := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	localC := monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client}
	return monitor.NewProviderRouter(map[string]monitor.Upstream{
		cfg.ModelA: localA, cfg.ModelB: localB, cfg.ModelC: localC,
		cfg.ModelA + "|local-a": localA, cfg.ModelA + "|local-c": localA,
		cfg.ModelB + "|local-a": localB, cfg.ModelB + "|local-c": localB,
		cfg.ModelC + "|local-a": localC, cfg.ModelC + "|local-c": localC,
	}, healthy(cfg))
}
