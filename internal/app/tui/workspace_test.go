package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
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
	w, err := m.store.Save(workspace.Workspace{Name: "Group", Connections: []workspace.Connection{{Name: "first", Tool: "kubernetes", Settings: map[string]string{"context": "test"}}, {Name: "second", Tool: "kubernetes", Settings: map[string]string{"context": "test"}}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	m.workspaces = []workspace.Workspace{w}
	m.selectScope(w.ID, "kubernetes", w.Connections[0].ID)
	first := m.conversationScope()
	conversation, err := m.history.NewConversation(first, "first connection")
	if err != nil {
		t.Fatal(err)
	}
	m.draft().Text = "first draft"
	m.selectScope(w.ID, "kubernetes", w.Connections[1].ID)
	if m.conversationScope() == first || m.draft().Text != "" || len(m.assistantConnections()) != 2 {
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

func launchDirectoryModel(t *testing.T) *model {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	m, _ := testModel(t)
	m.currentRequest, m.currentLoading = &currentRequest{Select: true, Startup: true}, nil
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.launchDirectory != canonical {
		t.Fatal("native launch directory changed", m.launchDirectory, canonical)
	}
	return m
}

func TestConnectionLabelsKeepMutedBadgesAndTruncateNames(t *testing.T) {
	m, _ := testModel(t)
	for _, badge := range []string{"k8s", "local", "remote", "tf"} {
		for _, width := range []int{20, 24, 32, 46} {
			for _, focused := range []bool{false, true} {
				m.focus = 1
				if focused {
					m.focus = 0
				}
				row := treeRow{kind: "connection", key: "cluster", connection: "cluster", tool: "files", label: "production-長い-connection-name-that-does-not-fit", badge: badge, depth: 2}
				lines := m.renderTreeRow([]treeRow{row}, 0, width)
				if len(lines) != 1 {
					t.Fatalf("%s wrapped at width %d", badge, width)
				}
				line := ansi.Strip(lines[0])
				if ansi.StringWidth(line) != width || !strings.Contains(line, "... ("+badge+")") {
					t.Fatalf("name displaced badge at %d: %q", width, line)
				}
				badgeStyle := ui.Muted
				if focused {
					badgeStyle = badgeStyle.Background(ui.Highlight)
				}
				if !strings.Contains(lines[0], badgeStyle.Render(" ("+badge+")")) {
					t.Fatalf("%s badge lost its muted style", badge)
				}
			}
		}
	}
}

func TestClusterIdentityNormalizesPathsButPreservesAccessOptions(t *testing.T) {
	testModel(t)
	path := installDefaultKubeconfig(t, contextFile(t, "minikube"))
	base := workspace.Connection{Tool: "kubernetes", Settings: map[string]string{"context": "minikube", "kubeconfig": path}}
	link := filepath.Join(t.TempDir(), "config-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"~/.kube/config", filepath.Dir(path) + "/../.kube/config", link} {
		other := base
		other.Tool, other.Settings = "helm", maps.Clone(base.Settings)
		other.Settings["kubeconfig"], other.Settings["driver"] = alias, "secret"
		if !maps.Equal(clusterSettings(base), clusterSettings(other)) {
			t.Fatal("equivalent Kubernetes/Helm connection was not recognized", alias)
		}
	}
	for key, value := range map[string]string{"context": "other", "namespace": "team", "driver": "configmap", "repository_config": "private-repositories", "future": "preserved"} {
		other := base
		other.Settings = maps.Clone(base.Settings)
		other.Settings[key] = value
		if maps.Equal(clusterSettings(base), clusterSettings(other)) {
			t.Fatal("distinct access options were merged", key)
		}
	}
}

func TestLegacyIdenticalMonitorRowsPreserveActiveIdentityAndHistory(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Legacy")
	path := contextFile(t, "minikube")
	w := *m.current()
	w.Connections = []workspace.Connection{
		{Tool: "kubernetes", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path}},
		{Tool: "helm", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "driver": "secret"}},
		{Tool: "helm", Name: "minikube", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "driver": "configmap"}},
	}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	before, _ := m.store.History(m.active)
	beforeJSON, _ := json.Marshal(before)
	active := m.current().Connections[1].ID
	m.selectScope(m.active, "helm", active)
	m.expanded[active] = true
	connections := []string{}
	for _, row := range m.treeRows() {
		if row.kind == "connection" && row.workspace == m.active {
			connections = append(connections, row.connection)
		}
	}
	if len(connections) != 2 || connections[0] != active || len(m.current().Connections) != 3 {
		t.Fatal("duplicate row was not grouped without losing the active identity", connections)
	}
	after, _ := m.store.History(m.active)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("sidebar grouping rewrote history")
	}
}

func TestContextCustomNameIsPendingAndDoesNotChangeKubeconfig(t *testing.T) {
	m, _ := testModel(t)
	context := "arn:aws:eks:us-east-2:123456789012:cluster/production-platform"
	path := installDefaultKubeconfig(t, contextFile(t, context, "staging"))
	before, _ := os.ReadFile(path)
	createWorkspace(t, m, "Platform")
	press(t, m, "enter")
	press(t, m, "space")
	picker := m.form
	press(t, m, "right")
	if m.form.Kind != "contextname" {
		t.Fatal("name control did not open")
	}
	if m.form.Fields[0].Value.Text != "" {
		t.Fatal("optional name requires clearing the context before typing")
	}
	press(t, m, "ctrl+u")
	typeText(m, "Production")
	press(t, m, "enter")
	m.execute("help")
	m.closeHelp()
	press(t, m, "enter")
	if m.form != picker || !m.chosen[0] || m.pendingContextName(0) != "Production" || len(m.current().Connections) != 0 {
		t.Fatal("naming submitted the connection or lost the selection")
	}
	press(t, m, "right")
	press(t, m, "ctrl+u")
	typeText(m, "cancelled name")
	press(t, m, "esc")
	if m.pendingContextName(0) != "Production" {
		t.Fatal("cancelled edit replaced the pending name")
	}
	addSelectedContexts(t, m)
	c := m.current().Connections[0]
	if c.Name != "Production" || c.Settings["context"] != context {
		t.Fatal("custom label changed the actual context", c)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("naming changed kubeconfig")
	}
}

func TestContextPickerBlocksRenamedClusterAcrossToolsAndOptions(t *testing.T) {
	m, _ := testModel(t)
	path := installDefaultKubeconfig(t, contextFile(t, "minikube"))
	createWorkspace(t, m, "Platform")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "helm", Name: "Local cluster", Settings: map[string]string{"context": "minikube", "kubeconfig": path, "namespace": "another-team", "driver": "configmap"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	press(t, m, "enter")
	if !m.contextAlreadyAdded(0) {
		t.Fatal("renamed or customized cluster could be added twice")
	}
	press(t, m, "space")
	press(t, m, "right")
	if m.chosen[0] || m.form.Kind != "kube" {
		t.Fatal("already-added context accepted selection or a new name")
	}
	m.chosen[0] = true
	m.addContexts()
	if len(m.current().Connections) != 1 || !strings.Contains(m.status, "already") {
		t.Fatal("save bypassed duplicate protection")
	}
}

func TestConnectionFormRejectsDuplicateButAllowsLegacyRename(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Platform")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "kubernetes", Name: "Cluster", Settings: map[string]string{"context": "minikube", "kubeconfig": contextFile(t, "minikube")}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	c := m.current().Connections[0]
	m.toolSetup = nil
	m.beginConnection("helm", nil)
	for i := range m.form.Fields {
		f := &m.form.Fields[i]
		if value, ok := c.Settings[f.Key]; ok {
			f.Value = textview.NewField(value)
		} else if f.Key == "name" {
			f.Value = textview.NewField("Different label")
		} else if f.Key == "namespace" {
			f.Value = textview.NewField("another-team")
		}
	}
	m.submitForm()
	if m.form == nil || len(m.current().Connections) != 1 || !strings.Contains(m.status, "already exists") {
		t.Fatal("manual form accepted duplicate cluster", m.status)
	}
	legacy := c
	legacy.ID, legacy.Tool = "", "helm"
	w = *m.current()
	w.Connections = append(w.Connections, legacy)
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	m.beginConnection(c.Tool, &c)
	m.form.Fields[0].Value = textview.NewField("Renamed cluster")
	m.submitForm()
	if m.current().Connections[0].Name != "Renamed cluster" || m.current().Connections[0].ID != c.ID {
		t.Fatal("renaming changed identity", fmt.Sprint(m.current().Connections))
	}
}

func TestContextNamingMouseAndCompactLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}, {180, 42}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, _ := testModel(t)
			m.width, m.height = size[0], size[1]
			context := "arn:aws:eks:us-east-2:123456789012:cluster/production-platform"
			installDefaultKubeconfig(t, contextFile(t, context, "staging"))
			createWorkspace(t, m, "Platform")
			press(t, m, "enter")
			capture := func(scene string, labels ...string) {
				t.Helper()
				view := m.View().Content
				plain := ansi.Strip(view)
				for _, label := range labels {
					if !strings.Contains(plain, label) {
						t.Fatalf("%s hides %q\n%s", scene, label, plain)
					}
				}
				lines := strings.Split(view, "\n")
				if len(lines) != m.height {
					t.Fatalf("%s: %d rows, want %d", scene, len(lines), m.height)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("%s exceeds terminal width: %q", scene, ansi.Strip(line))
					}
				}
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("connections-%s-%d.txt", scene, m.width)), []byte(plain), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			capture("picker", "[→ Name]", "Add Selected Contexts")
			clickMouse(t, m, "main", "contextname/0")
			capture("name", "Name This Connection", "Name", "Use name")
			press(t, m, "ctrl+u")
			typeText(m, "Production")
			clickMouse(t, m, "main", "formaction/0")
			capture("named", "[ ] Production")
			press(t, m, "space")
			addSelectedContexts(t, m)
			m.beginToolSetup(true)
			capture("duplicate", "Production · Already added")
			m.current().Connections[0].Name = strings.Repeat("production-platform-", 6)
			capture("duplicate-long", "... · Already added")
			press(t, m, "down")
			press(t, m, "right")
			press(t, m, "ctrl+u")
			typeText(m, "Pending staging")
			clickMouse(t, m, "main", "formaction/0")
			clickMouse(t, m, "main", "context/1")
			if m.pendingContextName(1) != "staging" {
				t.Fatal("refresh retained a stale pending name")
			}
		})
	}
}

func TestRetiredSourcesStayArchivedAcrossRestart(t *testing.T) {
	m, profile := testModel(t)
	source := workspace.Connection{ID: "retired-source", Tool: "code", Name: "Retired repository", Settings: map[string]string{"path": t.TempDir()}}
	cluster := workspace.Connection{ID: "cluster", Tool: "kubernetes", Name: "Cluster", Settings: map[string]string{"context": "fixture"}}
	if !m.saveWorkspace(workspace.Workspace{Name: "Mixed", Connections: []workspace.Connection{source, cluster}}) {
		t.Fatal(m.status)
	}
	w := *m.current()
	scope := workspace.ConnectionScopeID(w.ID, source.ID)
	if err := m.store.EnsureScope(workspace.Scope{ID: scope, Kind: "connection", WorkspaceID: w.ID, Connection: source}); err != nil {
		t.Fatal(err)
	}
	conversation, err := m.history.NewConversation(scope, "Saved source evidence")
	if err != nil {
		t.Fatal(err)
	}
	m.expanded[w.ID] = true
	if err = m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.selectScope(w.ID, "", "")
	for _, row := range restored.treeRowsExpanded(true) {
		if row.connection == source.ID || row.tool == "code" || row.label == "Code" {
			t.Fatal("retired source returned to navigation", row)
		}
	}
	for _, row := range restored.settingsRows() {
		if strings.Contains(row.ID, source.ID) {
			t.Fatal("retired source returned to setup", row)
		}
	}
	restored.selectScope(w.ID, "files", source.ID)
	if restored.connection != "" || len(restored.assistantConnections()) != 0 {
		t.Fatal("stale source selection granted a live connection")
	}
	restored.chooseHistory(conversation)
	if restored.historyView != scope || len(restored.assistantConnections()) != 0 {
		t.Fatal("archived history could not open without live tools")
	}
	saved, err := m.store.List()
	if err != nil || len(saved) != 1 || len(saved[0].Connections) != 2 || saved[0].Revision != w.Revision || saved[0].Connections[0].ID != source.ID {
		t.Fatal("retirement changed saved workspace identity or revision", saved, err)
	}
	if _, err = os.Stat(filepath.Join(profile, ".plousignore")); !os.IsNotExist(err) {
		t.Fatal("startup created retired source scanning policy", err)
	}
}

func TestArchivedSourceOnlyWorkspaceOffersClusterSetup(t *testing.T) {
	m, _ := testModel(t)
	if !m.saveWorkspace(workspace.Workspace{Name: "Archived", Connections: []workspace.Connection{{Tool: "kustomize", Name: "Old source", Settings: map[string]string{"path": t.TempDir()}}}}) {
		t.Fatal(m.status)
	}
	m.closeSettings()
	m.mode = 0
	if body := ansi.Strip(m.mainBody(90, 24)); !strings.Contains(body, "Add a cluster") {
		t.Fatal("archived connections prevented cluster setup", body)
	}
	m.updateMain("enter")
	if m.toolSetup == nil {
		t.Fatal("empty overview did not open cluster setup")
	}
}

func TestRetirementPreservesSeparateLegacyClusterSettings(t *testing.T) {
	m, _ := testModel(t)
	connections := []workspace.Connection{
		{ID: "kubernetes", Tool: "kubernetes", Name: "Cluster", Settings: map[string]string{"context": "fixture"}},
		{ID: "helm", Tool: "helm", Name: "Cluster", Settings: map[string]string{"context": "fixture"}},
	}
	if !m.saveWorkspace(workspace.Workspace{Name: "Legacy", Connections: connections}) {
		t.Fatal(m.status)
	}
	seen := map[string]bool{}
	for _, row := range m.settingsRows() {
		seen[row.ID] = true
	}
	if !seen["connection/kubernetes"] || !seen["connection/helm"] {
		t.Fatal("retirement hid a legacy cluster record from Settings", seen)
	}
}

func TestExploreReusesClusterDiscoveryAndExplicitRefreshInvalidatesIt(t *testing.T) {
	m := exploreModel(t)
	c := *m.savedConnection()
	cluster := m.clusterFor(m.active, c)
	cluster.Client, cluster.Catalog, cluster.Loaded = m.explore.Client, m.explore.Resources, true
	m.explore.Key = ""
	m.syncExplore()
	if m.explore.Connecting || m.explore.Client != cluster.Client || m.explore.Stream == nil || cluster.Cancel != nil {
		t.Fatal("Explore rediscovered a loaded cluster or did not start browsing")
	}
	m.explore.Preference.Namespace = ""
	m.rememberExplore()
	m.stopExplore()
	m.explore.Key = ""
	m.syncExplore()
	if m.explore.Preference.Namespace != "" || m.explore.Client != cluster.Client {
		t.Fatal("shared discovery lost all-namespace selection")
	}
	m.refreshExplore()
	if cluster.Loaded || m.explore.Key != "" {
		t.Fatal("explicit refresh reused the previous catalog")
	}
	if m.syncExplore() == nil || cluster.Cancel == nil || !m.explore.Connecting {
		t.Fatal("refresh did not start shared discovery")
	}
	if m.syncExplore() != nil || m.syncClusters() != nil {
		t.Fatal("pending discovery was started twice")
	}
	m.stopClusters()
}

func TestSharedDiscoveryCompletionCannotRestartInactiveExplore(t *testing.T) {
	m := exploreModel(t)
	previous := m.clusterFor(m.active, *m.savedConnection())
	m.explore.Key = ""
	m.syncExplore()
	run := previous.Run
	m.mode = 0
	m.syncExplore()
	m.applyClusterDiscovery(clusterDiscoveryResult{Cluster: previous, Run: run, Client: m.explore.Client, Catalog: []kubecommon.Resource{{Name: "pods", Version: "v1"}}})
	if m.syncExplore() != nil || m.explore.Key != "" || m.explore.Stream != nil {
		t.Fatal("background discovery restarted inactive Explore")
	}
	m.stopClusters()
}

func TestClusterCatalogDoesNotCrossCapturedTargets(t *testing.T) {
	m, _ := testModel(t)
	c := workspace.Connection{ID: "cluster", Tool: "kubernetes", TargetCaptured: true, Target: &kubecommon.Target{Fingerprint: "first"}}
	previous := m.clusterFor("workspace", c)
	previous.Loaded = true
	ctx, cancel := context.WithCancel(t.Context())
	previous.Cancel = cancel
	c.Target = &kubecommon.Target{Fingerprint: "replacement"}
	replacement := m.clusterFor("workspace", c)
	if replacement == previous || replacement.Loaded || ctx.Err() == nil {
		t.Fatal("changed target reused a catalog or left its discovery running")
	}
	c.Target = nil
	if missing := m.clusterFor("workspace", c); missing == replacement || missing.Loaded {
		t.Fatal("unavailable captured target reused the previous client")
	}
}

func TestClusterCapabilitiesShareCurrentAndSavedTree(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "current"}[current], func(t *testing.T) {
			m, _ := testModel(t)
			if current {
				m.currentPaths = currentFixture(t)
				resolveCurrentTest(t, m, "")
			} else {
				if !m.saveWorkspace(workspace.Workspace{Name: "Saved", Connections: []workspace.Connection{{Name: "alpha", Tool: "kubernetes", Settings: map[string]string{"kubeconfig": currentFixture(t), "context": "alpha", "namespace": "apps"}}}}) {
					t.Fatal(m.status)
				}
				m.selectScope(m.active, "kubernetes", m.current().Connections[0].ID)
			}
			t.Cleanup(m.stopClusters)
			c := *m.savedConnection()
			m.expanded[m.active], m.expanded[c.ID] = true, true
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes"}) {
				t.Fatal("pre-populated optional tools", got)
			}
			if m.syncClusters() == nil {
				t.Fatal("cluster omitted from discovery")
			}
			cluster := m.clusterStates[clusterKey(m.active, c)]
			if cluster == nil || cluster.Cancel == nil {
				t.Fatal("discovery not started")
			}
			catalog := []kubecommon.Resource{
				{Group: "argoproj.io", Version: "v1alpha1", Name: "applications", Kind: "Application", Namespaced: true},
				{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Name: "kustomizations", Kind: "Kustomization", Namespaced: true},
			}
			m.applyClusterDiscovery(clusterDiscoveryResult{Cluster: cluster, Run: cluster.Run, Target: c.Target, Catalog: catalog})
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes", "argocd", "flux"}) {
				t.Fatal(got)
			}
			_, cluster.HelmCancel = context.WithCancel(t.Context())
			cluster.HelmRun = 1
			m.applyHelmDetection(helmDetectionResult{Cluster: cluster, Run: 1, Driver: "configmap"})
			if got := m.clusterCapabilities(m.active, c); !slices.Equal(got, []string{"kubernetes", "helm", "argocd", "flux"}) {
				t.Fatal(got)
			}
			for _, tool := range []string{"kubernetes", "helm", "argocd", "flux"} {
				row := m.treeRows()[treeIndexFor(t, m, c.ID+"/view/"+tool)]
				if row.kind != "capability" || row.parent != c.ID || !row.isBranch() {
					t.Fatalf("divergent branch: %+v", row)
				}
				m.mode, m.focus, m.treeCursor = 1, 0, treeIndexFor(t, m, row.key)
				m.updateTree("enter")
				if m.tool != tool || m.connection != c.ID {
					t.Fatal("branch selected wrong scope")
				}
				active, _ := m.treeRowState(row)
				if !active {
					t.Fatal("selected tool is not styled active")
				}
			}
			m.tool = "helm"
			if m.currentConnection().Settings["driver"] != "configmap" || m.assistantConnections()[1].Driver != "configmap" || m.savedConnection().Settings["driver"] != "" {
				t.Fatal("detected driver not shared or persisted unexpectedly")
			}
		})
	}
}

func TestClusterRefreshRejectsLateDetectionAndPreservesCursor(t *testing.T) {
	m, v := gitOpsModel(t)
	c := v.Cluster
	m.expanded[m.active], m.expanded[c.Connection.ID] = true, true
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun = 1
	key := c.Connection.ID + "/view/argocd"
	m.treeCursor = treeIndexFor(t, m, key)
	m.Update(helmDetectionResult{Cluster: c, Run: 1, Driver: "secret"})
	if m.treeRows()[m.treeCursor].key != key {
		t.Fatal("tool insertion moved cursor")
	}
	c.refresh()
	m.applyHelmDetection(helmDetectionResult{Cluster: c, Run: 1, Driver: "configmap"})
	if c.HelmDriver != "secret" {
		t.Fatal("late detection changed driver")
	}
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun++
	m.applyHelmDetection(helmDetectionResult{Cluster: c, Run: c.HelmRun, Err: errors.New("denied")})
	if c.HelmDriver != "secret" {
		t.Fatal("unknown refresh removed observed Helm")
	}
	m.expanded[c.Connection.ID+"/view/helm"] = true
	m.treeCursor = treeIndexFor(t, m, c.Connection.ID+"/helm/Releases")
	_, c.HelmCancel = context.WithCancel(t.Context())
	c.HelmRun++
	m.Update(helmDetectionResult{Cluster: c, Run: c.HelmRun})
	if c.HelmDriver != "" || m.treeRows()[m.treeCursor].key != c.Connection.ID {
		t.Fatal("empty discovery retained Helm or moved cursor")
	}
	m.stopGitOps()
	m.stopClusters()
}

func TestTreeSelectionColorsCoverEveryCell(t *testing.T) {
	for _, kind := range []string{"current", "workspace", "connection", "capability", "category", "resource", "helmsection", "gitopssection"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := testModel(t)
			m.focus, m.treeCursor = 0, 0
			row := treeRow{key: "selected", kind: kind, label: "Selected cluster", depth: 3, badge: "k8s"}
			lines := m.renderTreeRow([]treeRow{row}, 0, 48)
			terminal := vt.NewSafeEmulator(48, 3)
			defer terminal.Close()
			if _, err := terminal.Write([]byte(ui.CanvasStyle.Render(strings.Join(lines, "\n")))); err != nil {
				t.Fatal(err)
			}
			want := color.NRGBAModel.Convert(ui.Highlight)
			for x := 0; x < 48; x++ {
				cell := terminal.CellAt(x, 0)
				if cell == nil || cell.Style.Bg == nil || color.NRGBAModel.Convert(cell.Style.Bg) != want {
					t.Fatalf("selection background missing at column %d: %+v", x, cell)
				}
			}
			label := terminal.CellAt(9, 0)
			if label.Style.Fg == nil || color.NRGBAModel.Convert(label.Style.Fg) != color.NRGBAModel.Convert(ui.ShortcutStyle.GetForeground()) {
				t.Fatal("focused label has no selection color")
			}
		})
	}
}
