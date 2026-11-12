package probe

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/store"
)

type countingProvider struct {
	mu       sync.Mutex
	active   int
	max      int
	calls    int
	failures int
	delay    time.Duration
}

func (p *countingProvider) ChatCompletion(ctx context.Context, _ provider.Request) (provider.Response, error) {
	p.mu.Lock()
	p.calls++
	p.active++
	if p.active > p.max {
		p.max = p.active
	}
	fail := p.failures > 0
	if fail {
		p.failures--
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return provider.Response{}, ctx.Err()
	case <-time.After(p.delay):
	}
	if fail {
		return provider.Response{}, errors.New("transient provider error")
	}
	return provider.Response{
		Model: "alias", SystemFingerprint: "fingerprint",
		Choices: []provider.Choice{{Message: provider.Message{Content: "ok"}, FinishReason: "stop"}},
		Usage:   provider.Usage{PromptTokens: 1, CompletionTokens: 1},
	}, nil
}

func testSuite() canary.Suite {
	return canary.Suite{
		CheckTypes: map[string]string{"output_equals": "check"},
		Schema:     canary.SchemaContract{AllowedTiers: []string{"easy"}, AllowedDivisions: []string{"test"}},
		Tasks: []canary.Task{
			{ID: "one", Division: "test", Tier: "easy", Prompt: "synthetic one", MaxTokens: 10, Checks: []canary.Check{{Type: "output_equals", Value: []byte(`"ok"`)}}},
			{ID: "two", Division: "test", Tier: "easy", Prompt: "synthetic two", MaxTokens: 10, Checks: []canary.Check{{Type: "output_equals", Value: []byte(`"ok"`)}}},
			{ID: "three", Division: "test", Tier: "easy", Prompt: "synthetic three", MaxTokens: 10, Checks: []canary.Check{{Type: "output_equals", Value: []byte(`"ok"`)}}},
		},
	}
}

func TestRunnerBoundsConcurrencyAndRetries(t *testing.T) {
	p := &countingProvider{delay: 5 * time.Millisecond, failures: 1}
	runner, err := NewRunner(Config{Concurrency: 2, Retries: 1})
	if err != nil {
		t.Fatal(err)
	}
	observations, err := runner.Run(context.Background(), testSuite(), p, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 3 || p.max > 2 || p.calls != 4 {
		t.Fatalf("observations=%d max=%d calls=%d", len(observations), p.max, p.calls)
	}
	if observations[0].Store.FeaturesJSON == "" {
		t.Fatal("features were not serialized")
	}
}

func TestRunnerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner, err := NewRunner(Config{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run(ctx, testSuite(), &countingProvider{delay: time.Second}, "cancelled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func TestRunAndStorePersistsResults(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner, err := NewRunner(Config{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	observations, err := runner.RunAndStore(context.Background(), testSuite(), &countingProvider{}, db, "stored")
	if err != nil || len(observations) != 3 {
		t.Fatalf("observations=%d error=%v", len(observations), err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM observations WHERE run_id = 'stored'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("stored observation count = %d", count)
	}
}
