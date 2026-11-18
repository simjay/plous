package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
)

type testUI struct{ calls int }

func (u *testUI) Read(ctx context.Context) (agent.UIResult, error) {
	u.calls++
	return agent.UIResult{Status: "ready"}, ctx.Err()
}
func (u *testUI) Show(ctx context.Context, q agent.ViewRequest) (agent.UIResult, error) {
	u.calls++
	return agent.UIResult{Status: "opened", View: &q, Focus: "assistant", Visible: true}, ctx.Err()
}

func TestUIToolBoundaryAndExecutionReceipts(t *testing.T) {
	o, h := workspaceFixture(t)
	u := &testUI{}
	o.UI, o.Connections = u, []Connection{{ConnectionID: "east", Tool: "kubernetes"}}
	op := &operation{o: o}
	if err := op.setupHistory(); err != nil {
		t.Fatal(err)
	}
	s := newToolSession(t.Context(), op)
	defer s.Close()
	for _, args := range []string{
		`{"purpose":"show","operation":"open","destination":"resource_list","connection_id":"foreign","resource":"pods"}`,
		`{"purpose":"show","operation":"open","destination":"source","connection_id":"east","path":"README.md"}`,
		`{"purpose":"show","operation":"open","destination":"resource_list","connection_id":"east","resource":"pods","name":"hidden"}`,
		`{"purpose":"show","operation":"open","destination":"pod_logs","connection_id":"east","name":"api","namespace":"apps"}`,
	} {
		body, err := s.Call(t.Context(), agent.ToolCall{ID: "blocked", Name: "plous_view", Args: json.RawMessage(args)}, 65536)
		if err != nil || !strings.Contains(string(body), `"status":"blocked"`) || u.calls != 0 {
			t.Fatal("invalid call reached UI", string(body), err)
		}
	}
	body, err := s.Call(t.Context(), agent.ToolCall{ID: "open", Name: "plous_view", Args: json.RawMessage(`{"purpose":"show","operation":"open","destination":"resource_list","connection_id":"east","resource":"pods","namespace":"apps"}`)}, 65536)
	if err != nil || !strings.Contains(string(body), `"status":"opened"`) || u.calls != 1 {
		t.Fatal(string(body), err)
	}
	evidence, err := h.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 0 {
		t.Fatal("UI receipts entered searchable evidence", evidence, err)
	}
	records, _, err := h.ReadTurnRecords(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID, 0, ^uint64(0), 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.Stream == "execution" && strings.Contains(string(record.Data), `"kind":"ui"`) {
			found = true
		}
	}
	if !found {
		t.Fatal("UI receipt was not saved")
	}
}

func TestPresentationToolScopesAndArguments(t *testing.T) {
	for _, domain := range []string{"", "kubernetes", "helm", "source"} {
		t.Run(domain, func(t *testing.T) {
			o, _ := workspaceFixture(t)
			u := &testUI{}
			o.UI = u
			o.Connections = []Connection{{ConnectionID: "target", Tool: domain}}
			op := &operation{o: o}
			s := newToolSession(t.Context(), op)
			defer s.Close()
			schema := compileToolSchema(t, op.viewSchema())
			for _, tc := range []struct{ owner, args, view string }{
				{"kubernetes", `{"operation":"open","destination":"resource_list","resource":"pods","connection_id":"target"}`, "list"},
				{"kubernetes", `{"operation":"open","destination":"pod_logs","name":"web","namespace":"apps","container":"app","connection_id":"target"}`, "logs"},
				{"helm", `{"operation":"open","destination":"helm_comparison","namespace":"apps","name":"web","revision":2,"compare_revision":1,"tab":"values","connection_id":"target"}`, "compare"},
			} {
				var input map[string]any
				if err := json.Unmarshal([]byte(tc.args), &input); err != nil {
					t.Fatal(err)
				}
				allowed := domain == tc.owner
				if (schema.Validate(input) == nil) != allowed {
					t.Fatal("incorrect destination scope", domain, input)
				}
				input["purpose"] = "Inspect view"
				body, err := s.Call(t.Context(), agent.ToolCall{ID: tc.view, Name: "plous_view", Args: marshal(input)}, 65536)
				var result agent.UIResult
				if err != nil || json.Unmarshal(body, &result) != nil {
					t.Fatal(string(body), err)
				}
				if allowed {
					if result.Status != "opened" || result.View == nil || result.View.View != tc.view {
						t.Fatal(string(body))
					}
				} else if result.Status != "blocked" {
					t.Fatal(string(body))
				}
				input["view"] = "source"
				out, err := s.Call(t.Context(), agent.ToolCall{ID: tc.view + "-invalid", Name: "plous_view", Args: marshal(input)}, 65536)
				if err != nil || !strings.Contains(string(out), `"status":"blocked"`) {
					t.Fatal(string(out), err)
				}
			}
		})
	}
}
