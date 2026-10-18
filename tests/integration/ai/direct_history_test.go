package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	agent "github.com/simjay/plous/internal/ai"
)

type directModel struct {
	next  func([]*schema.Message) *schema.Message
	tools []*schema.ToolInfo
}

func (m *directModel) WithTools(t []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.tools = t
	return m, nil
}
func (m *directModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return m.next(input), nil
}
func (m *directModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, input, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), err
}

func TestDirectHistoryUsesProductionSourcesWithoutWorker(t *testing.T) {
	o, histories := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	o.Connections = []agent.Connection{*connection}
	step := 0
	m := &directModel{}
	m.next = func(input []*schema.Message) *schema.Message {
		step++
		for _, tool := range m.tools {
			if tool.Name == "recall" {
				t.Fatal("removed worker tool advertised")
			}
		}
		switch step {
		case 1:
			return call("kubernetes_resources_read", `{"purpose":"Inspect the current Pod","connection_id":"captured","operation":"get","resource":"pods","name":"checkout","namespace":"payments"}`)
		case 2:
			var saved struct {
				Ref string `json:"ref"`
			}
			last := input[len(input)-1].Content
			if err := json.Unmarshal([]byte(last), &saved); err != nil || saved.Ref == "" {
				t.Fatal(last, err)
			}
			return call("history_read", string(marshal(map[string]any{"purpose": "Read saved evidence", "operation": "read", "ref": saved.Ref})))

		default:
			if !strings.Contains(input[len(input)-1].Content, "Running") {
				t.Fatal("saved original missing", input[len(input)-1].Content)
			}
			return answer("The saved observation says Running.")
		}
	}
	o.Model = m
	result, err := agent.Run(t.Context(), o)
	if err != nil || result.Text == "" || step != 3 || requests.Load() != 2 {
		t.Fatal(result, step, requests.Load(), err)
	}
	entries, err := histories.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || entry.Tool == "history_read" && entry.Projection != nil && strings.Contains(entry.Content, "Running")
	}
	if !found {
		t.Fatal("direct read lost its Sources entry")
	}
}
