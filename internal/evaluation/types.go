package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = "evaluation.v1"

type Objective string

const (
	Behavioural Objective = "behavioural"
	Performance Objective = "performance"
)

type TestCase struct {
	ID           string         `json:"id"`
	Category     string         `json:"category"`
	Prompt       string         `json:"prompt"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Expected     string         `json:"expected,omitempty"`
	Evaluator    EvaluatorSpec  `json:"evaluator"`
	Scoring      string         `json:"scoring"`
	Severity     string         `json:"severity"`
	Repetitions  int            `json:"repetitions,omitempty"`
	TimeoutMS    int            `json:"timeout_ms,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type EvaluatorSpec struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params,omitempty"`
}

type PerformanceCase struct {
	ID           string         `json:"id"`
	Category     string         `json:"category"`
	Prompt       string         `json:"prompt"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Requests     int            `json:"requests"`
	Concurrency  []int          `json:"concurrency,omitempty"`
	TimeoutMS    int            `json:"timeout_ms,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Dataset struct {
	Version     string            `json:"version"`
	Behavioural []TestCase        `json:"behavioural,omitempty"`
	Performance []PerformanceCase `json:"performance,omitempty"`
}

type Config struct {
	Profile             string        `json:"profile"`
	Repetitions         int           `json:"repetitions"`
	ConsistencyRepeats  int           `json:"consistency_repetitions"`
	PerformanceRequests int           `json:"performance_requests"`
	ResourceSamples     int           `json:"resource_samples"`
	StressTrials        int           `json:"stress_trials"`
	Concurrency         int           `json:"concurrency"`
	Timeout             time.Duration `json:"timeout"`
	Retries             int           `json:"retries"`
	Seed                int64         `json:"seed"`
	Temperature         float64       `json:"temperature"`
	TopP                float64       `json:"top_p"`
	MaxTokens           int           `json:"max_tokens"`
	ContextLength       int           `json:"context_length"`
	Warmup              int           `json:"warmup"`
	Duration            time.Duration `json:"duration"`
	Resume              bool          `json:"resume"`
}

func DefaultConfig(profile string) Config {
	c := Config{Profile: profile, Repetitions: 3, ConsistencyRepeats: 10, PerformanceRequests: 50,
		ResourceSamples: 20, StressTrials: 3, Concurrency: 1, Timeout: 2 * time.Minute,
		Retries: 1, Seed: 1, Temperature: 0.2, TopP: 1, MaxTokens: 256, Warmup: 3,
		Duration: 2 * time.Hour}
	if profile == "development" {
		c.Repetitions, c.ConsistencyRepeats, c.PerformanceRequests, c.ResourceSamples = 1, 2, 3, 3
		c.Timeout, c.Duration = 30*time.Second, 5*time.Minute
	} else if profile == "deep_analysis" {
		c.Repetitions, c.ConsistencyRepeats, c.PerformanceRequests, c.ResourceSamples = 10, 20, 100, 50
	}
	return c
}

type Metadata struct {
	RunID            string         `json:"run_id"`
	Model            string         `json:"model"`
	ModelVersion     string         `json:"model_version,omitempty"`
	Quantization     string         `json:"quantization,omitempty"`
	Provider         string         `json:"provider"`
	Runtime          string         `json:"runtime,omitempty"`
	Hardware         map[string]any `json:"hardware,omitempty"`
	SystemPromptHash string         `json:"system_prompt_hash,omitempty"`
	DatasetVersion   string         `json:"dataset_version"`
	StartedAt        time.Time      `json:"started_at"`
	FinishedAt       time.Time      `json:"finished_at,omitempty"`
}

type RequestConfig struct {
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	Seed          int64   `json:"seed"`
	ContextLength int     `json:"context_length"`
	MaxTokens     int     `json:"max_tokens"`
	Concurrency   int     `json:"concurrency"`
}

type TrialStatus string

const (
	Pass            TrialStatus = "PASS"
	Fail            TrialStatus = "FAIL"
	Inconclusive    TrialStatus = "INCONCLUSIVE"
	EvaluationError TrialStatus = "EVALUATION_ERROR"
)

type Trial struct {
	TestID       string      `json:"test_id"`
	Repetition   int         `json:"repetition"`
	Concurrency  int         `json:"concurrency,omitempty"`
	Status       TrialStatus `json:"status"`
	Score        *float64    `json:"score,omitempty"`
	Response     string      `json:"response,omitempty"`
	Error        string      `json:"error,omitempty"`
	RetryCount   int         `json:"retry_count"`
	LatencyMS    float64     `json:"latency_ms"`
	TTFTMS       *float64    `json:"ttft_ms,omitempty"`
	InputTokens  int         `json:"input_tokens,omitempty"`
	OutputTokens int         `json:"output_tokens,omitempty"`
	StartedAt    time.Time   `json:"started_at"`
	FinishedAt   time.Time   `json:"finished_at"`
}

type Evaluation struct {
	SchemaVersion string        `json:"schema_version"`
	Objective     Objective     `json:"objective"`
	Dataset       string        `json:"dataset"`
	Metadata      Metadata      `json:"metadata"`
	Config        Config        `json:"config"`
	Request       RequestConfig `json:"request"`
	Trials        []Trial       `json:"trials"`
	Metrics       Metrics       `json:"metrics"`
	Status        string        `json:"status"`
	Errors        []string      `json:"errors,omitempty"`
}

type Metrics struct {
	Values      map[string]float64 `json:"values"`
	Unavailable []string           `json:"unavailable,omitempty"`
	Coverage    float64            `json:"coverage"`
}

type ModelClient interface {
	Complete(context.Context, Request) (Response, error)
}

type Request struct {
	Model        string
	SystemPrompt string
	Prompt       string
	Temperature  float64
	TopP         float64
	Seed         int64
	MaxTokens    int
}

type Response struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	TTFT         *time.Duration
}

func (d Dataset) Validate() error {
	seen := map[string]bool{}
	if len(d.Behavioural) == 0 && len(d.Performance) == 0 {
		return errors.New("evaluation dataset has no tests")
	}
	for _, test := range d.Behavioural {
		if err := validateID(seen, test.ID, test.Category, test.Prompt); err != nil {
			return err
		}
		if test.Evaluator.Type == "" {
			return fmt.Errorf("test %q has no evaluator", test.ID)
		}
	}
	for _, test := range d.Performance {
		if err := validateID(seen, test.ID, test.Category, test.Prompt); err != nil {
			return err
		}
		if test.Requests <= 0 {
			return fmt.Errorf("performance test %q requests must be positive", test.ID)
		}
	}
	return nil
}

func validateID(seen map[string]bool, id, category, prompt string) error {
	if strings.TrimSpace(id) == "" || seen[id] {
		return fmt.Errorf("missing or duplicate test id %q", id)
	}
	if strings.TrimSpace(category) == "" || strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("test %q requires category and prompt", id)
	}
	seen[id] = true
	return nil
}
