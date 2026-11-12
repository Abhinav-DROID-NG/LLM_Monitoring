package probe

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/store"
)

type Config struct {
	Concurrency int
	Retries     int
	RetryDelay  time.Duration
	Alias       string
}

type Observation struct {
	Store    store.Observation
	Features features.Features
}

type Runner struct {
	config Config
}

func NewRunner(config Config) (*Runner, error) {
	if config.Concurrency < 1 {
		return nil, fmt.Errorf("concurrency must be at least 1")
	}
	if config.Retries < 0 {
		return nil, fmt.Errorf("retries cannot be negative")
	}
	if config.RetryDelay < 0 {
		return nil, fmt.Errorf("retry delay cannot be negative")
	}
	if config.Alias == "" {
		config.Alias = "support-bot-v1"
	}
	return &Runner{config: config}, nil
}

func (r *Runner) Run(ctx context.Context, suite canary.Suite, provider provider.Provider, runID string) ([]Observation, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}

	if runID == "" {
		return nil, fmt.Errorf("run ID cannot be empty")
	}
	if err := suite.Validate(); err != nil {
		return nil, err
	}
	type result struct {
		observation Observation
		err         error
	}
	tasks := make(chan canary.Task)
	results := make(chan result, len(suite.Tasks))
	var workers sync.WaitGroup
	for i := 0; i < r.config.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-tasks:
					if !ok {
						return
					}
					results <- result{observation: r.execute(ctx, suite, provider, runID, task)}
				}
			}
		}()
	}
	go func() {
		defer close(tasks)
		for _, task := range suite.Tasks {
			select {
			case <-ctx.Done():
				return
			case tasks <- task:
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	observations := make([]Observation, 0, len(suite.Tasks))
	for item := range results {
		if item.err != nil {
			return nil, item.err
		}
		observations = append(observations, item.observation)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

func (r *Runner) RunAndStore(ctx context.Context, suite canary.Suite, provider provider.Provider, db *sql.DB, runID string) ([]Observation, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if err := store.CreateRun(ctx, db, runID, "running"); err != nil {
		return nil, err
	}
	observations, err := r.Run(ctx, suite, provider, runID)
	status := "complete"
	if err != nil {
		status = "failed"
	}
	if updateErr := store.UpdateRunStatus(ctx, db, runID, status); updateErr != nil {
		return nil, updateErr
	}
	if err != nil {
		return nil, err
	}
	for _, observation := range observations {
		if err := store.SaveObservation(ctx, db, observation.Store); err != nil {
			return nil, err
		}
	}
	return observations, nil
}

func (r *Runner) execute(ctx context.Context, suite canary.Suite, p provider.Provider, runID string, task canary.Task) Observation {
	started := time.Now()
	var result canary.Result
	var err error
	retries := 0
	for attempt := 0; attempt <= r.config.Retries; attempt++ {
		result, err = suite.ExecuteWithAlias(ctx, p, task.ID, r.config.Alias)
		if err == nil {
			break
		}
		if ctx.Err() != nil || attempt == r.config.Retries {
			break
		}
		retries++
		if r.config.RetryDelay > 0 {
			timer := time.NewTimer(r.config.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				break
			case <-timer.C:
			}
		}
	}
	latency := time.Since(started).Milliseconds()
	f := features.Extract(result, latency)
	featuresJSON, marshalErr := features.Marshal(f)
	if marshalErr != nil && err == nil {
		err = marshalErr
	}
	status := 200
	if err != nil {
		status = 0
	}
	return Observation{
		Features: f,
		Store: store.Observation{
			ID: task.ID + "-" + runID, RunID: runID, TaskID: task.ID,
			ReturnedModel: result.Response.Model, SystemFingerprint: result.Response.SystemFingerprint,
			LatencyMS: latency, InputTokens: result.Response.Usage.PromptTokens,
			OutputTokens: result.Response.Usage.CompletionTokens, FinishReason: finishReason(result),
			HTTPStatus: status, Retries: retries, FeaturesJSON: featuresJSON, Error: errorString(err),
		},
	}
}

func finishReason(result canary.Result) string {
	if len(result.Response.Choices) == 0 {
		return ""
	}
	return result.Response.Choices[0].FinishReason
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
