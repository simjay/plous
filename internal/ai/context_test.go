package ai

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/schema"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"reflect"
	"strings"
	"testing"
)

func TestPromptAndToolsUseProjectedHistory(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range ToolDefinitions(Options{}) {
		names[tool.Name] = true
	}
	if names["bash"] || names["history"] || !names["history_read"] || names["recall"] {
		t.Fatal(names)
	}
	text := string(marshal(PreviewMessages(Instructions, "captured", "question")))
	if strings.Contains(text, "active.md") || strings.Contains(text, "bash") || !strings.Contains(text, "projected facts") {
		t.Fatal(text)
	}
}

func TestCompactionKeepsRecentNativeToolPairs(t *testing.T) {
	recent := []aimodel.RequestMessage{
		{Role: "user", Content: "Current question " + strings.Repeat("x", 8000)},
		{Role: "assistant", ToolCalls: []aimodel.ToolCall{{ID: "current-call", Name: "history_read", Args: json.RawMessage(`{"operation":"read","ref":"saved"}`)}}},
		{Role: "tool", ToolCallID: "current-call", Content: `{"ref":"saved","complete":true}`},
	}
	older := []aimodel.RequestMessage{{Role: "user", Content: strings.Repeat("a", 51000)}, {Role: "assistant", Content: "Earlier conclusion with record:one:source."}}
	messages := append(older, recent...)
	m := &scriptedModel{steps: []*schema.Message{answer("Earlier conclusion cites record:one:source.")}}
	op := &operation{model: m, capacity: 18000, runtime: DefaultRuntimeConfig()}
	got, err := op.fitContext(context.Background(), Instructions, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(recent)+1 || !reflect.DeepEqual(got[1:], recent) || !strings.Contains(got[0].Content, "not original evidence") {
		t.Fatal("recent exchange changed", len(got))
	}
	if len(m.inputs) != 1 || strings.Contains(m.inputs[0][1].Content, "Current question") {
		t.Fatal("compaction crossed current exchange boundary")
	}
	if op.calls != 1 {
		t.Fatal("compaction bypassed shared spending")
	}
}

func TestOversizedCurrentExchangeDoesNotTriggerUnboundedCompaction(t *testing.T) {
	m := &scriptedModel{}
	op := &operation{model: m, capacity: 12000, runtime: DefaultRuntimeConfig()}
	_, err := op.fitContext(context.Background(), Instructions, []aimodel.RequestMessage{{Role: "user", Content: strings.Repeat("x", 80000)}}, nil)
	if err == nil || len(m.inputs) != 0 {
		t.Fatal("oversized current exchange was dispatched", err)
	}
}

func TestCompactionAfterModelDownsizeUsesBoundedCalls(t *testing.T) {
	older := []aimodel.RequestMessage{{Role: "user", Content: strings.Repeat("older evidence ", 10000)}, {Role: "assistant", Content: "Remember record:conversation:source."}}
	recent := aimodel.RequestMessage{Role: "user", Content: "What did we decide?"}
	m := &scriptedModel{}
	for range 12 {
		m.steps = append(m.steps, answer("Decision retained, citing record:conversation:source."))
	}
	op := &operation{model: m, capacity: 12000, runtime: DefaultRuntimeConfig()}
	got, err := op.fitContext(t.Context(), Instructions, append(older, recent), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.inputs) < 2 || len(got) != 2 || !reflect.DeepEqual(got[1], recent) {
		t.Fatal("history did not compact in bounded calls", len(m.inputs), got)
	}
	for _, input := range m.inputs {
		if estimateText(string(marshal(map[string]any{"messages": input, "tools": []aimodel.Tool(nil)})))+aimodel.MaxOutputTokens > op.capacity {
			t.Fatal("oversized compaction request")
		}
		if strings.Contains(input[1].Content, recent.Content) {
			t.Fatal("recent original sent to compaction")
		}
	}
	if op.calls != len(m.inputs) {
		t.Fatal("compaction escaped turn budget")
	}
}
