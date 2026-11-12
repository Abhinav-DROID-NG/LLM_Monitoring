package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

type fakeUpstream struct{}

func (fakeUpstream) Complete(context.Context, Request) Response {
	return Response{Text: "stable", FinishReason: "stop", Latency: time.Millisecond, InputTokens: 2, OutputTokens: 1}
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
