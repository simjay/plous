package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testModel(t *testing.T) (*model, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".plous")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	m, err := newModel(store, openTestHistory(t, store), "test-version")
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	t.Cleanup(func() { m.quitting = true; m.drainPersistence() })
	return m, root
}

func press(t *testing.T, m *model, key string) tea.Cmd {
	t.Helper()
	keys := map[string]tea.KeyPressMsg{
		"backspace": {Code: tea.KeyBackspace},
		"enter":     {Code: tea.KeyEnter}, "shift+enter": {Code: tea.KeyEnter, Mod: tea.ModShift}, "ctrl+j": {Code: 'j', Mod: tea.ModCtrl}, "ctrl+n": {Code: 'n', Mod: tea.ModCtrl}, "ctrl+o": {Code: 'o', Mod: tea.ModCtrl}, "tab": {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"alt+left":  {Code: tea.KeyLeft, Mod: tea.ModAlt},
		"alt+right": {Code: tea.KeyRight, Mod: tea.ModAlt},
		"esc":       {Code: tea.KeyEscape}, "up": {Code: tea.KeyUp},
		"down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft},
		"right": {Code: tea.KeyRight}, "home": {Code: tea.KeyHome},
		"end": {Code: tea.KeyEnd}, "space": {Code: tea.KeySpace, Text: " "},
		"ctrl+]": {Code: ']', Mod: tea.ModCtrl},
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
	// Leave the Settings menu before exercising the underlying panels.
	if m.settingsOverlay != nil && m.focus == 1 && panel != 1 && m.form == nil && m.help == nil && m.command == nil {
		press(t, m, "esc")
	}
	for i := 0; i < 3 && m.focus != panel; i++ {
		press(t, m, "alt+right")
	}
	if m.focus != panel {
		t.Fatalf("could not focus panel %d", panel)
	}
}

func mainCommand(t *testing.T, m *model, command string) tea.Cmd {
	t.Helper()
	if m.help == nil {
		focusPanel(t, m, 1)
	}
	typeText(m, ":"+command)
	cmd := press(t, m, "enter")
	if m.mode == 2 && m.page == "" && m.form == nil {
		completeActivity(t, m)
	}
	return cmd
}

func completeForm(t *testing.T, m *model, values ...string) {
	t.Helper()
	if m.form == nil || len(m.form.Fields) != len(values) {
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
	mainCommand(t, m, "new")
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
			if m.mode == 2 && m.page == "" && m.form == nil {
				completeActivity(t, m)
			}
			return
		}
		press(t, m, "down")
	}
	t.Fatalf("tree entry %q not found", key)
}

func openTool(t *testing.T, m *model, tool string) {
	t.Helper()
	openSettingsItem(t, m, "Add Connection")
	// Legacy fixtures exercise persisted Kustomize records, which are still
	// supported even though new setup creates general Code connections.
	if tool == "kustomize" {
		m.beginConnection(tool, nil)
		return
	}
	for _, name := range appsettings.ToolTypes {
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
	path = installDefaultKubeconfig(t, path)
	openTool(t, m, "kubernetes")
	if m.page != "contexts" || len(m.contexts) != 2 {
		t.Fatalf("expected context picker, got %q: %s", m.page, m.status)
	}
	press(t, m, "space")
	press(t, m, "down")
	press(t, m, "space")
	press(t, m, "down")
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
	if !strings.Contains(m.View().Content, "Current Workspace") {
		t.Fatal("fresh launch must offer Current Workspace")
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
	mainCommand(t, m, "3")
	selectTree(t, m, connections[1].ID)
	if m.active != first || m.connection != connections[1].ID || m.tool != "kubernetes" || m.mode != 2 || m.focus != 0 {
		t.Fatal("tree selection should change scope, preserving mode and tree focus")
	}
	selectTree(t, m, second)
	press(t, m, "right")
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
	for _, mode := range []int{3, 2, 1} {
		mainCommand(t, m, fmt.Sprint(mode))
		if m.mode != mode-1 || m.focus != 1 || m.active != id {
			t.Fatalf(":%d did not change only the mode", mode)
		}
	}
	typeText(m, "1234!@#$qjk")
	if m.mode != 0 || m.command != nil {
		t.Fatal("main navigation must not retain the old number or shifted mode shortcuts")
	}
	press(t, m, "alt+left")
	if m.focus != 0 {
		t.Fatal("Alt+Left must move focus backward")
	}
	focusPanel(t, m, 0)
	typeText(m, "1234!@#$qjk")
	if m.focus != 0 || m.mode != 0 || m.command != nil {
		t.Fatal("tree text must not invoke main commands")
	}
	startChat(t, m)
	typeText(m, ":1qjk!")
	press(t, m, "shift+enter")
	m.Update(tea.PasteMsg{Content: ":quit\n:4\n/etc/kubernetes"})
	if m.draft().Text != ":1qjk!\n:quit\n:4\n/etc/kubernetes" || m.mode != 0 || m.command != nil {
		t.Fatalf("chat must preserve literal text: %q", m.draft().Text)
	}
	mainCommand(t, m, "new")
	typeText(m, ":1qjk!")
	m.Update(tea.PasteMsg{Content: " :quit"})
	form := m.form
	press(t, m, "tab")
	press(t, m, "shift+tab")
	if m.focus != 1 || m.form != form || form.Fields[0].Value.Text != ":1qjk! :quit" {
		t.Fatal("Tab must keep form focus and values")
	}
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
	mainCommand(t, m, "quit")
	m.drainPersistence()
	cmd := m.syncQuit()
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
	mainCommand(t, m, "3")
	press(t, m, "down")
	activityKey := m.viewKey()
	mainCommand(t, m, "2")
	selectTree(t, m, connections[0].ID)
	mainCommand(t, m, "pod")
	if m.explore == nil || m.explore.Preference.Resource != "pods" {
		t.Fatalf("Kubernetes Explore should select Pods: %s", m.status)
	}
	mainCommand(t, m, "1")
	mainCommand(t, m, "pod")
	if m.mode != 1 || m.explore.Preference.Resource != "pods" {
		t.Fatalf(":pod should open Kubernetes Explore from Overview: %s", m.status)
	}
	selectTree(t, m, m.active)
	mainCommand(t, m, "3")
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
	if len(m.workspaces) != 1 || m.form == nil || m.form.Fields[0].Value.Text != "Unfinished" {
		t.Fatal("undersized layout must not accept unseen edits or form actions")
	}
	mainCommand(t, m, "new")
	if len(m.workspaces) != 1 || m.form.Fields[0].Value.Text != "Unfinished" {
		t.Fatal("undersized layout must not execute configuration commands")
	}
	mainCommand(t, m, "quit")
	m.drainPersistence()
	cmd := m.syncQuit()
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
	openSettingsItem(t, m, "Rename Workspace")
	completeForm(t, m, "Payments renamed")
	if m.active != id {
		t.Fatal("rename must preserve workspace identity")
	}
	openSettingsItem(t, m, "Edit Kubernetes / "+connections[0].Name)
	completeForm(t, m, "Development renamed", connections[0].Settings["kubeconfig"], "new-context", "new-namespace", "secret", "", "", "")
	if got := m.current().Connections[0]; got.ID != connections[0].ID || got.Settings["context"] != "new-context" {
		t.Fatalf("editing lost identity/settings: %#v", got)
	}
	openRemove := func() {
		openSettingsItem(t, m, "Edit Kubernetes / Development renamed")
		for i := 0; i < len(m.form.Fields)+2; i++ {
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
	mainCommand(t, m, "3")
	press(t, m, "enter")
	detail := ansi.Strip(m.detail)
	if !strings.Contains(detail, "Removed connection") || !strings.Contains(detail, "Development renamed") || !strings.Contains(detail, "new-context") || !strings.Contains(detail, "new-namespace") {
		t.Fatal("Activity must describe the removed connection and its saved settings")
	}
}

func TestLocalSourceCanBeConfiguredThroughSettings(t *testing.T) {
	directory := t.TempDir()
	m, _ := testModel(t)
	createWorkspace(t, m, "Workspace")
	openTool(t, m, "code")
	completeForm(t, m, "Source", "", directory)
	if len(m.current().Connections) != 1 {
		t.Fatalf("source not created: %s", m.status)
	}
	c := m.current().Connections[0]
	if c.Tool != "code" || c.Name != "Source" || c.Settings["path"] != directory {
		t.Fatalf("wrong saved source: %#v", c)
	}
}

func TestRestartPreservesConnectionAndGeneralDrafts(t *testing.T) {
	m, root := testModel(t)
	first := createWorkspace(t, m, "Payments")
	connections := addKubernetes(t, m)
	selectTree(t, m, connections[0].ID)
	startChat(t, m)
	typeText(m, "Payments draft: :4 is text")
	second := createWorkspace(t, m, "Platform")
	startChat(t, m)
	typeText(m, "Platform draft")
	mainCommand(t, m, "3")
	selectTree(t, m, connections[0].ID)
	focusPanel(t, m, 1)
	press(t, m, "down")
	focusPanel(t, m, 2)
	m.Update(autosaveMsg{})
	m.drainPersistence()
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := newModel(store, openTestHistory(t, store), "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if restored.active != "" || !restored.currentRoot || restored.connection != "" || restored.tool != "" || restored.mode != 0 || restored.focus != 1 {
		t.Fatal("restart did not select Current Workspace")
	}
	restored.selectScope(first, "kubernetes", connections[0].ID)
	if restored.draft().Text != "Payments draft: :4 is text" || restored.positions[first+"/kubernetes/"+connections[0].ID+"/Activity"] != 1 {
		t.Fatal("restart lost draft or Activity position")
	}
	selectTree(t, restored, second)
	if restored.draft().Text != "Platform draft" {
		t.Fatal("connection and general drafts must remain separate after restart")
	}
}

func TestSavedModesStartAtCurrentContextAndKeepLegacyState(t *testing.T) {
	for _, tc := range []struct {
		version, before int
	}{
		{1, 0}, {1, 1}, {1, 2}, {1, 3},
		{2, 0}, {2, 1}, {2, 2},
	} {
		t.Run(fmt.Sprintf("v%d/mode%d", tc.version, tc.before), func(t *testing.T) {
			m, _ := testModel(t)
			id := createWorkspace(t, m, "Payments")
			exploreKey, activityKey := id+"///Explore", id+"///Activity"
			previousKey := exploreKey
			if tc.version == 1 {
				previousKey = id + "///Resources"
			}
			saved := session{
				Version: tc.version, Workspace: id, Mode: tc.before, Focus: 2,
				Drafts:    map[string]string{id: "Keep this draft"},
				Positions: map[string]int{previousKey: 7, activityKey: 1},
			}
			data, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.store.WriteSession(data); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test-version")
			if err != nil {
				t.Fatal(err)
			}
			for restart := 0; restart < 2; restart++ {
				if restored.mode != 0 || restored.active != "" || !restored.currentRoot || restored.focus != 1 || restored.drafts[id].Text != "Keep this draft" {
					t.Fatalf("restart %d lost saved navigation or draft: mode=%d", restart, restored.mode)
				}
				if restored.positions[exploreKey] != 7 || restored.positions[activityKey] != 1 {
					t.Fatalf("restart %d lost saved view positions: %v", restart, restored.positions)
				}
				if err := restored.saveSession(); err != nil {
					t.Fatal(err)
				}
				restored, err = newModel(m.store, m.history, "test-version")
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLayoutFitsTerminalAcrossWorkspaceViews(t *testing.T) {
	m, _ := testModel(t)
	states := []struct {
		name  string
		setup func()
	}{
		{"first launch", func() {}},
		{"creation form", func() { mainCommand(t, m, "n") }},
		{"tool picker", func() { completeForm(t, m, "Payments 日本語") }},
		{"context picker", func() { press(t, m, "enter") }},
		{"kubeconfig editing", func() { press(t, m, "home"); typeText(m, "-edited") }},
		{"activity", func() { press(t, m, "esc"); mainCommand(t, m, "3") }},
		{"change detail", func() { press(t, m, "enter") }},
		{"chat editing", func() { startChat(t, m); m.Update(tea.PasteMsg{Content: strings.Repeat("hello 界 🌍 ", 40)}) }},
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
					for row, line := range lines[2 : size[1]-2] {
						if !strings.HasPrefix(line, "│") || !strings.HasSuffix(line, "│") || (row != 1 && m.settingsOverlay == nil && strings.Count(line, "││") != 2) {
							t.Errorf("panel borders lost alignment: %q", line)
						}
					}
					for _, title := range []string{"Connections", "AI assistant"} {
						if m.settingsOverlay == nil && !strings.Contains(text, title) {
							t.Errorf("missing panel title %q", title)
						}
					}
				}
			})
		}
	}
}

func openSettingsItem(t *testing.T, m *model, label string) {
	t.Helper()
	mainCommand(t, m, "settings")
	for _, item := range m.settingsRows() {
		if item.Label == label {
			press(t, m, "enter")
			return
		}
		press(t, m, "down")
	}
	t.Fatalf("settings item %q missing", label)
}

func openProviderItem(t *testing.T, m *model, label string) {
	t.Helper()
	openSettingsItem(t, m, "Agent: "+m.settings.MainAgent.Effective().Label())
	for m.form.Fields[0].Value.Text != "Plous" {
		press(t, m, "right")
	}
	for i, row := range m.form.Rows {
		if row.Label == label {
			m.form.Index = len(m.form.Fields) + i
			press(t, m, "enter")
			return
		}
	}
	t.Fatalf("provider item %q missing", label)
}

func startChat(t *testing.T, m *model) {
	t.Helper()
	if m.focus == 2 && m.chatEditing {
		press(t, m, "esc")
	}
	typeText(m, ":chat")
	press(t, m, "enter")
	if m.focus != 2 || !m.chatEditing {
		t.Fatalf("could not enter chat input: %s", m.status)
	}
}

func openTestHistory(t *testing.T, profile *workspace.Store) *history.Store {
	t.Helper()
	archive, err := history.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func (m *model) selectSettingsItem(label string) {
	for _, row := range m.settingsRows() {
		if row.Label == label {
			m.selectSettingsRow(row.ID)
			return
		}
	}
}
