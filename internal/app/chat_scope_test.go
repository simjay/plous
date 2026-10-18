package app

import (
	"encoding/json"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/monitor"

	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAssistantNamespaceIsPageContextOnly(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm", "argocd", "flux"} {
		t.Run(tool, func(t *testing.T) {
			var m *model
			var setNamespace func(string)
			switch tool {
			case "kubernetes":
				m = exploreModel(t)
				setNamespace = func(ns string) { m.explore.Preference.Namespace = ns }
			case "helm":
				var h *monitor.HelmState
				m, h = helmModel(t)
				setNamespace = func(ns string) { h.Preference.Namespace = ns }
			default:
				var view *gitOpsView
				m, view = gitOpsModel(t)
				if tool == "flux" {
					m.tool = tool
					view = m.gitOpsCurrent()
				}
				setNamespace = func(ns string) { view.State.Namespace = ns }
			}
			connections := m.assistantConnections()
			conversation, draft := m.chat(), m.draft()
			name := m.savedConnection().Settings["context"]
			for _, namespace := range []string{"team-a", "", "team-b"} {
				setNamespace(namespace)
				want := namespace
				if want == "" {
					want = "*"
				}
				var context struct {
					Viewing struct{ Namespace string }
				}
				if err := json.Unmarshal([]byte(m.contextText()), &context); err != nil || context.Viewing.Namespace != want {
					t.Fatal("page hint lost the namespace selection", context, err)
				}
				if m.assistantScopeLabel() != name || !reflect.DeepEqual(m.assistantConnections(), connections) {
					t.Fatal("browsing namespace changed the assistant's connection scope")
				}
				if m.chat() != conversation || m.draft() != draft {
					t.Fatal("browsing namespace changed the conversation or draft")
				}
			}
		})
	}
}

func TestAssistantClusterResourceHasNoNamespaceHint(t *testing.T) {
	m := exploreModel(t)
	m.explore.Resource = m.explore.Resources[3]
	if page := m.assistantPageContext(); page.Namespace != "" {
		t.Fatal("cluster resource inherited a browsing namespace", page)
	}
}

func TestAssistantCapturesVisibleResourceScopeWithoutBodies(t *testing.T) {
	m := exploreModel(t)
	m.explore.Detail = m.explore.Collection.Objects[0].DeepCopy()
	m.explore.Detail.SetNamespace("visible-ns")
	m.explore.Detail.Object["private_body"] = "must-not-enter-context"
	m.explore.Preference.Filter = "visible-filter"
	scope := m.assistantPageContext()
	if scope.Namespace != "visible-ns" {
		t.Fatalf("scope=%+v", scope)
	}
	text := m.contextText()
	if !strings.Contains(text, "visible-filter") || !strings.Contains(text, m.explore.Detail.GetName()) || strings.Contains(text, "must-not-enter-context") {
		t.Fatal(text)
	}
	m.mode = 0
	if got := m.assistantPageContext(); got.Namespace != "" {
		t.Fatal("hidden Explore state changed scope")
	}
	if strings.Contains(m.contextText(), "visible-filter") {
		t.Fatal("hidden page leaked")
	}
}

func TestContextPreviewUsesNativeMessagesInsteadOfRenderedTurns(t *testing.T) {
	m, _ := testModel(t)
	c := m.chat()
	c.Turns = []history.Turn{{Sequence: 1, Request: history.Request{ID: "a", Prompt: "old question"}, Response: history.Response{Status: "completed", Text: "old answer"}}, {Sequence: 2, Request: history.Request{ID: "b", Prompt: "new question"}, Response: history.Response{Status: "completed", Text: "new answer"}}}
	msgs, _ := m.chatMessages("next", false)
	all := ""
	for _, msg := range msgs {
		all += msg.Text
	}
	if strings.Contains(all, "old question") || strings.Contains(all, "compact facts") || strings.Contains(all, "new question") || msgs[len(msgs)-1].Text != "next" {
		t.Fatal(all)
	}
}

func TestConnectionSelectionBoundsInventoryConversationAndDraft(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	m.current().Connections = []workspace.Connection{
		{ID: "kube", Tool: "kubernetes", Name: "Staging", Settings: map[string]string{"context": "staging", "namespace": "apps", "kubeconfig": "/private/kubeconfig", "token": "private-token"}},
		{ID: "helm", Tool: "helm", Name: "Production", Settings: map[string]string{"context": "production", "namespace": "system", "kubeconfig": "/private/other", "driver": "configmap"}},
	}
	m.selectScope(m.active, "kubernetes", "kube")
	m.draft().Text = "Inspect staging"
	conversation, draft := m.chat(), m.draft()
	captured := m.assistantConnections()
	m.selectScope(m.active, "helm", "helm")
	if m.chat() == conversation || m.draft() == draft || m.draft().Text != "" {
		t.Fatal("connection switch reused conversation or draft")
	}
	if len(captured) != 2 || captured[0].Context != "staging" || captured[1].Context != "staging" {
		t.Fatal("captured sibling connection", captured)
	}
	text := m.contextText()
	if strings.Contains(text, "staging") || strings.Contains(text, "/private") || strings.Contains(text, "private-token") || !strings.Contains(text, "production") {
		t.Fatal(text)
	}
	m.selectScope(m.active, "helm", "kube")
	if m.draft() != draft || m.draft().Text != "Inspect staging" {
		t.Fatal("same cluster's Helm view changed connection scope")
	}
	m.active = ""
	if len(m.assistantConnections()) != 0 {
		t.Fatal("General chat inherited connections")
	}
}

func TestAssistantCapturesLocalSourceRootsWithoutAcquiringGit(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Sources")
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	m.current().Connections = []workspace.Connection{
		{ID: "local", Tool: "code", Name: "Local code", Settings: map[string]string{"source_type": "Local directory", "path": root}},
		{ID: "remote", Tool: "code", Name: "Remote code", Settings: map[string]string{"source_type": "Git repository", "path": "https://example.invalid/private.git"}},
	}
	m.connection, m.tool = "local", "files"
	captured := m.assistantConnections()
	if len(captured) != 1 || captured[0].Tool != "source" || captured[0].Path != canonical {
		t.Fatal("source capture broadened its supported scope", captured)
	}
	m.current().Connections[0].Settings["path"] = filepath.Join(root, "changed")
	if captured[0].Path != canonical {
		t.Fatal("captured root changed after send")
	}
	if strings.Contains(m.contextText(), canonical) {
		t.Fatal("local source root entered provider metadata")
	}
	if _, err := os.Stat(filepath.Join(root, "changed")); !os.IsNotExist(err) {
		t.Fatal("capture performed acquisition", err)
	}
}
