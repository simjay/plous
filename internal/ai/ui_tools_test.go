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
		`{"purpose":"show","view":"list","connection_id":"foreign","resource":"pods"}`,
		`{"purpose":"show","view":"source","connection_id":"east","path":"README.md"}`,
		`{"purpose":"show","view":"list","connection_id":"east","resource":"pods","name":"hidden"}`,
		`{"purpose":"show","view":"logs","connection_id":"east","resource":"pods","name":"api","namespace":"apps"}`,
	} {
		body, err := s.Call(t.Context(), agent.ToolCall{ID: "blocked", Name: "kubernetes_show", Args: json.RawMessage(args)}, 65536)
		if err != nil || !strings.Contains(string(body), `"status":"blocked"`) || u.calls != 0 {
			t.Fatal("invalid call reached UI", string(body), err)
		}
	}
	body, err := s.Call(t.Context(), agent.ToolCall{ID: "open", Name: "kubernetes_show", Args: json.RawMessage(`{"purpose":"show","view":"list","connection_id":"east","resource":"pods","namespace":"apps"}`)}, 65536)
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

func TestUISchemasAreIndependentAndStrict(t *testing.T) {
	for _, tool := range uiDefinitions() {
		compileToolSchema(t, tool.Parameters)
	}
	var read map[string]any
	json.Unmarshal(uiDefinitions()[0].Parameters, &read)
	if len(read["properties"].(map[string]any)) != 1 {
		t.Fatal("show mutated read schema")
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
			if err := op.setupHistory(); err != nil {
				t.Fatal(err)
			}
			s := newToolSession(t.Context(), op)
			defer s.Close()
			names := map[string]bool{}
			for _, tool := range s.Definitions() {
				names[tool.Name] = true
			}
			for _, tool := range showTools() {
				if names[tool.Name] != (tool.Owner == "" || tool.Owner == domain) {
					t.Fatal("incorrect presentation scope", names)
				}
				if tool.Owner != "" && tool.Owner != domain {
					out, err := s.Call(t.Context(), agent.ToolCall{ID: tool.Name, Name: tool.Name, Args: marshal(map[string]any{"purpose": "wrong scope"})}, 65536)
					if err != nil || !strings.Contains(string(out), "not available in captured scope") || u.calls != 0 {
						t.Fatal(string(out), err)
					}
				}
			}
		})
	}
	for _, tc := range []struct{ tool, owner, input, view string }{
		{"kubernetes_show", "kubernetes", `{"view":"list","resource":"pods"}`, "list"},
		{"helm_show", "helm", `{"namespace":"apps","name":"web","revision":2,"compare_revision":1}`, "compare"},
		{"source_show", "source", `{"path":"README.md"}`, "source"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			o, _ := workspaceFixture(t)
			u := &testUI{}
			o.UI, o.Connections = u, []Connection{{ConnectionID: "target", Tool: tc.owner}}
			op := &operation{o: o}
			if err := op.setupHistory(); err != nil {
				t.Fatal(err)
			}
			s := newToolSession(t.Context(), op)
			defer s.Close()
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.input), &args)
			args["purpose"], args["connection_id"] = "show", "target"
			var schemaTool agent.Tool
			for _, tool := range s.Definitions() {
				if tool.Name == tc.tool {
					schemaTool = tool
				}
			}
			if err := compileToolSchema(t, schemaTool.Parameters).Validate(args); err != nil {
				t.Fatal(err)
			}
			out, err := s.Call(t.Context(), agent.ToolCall{ID: tc.tool, Name: tc.tool, Args: marshal(args)}, 65536)
			var result agent.UIResult
			if err != nil || json.Unmarshal(out, &result) != nil || result.Status != "opened" || result.View == nil || result.View.View != tc.view {
				t.Fatal(string(out), err)
			}
			args["view"] = "source"
			if tc.tool == "kubernetes_show" {
				args["path"] = "README.md"
			}
			if err := compileToolSchema(t, schemaTool.Parameters).Validate(args); err == nil {
				t.Fatal("schema accepted an unrelated or redundant view")
			}
			out, err = s.Call(t.Context(), agent.ToolCall{ID: tc.tool + "-invalid", Name: tc.tool, Args: marshal(args)}, 65536)
			if err != nil || !strings.Contains(string(out), `"status":"blocked"`) || u.calls != 1 {
				t.Fatal(string(out), err)
			}
		})
	}
}

func TestPresentationOwnedContractsRejectUnrelatedArguments(t *testing.T) {
	for _, tc := range []struct {
		tool, input string
		valid       bool
	}{
		{"kubernetes_show", `{"view":"resource","resource":"pods","name":"web","namespace":"apps","tab":"yaml"}`, true},
		{"kubernetes_show", `{"view":"list","resource":"pods","labels":"app=web"}`, true},
		{"kubernetes_show", `{"view":"logs","resource":"pods","name":"web","namespace":"apps","container":"app","previous":false}`, true},
		{"kubernetes_show", `{"view":"list","resource":"pods","previous":false}`, false},
		{"kubernetes_show", `{"view":"logs","resource":"deployments.apps","name":"web","namespace":"apps","container":"app"}`, false},
		{"kubernetes_show", `{"view":"resource","resource":"pods","name":"web","tab":"values"}`, false},
		{"helm_show", `{"name":"web","namespace":"apps","revision":3,"compare_revision":2,"tab":"manifests"}`, true},
		{"helm_show", `{"name":"web","namespace":"apps","revision":3,"compare_revision":2,"tab":"yaml"}`, false},
		{"helm_show", `{"name":"web","namespace":"apps","revision":3,"compare_revision":2,"view":"compare"}`, false},
		{"helm_show", `{"name":"web","namespace":"apps","revision":0,"compare_revision":2}`, false},
		{"source_show", `{"path":"README.md","line":0}`, true},
		{"source_show", `{"path":"README.md","line":-1}`, false},
		{"source_show", `{"path":"README.md","offset":0}`, false},
		{"history_show", `{"ref":"record:c:r","offset":0}`, true},
		{"history_show", `{"ref":"record:c:r","limit":0}`, false},
	} {
		t.Run(tc.tool+tc.input, func(t *testing.T) {
			tool, ok := showDefinition(tc.tool)
			if !ok {
				t.Fatal(tc.tool)
			}
			var input map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.input), &input); err != nil {
				t.Fatal(err)
			}
			_, err := tool.Validate(input)
			if (err == nil) != tc.valid {
				t.Fatal("runtime", err)
			}
			var value any
			_ = json.Unmarshal([]byte(tc.input), &value)
			if got := compileToolSchema(t, tool.Schema()).Validate(value) == nil; got != tc.valid {
				t.Fatal("schema", got)
			}
		})
	}
}
