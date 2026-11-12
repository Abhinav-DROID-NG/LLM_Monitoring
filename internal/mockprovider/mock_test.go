package mockprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
)

func TestMockProviderResponseAndLayerSwitches(t *testing.T) {
	server := New(7)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, err := provider.NewHTTPClient(httpServer.URL, httpServer.Client(), 0)
	if err != nil {
		t.Fatal(err)
	}
	request := provider.Request{Model: "stable-alias", Messages: []provider.Message{
		{Role: "user", Content: "Give a short synthetic answer."},
	}}
	baseline, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ID == "" || baseline.Model != "stable-alias" || baseline.SystemFingerprint == "" ||
		len(baseline.Choices) != 1 || baseline.Choices[0].FinishReason != "stop" {
		t.Fatalf("missing OpenAI-compatible metadata: %+v", baseline)
	}

	server.SetLayers(Layers{Model: true})
	modelChanged, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if modelChanged.Model == baseline.Model || modelChanged.Choices[0].Message.Content == baseline.Choices[0].Message.Content {
		t.Fatal("model layer did not change observable profile")
	}

	server.SetLayers(Layers{Serving: true})
	servingChanged, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if servingChanged.SystemFingerprint == baseline.SystemFingerprint {
		t.Fatal("serving layer did not change fingerprint")
	}

	server.SetLayers(Layers{Safety: true})
	safetyChanged, err := client.ChatCompletion(context.Background(), provider.Request{
		Model: "stable-alias", Messages: []provider.Message{{Role: "user", Content: "Which cleaning products should never mix?"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if safetyChanged.Choices[0].Message.Content == baseline.Choices[0].Message.Content {
		t.Fatal("safety layer did not change response")
	}

	server.SetLayers(Layers{RequestConfig: true})
	configChanged, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if configChanged.Choices[0].Message.Content == baseline.Choices[0].Message.Content {
		t.Fatal("request configuration layer did not change default behavior")
	}
}

func TestMockProviderExplicitParametersOverrideDefaults(t *testing.T) {
	server := New(11)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, err := provider.NewHTTPClient(httpServer.URL, httpServer.Client(), 0)
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.2
	maxTokens := 256
	request := provider.Request{
		Model: "alias", Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []provider.Message{{Role: "user", Content: "Give a short synthetic answer."}},
	}
	baseline, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	server.SetLayers(Layers{RequestConfig: true})
	pinned, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Choices[0].Message.Content != baseline.Choices[0].Message.Content {
		t.Fatalf("explicit parameters did not override defaults: %q != %q",
			pinned.Choices[0].Message.Content, baseline.Choices[0].Message.Content)
	}
}

func TestMockProviderAdminEndpoint(t *testing.T) {
	server := New(1)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/admin/layers", strings.NewReader(`{"model":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || !server.Layers().Model {
		t.Fatalf("admin endpoint failed: status=%d layers=%+v", response.StatusCode, server.Layers())
	}
}
