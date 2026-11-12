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
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

const Alias = "support-bot-v1"

type Config struct {
	APIKey       string        `json:"api_key"`
	Endpoint     string        `json:"endpoint"`
	ModelA       string        `json:"model_a"`
	ModelB       string        `json:"model_b"`
	ModelC       string        `json:"model_c"`
	Interval     time.Duration `json:"-"`
	IntervalText string        `json:"interval"`
	SystemPrompt string        `json:"system_prompt"`
	Temperature  float64       `json:"temperature"`
	MaxTokens    int           `json:"max_tokens"`
	DataDir      string        `json:"data_dir"`
}

func DefaultConfig() Config {
	return Config{APIKey: os.Getenv("GROQ_API_KEY"), Endpoint: "https://api.groq.com/openai/v1/chat/completions",
		ModelA: "llama-3.1-8b-instant", ModelB: "llama-3.3-70b-versatile", ModelC: "mixtral-8x7b-32768",
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
	return Layers{Model: c.ModelA, Safety: "strict", RequestConfig: "default", Serving: "groq-a",
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
}
type Message struct {
	Role string `json:"role"`
	Text string `json:"content"`
}
type Response struct {
	Text         string
	InputTokens  int
	OutputTokens int
	FinishReason string
	Latency      time.Duration
	Error        string
}

type Upstream interface {
	Complete(context.Context, Request) Response
}

type Groq struct {
	Endpoint string
	APIKey   string
	Client   *http.Client
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
	mu       sync.RWMutex
	alias    string
	upstream Upstream
	layers   Layers
}

func NewRouter(upstream Upstream, layers Layers) (*Router, error) {
	if upstream == nil {
		return nil, errors.New("router upstream is required")
	}
	return &Router{alias: Alias, upstream: upstream, layers: layers}, nil
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
	return r.upstream.Complete(ctx, Request{Model: layers.Model, Temperature: layers.Temperature,
		MaxTokens: layers.MaxTokens, Messages: []Message{{Role: "system", Text: layers.SystemPrompt}, {Role: "user", Text: task.Prompt}}})
}

type Observation struct {
	CanaryID     string    `json:"canary_id"`
	Timestamp    time.Time `json:"timestamp"`
	Response     string    `json:"response"`
	LatencyMS    int64     `json:"latency_ms"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	FinishReason string    `json:"finish_reason"`
	Error        string    `json:"error,omitempty"`
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
					OutputTokens: response.OutputTokens, FinishReason: response.FinishReason, Error: response.Error}
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

func Compare(reference, current []Observation) (changed bool, confidence float64, evidence []string) {
	base := make(map[string]Observation, len(reference))
	for _, observation := range reference {
		base[observation.CanaryID] = observation
	}
	var differences, comparable int
	for _, observation := range current {
		previous, ok := base[observation.CanaryID]
		if !ok || observation.Error != "" || previous.Error != "" {
			continue
		}
		comparable++
		if strings.TrimSpace(previous.Response) != strings.TrimSpace(observation.Response) {
			differences++
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
	}
	if comparable == 0 {
		return false, 0, []string{"no comparable successful responses"}
	}
	rate := float64(differences) / float64(comparable)
	return rate >= 0.1, rate, evidence
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
