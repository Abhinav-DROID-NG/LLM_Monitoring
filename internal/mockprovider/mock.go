package mockprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
)

type Layers struct {
	Model         bool `json:"model"`
	Serving       bool `json:"serving"`
	Safety        bool `json:"safety"`
	RequestConfig bool `json:"request_config"`
}

type Server struct {
	mu       sync.RWMutex
	seed     int64
	layers   Layers
	sequence uint64
}

func New(seed int64) *Server {
	return &Server{seed: seed}
}

func (s *Server) SetLayers(layers Layers) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layers = layers
}

func (s *Server) Layers() Layers {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.layers
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.completions)
	mux.HandleFunc("/admin/layers", s.adminLayers)
	return mux
}

func (s *Server) Provider() provider.Provider {
	return providerFromHandler{server: s}
}

type providerFromHandler struct{ server *Server }

func (p providerFromHandler) ChatCompletion(ctx context.Context, request provider.Request) (provider.Response, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return provider.Response{}, err
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
	p.server.completions(rec, req)
	if rec.Code != http.StatusOK {
		return provider.Response{}, fmt.Errorf("mock provider returned HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var response providerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		return provider.Response{}, err
	}
	return response.toProvider(), nil
}

type chatRequest struct {
	Model          string             `json:"model"`
	Messages       []provider.Message `json:"messages"`
	Temperature    *float64           `json:"temperature"`
	MaxTokens      *int               `json:"max_tokens"`
	ResponseFormat string             `json:"response_format"`
}

type providerResponse struct {
	ID                string `json:"id"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		Message      provider.Message `json:"message"`
		FinishReason string           `json:"finish_reason"`
	} `json:"choices"`
	Usage provider.Usage `json:"usage"`
}

func (r providerResponse) toProvider() provider.Response {
	response := provider.Response{
		ID: r.ID, Model: r.Model, SystemFingerprint: r.SystemFingerprint, Usage: r.Usage,
	}
	for _, choice := range r.Choices {
		response.Choices = append(response.Choices, provider.Choice{
			Message: choice.Message, FinishReason: choice.FinishReason,
		})
	}
	return response
}

func (s *Server) completions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method must be POST", http.StatusMethodNotAllowed)
		return
	}
	var request chatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}
	if request.Model == "" || len(request.Messages) == 0 {
		http.Error(w, "model and messages are required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.sequence++
	sequence := s.sequence
	layers := s.layers
	seed := s.seed
	s.mu.Unlock()

	prompt := request.Messages[len(request.Messages)-1].Content
	rng := rand.New(rand.NewSource(seedFor(seed, request, sequence)))
	temperature := valueOr(request.Temperature, 0.2)
	maxTokens := valueOr(request.MaxTokens, 256)
	if layers.RequestConfig {
		temperature = valueOr(request.Temperature, 0.8)
		maxTokens = valueOr(request.MaxTokens, 128)
	}
	content, refused := responseFor(prompt, temperature, maxTokens, layers, rng)
	if layers.Safety && strings.Contains(strings.ToLower(prompt), "never mix") {
		content, refused = "I can't help with that request.", true
	}
	finishReason := "stop"
	if len(content) > maxTokens*4 {
		content = content[:maxTokens*4]
		finishReason = "length"
	}
	if refused {
		finishReason = "stop"
	}
	latency := 0
	if layers.Serving {
		latency = 1 + rng.Intn(3)
		time.Sleep(time.Duration(latency) * time.Millisecond)
	}
	model := request.Model
	if layers.Model {
		model = request.Model + "-b"
	}
	fingerprint := fingerprint(seed, layers, request.Model)
	if layers.Serving {
		fingerprint = fingerprint + "-s"
	}
	response := providerResponse{
		ID: "mock-" + strconv.FormatUint(sequence, 10), Model: model,
		SystemFingerprint: fingerprint, Usage: provider.Usage{
			PromptTokens: tokenCount(prompt), CompletionTokens: tokenCount(content),
		},
	}
	response.Choices = append(response.Choices, struct {
		Message      provider.Message `json:"message"`
		FinishReason string           `json:"finish_reason"`
	}{Message: provider.Message{Role: "assistant", Content: content}, FinishReason: finishReason})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) adminLayers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(s.Layers())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method must be GET or POST", http.StatusMethodNotAllowed)
		return
	}
	var layers Layers
	if err := json.NewDecoder(r.Body).Decode(&layers); err != nil {
		http.Error(w, "invalid layers JSON", http.StatusBadRequest)
		return
	}
	s.SetLayers(layers)
	w.WriteHeader(http.StatusNoContent)
}

func responseFor(prompt string, temperature float64, maxTokens int, layers Layers, rng *rand.Rand) (string, bool) {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "cleaning products") {
		return "Bleach and ammonia should never be mixed because they release toxic fumes.", false
	}
	if strings.Contains(lower, "hung python process") {
		return "Use kill -TERM <pid> first, then escalate only if necessary.", false
	}
	if strings.Contains(lower, "phishing email") {
		return "Phishing is a scam message that tricks people into revealing information.", false
	}
	if strings.Contains(lower, "never mix") && layers.Safety {
		return "I can't help with that request.", true
	}
	base := "Synthetic response for the requested canary."
	if layers.Model {
		base = "Synthetic response with concise structured style for the requested canary."
	}
	if layers.RequestConfig && temperature > 0.7 {
		base += " Additional context is included by the server default."
	}
	if temperature > 0.6 && rng.Intn(4) == 0 {
		base += " Variation."
	}
	if maxTokens < 20 && len(base) > maxTokens*4 {
		base = base[:maxTokens*4]
	}
	return base, false
}

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}

func seedFor(seed int64, request chatRequest, sequence uint64) int64 {
	data, _ := json.Marshal(request)
	hash := sha256.Sum256(append(data, []byte(strconv.FormatInt(seed, 10))...))
	var value int64
	for _, b := range hash[:8] {
		value = value<<8 | int64(b)
	}
	return value + int64(sequence)
}

func fingerprint(seed int64, layers Layers, model string) string {
	raw := fmt.Sprintf("%d:%t:%t:%t:%t:%s", seed, layers.Model, layers.Serving, layers.Safety, layers.RequestConfig, model)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])[:12]
}

func tokenCount(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	return len(strings.Fields(value))
}
