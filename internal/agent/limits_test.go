package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai"
)

func requireUserLimit(t *testing.T, err error, text string) {
	t.Helper()
	var local ai.UserError
	if !errors.As(err, &local) || !strings.Contains(err.Error(), text) {
		t.Fatalf("expected clear %q error, got %T: %v", text, err, err)
	}
	if strings.Contains(err.Error(), "connection failed") || strings.Contains(err.Error(), "Settings") {
		t.Fatalf("local limit became a provider failure: %v", err)
	}
}

func TestAgentSharedModelBudgetStopsDirectAndRecursiveCalls(t *testing.T) {
	o, _ := fixture(t)
	fake := &scriptedModel{}
	for investigation := 0; investigation < 3; investigation++ {
		outer := call("investigate", `{"question":"Examine the saved evidence"}`)
		outer.ToolCalls[0].ID = fmt.Sprintf("investigation-%d", investigation)
		fake.steps = append(fake.steps, outer)
		for iteration := 0; iteration < 7; iteration++ {
			fake.steps = append(fake.steps, answer(`print("Continue examining the available evidence.")`))
		}
		fake.steps = append(fake.steps, answer(`finish("Investigation complete.")`))
	}
	o.Model = fake
	_, err := Run(context.Background(), o)
	requireUserLimit(t, err, "model-call or context budget")
	fake.Lock()
	defer fake.Unlock()
	if len(fake.inputs) != 24 {
		t.Fatalf("shared budget allowed %d provider calls, want 24", len(fake.inputs))
	}
}

func TestAgentSharedToolBudgetStopsDirectAndRecursiveCalls(t *testing.T) {
	o, _ := fixture(t)
	outer := call("investigate", `{"question":"Read saved evidence"}`)
	for i := 0; i < 17; i++ {
		outer.ToolCalls = append(outer.ToolCalls, schema.ToolCall{
			ID: fmt.Sprintf("direct-memory-%d", i), Type: "function",
			Function: schema.FunctionCall{Name: "memory", Arguments: `{"action":"search","source":"conversation","limit":1}`},
		})
	}
	program := "for i in range(16):\n    read({\"action\":\"search\",\"source\":\"conversation\",\"limit\":1})\nfinish(\"Evidence collected.\")"
	fake := &scriptedModel{steps: []*schema.Message{outer, answer(program)}}
	o.Model = fake
	_, err := Run(context.Background(), o)
	requireUserLimit(t, err, "tool-call limit")
	fake.Lock()
	defer fake.Unlock()
	if len(fake.inputs) != 2 {
		t.Fatalf("expected outer call and one investigation call, got %d", len(fake.inputs))
	}
}

func TestAgentInternalContextLimitKeepsItsStopReason(t *testing.T) {
	fake := &scriptedModel{}
	op := operation{model: &boundedModel{base: fake, state: &modelBudget{capacity: 20000}}}
	_, err := op.complete(context.Background(), "Summarize.", strings.Repeat("x", 30000))
	requireUserLimit(t, err, "model-call or context budget")
	if len(fake.inputs) != 0 {
		t.Fatal("oversized context reached the provider")
	}
}

func TestAgentInternalSharedOutputLimitKeepsItsStopReason(t *testing.T) {
	fake := &scriptedModel{steps: []*schema.Message{answer(strings.Repeat("x", 256))}}
	op := operation{model: &boundedModel{base: fake, state: &modelBudget{capacity: 20000, tokens: 4*20000 - 20}}}
	_, err := op.complete(context.Background(), "Summarize.", "data")
	requireUserLimit(t, err, "shared token budget")
	if len(fake.inputs) != 1 {
		t.Fatal("output limit was not checked while receiving the model response")
	}
}

func TestAgentIterationLimitKeepsItsStopReason(t *testing.T) {
	o, _ := fixture(t)
	fake := &scriptedModel{}
	for i := 0; i < 13; i++ {
		next := call("memory", `{"action":"search","source":"conversation","limit":1}`)
		next.ToolCalls[0].ID = fmt.Sprintf("iteration-%d", i)
		fake.steps = append(fake.steps, next)
	}
	o.Model = fake
	_, err := Run(context.Background(), o)
	requireUserLimit(t, err, "iteration limit")
	fake.Lock()
	defer fake.Unlock()
	if len(fake.inputs) != 12 {
		t.Fatalf("iteration limit allowed %d provider calls, want 12", len(fake.inputs))
	}
}
