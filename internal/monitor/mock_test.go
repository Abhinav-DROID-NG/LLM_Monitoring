package monitor

import (
	"context"
	"testing"
)

func TestMockProviderIsDeterministic(t *testing.T) {
	request := Request{
		Model: "v1",
		Messages: []Message{
			{Role: "system", Text: "synthetic"},
			{Role: "user", Text: "What is 17 * 23?"},
		},
		MaxTokens: 20,
	}
	first := NewMockProvider(7).Complete(context.Background(), request)
	second := NewMockProvider(7).Complete(context.Background(), request)
	if first.Text != second.Text || first.FinishReason != second.FinishReason {
		t.Fatalf("same seed produced different responses: %#v %#v", first, second)
	}
	if first.ReturnedModel != request.Model {
		t.Fatalf("returned model = %q, want %q", first.ReturnedModel, request.Model)
	}
}

func TestMockProviderHonorsSafetyAndTokenCap(t *testing.T) {
	request := Request{
		Model:  "v1",
		Safety: "strict",
		Messages: []Message{
			{Role: "user", Text: "This is a borderline toxic drug pair question."},
		},
		MaxTokens: 20,
	}
	response := NewMockProvider(1).Complete(context.Background(), request)
	if response.FinishReason != "stop" {
		t.Fatalf("safety response finish reason = %q, want stop", response.FinishReason)
	}
	if response.Text == "" {
		t.Fatal("safety response was empty")
	}

	request.Safety = "normal"
	request.Messages[0].Text = "Reply with a long stable synthetic answer."
	request.MaxTokens = 2
	response = NewMockProvider(1).Complete(context.Background(), request)
	if response.FinishReason != "length" {
		t.Fatalf("capped response finish reason = %q, want length", response.FinishReason)
	}
}
