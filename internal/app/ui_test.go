package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/workspace"
)

func testModel(t *testing.T) (*model, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".plous")
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	m, err := newModel(store, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, root
}

func press(t *testing.T, m *model, key string) tea.Cmd {
	t.Helper()
	keys := map[string]tea.KeyPressMsg{
		"enter": {Code: tea.KeyEnter}, "tab": {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"esc":       {Code: tea.KeyEscape}, "up": {Code: tea.KeyUp},
		"down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft},
		"right": {Code: tea.KeyRight}, "home": {Code: tea.KeyHome},
		"end": {Code: tea.KeyEnd}, "space": {Code: tea.KeySpace, Text: " "},
		"ctrl+u": {Code: 'u', Mod: tea.ModCtrl}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	}
	msg, ok := keys[key]
	if !ok {
		t.Fatalf("unknown test key %q", key)
	}
	_, cmd := m.Update(msg)
	return cmd
}

func typeText(m *model, text string) {
	for _, r := range text {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func focusPanel(t *testing.T, m *model, panel int) {
	t.Helper()
	for i := 0; i < 3 && m.focus != panel; i++ {
		press(t, m, "tab")
	}
	if m.focus != panel {
		t.Fatalf("could not focus panel %d", panel)
	}
}

func mainCommand(t *testing.T, m *model, command string) tea.Cmd {
	t.Helper()
	focusPanel(t, m, 1)
	typeText(m, ":"+command)
	return press(t, m, "enter")
}

func completeForm(t *testing.T, m *model, values ...string) {
	t.Helper()
	if m.form == nil || len(m.form.fields) != len(values) {
		t.Fatalf("expected form with %d fields, got %#v; status: %s", len(values), m.form, m.status)
	}
	for _, value := range values {
		press(t, m, "ctrl+u")
		typeText(m, value)
		press(t, m, "enter")
	}
	press(t, m, "enter")
	if m.form != nil {
		t.Fatalf("form did not save: %s", m.status)
	}
}

func createWorkspace(t *testing.T, m *model, name string) string {
	t.Helper()
	if len(m.workspaces) == 0 {
		press(t, m, "enter")
	} else {
		mainCommand(t, m, "new")
	}
	completeForm(t, m, name)
	if m.current() == nil || m.current().Name != name {
		t.Fatalf("workspace %q not created: %s", name, m.status)
	}
	return m.active
}

func selectTree(t *testing.T, m *model, key string) {
	t.Helper()
	focusPanel(t, m, 0)
	press(t, m, "home")
	for _, row := range m.treeRows() {
		if row.key == key {
			press(t, m, "enter")
			return
		}
		press(t, m, "down")
	}
	t.Fatalf("tree entry %q not found", key)
}

func openTool(t *testing.T, m *model, tool string) {
	t.Helper()
	mainCommand(t, m, "settings")
	press(t, m, "down")
	press(t, m, "enter")
	for _, name := range toolTypes {
		if name == tool {
			press(t, m, "enter")
			return
		}
		press(t, m, "down")
	}
	t.Fatalf("tool %q not available", tool)
}

func addKubernetes(t *testing.T, m *model) []workspace.Connection {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	data := `apiVersion: v1
kind: Config
contexts:
  - name: development
    context: {cluster: dev, user: developer, namespace: sandbox}
  - name: production
    context: {cluster: prod, user: operator, namespace: payments}
users:
  - name: developer
    user: {token: SECRET_MUST_STAY_IN_KUBECONFIG}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	openTool(t, m, "kubernetes")
	completeForm(t, m, path)
	if m.page != "contexts" || len(m.contexts) != 2 {
		t.Fatalf("expected context picker, got %q: %s", m.page, m.status)
	}
	press(t, m, "space")
	press(t, m, "down")
	press(t, m, "space")
	press(t, m, "enter")
	connections := m.current().Connections
	if len(connections) != 2 {
		t.Fatalf("wanted two connections, got %d: %s", len(connections), m.status)
	}
	for _, c := range connections {
		if c.ID == "" || c.Tool != "kubernetes" || c.Settings["kubeconfig"] != path || c.Settings["context"] != c.Name {
			t.Fatalf("context reference not saved correctly: %#v", c)
		}
	}
	return append([]workspace.Connection(nil), connections...)
}

func TestWorkspaceCreationAndMultipleConnections(t *testing.T) {
	m, _ := testModel(t)
	if !strings.Contains(m.View().Content, "Create your first") {
		t.Fatal("fresh launch must offer workspace creation")
	}
	first := createWorkspace(t, m, "Payments")
	connections := addKubernetes(t, m)
	if connections[0].ID == connections[1].ID {
		t.Fatal("connections need distinct identities")
	}
	second := createWorkspace(t, m, "Platform")
	if first == second || len(m.workspaces) != 2 || len(m.current().Connections) != 0 {
		t.Fatal("second empty workspace must coexist with first")
	}
	mainCommand(t, m, "4")
	selectTree(t, m, connections[1].ID)
	if m.active != first || m.connection != connections[1].ID || m.tool != "kubernetes" || m.mode != 3 || m.focus != 0 {
		t.Fatal("tree selection should change scope, preserving mode and tree focus")
	}
	selectTree(t, m, second)
	press(t, m, "left")
	if m.expanded[second] {
		t.Fatal("left should collapse a workspace")
	}
	press(t, m, "right")
	if !m.expanded[second] || m.active != second {
		t.Fatal("right should expand without changing scope")
	}
	history, err := m.store.History(first)
	if err != nil || len(history) != 2 {
		t.Fatalf("creation and adding contexts need two changes: %#v, %v", history, err)
	}
	serialized, err := json.Marshal(history)
	if err != nil || strings.Contains(string(serialized), "SECRET_MUST_STAY_IN_KUBECONFIG") {
		t.Fatal("history must not copy credential values")
	}
}

func TestFocusedInputKeepsTextAndPasteLiteral(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Payments")
	for _, mode := range []int{4, 2, 3, 1} {
		mainCommand(t, m, fmt.Sprint(mode))
		if m.mode != mode-1 || m.focus != 1 || m.active != id {
			t.Fatalf(":%d did not change only the mode", mode)
		}
	}
	typeText(m, "1234!@#$qjk")
	if m.mode != 0 || m.command != nil {
		t.Fatal("main navigation must not retain the old number or shifted mode shortcuts")
	}
	press(t, m, "shift+tab")
	if m.focus != 0 {
		t.Fatal("Shift+Tab must move focus backward")
	}
	focusPanel(t, m, 0)
	typeText(m, "1234!@#$:4qjk")
	if m.focus != 0 || m.mode != 0 || m.command != nil {
		t.Fatal("tree text must not invoke main commands")
	}
	focusPanel(t, m, 2)
	typeText(m, ":1qjk!")
	press(t, m, "enter")
	m.Update(tea.PasteMsg{Content: ":quit\n:4\n/etc/kubernetes"})
	if m.draft().text != ":1qjk!\n:quit\n:4\n/etc/kubernetes" || m.mode != 0 || m.command != nil {
		t.Fatalf("chat must preserve literal text: %q", m.draft().text)
	}
	mainCommand(t, m, "new")
	typeText(m, ":1qjk!")
	m.Update(tea.PasteMsg{Content: " :quit"})
	form := m.form
	press(t, m, "tab")
	if m.focus != 2 || m.form != form || form.fields[0].value.text != ":1qjk! :quit" {
		t.Fatal("Tab must preserve form values")
	}
	press(t, m, "tab")
	press(t, m, "enter")
	if m.form != form || m.active != id {
		t.Fatal("tree activation must not discard an unfinished form")
	}
	press(t, m, "tab")
	press(t, m, "enter")
	press(t, m, "enter")
	if m.current().Name != ":1qjk! :quit" || m.mode != 0 {
		t.Fatal("form punctuation and raw letters must stay literal")
	}
	press(t, m, "esc")
	m.Update(tea.PasteMsg{Content: ":4\n:quit\n"})
	if m.command != nil || m.mode != 0 {
		t.Fatal("pasted navigation text must not execute")
	}
	if cmd := press(t, m, "esc"); cmd != nil {
		t.Fatal("cancel must not unexpectedly quit")
	}
	cmd := mainCommand(t, m, "quit")
	if cmd == nil {
		t.Fatal("explicit main-panel :quit must exit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal(":quit did not request exit")
	}
}

func TestModePositionsAndToolCommandScope(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	connections := addKubernetes(t, m)
	mainCommand(t, m, "4")
	press(t, m, "down")
	activityKey := m.viewKey()
	mainCommand(t, m, "2")
	selectTree(t, m, connections[0].ID)
	mainCommand(t, m, "pod")
	if !strings.Contains(m.status, "not implemented") {
		t.Fatalf("Kubernetes Resources should explain its unavailable command: %s", m.status)
	}
	mainCommand(t, m, "1")
	mainCommand(t, m, "pod")
	if !strings.Contains(m.status, "Unknown command") {
		t.Fatalf(":pod should have no meaning in Overview: %s", m.status)
	}
	selectTree(t, m, m.active)
	mainCommand(t, m, "4")
	if m.viewKey() != activityKey || m.positions[m.viewKey()] != 1 {
		t.Fatal("switching modes and scope lost the previous Activity selection")
	}
	press(t, m, "enter")
	if !strings.Contains(m.detail, "Workspace created") {
		t.Fatal("restored Activity selection opened the wrong revision")
	}
}

func TestUndersizedTerminalDoesNotTriggerHiddenActions(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "new")
	typeText(m, "Unfinished")
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	typeText(m, "ignored")
	m.Update(tea.PasteMsg{Content: ":quit\n"})
	press(t, m, "enter")
	if len(m.workspaces) != 1 || m.form == nil || m.form.fields[0].value.text != "Unfinished" {
		t.Fatal("undersized layout must not accept unseen edits or form actions")
	}
	mainCommand(t, m, "new")
	if len(m.workspaces) != 1 || m.form.fields[0].value.text != "Unfinished" {
		t.Fatal("undersized layout must not execute configuration commands")
	}
	cmd := mainCommand(t, m, "quit")
	if cmd == nil {
		t.Fatal("must still be able to exit from an undersized terminal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("undersized :quit did not exit")
	}
}

func TestConnectionSettingsAndActivityKeepExactHistory(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Payments")
	connections := addKubernetes(t, m)
	mainCommand(t, m, "settings")
	press(t, m, "enter")
	completeForm(t, m, "Payments renamed")
	if m.active != id {
		t.Fatal("rename must preserve workspace identity")
	}
	mainCommand(t, m, "settings")
	press(t, m, "down")
	press(t, m, "down")
	press(t, m, "enter")
	completeForm(t, m, "Development renamed", connections[0].Settings["kubeconfig"], "new-context", "new-namespace")
	if got := m.current().Connections[0]; got.ID != connections[0].ID || got.Settings["context"] != "new-context" {
		t.Fatalf("editing lost identity/settings: %#v", got)
	}
	openRemove := func() {
		mainCommand(t, m, "settings")
		press(t, m, "down")
		press(t, m, "down")
		press(t, m, "enter")
		for i := 0; i < len(m.form.fields)+2; i++ {
			press(t, m, "down")
		}
		press(t, m, "enter")
		if m.page != "remove" {
			t.Fatal("removing should ask for confirmation")
		}
	}
	connectionBefore := m.connection
	openRemove()
	press(t, m, "enter") // Default choice is Cancel.
	if len(m.current().Connections) != 2 || m.connection != connectionBefore {
		t.Fatal("canceled removal changed configuration or scope")
	}
	openRemove()
	press(t, m, "down")
	press(t, m, "enter")
	if len(m.current().Connections) != 1 || m.current().Connections[0].ID != connections[1].ID {
		t.Fatal("confirmed removal should remove only chosen connection")
	}
	history, err := m.store.History(id)
	if err != nil || len(history) != 5 {
		t.Fatalf("wanted five committed changes, got %d: %v", len(history), err)
	}
	if history[2].Before.Name != "Payments" || history[2].After.Name != "Payments renamed" ||
		history[3].Before.Connections[0].Settings["context"] != "development" || history[3].After.Connections[0].Settings["context"] != "new-context" ||
		len(history[4].Before.Connections) != 2 || len(history[4].After.Connections) != 1 {
		t.Fatal("history lost exact before/after settings")
	}
	mainCommand(t, m, "4")
	press(t, m, "enter")
	if !strings.Contains(m.detail, "BEFORE") || !strings.Contains(m.detail, "AFTER") || !strings.Contains(m.detail, connections[0].ID) {
		t.Fatal("Activity must expose recorded change details")
	}
}

func TestOtherToolsCanBeConfiguredThroughSettings(t *testing.T) {
	cases := []struct {
		tool       string
		values     []string
		key, value string
	}{
		{"helm", []string{"Charts", "", "local-cluster", "apps"}, "context", "local-cluster"},
		{"kustomize", []string{"Overlay", "./overlays/dev"}, "path", "./overlays/dev"},
		{"terraform", []string{"Infrastructure", "./infra", "dev"}, "workspace", "dev"},
		{"argo", []string{"GitOps", "git@github.com:example/apps.git", "argocd.example", "main", "apps"}, "repo", "git@github.com:example/apps.git"},
		{"kargo", []string{"Promotions", "", "local-cluster", "delivery"}, "namespace", "delivery"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			m, _ := testModel(t)
			createWorkspace(t, m, "Workspace")
			openTool(t, m, tc.tool)
			completeForm(t, m, tc.values...)
			if len(m.current().Connections) != 1 {
				t.Fatalf("connection not created: %s", m.status)
			}
			c := m.current().Connections[0]
			if c.Tool != tc.tool || c.Name != tc.values[0] || c.Settings[tc.key] != tc.value {
				t.Fatalf("wrong saved connection: %#v", c)
			}
		})
	}
}

func TestRestartRestoresScopeModeAndSeparateDrafts(t *testing.T) {
	m, root := testModel(t)
	first := createWorkspace(t, m, "Payments")
	connections := addKubernetes(t, m)
	focusPanel(t, m, 2)
	typeText(m, "Payments draft: :4 is text")
	second := createWorkspace(t, m, "Platform")
	focusPanel(t, m, 2)
	typeText(m, "Platform draft")
	mainCommand(t, m, "4")
	selectTree(t, m, connections[0].ID)
	focusPanel(t, m, 1)
	press(t, m, "down")
	focusPanel(t, m, 2)
	m.Update(autosaveMsg{})
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := newModel(store, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if restored.active != first || restored.connection != connections[0].ID || restored.tool != "kubernetes" || restored.mode != 3 || restored.focus != 2 {
		t.Fatal("restart lost selected scope, mode, or panel")
	}
	if restored.draft().text != "Payments draft: :4 is text" || restored.positions[restored.viewKey()] != 1 {
		t.Fatal("restart lost draft or Activity position")
	}
	selectTree(t, restored, second)
	if restored.draft().text != "Platform draft" {
		t.Fatal("workspace drafts must remain separate after restart")
	}
}

func TestLayoutFitsTerminalAcrossWorkspaceViews(t *testing.T) {
	m, _ := testModel(t)
	states := []struct {
		name  string
		setup func()
	}{
		{"first launch", func() {}},
		{"creation form", func() { press(t, m, "enter") }},
		{"settings", func() { completeForm(t, m, "Payments 日本語") }},
		{"tool picker", func() { press(t, m, "enter") }},
		{"kubeconfig form", func() { press(t, m, "enter") }},
		{"activity", func() { press(t, m, "esc"); mainCommand(t, m, "4") }},
		{"change detail", func() { press(t, m, "enter") }},
		{"chat editing", func() { focusPanel(t, m, 2); m.Update(tea.PasteMsg{Content: strings.Repeat("hello 界 🌍 ", 40)}) }},
	}
	for _, state := range states {
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		state.setup()
		for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}, {79, 24}, {120, 15}, {10, 3}, {1, 1}, {0, 0}} {
			t.Run(fmt.Sprintf("%s/%dx%d", state.name, size[0], size[1]), func(t *testing.T) {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				view := m.View()
				if !view.AltScreen {
					t.Fatal("must use alternate screen")
				}
				if size[0] == 0 {
					if view.Content != "" {
						t.Fatal("must wait for terminal size")
					}
					return
				}
				if w, h := lipgloss.Size(view.Content); w != size[0] || h != size[1] {
					t.Fatalf("layout is %dx%d, terminal is %dx%d", w, h, size[0], size[1])
				}
				if size[0] >= minWidth && size[1] >= minHeight {
					text := ansi.Strip(view.Content)
					lines := strings.Split(text, "\n")
					for row, line := range lines {
						if lipgloss.Width(line) != size[0] {
							t.Errorf("row %d does not fill terminal width", row)
						}
					}
					// Adjacent borders must stay adjacent; their outside edges must
					// meet the terminal edges even with wide Unicode draft text.
					for _, line := range lines[2 : size[1]-2] {
						if !strings.HasPrefix(line, "│") || !strings.HasSuffix(line, "│") || strings.Count(line, "││") != 2 {
							t.Errorf("panel borders lost alignment: %q", line)
						}
					}
					for _, title := range []string{"Workspaces", "AI assistant"} {
						if !strings.Contains(text, title) {
							t.Errorf("missing panel title %q", title)
						}
					}
				}
			})
		}
	}
}
