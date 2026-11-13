package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// MockProvider is deterministic for a request/config pair, with optional
// serving noise. It is intentionally an offline approximation, not a model.
type MockProvider struct {
	mu   sync.Mutex
	rand *rand.Rand
}

func NewMockProvider(seed int64) *MockProvider {
	return &MockProvider{rand: rand.New(rand.NewSource(seed))}
}

func (m *MockProvider) Complete(ctx context.Context, request Request) Response {
	select {
	case <-ctx.Done():
		return Response{Error: ctx.Err().Error(), HTTPStatus: 499}
	default:
	}
	start := time.Now()
	base := 1
	if request.Serving == "local-b" {
		base = 2
	}
	if request.Serving == "local-c" {
		base = 20
	}
	latency := base
	if request.Serving == "slow" {
		latency += 40
	}
	// Keep the default path fast enough for baseline and evaluation runs.
	if latency > 0 {
		timer := time.NewTimer(time.Duration(latency) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{Error: ctx.Err().Error(), HTTPStatus: 499, Latency: time.Since(start)}
		case <-timer.C:
		}
	}
	seed := sha256.Sum256([]byte(request.Model + "\x00" + request.MessagesText() + fmt.Sprintf("\x00%.3f", request.Temperature)))
	r := rand.New(rand.NewSource(int64(binarySeed(seed))))
	text := mockAnswer(request, r)
	finish := "stop"
	outputTokens := tokenCount(text)
	if request.MaxTokens > 0 && outputTokens > request.MaxTokens {
		text = truncateWords(text, request.MaxTokens)
		outputTokens = tokenCount(text)
		finish = "length"
	}
	fingerprint := hex.EncodeToString(seed[:6])
	return Response{Text: text, InputTokens: tokenCount(request.MessagesText()), OutputTokens: outputTokens,
		FinishReason: finish, Latency: time.Since(start), HTTPStatus: 200,
		ReturnedModel: request.Model, SystemFingerprint: &fingerprint}
}

func (r Request) MessagesText() string {
	var b strings.Builder
	for _, message := range r.Messages {
		b.WriteString(message.Role)
		b.WriteByte(':')
		b.WriteString(message.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func mockAnswer(request Request, r *rand.Rand) string {
	prompt := strings.ToLower(request.MessagesText())
	if request.Safety == "strict" && (strings.Contains(prompt, "borderline") ||
		strings.Contains(prompt, "drug pair") || strings.Contains(prompt, "toxic")) {
		return "I can't help with that request."
	}
	if strings.Contains(prompt, "json") || request.ResponseFormat == "json_object" {
		if request.ResponseFormat == "" || request.ResponseFormat == "json_object" || r.Float64() > 0.08 {
			return `{"ok":true}`
		}
		return "The answer is ok."
	}
	switch {
	case strings.Contains(prompt, "17 * 23"):
		return "391"
	case strings.Contains(prompt, "capital of france"):
		return "Paris"
	case strings.Contains(prompt, "photosynthesis"):
		return "Plants use sunlight to turn water and carbon dioxide into food, releasing oxygen."
	case strings.Contains(prompt, "three bullets") || strings.Contains(prompt, "3 bullets"):
		return "- one\n- two\n- three"
	case strings.Contains(prompt, "washing hands"):
		return "1. Wet hands.\n2. Add soap.\n3. Scrub all surfaces.\n4. Rinse well.\n5. Dry hands.\n<<END>>"
	default:
		if request.Temperature > 0.6 || request.Serving == "noisy" {
			return []string{"A concise synthetic answer.", "A stable synthetic answer.", "A varied synthetic answer."}[r.Intn(3)]
		}
		return "A stable synthetic answer."
	}
}

func tokenCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}

func truncateWords(text string, max int) string {
	words := strings.Fields(text)
	if len(words) <= max {
		return text
	}
	return strings.Join(words[:max], " ")
}

func binarySeed(sum [32]byte) int64 {
	var value int64
	for _, b := range sum[:8] {
		value = value<<8 | int64(b)
	}
	return value
}

func (m *MockProvider) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: "offline-mock"})
}
