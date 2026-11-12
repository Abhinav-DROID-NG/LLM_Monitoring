package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/groundtruth"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/provider"
)

func TestAliasForwardsAndLayerSwitchChangesResponse(t *testing.T) {
	mock := mockprovider.New(3)
	r, err := New(Config{Alias: "support-bot-v1", Upstream: "mock"}, mock.Handler(), mock)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r.Handler())
	defer server.Close()
	client, err := provider.NewHTTPClient(server.URL, &http.Client{Timeout: time.Second}, 0)
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := 80
	request := provider.Request{
		Model:     "support-bot-v1",
		Messages:  []provider.Message{{Role: "user", Content: "Which cleaning products should never mix?"}},
		MaxTokens: &maxTokens,
	}
	base, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	r.SetLayers(groundtruth.Layers{Safety: true})
	changed, err := client.ChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if base.Choices[0].Message.Content == changed.Choices[0].Message.Content {
		t.Fatal("safety layer did not change response")
	}
}

func TestRouterRejectsNonAliasAndAdminPaths(t *testing.T) {
	mock := mockprovider.New(1)
	r, err := New(Config{Alias: "support-bot-v1"}, mock.Handler(), mock)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r.Handler())
	defer server.Close()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Post(server.URL+"/admin/layers", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("admin status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}
