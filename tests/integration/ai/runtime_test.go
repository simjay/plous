package integration

import (
	"github.com/cloudwego/eino/schema"
	agent "github.com/simjay/plous/internal/ai"
	journal "github.com/simjay/plous/internal/ai/history"
	"strings"
	"testing"
)

func TestNativeFollowupSourcesResolveRetainedOriginal(t *testing.T) {
	o, store := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	o.Connections = []agent.Connection{*connection}
	o.Model = &scriptedModel{steps: []*schema.Message{call("kubernetes_resources_read", `{"purpose":"Read the saved phase","connection_id":"captured","operation":"get","resource":"pods","name":"checkout","namespace":"payments"}`), answer("First observation")}}
	first, err := agent.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: first.Text}); err != nil {
		t.Fatal(err)
	}
	turn, err := store.StartTurn(o.WorkspaceID, o.ConversationID, journal.Request{Prompt: "Explain that saved observation", Agent: o.Config})
	if err != nil {
		t.Fatal(err)
	}
	o.TurnID, o.Prompt = turn.Request.ID, turn.Request.Prompt
	fake := &scriptedModel{steps: []*schema.Message{answer("The saved phase was Running.")}}
	o.Model = fake
	second, err := agent.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveResponse(o.WorkspaceID, o.ConversationID, o.TurnID, journal.Response{Status: "completed", Text: second.Text}); err != nil {
		t.Fatal(err)
	}
	if len(fake.inputs) != 1 || requests.Load() != 2 || !strings.Contains(string(marshal(fake.inputs)), "Running") {
		t.Fatal("follow-up failed to use retained context", requests.Load())
	}
	evidence, err := store.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range evidence {
		found = found || entry.Projection != nil && strings.Contains(entry.Content, "Running")
	}
	if !found {
		t.Fatal("follow-up Sources lost saved observation", evidence)
	}

}
func TestNativeMainReadsDirectlyWithPurposeAndCapturedScope(t *testing.T) {
	o, store := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	connection.Namespace = "viewed-namespace"
	o.ScopeContext = `{"viewing":{"namespace":"viewed-namespace"}}`
	o.Connections = []agent.Connection{*connection}
	fake := &scriptedModel{steps: []*schema.Message{call("kubernetes_resources_read", `{"purpose":"Read checkout's phase","connection_id":"captured","operation":"get","resource":"pods","name":"checkout","namespace":"payments"}`), answer("The checkout Pod is running.")}}
	o.Model = fake
	r, err := agent.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || r.Text != "The checkout Pod is running." {
		t.Fatalf("result=%+v requests=%d", r, requests.Load())
	}
	if len(fake.inputs) != 2 {
		t.Fatalf("unexpected delegation: %d model calls", len(fake.inputs))
	}
	all := string(marshal(fake.inputs))
	if !strings.Contains(all, "Running") || strings.Contains(all, "fixture-kube-token") || strings.Contains(all, "private-payload") {
		t.Fatal(all)
	}
	evidence, err := store.TurnEvidence(t.Context(), o.WorkspaceID, o.ConversationID, o.TurnID)
	if err != nil || len(evidence) != 1 {
		t.Fatalf("sources=%+v %v", evidence, err)
	}
	if !strings.Contains(string(evidence[0].Input), `"purpose":"Read checkout's phase"`) {
		t.Fatal("purpose not saved", string(evidence[0].Input))
	}
	if evidence[0].Projection == nil || strings.Contains(evidence[0].Content, "nested") {
		t.Fatal("raw result retained")
	}

}

func TestNativeMainMissingPurposeCannotRead(t *testing.T) {
	o, _ := workspaceFixture(t)
	nativeOptions(t, &o)
	connection, requests := scopedKubernetesFixture(t)
	o.Connections = []agent.Connection{*connection}
	fake := &scriptedModel{steps: []*schema.Message{call("kubernetes_resources_read", `{"connection_id":"captured","operation":"get","resource":"pods","name":"checkout"}`), answer("I need to provide the read purpose.")}}
	o.Model = fake
	if _, err := agent.Run(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("read ran before purpose validation")
	}
	if !strings.Contains(string(marshal(fake.inputs[1])), "purpose is required") {
		t.Fatal("missing purpose feedback")
	}
}

func TestNativeFirstTurnHasNoInventedHistory(t *testing.T) {
	o, _ := fixture(t)
	nativeOptions(t, &o)
	fake := &scriptedModel{steps: []*schema.Message{answer("Hello.")}}
	o.Model = fake
	r, err := agent.Run(t.Context(), o)
	if err != nil || r.Text != "Hello." || len(fake.inputs) != 1 {
		t.Fatalf("%+v %v calls=%d", r, err, len(fake.inputs))
	}
}
