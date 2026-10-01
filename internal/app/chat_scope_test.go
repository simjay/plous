package app

import (
	"github.com/simjay/plous/internal/workspace"
	"strings"
	"testing"
)

func TestAssistantCapturesVisibleResourceScopeWithoutBodies(t *testing.T) {
	m := exploreModel(t)
	m.explore.detail = m.explore.collection.Objects[0].DeepCopy()
	m.explore.detail.SetNamespace("visible-ns")
	m.explore.detail.Object["private_body"] = "must-not-enter-context"
	m.explore.preference.Filter = "visible-filter"
	scope := m.assistantKubernetesScope()
	if scope == nil || scope.Namespace != "visible-ns" {
		t.Fatalf("scope=%+v", scope)
	}
	text := m.contextText()
	if !strings.Contains(text, "visible-filter") || !strings.Contains(text, m.explore.detail.GetName()) || strings.Contains(text, "must-not-enter-context") {
		t.Fatal(text)
	}
	m.mode = 0
	if got := m.assistantKubernetesScope(); got.Namespace != m.currentConnection().Settings["namespace"] {
		t.Fatal("hidden Explore state changed scope")
	}
	if strings.Contains(m.contextText(), "visible-filter") {
		t.Fatal("hidden page leaked")
	}
}
func TestSummaryContextMeterExcludesCoveredExchanges(t *testing.T) {
	m, _ := testModel(t)
	c := m.chat()
	c.turns = []workspace.Turn{{Request: workspace.Request{ID: "a", Prompt: "old question"}, Response: workspace.Response{Status: "completed", Text: "old answer"}}, {Request: workspace.Request{ID: "b", Prompt: "new question"}, Response: workspace.Response{Status: "completed", Text: "new answer"}}}
	c.summary = &workspace.ReasoningEntry{Content: "compact facts", CoveredTurnID: "a"}
	msgs, _ := m.chatMessages("next", false)
	all := ""
	for _, msg := range msgs {
		all += msg.Text
	}
	if strings.Contains(all, "old question") || !strings.Contains(all, "compact facts") || !strings.Contains(all, "new question") {
		t.Fatal(all)
	}
}
