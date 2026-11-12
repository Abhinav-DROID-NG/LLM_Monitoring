package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	if cfg.APIKey == "" {
		return errors.New("GROQ_API_KEY is required for live monitoring")
	}
	router, err := monitor.NewRouter(monitor.Groq{Endpoint: cfg.Endpoint, APIKey: cfg.APIKey, Client: &http.Client{Timeout: 60 * time.Second}}, healthy(cfg))
	if err != nil {
		return err
	}
	run := func() error {
		observations := monitor.Run(context.Background(), library, router, 4)
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

func healthy(cfg monitor.Config) monitor.Layers {
	return monitor.Layers{Model: cfg.ModelA, Safety: "strict", RequestConfig: "default", Serving: "groq-a", Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, SystemPrompt: cfg.SystemPrompt}
}

func evaluate(cfg monitor.Config) error {
	if cfg.APIKey == "" {
		return errors.New("GROQ_API_KEY is required for evaluation")
	}
	library, err := canary.Load("canaries_fixed.json")
	if err != nil {
		return err
	}
	router, err := monitor.NewRouter(monitor.Groq{Endpoint: cfg.Endpoint, APIKey: cfg.APIKey, Client: &http.Client{Timeout: 60 * time.Second}}, healthy(cfg))
	if err != nil {
		return err
	}
	reference := monitor.Run(context.Background(), library, router, 4)
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
			next.Serving = "groq-c"
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
	for _, scenario := range scenarios {
		router.SetLayers(scenario.New)
		current := monitor.Run(context.Background(), library, router, 4)
		changed, confidence, evidence := monitor.Compare(reference, current)
		results = append(results, map[string]any{"experiment_id": scenario.Experiment, "alias": monitor.Alias, "detected": changed, "confidence": confidence, "predicted_layer": inferLayer(evidence), "evidence": evidence})
	}
	result := map[string]any{"experiment_id": experimentID, "results": results}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return monitor.SaveJSON(filepath.Join(cfg.DataDir, "evaluation.json"), result)
}

func inferLayer(evidence []string) string {
	if len(evidence) == 0 {
		return "unknown"
	}
	refusal, config, latency := 0, 0, 0
	for _, item := range evidence {
		switch {
		case len(item) >= 4 && item[:4] == "REF-":
			refusal++
		case len(item) >= 4 && (item[:4] == "FMT-" || item[:4] == "INS-"):
			config++
		case len(item) >= 8 && item[len(item)-8:] == "increased":
			latency++
		}
	}
	if latency > refusal+config && latency > 0 {
		return "serving"
	}
	if refusal > config && refusal > 0 {
		return "safety"
	}
	if config > 0 {
		return "request_config"
	}
	if len(evidence) >= 3 {
		return "model"
	}
	return "unknown"
}
