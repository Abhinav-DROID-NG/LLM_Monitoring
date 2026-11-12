package features

import (
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
)

func TestExtractDoesNotRetainResponseText(t *testing.T) {
	result := canary.Result{
		Checks: []canary.CheckResult{{Passed: true}, {Passed: false}},
		Response: provider.Response{
			Usage:   provider.Usage{PromptTokens: 4, CompletionTokens: 3},
			Choices: []provider.Choice{{Message: provider.Message{Content: "I cannot help with that."}, FinishReason: "stop"}},
		},
	}
	extracted := Extract(result, 12)
	if !extracted.Refused || extracted.Accuracy != 0.5 || extracted.LatencyMS != 12 ||
		extracted.InputTokens != 4 || extracted.OutputTokens != 3 || extracted.RefusalHash == "" {
		t.Fatalf("unexpected features: %+v", extracted)
	}
	if extracted.JSONValid {
		t.Fatal("refusal text should not be valid JSON")
	}
}

func TestStyleVectorAndCosineDistance(t *testing.T) {
	same := StyleVector("A short answer.")
	if distance := CosineDistance(same, same); distance > 1e-12 {
		t.Fatalf("same vectors have distance %v", distance)
	}
	if distance := CosineDistance(same, StyleVector("Completely different output with bullets - one")); distance <= 0 {
		t.Fatalf("different vectors have distance %v", distance)
	}
}
