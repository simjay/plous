package ai

import (
	"testing"

	journal "github.com/simjay/plous/internal/ai/history"
)

func TestHistorySemanticConfigurationDependsOnLiveCapabilities(t *testing.T) {
	for _, test := range []struct {
		name, tool, semantic string
	}{
		{"kubernetes", "kubernetes", "unavailable"},
		{"helm", "helm", "unavailable"},
		{"code", "source", "not configured"},
		{"history only", "", "not configured"},
	} {
		t.Run(test.name, func(t *testing.T) {
			o, _ := workspaceFixture(t)
			o.Connections = nil
			if test.tool != "" {
				o.Connections = []Connection{{ConnectionID: "east", Tool: test.tool}}
			}
			config := DefaultRuntimeConfig()
			// A missing provider distinguishes attempted setup from disabled semantics without a network call.
			config.Embedding.Connection = "missing-test-provider"
			config.Embedding.Model = "test-embedding"
			op := &operation{o: o, runtime: config}
			if err := op.setupHistory(); err != nil {
				t.Fatal(err)
			}
			out, err := op.historyAccess.Call(t.Context(), journal.HistoryRequest{Operation: "search", Query: "earlier"}, 65536)
			if err != nil || out.Search == nil || out.Search.Semantic != test.semantic {
				t.Fatalf("unexpected semantic configuration: %+v, %v", out.Search, err)
			}
		})
	}
}
