package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat-1","model":"alias","system_fingerprint":"fp-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client(), 0)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ChatCompletion(context.Background(), Request{
		Model: "alias", Messages: []Message{{Role: "user", Content: "synthetic"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "chat-1" || response.Model != "alias" || len(response.Choices) != 1 ||
		response.Choices[0].Message.Content != "ok" || response.Usage.PromptTokens != 3 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestHTTPClientRetriesRateLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "try again", http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chat-2","choices":[]}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client(), 1)
	if err != nil {
		t.Fatal(err)
	}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.ChatCompletion(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestHTTPClientDoesNotRetryClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, server.Client(), 3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ChatCompletion(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected HTTP 400 error, got %v", err)
	}
}
