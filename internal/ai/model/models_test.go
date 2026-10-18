package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCatalogHasOneDefaultAndCannotBeMutatedByCallers(t *testing.T) {
	providers := Providers()
	if len(providers) != 2 || providers[0] != "openai" || providers[1] != "anthropic" {
		t.Fatal("only OpenAI and Anthropic are enabled")
	}
	for _, provider := range providers {
		seen, defaults := map[string]bool{}, 0
		for _, model := range Models(provider) {
			if model.ID == "" || model.Name == "" || seen[model.ID] {
				t.Fatal("catalog has an empty or duplicate model")
			}
			if model.ContextWindow <= 4096 {
				t.Fatal("catalog must provide context capacity beyond the reply budget")
			}
			seen[model.ID] = true
			if model.Default {
				defaults++
			}
			c := DefaultConfig()
			c.Provider, c.Model = provider, model.ID
			if err := c.Ready("test-key"); err != nil {
				t.Fatal(err)
			}
		}
		if defaults != 1 {
			t.Fatal("each vendor must have exactly one setup default")
		}
		copy := Models(provider)
		copy[0].ID = "injected-model"
		if _, ok := LookupModel(provider, "injected-model"); ok {
			t.Fatal("caller mutated the catalog")
		}
	}
}

func TestUnsupportedRequestsAreRejectedBeforeNetworkActivity(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, pair := range [][2]string{
		{"gemini", "gemini-test"}, {"xai", "grok-test"}, {"openai-compatible", "gpt-5.4"},
		{"unknown", "gpt-5.4"}, {"openai", "custom-model"}, {"openai", "claude-opus-5-5"},
		{"anthropic", "gpt-5.4"}, {"openai", "GPT-5.4"}, {"openai", "gpt-5.4-future"},
	} {
		c := DefaultConfig()
		c.Provider, c.Model, c.Endpoint = pair[0], pair[1], server.URL
		_, err := streamText(context.Background(), c, "test-key", []Message{{Role: "user", Text: "Hi"}}, func(string) error {
			t.Error("unsupported request emitted text")
			return nil
		})
		if err == nil || strings.Contains(err.Error(), "test-key") {
			t.Fatalf("unsupported configuration accepted: %v", pair)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unsupported request reached a provider")
	}
}

func TestSetupDefaultPreservesConnection(t *testing.T) {
	for provider, model := range map[string]string{"openai": "gpt-5.4-mini", "anthropic": "claude-sonnet-5-5"} {
		connection := DefaultConfig()
		connection.Provider = provider
		connection.CredentialRef, connection.Instructions = "saved-key", "Keep it short"
		resolved, err := connection.WithDefaultModel()
		if err != nil || resolved.Model != model || resolved.Connection() != connection {
			t.Fatal("setup changed connection settings or chose the wrong default", err)
		}
		if err := resolved.Supported(); err != nil {
			t.Fatal("setup chose an unsupported model", err)
		}
	}
	c := DefaultConfig()
	c.Provider = "gemini"
	if _, err := c.WithDefaultModel(); err == nil {
		t.Fatal("unsupported vendor got a fallback model")
	}
}
