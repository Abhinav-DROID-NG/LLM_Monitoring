package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/evaluation"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/monitor"
)

type evaluationClient struct {
	provider monitor.Upstream
	model    string
}

func (c evaluationClient) Complete(ctx context.Context, request evaluation.Request) (evaluation.Response, error) {
	response := c.provider.Complete(ctx, monitor.Request{Model: c.model, Messages: []monitor.Message{{Role: "system", Text: request.SystemPrompt}, {Role: "user", Text: request.Prompt}}, Temperature: request.Temperature, MaxTokens: request.MaxTokens})
	if response.Error != "" {
		return evaluation.Response{Latency: response.Latency, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens}, fmt.Errorf("%s", response.Error)
	}
	return evaluation.Response{Text: response.Text, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens, Latency: response.Latency}, nil
}

func executeEvaluation(cfg monitor.Config, objective evaluation.Objective) error {
	result, err := runEvaluation(cfg, objective)
	if err != nil {
		return err
	}
	path, err := evaluation.Save(cfg.DataDir, result)
	if err != nil {
		return err
	}
	fmt.Printf("%s evaluation saved to %s (%d trials, coverage %.1f%%)\n", objective, path, len(result.Trials), result.Metrics.Coverage*100)
	return nil
}

func runEvaluation(cfg monitor.Config, objective evaluation.Objective) (evaluation.Evaluation, error) {
	var dataset evaluation.Dataset
	var err error
	if objective == evaluation.Behavioural {
		dataset, err = evaluation.LoadDataset("behavioral_evaluation.json")
	} else {
		dataset, err = evaluation.LoadDataset("performance_evaluation.json")
	}
	if err != nil {
		return evaluation.Evaluation{}, err
	}
	var provider monitor.Upstream
	if cfg.Mock {
		provider = monitor.NewMockProvider(cfg.Seed)
	} else {
		provider = monitor.Local{Endpoint: cfg.LocalEndpoint, Client: http.DefaultClient}
	}
	client := evaluationClient{provider: provider, model: cfg.ModelA}
	settings := evaluation.DefaultConfig(cfg.Profile)
	settings.Seed, settings.Temperature, settings.MaxTokens, settings.Concurrency = cfg.Seed, cfg.Temperature, cfg.MaxTokens, cfg.Concurrency
	settings.Timeout = 10 * time.Minute
	meta := evaluation.Metadata{Model: cfg.ModelA, Provider: map[bool]string{true: "offline-mock", false: "ollama"}[cfg.Mock], Runtime: "monitor", DatasetVersion: dataset.Version}
	runner := evaluation.Runner{Client: client}
	var result evaluation.Evaluation
	if objective == evaluation.Behavioural {
		result, err = runner.Run(context.Background(), dataset, objective, meta, settings, nil)
	} else {
		result, err = runner.RunPerformance(context.Background(), dataset, meta, settings, nil)
	}
	if err != nil {
		return evaluation.Evaluation{}, err
	}
	return result, nil
}
