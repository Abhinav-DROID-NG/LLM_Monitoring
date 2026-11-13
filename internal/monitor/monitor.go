package monitor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/features"
)

const Alias = "support-bot-v1"

type Config struct {
	Endpoint      string        `json:"endpoint"`
	LocalEndpoint string        `json:"local_endpoint"`
	ModelA        string        `json:"model_a"`
	ModelB        string        `json:"model_b"`
	ModelC        string        `json:"model_c"`
	Interval      time.Duration `json:"-"`
	IntervalText  string        `json:"interval"`
	SystemPrompt  string        `json:"system_prompt"`
	Temperature   float64       `json:"temperature"`
	MaxTokens     int           `json:"max_tokens"`
	DataDir       string        `json:"data_dir"`
	Concurrency   int           `json:"concurrency"`
	Mock          bool          `json:"mock"`
	Profile       string        `json:"profile"`
	Tier          string        `json:"tier"`
	Seed          int64         `json:"seed"`
	Trials        int           `json:"trials"`
	Quiet         bool          `json:"quiet"`
}

func DefaultConfig() Config {
	return Config{LocalEndpoint: "http://127.0.0.1:11434/api/chat",
		ModelA: "qwen2.5:3b-instruct-fp16", ModelB: "qwen2.5:3b-instruct-q8_0", ModelC: "qwen2.5:3b-instruct-q4_K_M",
		Interval: 15 * time.Minute, IntervalText: "15m", SystemPrompt: "Answer the synthetic canary precisely.",
		Temperature: 0.2, MaxTokens: 256, DataDir: "data", Concurrency: 1}
}

func (c *Config) Normalize() error {
	if c.Endpoint == "" {
		c.Endpoint = DefaultConfig().Endpoint
	}
	if c.ModelA == "" || c.ModelB == "" || c.ModelC == "" {
		return errors.New("model_a, model_b, and model_c are required")
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 256
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 1
	}
	if c.IntervalText != "" {
		d, err := time.ParseDuration(c.IntervalText)
		if err != nil {
			return fmt.Errorf("parse interval: %w", err)
		}
		c.Interval = d
	}
	if c.Interval <= 0 {
		c.Interval = 15 * time.Minute
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	return nil
}

type Layers struct {
	Model          string        `json:"model"`
	Safety         string        `json:"safety"`
	RequestConfig  string        `json:"request_config"`
	Serving        string        `json:"serving"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	SystemPrompt   string        `json:"system_prompt"`
	ResponseFormat string        `json:"response_format,omitempty"`
	LatencyMS      int           `json:"latency_ms,omitempty"`
	JitterMS       int           `json:"jitter_ms,omitempty"`
	Noise          float64       `json:"noise,omitempty"`
	Quantization   string        `json:"quantization,omitempty"`
	App            RequestConfig `json:"app,omitempty"`
}

type RequestConfig struct {
	Temperature    *float64 `json:"temperature,omitempty"`
	MaxTokens      *int     `json:"max_tokens,omitempty"`
	SystemPrompt   *string  `json:"system_prompt,omitempty"`
	ResponseFormat string   `json:"response_format,omitempty"`
}

func (r RequestConfig) Hash() string {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func healthyLayers(c Config) Layers {
	return Layers{Model: c.ModelA, Safety: "strict", RequestConfig: "default", Serving: "local-a",
		Temperature: c.Temperature, MaxTokens: c.MaxTokens, SystemPrompt: c.SystemPrompt}
}

type GroundTruthEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Previous   Layers    `json:"previous"`
	New        Layers    `json:"new"`
	Layer      string    `json:"layer"`
	Experiment string    `json:"experiment_id"`
}

type SealedLog struct {
	Entries    []GroundTruthEntry `json:"entries"`
	Commitment string             `json:"commitment"`
}

func NewSealedLog(entries []GroundTruthEntry) (SealedLog, error) {
	raw, err := json.Marshal(entries)
	if err != nil {
		return SealedLog{}, err
	}
	sum := sha256.Sum256(raw)
	return SealedLog{Entries: entries, Commitment: hex.EncodeToString(sum[:])}, nil
}

func (l SealedLog) Save(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func (l SealedLog) Unseal(commitment string) ([]GroundTruthEntry, error) {
	if commitment != l.Commitment {
		return nil, errors.New("ground-truth commitment verification failed")
	}
	raw, err := json.Marshal(l.Entries)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != l.Commitment {
		return nil, errors.New("ground-truth log was modified")
	}
	return append([]GroundTruthEntry(nil), l.Entries...), nil
}

type Request struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
	ResponseFormat string    `json:"response_format,omitempty"`
	System         string    `json:"system,omitempty"`
	Safety         string    `json:"-"`
	Serving        string    `json:"-"`
}
type Message struct {
	Role string `json:"role"`
	Text string `json:"content"`
}
type Response struct {
	Text              string        `json:"text"`
	InputTokens       int           `json:"input_tokens"`
	OutputTokens      int           `json:"output_tokens"`
	FinishReason      string        `json:"finish_reason"`
	Latency           time.Duration `json:"latency"`
	Error             string        `json:"error,omitempty"`
	HTTPStatus        int           `json:"http_status,omitempty"`
	ReturnedModel     string        `json:"returned_model,omitempty"`
	SystemFingerprint *string       `json:"system_fingerprint,omitempty"`
	RetryCount        int           `json:"retry_count"`
	RunID             string        `json:"run_id,omitempty"`
	ProbeExecutionID  string        `json:"probe_execution_id,omitempty"`
	RequestConfigHash string        `json:"request_config_hash,omitempty"`
	Route             string        `json:"route,omitempty"`
}

type Upstream interface {
	Complete(context.Context, Request) Response
}

type Groq struct {
	Endpoint string
	APIKey   string
	Client   *http.Client
}

type Local struct {
	Endpoint string
	Client   *http.Client
}

func (l Local) Complete(ctx context.Context, request Request) Response {
	start := time.Now()
	body, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Options  struct {
			Temperature float64 `json:"temperature"`
			NumPredict  int     `json:"num_predict"`
		} `json:"options"`
		Format string `json:"format,omitempty"`
		Stream bool   `json:"stream"`
	}{Model: request.Model, Messages: request.Messages, Options: struct {
		Temperature float64 `json:"temperature"`
		NumPredict  int     `json:"num_predict"`
	}{request.Temperature, request.MaxTokens}, Format: map[bool]string{true: "json", false: ""}[request.ResponseFormat == "json_object"], Stream: false})
	if err != nil {
		return Response{Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	defer res.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if readErr != nil {
		return Response{Error: readErr.Error(), Latency: time.Since(start)}
	}
	if res.StatusCode/100 != 2 {
		return Response{Error: fmt.Sprintf("local provider status %d: %s", res.StatusCode, strings.TrimSpace(string(data))), Latency: time.Since(start)}
	}
	var decoded struct {
		Message    Message `json:"message"`
		Done       bool    `json:"done"`
		DoneReason string  `json:"done_reason"`
		Model      string  `json:"model"`
		Prompt     int     `json:"prompt_eval_count"`
		Output     int     `json:"eval_count"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	finish := normalizeFinishReason(decoded.DoneReason)
	return Response{Text: decoded.Message.Text, FinishReason: finish, ReturnedModel: decoded.Model, HTTPStatus: res.StatusCode, Latency: time.Since(start),
		InputTokens: decoded.Prompt, OutputTokens: decoded.Output}
}

func (g Groq) Complete(ctx context.Context, request Request) Response {
	start := time.Now()
	body, err := json.Marshal(request)
	if err != nil {
		return Response{Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := g.Client.Do(req)
	if err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	defer res.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if readErr != nil {
		return Response{Error: readErr.Error(), Latency: time.Since(start)}
	}
	if res.StatusCode/100 != 2 {
		return Response{Error: fmt.Sprintf("upstream status %d: %s", res.StatusCode, strings.TrimSpace(string(data))), Latency: time.Since(start)}
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	out := Response{Latency: time.Since(start), InputTokens: decoded.Usage.Prompt, OutputTokens: decoded.Usage.Completion}
	if len(decoded.Choices) > 0 {
		out.Text, out.FinishReason = decoded.Choices[0].Message.Content, decoded.Choices[0].Finish
	}
	return out
}

type Router struct {
	mu        sync.RWMutex
	alias     string
	upstream  Upstream
	providers map[string]Upstream
	layers    Layers
	allowed   map[string]struct{}
}

func NewRouter(upstream Upstream, layers Layers) (*Router, error) {
	if upstream == nil {
		return nil, errors.New("router upstream is required")
	}
	return &Router{alias: Alias, upstream: upstream, layers: layers}, nil
}

func NewProviderRouter(providers map[string]Upstream, layers Layers) (*Router, error) {
	if len(providers) == 0 {
		return nil, errors.New("router providers are required")
	}
	return &Router{alias: Alias, providers: providers, layers: layers}, nil
}

func (r *Router) SetLayers(layers Layers) { r.mu.Lock(); r.layers = layers; r.mu.Unlock() }
func (r *Router) Layers() Layers          { r.mu.RLock(); defer r.mu.RUnlock(); return r.layers }
func (r *Router) SetCanaryAllowlist(library canary.Library) {
	allowed := make(map[string]struct{}, len(library.Tasks))
	for _, task := range library.Tasks {
		allowed[canaryPromptHash(fillerText(task.Filler)+task.Prompt)] = struct{}{}
	}
	r.mu.Lock()
	r.allowed = allowed
	r.mu.Unlock()
}
func (r *Router) Complete(ctx context.Context, alias string, task canary.Task) Response {
	r.mu.RLock()
	layers := r.layers
	r.mu.RUnlock()
	if alias != r.alias {
		return Response{Error: "unknown model alias"}
	}
	upstream := r.upstream
	if r.providers != nil {
		upstream = r.providers[layers.Model]
		if selected := r.providers[layers.Model+"|"+layers.Serving]; selected != nil {
			upstream = selected
		}
		if upstream == nil {
			return Response{Error: "hidden model configuration is unavailable"}
		}
	}
	if shouldRefuse(layers.Safety, task.Prompt) {
		return Response{Text: "I can't help with that request.", FinishReason: "content_filter"}
	}
	system := task.System
	if layers.App.SystemPrompt != nil {
		system = *layers.App.SystemPrompt
	} else if system == "" {
		system = layers.SystemPrompt
	}
	temp := task.Temperature
	if temp == 0 {
		temp = divisionTemperature(task.Division)
	}
	if layers.App.Temperature != nil {
		temp = *layers.App.Temperature
	} else if temp == 0 {
		temp = layers.Temperature
	}
	maxTokens := task.MaxTokens
	if layers.App.MaxTokens != nil {
		maxTokens = *layers.App.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = layers.MaxTokens
	}
	responseFormat := task.ResponseFormat
	if layers.App.ResponseFormat != "" {
		responseFormat = layers.App.ResponseFormat
	}
	return upstream.Complete(ctx, Request{Model: layers.Model, Safety: layers.Safety, Serving: layers.Serving,
		Temperature: temp, MaxTokens: maxTokens, ResponseFormat: responseFormat, System: system,
		Messages: []Message{{Role: "system", Text: system}, {Role: "user", Text: fillerText(task.Filler) + task.Prompt}}})
}

func (r *Router) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, req)
			return
		}

		var payload struct {
			Model          string    `json:"model"`
			Messages       []Message `json:"messages"`
			Temperature    float64   `json:"temperature"`
			MaxTokens      int       `json:"max_tokens"`
			System         string    `json:"system"`
			ResponseFormat string    `json:"response_format"`
		}
		if err := json.NewDecoder(io.LimitReader(req.Body, 4<<20)).Decode(&payload); err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if payload.Model != r.alias || len(payload.Messages) == 0 {
			http.Error(w, "only the configured alias is accepted", http.StatusBadRequest)
			return
		}
		r.mu.RLock()
		layers := r.layers
		allowed := r.allowed
		r.mu.RUnlock()
		if len(allowed) > 0 {
			prompt := payload.Messages[len(payload.Messages)-1].Text
			if _, ok := allowed[canaryPromptHash(prompt)]; !ok {
				http.Error(w, "prompt is not an allowed synthetic canary", http.StatusForbidden)
				return
			}
		}
		upstream := r.upstream
		if r.providers != nil {
			upstream = r.providers[layers.Model]
			if selected := r.providers[layers.Model+"|"+layers.Serving]; selected != nil {
				upstream = selected
			}
		}
		if upstream == nil {
			http.Error(w, "hidden provider unavailable", http.StatusBadGateway)
			return
		}
		if layers.Safety == "strict" && unsafeRequest(payload.Messages[len(payload.Messages)-1].Text) {
			_ = json.NewEncoder(w).Encode(Response{Text: "I can't help with that request.", FinishReason: "content_filter"})
			return
		}
		response := upstream.Complete(req.Context(), Request{Model: layers.Model, Messages: payload.Messages,
			Temperature: payload.Temperature, MaxTokens: payload.MaxTokens, System: payload.System,
			ResponseFormat: payload.ResponseFormat, Safety: layers.Safety, Serving: layers.Serving})
		status := http.StatusOK
		if response.Error != "" {
			status = http.StatusBadGateway
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	})
}

func normalizeFinishReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop", "done":
		return "stop"
	case "length", "max_tokens":
		return "length"
	case "content_filter", "content_filtered":
		return "content_filter"
	default:
		return "unknown"
	}
}

func divisionTemperature(division string) float64 {
	switch division {
	case "refusal":
		return 0.5
	case "style":
		return 0.7
	default:
		return 0
	}
}

func unsafeRequest(prompt string) bool {
	prompt = strings.ToLower(prompt)
	for _, phrase := range []string{"make a bomb", "steal a password", "harm someone"} {
		if strings.Contains(prompt, phrase) {
			return true
		}
	}
	return false
}

func shouldRefuse(level, prompt string) bool {
	if !unsafeRequest(prompt) {
		return false
	}
	return level != "loose"
}

func fillerText(filler any) string {
	if filler == nil {
		return ""
	}

	if text, ok := filler.(string); ok {
		return text
	}
	raw, err := json.Marshal(filler)
	if err != nil {
		return ""
	}
	return string(raw)
}

func canaryPromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func styleSlice(vector [9]float64) []float64 {
	return append([]float64(nil), vector[:]...)
}

type Observation struct {
	CanaryID          string             `json:"canary_id"`
	RunID             string             `json:"run_id"`
	ProbeExecutionID  string             `json:"probe_execution_id"`
	RequestConfigHash string             `json:"request_config_hash"`
	RequestedModel    string             `json:"requested_model,omitempty"`
	ReturnedModel     string             `json:"returned_model,omitempty"`
	SystemFingerprint *string            `json:"system_fingerprint,omitempty"`
	Timestamp         time.Time          `json:"timestamp"`
	Response          string             `json:"response"`
	LatencyMS         int64              `json:"latency_ms"`
	InputTokens       int                `json:"input_tokens"`
	OutputTokens      int                `json:"output_tokens"`
	FinishReason      string             `json:"finish_reason"`
	HTTPStatus        int                `json:"http_status"`
	RetryCount        int                `json:"retry_count"`
	Error             string             `json:"error,omitempty"`
	Checks            []CheckObservation `json:"checks"`
	Style             []float64          `json:"style,omitempty"`
}

type CheckObservation struct {
	CheckID     string `json:"check_id,omitempty"`
	Type        string `json:"type"`
	Expected    string `json:"expected"`
	Passed      bool   `json:"passed"`
	Unavailable bool   `json:"unavailable,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

func Run(ctx context.Context, library canary.Library, router *Router, concurrency int) []Observation {
	tasks := library.Pinned()
	runID := newID()
	if concurrency < 1 {
		concurrency = 1
	}
	work := make(chan canary.Task)
	out := make(chan Observation, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range work {
				response := router.Complete(ctx, Alias, task)
				out <- Observation{CanaryID: task.ID, Timestamp: time.Now().UTC(), Response: response.Text,
					RunID: runID, ProbeExecutionID: newID(), RequestedModel: Alias,
					RequestConfigHash: router.Layers().App.Hash(),
					ReturnedModel:     response.ReturnedModel, SystemFingerprint: response.SystemFingerprint,
					LatencyMS: response.Latency.Milliseconds(), InputTokens: response.InputTokens,
					OutputTokens: response.OutputTokens, FinishReason: response.FinishReason, Error: response.Error,
					Checks: checkTask(task, response), Style: styleSlice(features.StyleVectorV1(response.Text))}
			}
		}()
	}
	go func() {
		for _, task := range tasks {
			work <- task
		}
		close(work)
		wg.Wait()
		close(out)
	}()
	observations := make([]Observation, 0, len(tasks))
	for observation := range out {
		observations = append(observations, observation)
	}
	return observations
}

func RunViaAlias(ctx context.Context, library canary.Library, endpoint string, client *http.Client, concurrency int) []Observation {
	tasks := library.Pinned()
	runID := newID()
	if concurrency < 1 {
		concurrency = 1
	}
	if client == nil {
		client = http.DefaultClient
	}
	work := make(chan canary.Task)
	out := make(chan Observation, len(tasks))
	var wg sync.WaitGroup
	var completed atomic.Int64
	startTime := time.Now()

	fmt.Printf("Starting execution of %d canary prompts (concurrency: %d)...\n\n", len(tasks), concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range work {
				response := callAlias(ctx, endpoint, client, task)
				obs := Observation{
					CanaryID: task.ID,
					RunID:    runID, ProbeExecutionID: newID(), RequestedModel: Alias,
					Timestamp:    time.Now().UTC(),
					Response:     response.Text,
					LatencyMS:    response.Latency.Milliseconds(),
					InputTokens:  response.InputTokens,
					OutputTokens: response.OutputTokens,
					FinishReason: response.FinishReason,
					Error:        response.Error,
					Checks:       checkTask(task, response),
					Style:        styleSlice(features.StyleVectorV1(response.Text)),
				}
				idx := completed.Add(1)
				elapsedSec := float64(obs.LatencyMS) / 1000.0
				if obs.Error != "" {
					fmt.Printf("[%d/%d] %-8s FAILED (%.2fs) - Error: %s\n", idx, len(tasks), task.ID, elapsedSec, obs.Error)
				} else {
					passedChecks := 0
					for _, chk := range obs.Checks {
						if chk.Passed {
							passedChecks++
						}
					}
					fmt.Printf("[%d/%d] %-8s completed in %.2fs [%d/%d checks passed] (tokens: in=%d, out=%d)\n",
						idx, len(tasks), task.ID, elapsedSec, passedChecks, len(obs.Checks), obs.InputTokens, obs.OutputTokens)
				}
				out <- obs
			}
		}()
	}
	go func() {
		for _, task := range tasks {
			work <- task
		}
		close(work)
		wg.Wait()
		close(out)
	}()
	observations := make([]Observation, 0, len(tasks))
	for observation := range out {
		observations = append(observations, observation)
	}
	fmt.Printf("\nAll %d/%d tasks completed in %s.\n", len(observations), len(tasks), time.Since(startTime).Round(time.Millisecond))
	return observations
}

func callAlias(ctx context.Context, endpoint string, client *http.Client, task canary.Task) Response {
	start := time.Now()
	payload := struct {
		Model          string    `json:"model"`
		Messages       []Message `json:"messages"`
		Temperature    float64   `json:"temperature"`
		MaxTokens      int       `json:"max_tokens"`
		System         string    `json:"system"`
		ResponseFormat string    `json:"response_format,omitempty"`
	}{Model: Alias, Messages: []Message{{Role: "system", Text: task.System}, {Role: "user", Text: fillerText(task.Filler) + task.Prompt}},
		Temperature: task.Temperature, MaxTokens: task.MaxTokens, System: task.System, ResponseFormat: task.ResponseFormat}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return Response{Error: fmt.Sprintf("alias status %d: %s", res.StatusCode, strings.TrimSpace(string(data))), Latency: time.Since(start)}
	}
	var response Response
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&response); err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	response.Latency = time.Since(start)
	return response
}

func checkTask(task canary.Task, response Response) []CheckObservation {
	results := make([]CheckObservation, 0, len(task.Checks))
	for _, check := range task.Checks {
		result := canary.CheckResponse(task, check, canary.ResponseView{
			Text: response.Text, FinishReason: response.FinishReason, Error: response.Error,
		})
		expected := check.ValueString()
		if expected == "" {
			expected = check.Type
		}
		results = append(results, CheckObservation{Type: check.Type, Expected: expected,
			CheckID: stableCheckID(check),
			Passed:  result.Passed, Unavailable: result.Unavailable, Detail: result.Detail})
	}
	return results
}

func isRefusal(text string) bool {
	for _, phrase := range []string{"i can't", "i cannot", "i'm unable", "i won't", "can't help"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func Compare(reference, current []Observation) (changed bool, confidence float64, evidence []string) {
	base := make(map[string]Observation, len(reference))
	for _, observation := range reference {
		base[observation.CanaryID] = observation
	}
	var differences float64
	var comparable int
	for _, observation := range current {
		previous, ok := base[observation.CanaryID]
		if !ok || observation.Error != "" || previous.Error != "" {
			continue
		}
		comparable++
		if strings.TrimSpace(previous.Response) != strings.TrimSpace(observation.Response) {
			differences += 0.25
			evidence = append(evidence, observation.CanaryID+": response changed")
		}
		if observation.FinishReason != previous.FinishReason {
			differences++
			evidence = append(evidence, observation.CanaryID+": finish reason changed")
		}
		if previous.LatencyMS > 0 && observation.LatencyMS > previous.LatencyMS*2 {
			differences++
			evidence = append(evidence, observation.CanaryID+": latency increased")
		}
		for i, check := range observation.Checks {
			if i >= len(previous.Checks) || check.Passed == previous.Checks[i].Passed {
				continue
			}
			differences += 2
			evidence = append(evidence, observation.CanaryID+": expected "+check.Expected+" check changed")
		}
	}
	if comparable == 0 {
		return false, 0, []string{"no comparable successful responses"}
	}
	rate := differences / float64(comparable)
	return rate >= 0.5, rate, evidence
}

type Analysis struct {
	Detected       bool     `json:"detected"`
	Confidence     float64  `json:"confidence"`
	PredictedLayer string   `json:"predicted_layer"`
	Evidence       []string `json:"evidence"`
}

func Analyze(reference, current []Observation) Analysis {
	changed, confidence, evidence := Compare(reference, current)
	return Analysis{Detected: changed, Confidence: confidence,
		PredictedLayer: attribute(evidence), Evidence: evidence}
}

func attribute(evidence []string) string {
	if len(evidence) == 0 {
		return "unknown"
	}
	refusal, config, latency := 0, 0, 0
	for _, item := range evidence {
		switch {
		case strings.HasPrefix(item, "REF-"):
			refusal++
		case strings.HasPrefix(item, "FMT-") || strings.HasPrefix(item, "INS-"):
			config++
		case strings.HasSuffix(item, "latency increased"):
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

func SaveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func RequestConfigHash(request Request) string {
	raw, _ := json.Marshal(struct {
		Model          string    `json:"model"`
		Temperature    float64   `json:"temperature"`
		MaxTokens      int       `json:"max_tokens"`
		ResponseFormat string    `json:"response_format"`
		Messages       []Message `json:"messages"`
	}{request.Model, request.Temperature, request.MaxTokens, request.ResponseFormat, request.Messages})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var idCounter atomic.Uint64

func newID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), idCounter.Add(1))
}
