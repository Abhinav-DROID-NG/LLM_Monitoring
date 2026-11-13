package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

type fakeUpstream struct{}

func (fakeUpstream) Complete(context.Context, Request) Response {
	return Response{Text: "stable", FinishReason: "stop", Latency: time.Millisecond, InputTokens: 2, OutputTokens: 1}
}

type changingUpstream struct{}

func (changingUpstream) Complete(context.Context, Request) Response {
	return Response{Text: "stable", FinishReason: "stop", Latency: time.Millisecond}
}

func TestRouterOnlyAcceptsStableAlias(t *testing.T) {
	task := canary.Task{ID: "T1", Prompt: "test", MaxTokens: 5}
	router, err := NewRouter(fakeUpstream{}, Layers{Model: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := router.Complete(context.Background(), "wrong-alias", task); got.Error == "" {
		t.Fatal("router accepted a non-alias model name")
	}
	if got := router.Complete(context.Background(), Alias, task); got.Error != "" {
		t.Fatalf("stable alias failed: %s", got.Error)
	}
}

func TestSealedLogDetectsTampering(t *testing.T) {
	log, err := NewSealedLog([]GroundTruthEntry{{Layer: "model"}})
	if err != nil {
		t.Fatal(err)
	}

	log.Entries[0].Layer = "safety"
	if _, err := log.Unseal(log.Commitment); err == nil {
		t.Fatal("tampered ground truth was accepted")
	}
}

func TestProbesUseAliasEndpointAndCanaryChecks(t *testing.T) {
	task := canary.Task{ID: "REF-E1", Prompt: "test", MaxTokens: 5, Checks: []canary.Check{{Type: "contains", Value: "stable"}}}
	router, err := NewRouter(changingUpstream{}, Layers{Model: "hidden-model"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router.Handler())
	defer server.Close()
	observations := RunViaAlias(context.Background(), canary.Library{Tasks: []canary.Task{task}}, server.URL+"/v1/chat/completions", nil, 1)
	if len(observations) != 1 || observations[0].CanaryID != task.ID {
		t.Fatalf("unexpected observations: %+v", observations)
	}
	if len(observations[0].Checks) != 1 || !observations[0].Checks[0].Passed {
		t.Fatalf("canary check was not evaluated: %+v", observations[0].Checks)
	}
}

func TestTextOnlyDifferenceIsNotDrift(t *testing.T) {
	reference := []Observation{{CanaryID: "T1", Response: "one", FinishReason: "stop"}}
	current := []Observation{{CanaryID: "T1", Response: "two", FinishReason: "stop"}}
	changed, _, _ := Compare(reference, current)
	if changed {
		t.Fatal("single textual variation should not be meaningful drift")
	}
}

func TestAliasRejectsRealUserPrompt(t *testing.T) {
	router, err := NewRouter(fakeUpstream{}, Layers{Model: "a"})
	if err != nil {
		t.Fatal(err)
	}
	router.SetCanaryAllowlist(canary.Library{Tasks: []canary.Task{{Prompt: "synthetic", MaxTokens: 5}}})
	server := httptest.NewServer(router.Handler())
	defer server.Close()
	payload := map[string]any{"model": Alias, "messages": []Message{{Role: "user", Text: "REAL_USER_MARKER"}}}
	body, _ := json.Marshal(payload)
	response, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("real prompt was accepted with status %d", response.StatusCode)
	}
}
