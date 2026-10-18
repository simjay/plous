// These offline acceptance tests exercise the production agent with scripted
// models and fixture APIs. They do not measure hosted model answer quality.
package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/integrations/observation"
	"github.com/simjay/plous/internal/workspace"
)

type scriptModel struct {
	t     *testing.T
	steps []func([]*schema.Message) (*schema.Message, error)
	calls int
}

func (m *scriptModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *scriptModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	stream, err := m.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return stream.Recv()
}
func (m *scriptModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.calls >= len(m.steps) {
		return nil, errors.New("unexpected model call")
	}
	step := m.steps[m.calls]
	m.calls++
	response, err := step(input)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{response}), nil
}
func reply(text string) func([]*schema.Message) (*schema.Message, error) {
	return func([]*schema.Message) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: text, ResponseMeta: &schema.ResponseMeta{FinishReason: "stop"}}, nil
	}
}
func tool(name, args string) func([]*schema.Message) (*schema.Message, error) {
	return func([]*schema.Message) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call", Type: "function", Function: schema.FunctionCall{Name: name, Arguments: args}}}, ResponseMeta: &schema.ResponseMeta{FinishReason: "tool_calls"}}, nil
	}
}
func newFixture(t *testing.T) ai.Options {
	t.Helper()
	profile, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { profile.Close() })
	w, err := profile.Save(workspace.Workspace{Name: "Fixture"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	journal, err := history.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { journal.Close() })
	conversation, err := journal.NewConversation(w.ID, "Acceptance")
	if err != nil {
		t.Fatal(err)
	}
	config := aimodel.DefaultConfig()
	config.Model = aimodel.Models("openai")[0].ID
	runtime := ai.DefaultRuntimeConfig()
	return ai.Options{Store: profile, History: journal, WorkspaceID: w.ID, ConversationID: conversation.ID, Config: config, Runtime: &runtime}
}
func start(t *testing.T, o *ai.Options, prompt string) {
	t.Helper()
	turn, err := o.History.StartTurn(o.WorkspaceID, o.ConversationID, history.Request{Agent: o.Config, Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	o.Prompt, o.TurnID = prompt, turn.Request.ID
}
func run(t *testing.T, o ai.Options) aimodel.Result {
	t.Helper()
	result, err := ai.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = o.History.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, history.Response{Status: "completed", Text: result.Text}); err != nil {
		t.Fatal(err)
	}
	return result
}
func clusterFixture(t *testing.T, o *ai.Options) *atomic.Int32 {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1" {
			fmt.Fprint(w, `{"resources":[{"name":"pods","kind":"Pod","namespaced":true,"verbs":["get","list"]}]}`)
			return
		}
		if r.URL.Path != "/api/v1/namespaces/payments/pods" {
			t.Error("unexpected API path", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		var pods []any
		for i, ready := range []string{"True", "False", "False"} {
			pods = append(pods, map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": fmt.Sprintf("api-%d", i), "namespace": "payments", "uid": fmt.Sprintf("uid-%d", i), "managedFields": strings.Repeat("RAW_SENTINEL ", 4000)}, "spec": map[string]any{"containers": []any{map[string]any{"name": "app"}}}, "status": map[string]any{"phase": "Running", "conditions": []any{map[string]any{"type": "Ready", "status": ready}}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"kind": "PodList", "apiVersion": "v1", "metadata": map[string]string{"resourceVersion": "19"}, "items": pods})
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: fixture\nclusters:\n- name: fixture\n  cluster: {server: %q}\ncontexts:\n- name: fixture\n  context: {cluster: fixture, namespace: payments}\n", server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	o.Connections = []ai.Connection{{ConnectionID: "cluster", Name: "Fixture cluster", Tool: "kubernetes", Path: path, Context: "fixture", Namespace: "payments"}}
	return calls
}

const readPods = `{"purpose":"Count unready Pods","connection_id":"cluster","operation":"get","resource":"pods"}`

func TestProjectedAgentPersistenceFollowupAndSavedSources(t *testing.T) {
	o := newFixture(t)
	calls := clusterFixture(t, &o)
	original := "How many Pods are not ready?\n  Preserve this original text: 한글\r\n"
	start(t, &o, original)
	o.Model = &scriptModel{t: t, steps: []func([]*schema.Message) (*schema.Message, error){tool("kubernetes_resources_read", readPods), func(input []*schema.Message) (*schema.Message, error) {
		var delivered struct {
			Ref         string
			Observation observation.Result
		}
		if err := json.Unmarshal([]byte(input[len(input)-1].Content), &delivered); err != nil {
			t.Fatal(err)
		}
		if delivered.Ref == "" || delivered.Observation.Facts["unready"].String() != "2" || !delivered.Observation.Coverage.Complete {
			t.Fatal(delivered)
		}
		// Persistence must precede delivery to the model.
		saved, err := o.History.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
		if err != nil || len(saved) != 1 || saved[0].Ref != delivered.Ref {
			t.Fatal(saved, err)
		}
		if strings.Contains(input[len(input)-1].Content, "RAW_SENTINEL") {
			t.Fatal("raw API body reached model")
		}
		return reply("Two of three Pods are not ready.")(input)
	}}}
	run(t, o)
	firstTurn := o.TurnID
	root, err := o.Store.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "RAW_SENTINEL") {
			t.Errorf("raw body persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = o.History.Close(); err != nil {
		t.Fatal(err)
	}
	o.History, err = history.Open(o.Store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { o.History.Close() })
	turns, err := o.History.Turns(o.WorkspaceID, o.ConversationID)
	if err != nil || len(turns) != 1 || turns[0].Request.Prompt != original {
		t.Fatal("original conversation changed", turns, err)
	}
	before := calls.Load()
	start(t, &o, "How many were ready?")
	o.Model = &scriptModel{t: t, steps: []func([]*schema.Message) (*schema.Message, error){func(input []*schema.Message) (*schema.Message, error) {
		data, _ := json.Marshal(input)
		if !strings.Contains(string(data), "unready") || !strings.Contains(string(data), "tool_call_id") || strings.Contains(string(data), "active.md") {
			t.Fatal("native continuity missing")
		}
		return reply("One was ready.")(input)
	}}}
	run(t, o)
	for _, turn := range []string{firstTurn, o.TurnID} {
		sources, err := o.History.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, turn)
		if err != nil || len(sources) != 1 || sources[0].Projection == nil {
			t.Fatal(sources, err)
		}
	}
	if calls.Load() != before {
		t.Fatal("followup or Sources refetched current APIs")
	}
}

func TestHistoryCalculationsShareTurnBudget(t *testing.T) {
	for _, budget := range []int{128, 1} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			o := newFixture(t)
			calls := clusterFixture(t, &o)
			start(t, &o, "Inspect Pods")
			o.Model = &scriptModel{t: t, steps: []func([]*schema.Message) (*schema.Message, error){tool("kubernetes_resources_read", readPods), reply("Two Pods were unready.")}}
			run(t, o)
			evidence, err := o.History.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
			if err != nil || len(evidence) != 1 {
				t.Fatal(evidence, err)
			}
			ref, before := evidence[0].Ref, calls.Load()
			start(t, &o, "What did the earlier inventory establish?")
			o.Runtime.MaxSteps = budget
			args, _ := json.Marshal(map[string]any{"purpose": "Check saved count", "operation": "count", "ref": ref, "field": "ready", "equals": map[string]bool{"boolean": false}})
			main := &scriptModel{t: t, steps: []func([]*schema.Message) (*schema.Message, error){tool("history_read", string(args)), func(input []*schema.Message) (*schema.Message, error) {
				var result history.HistoryReply
				if err := json.Unmarshal([]byte(input[len(input)-1].Content), &result); err != nil || result.Calculation == nil || result.Calculation.Matched != 2 || !result.Calculation.Complete {
					t.Fatal(result, err)
				}
				return reply("The saved inventory had two unready Pods.")(input)
			}}}
			o.Model = main
			result, err := ai.Run(t.Context(), o)
			if budget == 1 {
				if err == nil || !strings.Contains(err.Error(), "budget") || main.calls != 1 {
					t.Fatal(err, main.calls)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err = o.History.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, history.Response{Status: "completed", Text: result.Text}); err != nil {
					t.Fatal(err)
				}
				sources, err := o.History.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, source := range sources {
					found = found || source.Ref == ref
				}
				if !found {
					t.Fatal("history calculation lost original Source")
				}
			}
			if calls.Load() != before {
				t.Fatal("saved calculation refetched a live API")
			}
		})
	}
}

func TestMainHasNoDelegationOrContextEditingTools(t *testing.T) {
	o := newFixture(t)
	start(t, &o, "Hello")
	o.Observe = func(event ai.Observation) error {
		if event.Phase == "start" && event.Component != "main" && event.Component != "compaction" {
			t.Error("unexpected model component")
		}
		if event.Phase == "start" {
			var input struct{ Tools []aimodel.Tool }
			if err := json.Unmarshal([]byte(event.Input), &input); err != nil {
				return err
			}
			for _, tool := range input.Tools {
				if tool.Name == "recall" || tool.Name == "bash" || tool.Name == "history" {
					t.Errorf("retired tool %s", tool.Name)
				}
			}
		}
		return nil
	}
	o.Model = &scriptModel{t: t, steps: []func([]*schema.Message) (*schema.Message, error){reply("Hello.")}}
	run(t, o)
}
