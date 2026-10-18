package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/workspace"
)

func currentDirectoryModel(t *testing.T) (*model, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"README.md":          "Launch directory document\n",
		"kustomization.yaml": "resources: [service.yaml]\n",
		"service.yaml":       "apiVersion: v1\nkind: Service\nmetadata: {name: launch-service}\nspec: {ports: [{port: 80}]}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	m, _ := testModel(t)
	m.currentRequest, m.currentLoading = m.currentLoading, nil
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m, canonical
}

func openCurrentFiles(t *testing.T, m *model) {
	t.Helper()
	c := m.currentDirectory.Connections[0]
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, c.ID)
	m.updateTree("enter")
	if !m.codeActive() || m.active != m.currentDirectory.ID || m.connection != c.ID {
		t.Fatal("source row did not open the shared Code view", m.status)
	}
	finishCode(t, m)
	m.focus = 1
}

func TestCurrentDirectoryUsesSharedCodeWithoutKubeconfig(t *testing.T) {
	m, dir := currentDirectoryModel(t)
	if m.syncCode() != nil || len(m.codeUI.States) != 0 {
		t.Fatal("startup scanned the launch directory")
	}
	cmd := m.syncCurrent()
	m.applyCurrent(cmd().(currentResult))
	if m.currentError == "" {
		t.Fatal("fixture must have no Kubernetes target")
	}
	openCurrentFiles(t, m)
	s := m.coder()
	if s.Config.Path != dir || s.Inventory == nil || len(s.Inventory.Entries) == 0 || !s.Inventory.Source.Has("README.md") {
		t.Fatal("launch directory did not use source acquisition")
	}
	for _, view := range []string{"files", "helm", "kustomize", "argo", "flux"} {
		row := m.treeRows()[treeIndexFor(t, m, m.connection+"/"+view)]
		if row.parent != m.connection || row.depth != 3 {
			t.Fatal("current source diverges from saved source hierarchy", row)
		}
	}
	tools := m.assistantConnections()
	if len(tools) != 1 || tools[0].Tool != "source" || tools[0].Path != dir {
		t.Fatal("assistant did not capture only the launch directory", tools)
	}
	if got := m.store; got == nil || m.chat().Err != "" {
		t.Fatal("directory history unavailable", m.chat().Err)
	}
	m.execute("connections")
	if m.codeVisible() || !m.exploreVisible() || !strings.Contains(ansi.Strip(m.mainBody(80, 24)), "connection") {
		t.Fatal("context picker was hidden behind Code")
	}
	press(t, m, "esc")
	if !m.codeVisible() || m.coder() != s {
		t.Fatal("picker cancellation did not restore Code")
	}
	m.execute("refresh")
	finishCode(t, m)
	if m.coder() != s {
		t.Fatal("refresh replaced Code state")
	}
	rows := m.treeRows()
	if _, selected := m.treeRowState(rows[0]); !selected {
		t.Fatal("Current Workspace root lost selected styling")
	}
	m.setTreeExpanded(m.currentTreeKey(), false)
	for _, row := range m.treeRows() {
		if row.kind == "codeview" || row.kind == "connection" || row.kind == "area" {
			t.Fatal("folded Current Workspace left visible descendants")
		}
	}
}

func TestCurrentDirectoryHistorySurvivesClustersAndRestart(t *testing.T) {
	m, dir := currentDirectoryModel(t)
	openCurrentFiles(t, m)
	id, connection, state := m.conversationScope(), m.connection, m.coder()
	conversation, err := m.history.NewConversation(id, "Directory conversation")
	if err != nil {
		t.Fatal(err)
	}
	m.chatIDs[id] = conversation.ID
	m.draft().Text = "Keep directory draft"
	state.Preference.File = "README.md"
	m.rememberCode(state)
	m.currentPaths = currentFixture(t)
	for _, name := range []string{"alpha", "beta"} {
		resolveCurrentTest(t, m, name)
		if m.conversationScope() == id || m.draft().Text != "" {
			t.Fatal("cluster inherited directory chat")
		}
		m.selectScope(id, "files", connection)
		if m.coder() != state || m.draft().Text != "Keep directory draft" {
			t.Fatal("cluster switch reset Code")
		}
	}
	if err = m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	if restored.syncCode() != nil {
		t.Fatal("restart scanned Code before opening it")
	}
	openCurrentFiles(t, restored)
	if restored.conversationScope() != id || restored.draft().Text != "Keep directory draft" || restored.chat().Conversation.ID != conversation.ID || restored.coder().Preference.File != "README.md" {
		t.Fatal("directory conversation or preference not restored")
	}
	other := t.TempDir()
	t.Chdir(other)
	next, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	openCurrentFiles(t, next)
	if next.conversationScope() == id || next.draft().Text != "" || restored.assistantConnections()[0].Path != dir {
		t.Fatal("launch directory changed a captured source or reused another directory's chat")
	}
	next.chooseHistory(conversation)
	if next.historyView != id || len(next.assistantConnections()) != 0 {
		t.Fatal("retained directory history granted tools")
	}
}

func TestCurrentDirectoryLateStartupAndModifiedDocument(t *testing.T) {
	m, _ := currentDirectoryModel(t)
	m.currentPaths = currentFixture(t)
	startup := m.syncCurrent()().(currentResult)
	openCurrentFiles(t, m)
	active := m.active
	m.applyCurrent(startup)
	if m.active != active || !m.codeActive() {
		t.Fatal("late cluster startup interrupted Code")
	}
	s := m.coder()
	selectCodeFile(t, s, "README.md")
	m.openCodeFile(true)
	s.Document.Paste("Unsaved change\n")
	m.beginSaveCurrent()
	if m.form != nil || !s.Document.Dirty() {
		t.Fatal("saving a workspace bypassed the modified document guard")
	}
	m.requestCurrent("beta", true)
	m.applyCurrent(m.syncCurrent()().(currentResult))
	if m.active != active || !s.Document.Dirty() || !strings.Contains(m.status, "edited source") {
		t.Fatal("context change discarded an edited source", m.status)
	}
	m.execute("w")
	if s.Document.Dirty() || s.Document.SavedStatus != "Saved to local file" {
		t.Fatal("current source did not use the shared save flow", s.Document.Diagnostic)
	}
}

func TestCurrentWorkspaceSaveIncludesAvailableConnections(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "cluster-and-directory"}[cluster], func(t *testing.T) {
			m, dir := currentDirectoryModel(t)
			if cluster {
				m.currentPaths = currentFixture(t)
				resolveCurrentTest(t, m, "alpha")
			}
			openCurrentFiles(t, m)
			origin := m.conversationScope()
			m.beginSaveCurrent()
			m.form.Fields[0].Value.Text = "Saved current"
			m.submitForm()
			want := 1
			if cluster {
				want++
			}
			if m.currentRoot || len(m.current().Connections) != want || m.current().OriginScope != origin || !m.codeActive() || m.savedConnection().Settings["path"] != dir || m.conversationScope() == origin {
				t.Fatal("save changed source or omitted current connections", m.status)
			}
			if !workspace.IsConnectionScope(m.conversationScope()) {
				t.Fatal("saved source retained transient identity")
			}
		})
	}
}

func TestCurrentWorkspaceSaveFromSavedWorkspaceUsesCurrentConnections(t *testing.T) {
	m, dir := currentDirectoryModel(t)
	m.currentPaths = currentFixture(t)
	resolveCurrentTest(t, m, "alpha")
	origin := m.active
	if !m.saveWorkspace(workspace.Workspace{Name: "Unrelated", Connections: []workspace.Connection{{Name: "Other source", Tool: "code", Settings: map[string]string{"path": t.TempDir()}}}}) {
		t.Fatal(m.status)
	}
	m.selectScope(m.active, "files", m.current().Connections[0].ID)
	m.beginSaveCurrent()
	if m.form.Fields[0].Value.Text != "alpha" {
		t.Fatal("save form inherited the unrelated source name")
	}
	m.submitForm()
	if m.current().OriginScope != origin || m.tool != "kubernetes" || m.savedConnection().Name != "alpha" || m.current().Connections[1].Settings["path"] != dir {
		t.Fatal("saving Current Workspace inherited an unrelated selection", m.status)
	}
}

func TestCurrentWorkspaceStartupPreservesSavedTreeSelection(t *testing.T) {
	m, _ := currentDirectoryModel(t)
	if !m.saveWorkspace(workspace.Workspace{Name: "Saved", Connections: []workspace.Connection{{Name: "Source", Tool: "code", Settings: map[string]string{"path": t.TempDir()}}}}) {
		t.Fatal(m.status)
	}
	connection := m.current().Connections[0].ID
	m.setTreeExpanded(m.currentTreeKey(), false)
	m.treeCursor = treeIndexFor(t, m, connection)
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	if restored.treeRows()[restored.treeCursor].key != connection {
		t.Fatal("opening Current Workspace at startup moved the saved tree selection")
	}
}
