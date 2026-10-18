package integration

import (
	"context"
	"encoding/json"
	"errors"
	agent "github.com/simjay/plous/internal/ai"
	journal "github.com/simjay/plous/internal/ai/history"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/workspace"
)

type scriptedModel struct {
	sync.Mutex
	steps  []*schema.Message
	inputs [][]*schema.Message
}

func (m *scriptedModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *scriptedModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	s, e := m.Stream(ctx, msgs, opts...)
	if e != nil {
		return nil, e
	}
	defer s.Close()
	return s.Recv()
}
func (m *scriptedModel) Stream(ctx context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.Lock()
	defer m.Unlock()
	if len(m.steps) == 0 {
		return nil, errors.New("unexpected model call")
	}
	m.inputs = append(m.inputs, append([]*schema.Message(nil), msgs...))
	next := m.steps[0]
	m.steps = m.steps[1:]
	return schema.StreamReaderFromArray([]*schema.Message{next}), nil
}
func answer(s string) *schema.Message {
	return &schema.Message{Role: schema.Assistant, Content: s, ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}}
}
func call(name, args string) *schema.Message {
	return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: name, Arguments: args}}}, ResponseMeta: &schema.ResponseMeta{FinishReason: "tool_calls"}}
}
func fixture(t *testing.T) (agent.Options, *journal.Store) {
	t.Helper()
	store, e := workspace.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	history, e := journal.Open(store)
	if e != nil {
		t.Fatal(e)
	}
	c, e := history.NewConversation("", "test")
	if e != nil {
		t.Fatal(e)
	}
	config := aimodel.DefaultConfig()
	config.Model = aimodel.Models("openai")[0].ID
	turn, e := history.StartTurn("", c.ID, journal.Request{Agent: config, Prompt: "Question"})
	if e != nil {
		t.Fatal(e)
	}
	return agent.Options{Store: store, History: history, ConversationID: c.ID, TurnID: turn.Request.ID, Config: config, System: agent.Instructions, Prompt: "Question"}, history
}

func workspaceFixture(t *testing.T) (agent.Options, *journal.Store) {
	t.Helper()
	store, e := workspace.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	w, e := store.Save(workspace.Workspace{Name: "Commerce"}, "test")
	if e != nil {
		t.Fatal(e)
	}
	history, e := journal.Open(store)
	if e != nil {
		t.Fatal(e)
	}
	c, e := history.NewConversation(w.ID, "test")
	if e != nil {
		t.Fatal(e)
	}
	config := aimodel.DefaultConfig()
	config.Model = aimodel.Models("openai")[0].ID
	turn, e := history.StartTurn(w.ID, c.ID, journal.Request{Agent: config, Prompt: "Question"})
	if e != nil {
		t.Fatal(e)
	}
	return agent.Options{WorkspaceID: w.ID, Store: store, History: history, ConversationID: c.ID, TurnID: turn.Request.ID, Config: config, System: agent.Instructions, Prompt: "Question"}, history
}

func nativeOptions(t *testing.T, o *agent.Options) {
	t.Helper()
	c := agent.DefaultRuntimeConfig()
	o.Runtime = &c
}
func marshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
