package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Runner struct{ Client ModelClient }

func (r Runner) RunPerformance(ctx context.Context, dataset Dataset, meta Metadata, cfg Config, resume map[string]Trial) (Evaluation, error) {
	if r.Client == nil {
		return Evaluation{}, fmt.Errorf("evaluation client is required")
	}
	if err := dataset.Validate(); err != nil {
		return Evaluation{}, err
	}
	e := Evaluation{SchemaVersion: SchemaVersion, Objective: Performance, Dataset: dataset.Version, Metadata: meta, Config: cfg,
		Request: RequestConfig{Temperature: cfg.Temperature, TopP: cfg.TopP, Seed: cfg.Seed, ContextLength: cfg.ContextLength, MaxTokens: cfg.MaxTokens, Concurrency: cfg.Concurrency}, Status: "RUNNING", Trials: []Trial{}}
	if e.Metadata.RunID == "" {
		e.Metadata.RunID = fmt.Sprintf("eval-%d", time.Now().UnixNano())
	}
	if e.Metadata.StartedAt.IsZero() {
		e.Metadata.StartedAt = time.Now().UTC()
	}
	jobs := make(chan PerformanceCase)
	results := make(chan Trial)
	var wg sync.WaitGroup
	workers := cfg.Concurrency
	if workers < 1 {
		workers = 1
	}
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for test := range jobs {
				requests := test.Requests
				if cfg.PerformanceRequests > 0 {
					requests = cfg.PerformanceRequests
				}
				for i := 0; i < requests; i++ {
					key := test.ID + fmt.Sprintf("/%d", i)
					if _, ok := resume[key]; ok {
						continue
					}
					trial := r.runPerformanceTrial(ctx, test, i, cfg)
					results <- trial
				}
			}
		}()
	}
	go func() {
		for _, test := range dataset.Performance {
			jobs <- test
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for trial := range results {
		e.Trials = append(e.Trials, trial)
	}
	e.Metadata.FinishedAt = time.Now().UTC()
	e.Status = "COMPLETED"
	e.Metrics = Aggregate(e.Trials)
	e.Metrics.Unavailable = []string{"cpu_utilization", "ram_usage", "gpu_utilization", "vram_usage", "peak_memory", "memory_growth"}
	if len(e.Trials) > 0 {
		successful := 0.0
		for _, trial := range e.Trials {
			if trial.Status == Pass {
				successful++
			}
		}
		e.Metrics.Coverage = successful / float64(len(e.Trials))
		e.Metrics.Values["evaluation_coverage"] = e.Metrics.Coverage
	}
	return e, nil
}

func (r Runner) runPerformanceTrial(parent context.Context, test PerformanceCase, repetition int, cfg Config) Trial {
	started := time.Now().UTC()
	timeout := cfg.Timeout
	if test.TimeoutMS > 0 {
		timeout = time.Duration(test.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	response, err := r.Client.Complete(ctx, Request{SystemPrompt: test.SystemPrompt, Prompt: test.Prompt, Temperature: cfg.Temperature, TopP: cfg.TopP, Seed: cfg.Seed, MaxTokens: cfg.MaxTokens})
	trial := Trial{TestID: test.ID, Repetition: repetition, Status: Pass, StartedAt: started, FinishedAt: time.Now().UTC(), LatencyMS: float64(response.Latency.Microseconds()) / 1000, Response: response.Text, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens}
	if response.TTFT != nil {
		value := float64(response.TTFT.Microseconds()) / 1000
		trial.TTFTMS = &value
	}
	if err != nil {
		trial.Status = EvaluationError
		trial.Error = err.Error()
	}
	return trial
}

func (r Runner) Run(ctx context.Context, dataset Dataset, objective Objective, meta Metadata, cfg Config, resume map[string]Trial) (Evaluation, error) {
	if r.Client == nil {
		return Evaluation{}, fmt.Errorf("evaluation client is required")
	}
	if err := dataset.Validate(); err != nil {
		return Evaluation{}, err
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	e := Evaluation{SchemaVersion: SchemaVersion, Objective: objective, Dataset: dataset.Version, Metadata: meta, Config: cfg,
		Request: RequestConfig{Temperature: cfg.Temperature, TopP: cfg.TopP, Seed: cfg.Seed, ContextLength: cfg.ContextLength, MaxTokens: cfg.MaxTokens, Concurrency: cfg.Concurrency}, Status: "RUNNING", Trials: []Trial{}}
	if e.Metadata.RunID == "" {
		e.Metadata.RunID = fmt.Sprintf("eval-%d", time.Now().UnixNano())
	}
	if e.Metadata.StartedAt.IsZero() {
		e.Metadata.StartedAt = time.Now().UTC()
	}
	var cases []TestCase
	if objective == Behavioural {
		cases = dataset.Behavioural
	}
	jobs := make(chan TestCase)
	results := make(chan Trial)
	var wg sync.WaitGroup
	for worker := 0; worker < cfg.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for test := range jobs {
				count := test.Repetitions
				if count <= 0 {
					count = cfg.Repetitions
				}
				if count <= 0 {
					count = 1
				}
				for i := 0; i < count; i++ {
					key := test.ID + fmt.Sprintf("/%d", i)
					if _, ok := resume[key]; ok {
						continue
					}
					results <- r.runTrial(ctx, test, i, cfg)
				}
			}
		}()
	}
	go func() {
		for _, test := range cases {
			jobs <- test
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for trial := range results {
		e.Trials = append(e.Trials, trial)
	}
	e.Metadata.FinishedAt = time.Now().UTC()
	e.Status = "COMPLETED"
	e.Metrics = Aggregate(e.Trials)
	return e, nil
}

func (r Runner) runTrial(parent context.Context, test TestCase, repetition int, cfg Config) Trial {
	started := time.Now().UTC()
	timeout := cfg.Timeout
	if test.TimeoutMS > 0 {
		timeout = time.Duration(test.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var response Response
	var err error
	retries := 0
	for {
		response, err = r.Client.Complete(ctx, Request{SystemPrompt: test.SystemPrompt, Prompt: test.Prompt, Temperature: cfg.Temperature, TopP: cfg.TopP, Seed: cfg.Seed, MaxTokens: cfg.MaxTokens})
		if err == nil || retries >= cfg.Retries || ctx.Err() != nil {
			break
		}
		retries++
	}
	trial := Trial{TestID: test.ID, Repetition: repetition, RetryCount: retries, StartedAt: started, FinishedAt: time.Now().UTC(), LatencyMS: float64(response.Latency.Microseconds()) / 1000, Response: response.Text, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens}
	if response.TTFT != nil {
		v := float64(response.TTFT.Microseconds()) / 1000
		trial.TTFTMS = &v
	}
	if err != nil {
		trial.Status, trial.Error = EvaluationError, err.Error()
		return trial
	}
	result := Evaluate(test.Evaluator, test.Expected, response.Text)
	trial.Status, trial.Error = result.Status, result.Detail
	if result.Status == Pass || result.Status == Fail {
		trial.Score = &result.Score
	}
	return trial
}

func HashSystemPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}
