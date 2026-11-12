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
)

const Alias = "support-bot-v1"

type Config struct {
	APIKey        string        `json:"api_key"`
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
}

func DefaultConfig() Config {
	return Config{LocalEndpoint: "http://127.0.0.1:11434/api/chat",
		ModelA: "qwen2.5:3b-instruct-fp16", ModelB: "qwen2.5:3b-instruct-q8_0", ModelC: "qwen2.5:3b-instruct-q4_K_M",
		Interval: 15 * time.Minute, IntervalText: "15m", SystemPrompt: "Answer the synthetic canary precisely.",
		Temperature: 0.2, MaxTokens: 256, DataDir: "data"}
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
	Model         string  `json:"model"`
	Safety        string  `json:"safety"`
	RequestConfig string  `json:"request_config"`
	Serving       string  `json:"serving"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
	SystemPrompt  string  `json:"system_prompt"`
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
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Safety      string    `json:"-"`
	Serving     string    `json:"-"`
}
type Message struct {
	Role string `json:"role"`
	Text string `json:"content"`
}
type Response struct {
	Text         string        `json:"text"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	FinishReason string        `json:"finish_reason"`
	Latency      time.Duration `json:"latency"`
	Error        string        `json:"error,omitempty"`
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
		Stream bool `json:"stream"`
	}{Model: request.Model, Messages: request.Messages, Options: struct {
		Temperature float64 `json:"temperature"`
		NumPredict  int     `json:"num_predict"`
	}{request.Temperature, request.MaxTokens}, Stream: false})
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
		Message Message `json:"message"`
		Done    bool    `json:"done"`
		Prompt  int     `json:"prompt_eval_count"`
		Output  int     `json:"eval_count"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Response{Error: err.Error(), Latency: time.Since(start)}
	}
	return Response{Text: decoded.Message.Text, FinishReason: "stop", Latency: time.Since(start),
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
	if layers.Safety == "strict" && unsafeRequest(task.Prompt) {
		return Response{Text: "I can't help with that request.", FinishReason: "content_filter"}
	}
	return upstream.Complete(ctx, Request{Model: layers.Model, Safety: layers.Safety, Serving: layers.Serving,
		Temperature: layers.Temperature, MaxTokens: layers.MaxTokens,
		Messages: []Message{{Role: "system", Text: layers.SystemPrompt}, {Role: "user", Text: task.Prompt}}})
}

func (r *Router) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, req)
			return
		}
		var payload struct {
			Model       string    `json:"model"`
			Messages    []Message `json:"messages"`
			Temperature float64   `json:"temperature"`
			MaxTokens   int       `json:"max_tokens"`
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
		r.mu.RUnlock()
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
			Temperature: layers.Temperature, MaxTokens: layers.MaxTokens, Safety: layers.Safety, Serving: layers.Serving})
		status := http.StatusOK
		if response.Error != "" {
			status = http.StatusBadGateway
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	})
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

type Observation struct {
	CanaryID     string             `json:"canary_id"`
	Timestamp    time.Time          `json:"timestamp"`
	Response     string             `json:"response"`
	LatencyMS    int64              `json:"latency_ms"`
	InputTokens  int                `json:"input_tokens"`
	OutputTokens int                `json:"output_tokens"`
	FinishReason string             `json:"finish_reason"`
	Error        string             `json:"error,omitempty"`
	Checks       []CheckObservation `json:"checks"`
}

type CheckObservation struct {
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Passed   bool   `json:"passed"`
}

func Run(ctx context.Context, library canary.Library, router *Router, concurrency int) []Observation {
	tasks := library.Pinned()
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
					LatencyMS: response.Latency.Milliseconds(), InputTokens: response.InputTokens,
					OutputTokens: response.OutputTokens, FinishReason: response.FinishReason, Error: response.Error,
					Checks: checkTask(task, response)}
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
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range work {
				response := callAlias(ctx, endpoint, client, task)
				if response.Error == "" {
					fmt.Fprintf(os.Stderr, "%d/%d done\n", completed.Add(1), len(tasks))
				}
				out <- Observation{CanaryID: task.ID, Timestamp: time.Now().UTC(), Response: response.Text,
					LatencyMS: response.Latency.Milliseconds(), InputTokens: response.InputTokens,
					OutputTokens: response.OutputTokens, FinishReason: response.FinishReason, Error: response.Error,
					Checks: checkTask(task, response)}
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

func callAlias(ctx context.Context, endpoint string, client *http.Client, task canary.Task) Response {
	start := time.Now()
	payload := struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		Temperature float64   `json:"temperature"`
		MaxTokens   int       `json:"max_tokens"`
	}{Model: Alias, Messages: []Message{{Role: "user", Text: task.Prompt}}, MaxTokens: task.MaxTokens}
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
	text := strings.ToLower(response.Text)
	for _, check := range task.Checks {
		passed := response.Error == ""
		expected := check.Type
		switch check.Type {
		case "not_refused":
			passed = passed && !isRefusal(text)
			expected = "non-refusal"
		case "contains_all":
			for _, value := range check.Values {
				passed = passed && strings.Contains(text, strings.ToLower(value))
			}
			expected = strings.Join(check.Values, ",")
		case "contains":
			passed = passed && strings.Contains(text, strings.ToLower(check.Value))
			expected = check.Value
		case "json_valid":
			var value any
			passed = passed && json.Unmarshal([]byte(response.Text), &value) == nil
			expected = "valid JSON"
		}
		results = append(results, CheckObservation{Type: check.Type, Expected: expected, Passed: passed})
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
