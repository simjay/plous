package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/workspace"
)

func TestCurrentStartupOpensOverview(t *testing.T) {
	for _, previousMode := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprint(previousMode), func(t *testing.T) {
			m, _ := testModel(t)
			if m.mode != 0 {
				t.Fatal("fresh launch did not select Overview")
			}
			if previousMode >= 0 {
				m.mode = previousMode
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				restored, err := newModel(m.store, m.history, "test")
				if err != nil {
					t.Fatal(err)
				}
				m = restored
				t.Cleanup(func() { m.quitting = true; m.drainPersistence() })
			}
			if m.mode != 0 || !m.currentRoot {
				t.Fatal("launch restored the previous tab instead of Current Workspace Overview")
			}
			m.currentPaths = currentFixture(t)
			m.currentLoading = nil
			m.currentRequest = &currentRequest{Select: true, Startup: true}
			m.applyCurrent(m.syncCurrent()().(currentResult))
			if !m.overviewActive() || m.savedConnection().Name != "alpha" {
				t.Fatal("context resolution did not retain Overview for the captured target")
			}
			pref := m.explorePreferences[m.active+"/"+m.connection]
			if pref.Resource != "pods" || pref.Namespace != "apps" {
				t.Fatal("startup changed the initial Explore resource or context namespace", pref)
			}
		})
	}
}

func TestCurrentStartupPreservesTabChosenDuringResolution(t *testing.T) {
	for _, command := range []string{"2", "3"} {
		t.Run(command, func(t *testing.T) {
			m, _ := testModel(t)
			m.currentPaths = currentFixture(t)
			m.currentLoading = nil
			m.currentRequest = &currentRequest{Select: true, Startup: true}
			resolve := m.syncCurrent()
			m.execute(command)
			mode := m.mode
			m.applyCurrent(resolve().(currentResult))
			if m.mode != mode || m.savedConnection() == nil || m.savedConnection().Name != "alpha" {
				t.Fatal("late startup changed the chosen tab or lost the captured context")
			}
		})
	}
}

func currentFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	data := "apiVersion: v1\nkind: Config\ncurrent-context: alpha\ncontexts:\n- name: alpha\n  context: {cluster: a, namespace: apps}\n- name: beta\n  context: {cluster: b, namespace: other}\nclusters:\n- name: a\n  cluster: {server: 'https://alpha.invalid'}\n- name: b\n  cluster: {server: 'https://beta.invalid'}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resolveCurrentTest(t *testing.T, m *model, name string) {
	t.Helper()
	m.currentRoot = true
	m.currentLoading = nil
	m.requestCurrent(name, true)
	cmd := m.syncCurrent()
	if cmd == nil {
		t.Fatal("missing resolution")
	}
	m.applyCurrent(cmd().(currentResult))
	if m.currentError != "" || m.savedConnection() == nil {
		t.Fatal(m.currentError, m.status)
	}
}

func TestCurrentContextStartsWithoutWorkspaceAndSeparatesTargets(t *testing.T) {
	m, _ := testModel(t)
	path := currentFixture(t)
	m.currentPaths = path
	resolveCurrentTest(t, m, "")
	if len(m.workspaces) != 0 || m.tool != "kubernetes" || m.savedConnection().Name != "alpha" {
		t.Fatal("startup required saved workspace")
	}
	saved, err := m.store.List()
	if err != nil || len(saved) != 0 {
		t.Fatal("runtime context became a saved workspace", err)
	}
	alpha := m.conversationScope()
	m.draft().Text = "alpha draft"
	captured := m.assistantConnections()
	resolveCurrentTest(t, m, "beta")
	if m.conversationScope() == alpha || m.draft().Text != "" || len(captured) != 2 || captured[0].Context != "alpha" {
		t.Fatal("target or draft crossed scopes")
	}
	m.draft().Text = "beta draft"
	resolveCurrentTest(t, m, "alpha")
	if m.draft().Text != "alpha draft" {
		t.Fatal("target draft was lost")
	}
	if m.chatScope() != "alpha" {
		t.Fatal(m.chatScope())
	}
}

func TestCurrentContextSaveAndRetainedHistory(t *testing.T) {
	m, _ := testModel(t)
	m.currentPaths = currentFixture(t)
	resolveCurrentTest(t, m, "")
	origin := m.active
	conversation, err := m.history.NewConversation(origin, "before saving")
	if err != nil {
		t.Fatal(err)
	}
	m.beginSaveCurrent()
	m.form.Fields[0].Value.Text = "Saved alpha"
	m.submitForm()
	if m.currentRoot || len(m.workspaces) != 1 || m.current().OriginScope != origin || m.savedConnection() == nil {
		t.Fatal("save did not create selected workspace", m.status)
	}
	if m.conversationScope() == origin {
		t.Fatal("save moved history identity")
	}
	choices, err := m.history.Conversations(origin)
	if err != nil || len(choices) != 1 || choices[0].ID != conversation.ID {
		t.Fatal("origin history lost", err)
	}
	m.chooseHistory(conversation)
	if m.historyView != origin || len(m.assistantConnections()) != 0 {
		t.Fatal("historical scope granted live tools")
	}
	m.draft().Text = "do not send from history"
	if m.sendChat() != nil || !strings.Contains(m.status, "read-only") {
		t.Fatal("retained history sent against another connection")
	}
}

func TestSavedConnectionHistorySeparatesConnections(t *testing.T) {
	m, _ := testModel(t)
	w, err := m.store.Save(workspace.Workspace{Name: "Group", Connections: []workspace.Connection{{Name: "first", Tool: "code", Settings: map[string]string{"path": t.TempDir()}}, {Name: "second", Tool: "code", Settings: map[string]string{"path": t.TempDir()}}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	m.workspaces = []workspace.Workspace{w}
	m.selectScope(w.ID, "files", w.Connections[0].ID)
	first := m.conversationScope()
	conversation, err := m.history.NewConversation(first, "first connection")
	if err != nil {
		t.Fatal(err)
	}
	m.draft().Text = "first draft"
	m.selectScope(w.ID, "files", w.Connections[1].ID)
	if m.conversationScope() == first || m.draft().Text != "" || len(m.assistantConnections()) != 1 {
		t.Fatal("connection scope leaked")
	}
	m.chooseHistory(conversation)
	if m.historyView != first || len(m.assistantConnections()) != 0 {
		t.Fatal("another connection history grants live access")
	}
}

func TestCurrentStartupDoesNotInterruptGeneralChat(t *testing.T) {
	m, _ := testModel(t)
	m.currentPaths = currentFixture(t)
	m.currentLoading = nil
	m.currentRequest = &currentRequest{Select: true, Startup: true}
	cmd := m.syncCurrent()
	m.focusChat(true)
	m.draft().Text = "Keep typing before setup"
	m.applyCurrent(cmd().(currentResult))
	if m.active != "" || m.focus != 2 || !m.chatEditing || m.draft().Text != "Keep typing before setup" {
		t.Fatal("late startup moved the user's current interaction")
	}
	if len(m.assistantConnections()) != 0 {
		t.Fatal("General chat gained connection tools")
	}
}

func TestCurrentSelectionPreservesUnfinishedHelmActions(t *testing.T) {
	m, _ := testModel(t)
	m.currentPaths = currentFixture(t)
	resolveCurrentTest(t, m, "alpha")
	m.tool = "helm"
	h := m.helmer()
	draft := &monitor.HelmDraft{Values: "keep: this"}
	h.Draft, h.Workflow = draft, &monitor.HelmWorkflow{Active: true}
	m.currentLoading = nil
	m.requestCurrent("beta", true)
	m.applyCurrent(m.syncCurrent()().(currentResult))
	if m.savedConnection().Name != "alpha" || h.Draft != draft {
		t.Fatal("selection discarded an open Helm action")
	}
	h.Workflow.Active = false
	m.resetCurrentClients(m.active, m.connection)
	if m.helmStates[h.Key] != h || h.Draft != draft {
		t.Fatal("reselection discarded a suspended Helm draft")
	}
}
