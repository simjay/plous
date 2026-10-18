package ai

import (
	"encoding/json"
	"github.com/cloudwego/eino/schema"
	journal "github.com/simjay/plous/internal/ai/history"
	aimodel "github.com/simjay/plous/internal/ai/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectedSourceSurvivesFollowupAndSources(t *testing.T) {
	o, h := workspaceFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "facts.txt"), []byte("release approved for staging"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "src", Tool: "source", Path: root}}
	m := &scriptedModel{steps: []*schema.Message{call("source_files_read", `{"purpose":"read release decision","connection_id":"src","operation":"read","path":"facts.txt"}`), answer("The release is approved for staging.")}}
	o.Model = m
	nativeOptions(t, &o)
	result, err := Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: result.Text}); err != nil {
		t.Fatal(err)
	}
	sources, err := h.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(sources) != 1 || sources[0].Projection == nil {
		t.Fatal(sources, err)
	}
	if !strings.Contains(sources[0].Content, "release approved for staging") {
		t.Fatal(sources)
	}
	bounds, err := h.RecordBoundaries(o.WorkspaceID, o.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := h.ReadRecords(t.Context(), o.WorkspaceID, o.ConversationID, "reasoning", 0, int64(bounds[0].Sequence), 100, journal.MaxRecordBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		var entry journal.ReasoningEntry
		if err = json.Unmarshal(row.Data, &entry); err != nil || entry.Projection == nil || entry.Content != "" || entry.Request != nil || len(entry.Messages) > 0 {
			t.Fatal("non-observation reasoning", string(row.Data), err)
		}
	}
	turn, err := h.StartTurn(o.WorkspaceID, o.ConversationID, journal.Request{Agent: o.Config, Prompt: "Which environment?"})
	if err != nil {
		t.Fatal(err)
	}
	o.Prompt, o.TurnID = "Which environment?", turn.Request.ID
	m = &scriptedModel{steps: []*schema.Message{answer("Staging.")}}
	o.Model = m
	if _, err = Run(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	input := string(marshal(m.inputs[0]))
	if !strings.Contains(input, "release approved for staging") || !strings.Contains(input, "tool_call_id") || strings.Contains(input, "active.md") {
		t.Fatal(input)
	}
}
func TestSharedBudgetStopsFurtherDispatch(t *testing.T) {
	o, _ := fixture(t)
	m := &scriptedModel{steps: []*schema.Message{call("history_read", `{"purpose":"find history","operation":"search","query":"old"}`), answer("must not run")}}
	o.Model = m
	c := DefaultRuntimeConfig()
	c.MaxSteps = 1
	o.Runtime = &c
	_, err := Run(t.Context(), o)
	if err == nil || !strings.Contains(err.Error(), "budget") || len(m.inputs) != 1 {
		t.Fatal(err, len(m.inputs))
	}
}
func TestProviderContinuationCheckpoint(t *testing.T) {
	o, h := fixture(t)
	native := answer("saved answer")
	native.Extra = map[string]any{"provider_state": "signed opaque continuation"}
	o.Model = &scriptedModel{steps: []*schema.Message{native}}
	nativeOptions(t, &o)
	out, err := Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.SaveResponse("", o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: out.Text}); err != nil {
		t.Fatal(err)
	}
	messages, err := h.AgentMessages(t.Context(), "", o.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	req, err := aimodel.Prepare(messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Messages[len(req.Messages)-1].Extra["provider_state"] != "signed opaque continuation" {
		t.Fatal(req.Messages)
	}
}

func TestProjectedToolEvidenceRedactsCapturedCredentials(t *testing.T) {
	o, h := workspaceFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o.Key = "quoted-\"test-key"
	if err = os.WriteFile(filepath.Join(root, "notes.txt"), []byte(o.Key), 0600); err != nil {
		t.Fatal(err)
	}
	o.Connections = []Connection{{ConnectionID: "src", Tool: "source", Path: root}}
	m := &scriptedModel{steps: []*schema.Message{call("source_files_read", string(marshal(map[string]string{"purpose": "Read " + o.Key, "connection_id": "src", "operation": "read", "path": "notes.txt"}))), answer("Read.")}}
	o.Model = m
	nativeOptions(t, &o)
	if _, err = Run(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	evidence, err := h.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 1 {
		t.Fatal(evidence, err)
	}
	text := evidence[0].Content + string(evidence[0].Input) + string(marshal(m.inputs))
	for _, key := range []string{o.Key, "quoted-"} {
		if strings.Contains(text, key) {
			t.Fatal("captured credential reached projected storage or model input")
		}
	}
	if !strings.Contains(text, "[redacted]") {
		t.Fatal("missing redaction")
	}
}

func TestMalformedModelArgumentsReceiveCorrectiveFeedback(t *testing.T) {
	for _, args := range []string{`{"operation":`, `{"purpose":"Read history","operation":"read","operation":"search","query":"old"}`} {
		o, _ := fixture(t)
		m := &scriptedModel{steps: []*schema.Message{call("history_read", args), answer("I need a valid history query.")}}
		o.Model = m
		if _, err := Run(t.Context(), o); err != nil {
			t.Fatal(err)
		}
		if len(m.inputs) != 2 || !strings.Contains(string(marshal(m.inputs[1])), "tool arguments must be an object") {
			t.Fatal("malformed model arguments did not receive corrective feedback")
		}
	}
}
