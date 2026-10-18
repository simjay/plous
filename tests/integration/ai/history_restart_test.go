package integration

import (
	"encoding/json"
	"github.com/cloudwego/eino/schema"
	agent "github.com/simjay/plous/internal/ai"
	journal "github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/workspace"
	"strings"
	"testing"
)

func TestProjectedEvidenceSurvivesHistoryReadAndRestart(t *testing.T) {
	o, history := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	o.Connections = []agent.Connection{*connection}
	// The history interface can read committed evidence before its background index catches up.
	step := 0
	main := &directModel{}
	main.next = func(input []*schema.Message) *schema.Message {
		step++
		switch step {
		case 1:
			return call("kubernetes_resources_read", `{"purpose":"Observe Pod phase","connection_id":"captured","operation":"get","resource":"pods","name":"checkout","namespace":"payments"}`)
		case 2:
			var saved struct {
				Ref string `json:"ref"`
			}
			if err := json.Unmarshal([]byte(input[len(input)-1].Content), &saved); err != nil || saved.Ref == "" {
				t.Fatal(err)
			}
			return call("history_read", string(marshal(map[string]any{"purpose": "Explain saved phase", "operation": "read", "ref": saved.Ref})))
		default:
			if !strings.Contains(input[len(input)-1].Content, "Running") {
				t.Fatal("history read lost phase")
			}
			return answer("The saved phase was Running.")
		}
	}
	o.Model = main
	result, err := agent.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = history.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: result.Text}); err != nil {
		t.Fatal(err)
	}
	root, _ := o.Store.DataRoot()
	if err = o.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	history, err = journal.Open(reopened)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := history.StartTurn(o.WorkspaceID, o.ConversationID, journal.Request{Agent: o.Config, Prompt: "Explain the earlier observation"})
	if err != nil {
		t.Fatal(err)
	}
	o.Store, o.History, o.TurnID, o.Prompt = reopened, history, turn.Request.ID, turn.Request.Prompt
	followup := &scriptedModel{steps: []*schema.Message{answer("Using the saved observation.")}}
	o.Model = followup
	if _, err = agent.Run(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if len(followup.inputs) != 1 || requests.Load() != 2 || !strings.Contains(string(marshal(followup.inputs)), "Running") {
		t.Fatal("restart lost native context or refetched live state", requests.Load())
	}
}
