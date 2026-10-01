package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/memory"
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
func fixture(t *testing.T) (Options, *workspace.Store) {
	t.Helper()
	store, e := workspace.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	c, e := store.NewConversation("", "test")
	if e != nil {
		t.Fatal(e)
	}
	config := ai.DefaultConfig()
	config.Model = ai.Models("openai")[0].ID
	turn, e := store.StartTurn("", c.ID, workspace.Request{Agent: config, Prompt: "Question"})
	if e != nil {
		t.Fatal(e)
	}
	return Options{Store: store, ConversationID: c.ID, TurnID: turn.Request.ID, Config: config, System: Instructions, Prompt: "Question"}, store
}

func TestAgentDirectAnswerMakesOneCall(t *testing.T) {
	o, _ := fixture(t)
	fake := &scriptedModel{steps: []*schema.Message{answer("Hello")}}
	o.Model = fake
	var streamed string
	o.Emit = func(s string) error { streamed = s; return nil }
	r, e := Run(context.Background(), o)
	if e != nil || r.Text != "Hello" || streamed != "Hello" || len(fake.inputs) != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", r, e, len(fake.inputs))
	}
}
func TestAgentDeniedWriteBeforeConnectionAndRecorded(t *testing.T) {
	o, store := fixture(t)
	o.Kubernetes = &KubernetesScope{Path: "/path/that/must/not/be/read", Context: "captured", Namespace: "dev", ConnectionID: "conn"}
	fake := &scriptedModel{steps: []*schema.Message{call("kubernetes_read", `{"operation":"apply","resource":"pods"}`), answer("Writes are disabled.")}}
	o.Model = fake
	r, e := Run(context.Background(), o)
	if e != nil || r.Text != "Writes are disabled." {
		t.Fatal(r, e)
	}
	last := fake.inputs[1]
	body := last[len(last)-1].Content
	if !strings.Contains(body, "read operations") || strings.Contains(body, "could not open") {
		t.Fatalf("write reached client creation: %s", body)
	}
	mem, e := memory.New(store, "", o.ConversationID)
	if e != nil {
		t.Fatal(e)
	}
	p, e := mem.Search(context.Background(), memory.Query{Source: "reasoning", Text: "denied"})
	if e != nil || len(p.Records) == 0 {
		t.Fatalf("denial missing: %+v %v", p, e)
	}
}
func TestAgentMemoryEvidenceDoesNotEnterNextConversationContext(t *testing.T) {
	o, store := fixture(t)
	old, e := store.StartTurn("", o.ConversationID, workspace.Request{Prompt: "Which release?", Agent: o.Config})
	if e != nil {
		t.Fatal(e)
	}
	e = store.SaveResponse("", o.ConversationID, old.Request.ID, workspace.Response{Status: "completed", Text: "release-blue"})
	if e != nil {
		t.Fatal(e)
	}
	o.Model = &scriptedModel{steps: []*schema.Message{call("memory", `{"action":"search","source":"conversation","text":"release-blue"}`), answer("The saved release was blue.")}}
	r, e := Run(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.SaveResponse("", o.ConversationID, o.TurnID, workspace.Response{Status: "completed", Text: r.Text}); e != nil {
		t.Fatal(e)
	}
	turns, e := store.Turns("", o.ConversationID)
	if e != nil {
		t.Fatal(e)
	}
	for _, msg := range Messages(Instructions, turns, nil, "next") {
		if strings.Contains(msg.Text, `"records"`) {
			t.Fatal("tool result leaked into continuing Q&A")
		}
	}
}
func TestAgentRecursiveInvestigationUsesSameModelAndGuard(t *testing.T) {
	o, _ := fixture(t)
	o.Kubernetes = &KubernetesScope{Path: "/not-loaded", Context: "captured", Namespace: "dev"}
	fake := &scriptedModel{steps: []*schema.Message{
		call("investigate", `{"question":"Check whether writes are allowed"}`),
		answer("v = call({\"operation\":\"delete\",\"resource\":\"pods\",\"name\":\"x\"})\nr = ask(\"Explain the refusal\", v[\"result\"][\"error\"])\nfinish(r[\"findings\"], evidence=[v[\"evidence\"]])"),
		answer("finish(\"Writes are denied by policy.\")"),
		answer("The investigation confirmed read-only access."),
	}}
	o.Model = fake
	r, e := Run(context.Background(), o)
	if e != nil || len(fake.inputs) != 4 || !strings.Contains(r.Text, "read-only") {
		t.Fatalf("%+v %v calls=%d", r, e, len(fake.inputs))
	}
	if !strings.Contains(fake.inputs[2][1].Content, "read operations") {
		t.Fatal("child did not receive selected denial evidence")
	}
}
func TestAgentRecursiveMemoryReadPassesStoredEvidenceToChild(t *testing.T) {
	o, store := fixture(t)
	entry, err := store.AppendReasoning("", o.ConversationID, workspace.ReasoningEntry{Kind: "tool", Tool: "fixture", Status: "completed", Content: "archive-lookup: chosen release is cobalt-731"})
	if err != nil {
		t.Fatal(err)
	}
	program := `response = read({"action":"search","source":"reasoning","text":"archive-lookup","limit":1})
record = response["result"]["records"][0]
detail = read({"action":"read","ref":record["ref"],"offset":0,"limit":4096})
child = ask("Which release was chosen?", evidence=detail["result"]["text"])
finish(child["findings"], evidence=[record["ref"], detail["evidence"]])`
	fake := &scriptedModel{steps: []*schema.Message{
		call("investigate", `{"question":"Find the saved release and ask a child to verify it"}`),
		answer(program),
		answer(`finish("The chosen release is cobalt-731.")`),
		answer("The chosen release is cobalt-731."),
	}}
	o.Model = fake
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if len(fake.inputs) != 4 || !strings.Contains(fake.inputs[2][1].Content, entry.ID) || !strings.Contains(fake.inputs[2][1].Content, entry.Content) {
		t.Fatal("child did not receive the saved record through search/read response envelopes")
	}
	last := fake.inputs[3]
	var envelope struct {
		Result struct {
			Findings string   `json:"findings"`
			Evidence []string `json:"evidence"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(last[len(last)-1].Content), &envelope); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(envelope.Result.Findings, "cobalt-731") || len(envelope.Result.Evidence) != 2 || !strings.HasPrefix(envelope.Result.Evidence[0], "reasoning:") || envelope.Result.Evidence[1] == "" {
		t.Fatal("investigation lost its child finding or evidence references")
	}
}

func TestAgentCompactionPreservesOriginalsAndReopens(t *testing.T) {
	o, store := fixture(t)
	o.contextWindow = 20000
	for i := 0; i < 8; i++ {
		turn, e := store.StartTurn("", o.ConversationID, workspace.Request{Prompt: strings.Repeat("old fact ", 1000), Agent: o.Config})
		if e != nil {
			t.Fatal(e)
		}
		turn.Response = workspace.Response{Status: "completed", Text: strings.Repeat("old answer ", 800)}
		if e = store.SaveResponse("", o.ConversationID, turn.Request.ID, turn.Response); e != nil {
			t.Fatal(e)
		}
		o.History = append(o.History, turn)
	}
	calls := 0
	input, e := prepareContext(context.Background(), o, func(ctx context.Context, system, prompt string) (string, error) {
		calls++
		if len(prompt) > 40000 {
			t.Fatal("oversized summarization request")
		}
		return "Release decision: blue. Original evidence remains available.", nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if calls == 0 || Estimate(input) > InputBudget(o.contextWindow) {
		t.Fatal("compaction not applied")
	}
	summary, e := store.LatestCompaction("", o.ConversationID)
	if e != nil || summary == nil || summary.CoveredTurnID == "" {
		t.Fatal("summary missing", e)
	}
	turns, e := store.Turns("", o.ConversationID)
	if e != nil || len(turns) != 9 {
		t.Fatal("full history lost", len(turns), e)
	}
	mem, e := memory.New(store, "", o.ConversationID)
	if e != nil {
		t.Fatal(e)
	}
	p, e := mem.Search(context.Background(), memory.Query{Source: "conversation", Text: "old fact", Limit: 1})
	if e != nil || len(p.Records) != 1 {
		t.Fatal("original not retrievable", e)
	}
	reopened := Messages(o.System, o.History, summary, o.Prompt)
	if string(marshal(input)) != string(marshal(reopened)) {
		t.Fatal("reopened context changed")
	}
}
func TestAgentCompactionFailureDoesNotAdvanceSummary(t *testing.T) {
	o, store := fixture(t)
	o.contextWindow = 16000
	for i := 0; i < 6; i++ {
		o.History = append(o.History, workspace.Turn{Request: workspace.Request{ID: string(rune('a' + i)), Prompt: strings.Repeat("word ", 1000)}, Response: workspace.Response{Status: "completed", Text: "answer"}})
	}
	_, e := prepareContext(context.Background(), o, func(context.Context, string, string) (string, error) { return "", errors.New("no summary") })
	if e == nil {
		t.Fatal("failed summary accepted")
	}
	summary, e := store.LatestCompaction("", o.ConversationID)
	if e != nil || summary != nil {
		t.Fatal("failed summary saved")
	}
}
func TestAgentUnknownCommandCannotCreateCapabilities(t *testing.T) {
	o, _ := fixture(t)
	o.Model = &scriptedModel{steps: []*schema.Message{call("shell", `{"command":"kubectl delete pods --all"}`), answer("Unsupported.")}}
	r, e := Run(context.Background(), o)
	if e != nil || r.Text != "Unsupported." {
		t.Fatal(r, e)
	}
}
func TestTraceRedactsProviderKey(t *testing.T) {
	o, store := fixture(t)
	o.Key = "test-secret-value"
	op := operation{o: o}
	_, e := op.trace(workspace.ReasoningEntry{Kind: "tool", Status: "denied", Content: o.Key, Input: json.RawMessage(`{"token":"test-secret-value"}`)})
	if e != nil {
		t.Fatal(e)
	}
	mem, e := memory.New(store, "", o.ConversationID)
	if e != nil {
		t.Fatal(e)
	}
	p, e := mem.Search(context.Background(), memory.Query{Source: "reasoning", Text: o.Key})
	if e != nil || len(p.Records) != 0 {
		t.Fatal("key reached trace")
	}
}

func TestAgentRedactsKeyFromFinalAnswerAndInternalCompletion(t *testing.T) {
	o, _ := fixture(t)
	o.Key = "key-that-must-stay-private"
	fake := &scriptedModel{steps: []*schema.Message{answer("Echo " + o.Key)}}
	o.Model = fake
	o.Emit = func(text string) error {
		if strings.Contains(text, o.Key) {
			t.Fatal("key emitted to UI")
		}
		return nil
	}
	result, err := Run(context.Background(), o)
	if err != nil || strings.Contains(result.Text, o.Key) {
		t.Fatal(result, err)
	}
	op := operation{o: o, model: &boundedModel{base: &scriptedModel{steps: []*schema.Message{answer("Summary " + o.Key)}}, state: &modelBudget{capacity: 400000}}}
	summary, err := op.complete(context.Background(), "summarize", "data")
	if err != nil || strings.Contains(summary, o.Key) {
		t.Fatal(summary, err)
	}
}

func TestModelUsageCountsCumulativeChunksOnce(t *testing.T) {
	base := &usageModel{}
	m := &boundedModel{base: base, state: &modelBudget{capacity: 400000}}
	if _, err := m.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	if got := m.usage(); got.Input != 10 || got.Output != 4 || got.Total != 14 {
		t.Fatalf("cumulative usage counted twice: %+v", got)
	}
}

type usageModel struct{}

func (*usageModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &usageModel{}, nil
}
func (m *usageModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("stream only")
}
func (*usageModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "hello", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11}}}, {Role: schema.Assistant, Content: " world", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop", Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}}}}), nil
}

func TestAgentStreamingRedactionWaitsForSplitCredential(t *testing.T) {
	o, _ := fixture(t)
	o.Key = "sk-private-split-key"
	m := &chunkModel{chunks: []*schema.Message{
		{Role: schema.Assistant, Content: "Result " + o.Key[:len(o.Key)-1]},
		{Role: schema.Assistant, Content: o.Key[len(o.Key)-1:] + ". Ends with s", ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}},
	}}
	o.Model = m
	var checkpoints []string
	o.Emit = func(s string) error { checkpoints = append(checkpoints, s); return nil }
	result, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range checkpoints {
		if strings.Contains(s, "sk-private") {
			t.Fatalf("credential prefix entered checkpoint: %q", s)
		}
	}
	if result.Text != "Result [redacted]. Ends with s" {
		t.Fatalf("final redaction lost ordinary trailing text: %q", result.Text)
	}
}

type chunkModel struct{ chunks []*schema.Message }

func (m *chunkModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *chunkModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("stream only")
}
func (m *chunkModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray(m.chunks), nil
}
