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
		observations := monitor.RunViaAlias(context.Background(), library, aliasServer.URL+"/v1/chat/completions", nil, 4)
		return monitor.SaveJSON(filepath.Join(cfg.DataDir, command+"-"+time.Now().UTC().Format("20060102T150405Z")+".json"), observations)
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

func evaluate(cfg monitor.Config) error {
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

	reference := monitor.RunViaAlias(context.Background(), library, aliasServer.URL+"/v1/chat/completions", nil, 4)
	previous := router.Layers()
	experimentID := fmt.Sprintf("exp-%d", time.Now().UnixNano())
	scenarios := make([]monitor.GroundTruthEntry, 0, 4)
	for _, layer := range []string{"model", "safety", "request_config", "serving"} {
		next := previous
		switch layer {
		case "model":
			next.Model = cfg.ModelB
		case "safety":
			next.Safety = "loose"
		case "request_config":
			next.RequestConfig, next.Temperature, next.MaxTokens, next.SystemPrompt = "changed", 0.8, 64, "Be concise and answer in JSON."
		case "serving":
			next.Serving = "local-c"
		}
		scenarios = append(scenarios, monitor.GroundTruthEntry{Timestamp: time.Now().UTC(), Previous: previous, New: next, Layer: layer, Experiment: experimentID + "-" + layer})
	}
	sealed, err := monitor.NewSealedLog(scenarios)
	if err != nil {
		return err
	}
	if err := sealed.Save(filepath.Join(cfg.DataDir, "ground-truth.sealed.json")); err != nil {
		return err
	}
	results := make([]map[string]any, 0, len(scenarios))
	control := monitor.Analyze(reference, reference)
	results = append(results, map[string]any{"experiment_id": experimentID + "-control", "alias": monitor.Alias,
		"actual_layer": "", "detected": control.Detected, "confidence": control.Confidence,
		"predicted_layer": control.PredictedLayer, "evidence": control.Evidence})
	for _, scenario := range scenarios {
		router.SetLayers(scenario.New)
		current := monitor.RunViaAlias(context.Background(), library, aliasServer.URL+"/v1/chat/completions", nil, 4)
		analysis := monitor.Analyze(reference, current)
		results = append(results, map[string]any{"experiment_id": scenario.Experiment, "alias": monitor.Alias,
			"actual_layer": scenario.Layer, "detected": analysis.Detected, "confidence": analysis.Confidence,
			"predicted_layer": analysis.PredictedLayer, "evidence": analysis.Evidence})
	}
	result := map[string]any{"experiment_id": experimentID, "results": results, "metrics": score(results)}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return monitor.SaveJSON(filepath.Join(cfg.DataDir, "evaluation.json"), result)
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
