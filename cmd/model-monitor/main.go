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
	router, err := newRouter(cfg)
	if err != nil {
		return err
	}
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
		if next.Serving == "groq-a" {
			next.Serving = "groq-c"
		} else {
			next.Serving = "groq-a"
		}
	}
	return next
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
	router, err := newRouter(cfg)
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

func newRouter(cfg monitor.Config) (*monitor.Router, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	return monitor.NewProviderRouter(map[string]monitor.Upstream{
		cfg.ModelA: monitor.Groq{Endpoint: cfg.Endpoint, APIKey: cfg.APIKey, Client: client},
		cfg.ModelB: monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client},
		cfg.ModelC: monitor.Local{Endpoint: cfg.LocalEndpoint, Client: client},
	}, healthy(cfg))
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
