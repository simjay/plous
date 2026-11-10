package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func completeAIForm(t *testing.T, m *model, values map[string]string) {
	t.Helper()
	setField := func(key, value string) {
		t.Helper()
		for i, field := range m.form.Fields {
			if field.Key != key {
				continue
			}
			m.form.Index = i
			if len(field.Options) > 0 {
				for range len(field.Options) {
					if m.form.Fields[i].Value.Text == value {
						return
					}
					press(t, m, "right")
				}
				t.Fatalf("unknown option %q for %s", value, key)
			}
			press(t, m, "ctrl+u")
			typeText(m, value)
			return
		}
		t.Fatalf("field %q missing", key)
	}
	if provider, ok := values["provider"]; ok {
		setField("provider", provider)
	}
	if _, ok := values["api_key"]; !ok && m.form.Fields[1].Required {
		setField("api_key", "test-api-key")
	}
	m.form.Index = len(m.form.Fields) + 2
	press(t, m, "enter")
	for key, value := range values {
		if key != "provider" {
			setField(key, value)
		}
	}
	m.form.Index = len(m.form.Fields)
	press(t, m, "enter")
	if m.form == nil || m.form.Kind != "mainagent" {
		t.Fatalf("provider form did not return to Agent: %s", m.status)
	}
	press(t, m, "esc")
}

func TestAISetupDefaultsAndAdvancedPreservation(t *testing.T) {
	m, _ := testModel(t)
	m.beginAppSettings()
	if len(m.form.Fields) != 2 {
		t.Fatal("initial setup should ask only for vendor and key")
	}
	m.Update(tea.PasteMsg{Content: "invalid-provider"})
	typeText(m, "invalid-provider")
	press(t, m, "right")
	if m.form.Fields[0].Value.Text != "anthropic" || m.form.Agent.KeyEnv != "" {
		t.Fatal("provider picker reused external credentials")
	}
	completeAIForm(t, m, map[string]string{"api_key": "test-key"})
	if m.form != nil || m.settings.Agent.Name != "Anthropic" {
		t.Fatal("simple setup did not save sensible defaults")
	}
	m.beginAISettings(0)
	completeAIForm(t, m, map[string]string{"name": "Custom", "key_env": "CUSTOM_KEY", "endpoint": "https://example.com", "instructions": "Brief answers"})
	next := m.settings
	next.Providers = []ai.Config{next.Agent}
	if !m.saveAISettings(next) {
		t.Fatal(m.status)
	}
	before := m.settings.Agent
	m.beginAISettings(0)
	m.form.Index = 2
	press(t, m, "enter")
	if m.settings.Agent != before {
		t.Fatal("basic edit changed instructions or credentials")
	}

	m.beginAppSettings()
	completeAIForm(t, m, map[string]string{"provider": "openai", "endpoint": "http://localhost:1234/v1"})
	config := m.settings.Configs()[1]
	if config.KeyEnv != "" || config.Endpoint != "http://localhost:1234/v1" {
		t.Fatal("custom endpoint defaults incorrect")
	}
}

func TestHelpTabCannotChangeSectionsOrUnderlyingFocus(t *testing.T) {
	m, _ := testModel(t)
	m.focus = 2
	typeText(m, ":h")
	press(t, m, "enter")
	for _, size := range [][2]int{{120, 30}, {60, 10}, {80, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, key := range []string{"tab", "shift+tab"} {
			press(t, m, key)
			if m.focus != 2 || m.help.section != 2 {
				t.Fatal("Tab changed Help focus")
			}
		}
	}
	press(t, m, "left")
	press(t, m, "end")
	if m.focus != 2 || m.help.section != 1 || m.help.scroll[1] == 0 {
		t.Fatal("Help navigation must stay inside the overlay")
	}
	press(t, m, "esc")
	if m.focus != 2 || m.help != nil {
		t.Fatal("Help did not restore original focus")
	}
}

func TestOneSettingsEntryAndAllSpellings(t *testing.T) {
	m, _ := testModel(t)
	for _, withWorkspace := range []bool{false, true} {
		if withWorkspace {
			createWorkspace(t, m, "Payments")
			addKubernetes(t, m)
		}
		for _, command := range []string{"s", "settings"} {
			mainCommand(t, m, command)
			if m.page != "settings" || m.form != nil {
				t.Fatalf("%s did not open Settings", command)
			}
			content := ansi.Strip(m.render())
			scope := "Current Workspace"
			if withWorkspace {
				scope = "Current Workspace"
			}
			for _, label := range []string{"Settings", "Application", "Shared across workspaces", scope} {
				if !strings.Contains(content, label) {
					t.Fatalf("missing %q", label)
				}
			}
		}
		count := 0
		for _, row := range m.treeRows() {
			if row.kind == "settings" {
				count++
				if row.label != "Settings" || row.workspace != "" {
					t.Fatal("workspace-specific settings entry remains")
				}
			}
			if row.kind == "config" {
				t.Fatal("separate configuration entry remains")
			}
		}
		if count != 1 {
			t.Fatalf("wanted one Settings entry, got %d", count)
		}
		active, connection, tool, mode := m.active, m.connection, m.tool, m.mode
		selectTree(t, m, "settings")
		if m.page != "settings" || m.focus != 1 || m.active != active || m.connection != connection || m.tool != tool || m.mode != mode {
			t.Fatal("opening Settings changed the selection")
		}
		press(t, m, "esc")
	}
}

func TestSettingsFormsReturnToSamePage(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	for _, item := range []string{"Add provider connection", "Rename Workspace"} {
		for _, action := range []string{"esc", "cancel", "save"} {
			if item == "Add provider connection" {
				openProviderItem(t, m, item)
			} else {
				openSettingsItem(t, m, item)
			}
			form := m.form
			press(t, m, "tab")
			press(t, m, "shift+tab")
			if m.form != form || m.focus != 1 {
				t.Fatal("modal focus escaped")
			}
			switch action {
			case "esc":
				press(t, m, "esc")
			case "cancel":
				for i := 0; i < len(form.Fields)+1; i++ {
					press(t, m, "down")
				}
				press(t, m, "enter")
			case "save":
				if form.Kind == "appsettings" {
					form.Fields[1].Value = textview.NewField("test-key")
				}
				for range form.Fields {
					press(t, m, "down")
				}
				press(t, m, "enter")
			}
			if item == "Add provider connection" {
				if m.form != form.Parent {
					t.Fatal("provider did not restore its Agent form")
				}
				press(t, m, "esc")
			}
			if m.form != nil || m.page != "settings" {
				t.Fatalf("%s %s did not return to Settings", item, action)
			}
		}
	}
	openSettingsItem(t, m, "Add Connection")
	press(t, m, "esc")
	if m.page != "settings" {
		t.Fatal("tool picker did not return to Settings")
	}
	mainCommand(t, m, "settings")
	press(t, m, "esc")
	if m.page != "" {
		t.Fatal("Esc did not close Settings")
	}
}

func TestSettingsCreationAndFirstRunHelp(t *testing.T) {
	m, _ := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	mainCommand(t, m, "help")
	for range 5 {
		press(t, m, "down")
	}
	if m.help == nil || m.help.scroll[1] == 0 {
		t.Fatal("help cannot scroll before creating a workspace")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mainCommand(t, m, "n")
	completeForm(t, m, "First")
	if m.page != "tools" || m.toolSetup == nil {
		t.Fatal("creation did not offer adding a tool")
	}
}

func TestRenderingDoesNotRecoverChatOnDisk(t *testing.T) {
	m, root := testModel(t)
	owner := createWorkspace(t, m, "Payments")
	id := selectChatConnection(t, m)
	c, err := m.history.NewConversation(id, "Interrupted chat")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.history.StartTurn(id, c.ID, history.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "connections", id, "conversations", c.ID, "conversation", "000001.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(m.chats, id)
	m.chatIDs[id] = c.ID
	for range 3 {
		m.View()
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("rendering modified persistent chat state", err)
	}
	if m.chats[id] != nil {
		t.Fatal("rendering loaded chat history")
	}
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	m.store, err = workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.store.Close() })
	m.history = openTestHistory(t, m.store)
	m.selectScope(owner, "files", m.connection)
	finishChatRestore(t, m)
	if m.chat().Turns[0].Response.Status != "interrupted" {
		t.Fatal("scope selection did not recover chat")
	}
}

func TestSettingsOverlayFitsTerminal(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.openSettings()
		for range len(m.settingsRows()) {
			lines := strings.Split(ansi.Strip(m.render()), "\n")
			if len(lines) != size[1] {
				t.Fatal("settings changed screen height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatal("settings overflowed a row")
				}
			}
			press(t, m, "down")
		}
	}
}

func TestSettingsOverlayRestoresViewAndTrapsTab(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.closeSettings()
	m.mode, m.detail, m.cursor, m.focus = 2, "Revision details", 2, 0
	typeText(m, "ss")
	if m.settingsOverlay == nil || m.focus != 1 {
		t.Fatal("Settings must open an overlay from any navigation panel")
	}
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "down")
	typeText(m, "unsaved-model")
	form := m.form
	for _, size := range [][2]int{{120, 30}, {60, 10}, {80, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, key := range []string{"tab", "shift+tab"} {
			press(t, m, key)
			if m.focus != 1 || m.form != form || form.Fields[1].Value.Text != "unsaved-model" || form.Index != 1 {
				t.Fatal("Tab must leave modal focus and input unchanged")
			}
		}
	}
	press(t, m, "esc")
	press(t, m, "esc")
	press(t, m, "esc")
	if m.settingsOverlay != nil || m.mode != 2 || m.detail != "Revision details" || m.cursor != 2 || m.focus != 0 {
		t.Fatal("closing Settings must restore the underlying view and focus")
	}
	if m.settings.Agent.Model == "unsaved-model" {
		t.Fatal("Esc saved unsaved edits")
	}
}

func TestOverlayKeepsEveryFieldAndActionReachable(t *testing.T) {
	m, _ := testModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.openSettings()
		m.beginAppSettings()
		for i, field := range m.form.Fields {
			m.form.Index = i
			screen := ansi.Strip(m.render())
			if !strings.Contains(screen, field.Label) || (!strings.Contains(screen, "▏") && len(field.Options) == 0) {
				t.Fatalf("%dx%d hides current field %q or its cursor", size[0], size[1], field.Label)
			}
		}
		m.form.Index = len(m.form.Fields)
		if !strings.Contains(ansi.Strip(m.render()), "› [ Save ]") {
			t.Fatal("Save action is unreachable")
		}
		m.beginCommand()
		if !strings.Contains(ansi.Strip(m.render()), ":▏") {
			t.Fatal("command box is hidden behind the overlay")
		}
		m.command, m.form = nil, nil
		m.closeSettings()
	}
}

func TestSelectingWorkspaceAfterClosingSettingsKeepsTreeFocus(t *testing.T) {
	m, _ := testModel(t)
	id := createWorkspace(t, m, "Payments")
	m.closeSettings()
	m.focus = 2
	typeText(m, "ss")
	if m.settingsOverlay == nil {
		t.Fatal("Settings did not open from chat navigation")
	}
	selectTree(t, m, id)
	if m.settingsOverlay != nil || m.active != id || m.focus != 0 {
		t.Fatal("selecting a workspace must dismiss suspended settings and retain tree focus")
	}
}

func contextFile(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	text := "apiVersion: v1\nkind: Config\nclusters:\n- name: example\n  cluster: {server: 'https://example.invalid'}\ncontexts:\n"
	for _, name := range names {
		text += fmt.Sprintf("- name: %s\n  context: {cluster: example, namespace: sandbox}\n", name)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func installDefaultKubeconfig(t *testing.T, source string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func editKubeconfigPath(t *testing.T, m *model, path string) {
	t.Helper()
	press(t, m, "home")
	press(t, m, "end")
	press(t, m, "ctrl+u")
	typeText(m, path)
}

func addSelectedContexts(t *testing.T, m *model) {
	t.Helper()
	// Leave the path editor before using End as a navigation key.
	if m.form.Index == 0 {
		press(t, m, "down")
	}
	press(t, m, "end")
	press(t, m, "up")
	press(t, m, "enter")
}

func TestWorkspaceToolEntryPointsAndAutomaticDiscovery(t *testing.T) {
	for _, entry := range []string{"creation", "overview", "tree", "settings"} {
		t.Run(entry, func(t *testing.T) {
			m, _ := testModel(t)
			path := installDefaultKubeconfig(t, contextFile(t, "dev", "prod"))
			t.Setenv("KUBECONFIG", contextFile(t, "not-the-selected-file"))
			id := createWorkspace(t, m, "Payments")
			if entry != "creation" {
				press(t, m, "esc")
				switch entry {
				case "overview":
					mainCommand(t, m, "1")
					press(t, m, "enter")
				case "tree":
					selectTree(t, m, id+"/add")
				case "settings":
					openSettingsItem(t, m, "Add Connection")
				}
			}
			if m.page != "tools" || m.toolSetup == nil {
				t.Fatal("entry did not open the shared tool picker")
			}
			press(t, m, "enter")
			if m.form == nil || m.form.Fields[0].Value.Text != "~/.kube/config" || m.page != "contexts" || len(m.contexts) != 2 || !m.contextsReady() {
				t.Fatal("Kubernetes must show the explicit default file and its contexts together")
			}
			press(t, m, "space")
			press(t, m, "down")
			press(t, m, "enter")
			if len(m.current().Connections) != 0 {
				t.Fatal("checking a context must not save before Add Selected Contexts")
			}
			addSelectedContexts(t, m)
			if m.settingsOverlay != nil || m.toolSetup != nil || m.form != nil || m.page != "" || len(m.current().Connections) != 2 || !m.expanded[id+"/monitor"] {
				t.Fatal("adding must return to the workspace with Kubernetes expanded")
			}
			for _, c := range m.current().Connections {
				if c.Settings["kubeconfig"] != path {
					t.Fatal("connection lost its discovered file reference")
				}
			}
		})
	}
}

func TestAddContextTargetsItsWorkspaceAndSupportsAnotherFile(t *testing.T) {
	m, _ := testModel(t)
	first := createWorkspace(t, m, "First")
	addKubernetes(t, m)
	second := createWorkspace(t, m, "Second")
	press(t, m, "esc")
	selectTree(t, m, first+"/monitor/add")
	if m.active != first || m.page != "contexts" || m.toolSetup == nil || !m.toolSetup.ContextsOnly {
		t.Fatal("tree action opened contexts for the wrong workspace")
	}
	press(t, m, "space")
	if m.chosen[0] || !strings.Contains(m.contextPickerBody(70, 20), "Already added") {
		t.Fatal("saved contexts must be marked and cannot be checked again")
	}
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 2 || m.page != "contexts" {
		t.Fatal("empty selection added duplicates")
	}
	path := contextFile(t, "another-context")
	editKubeconfigPath(t, m, path)
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "space")
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 3 || m.current().Connections[2].Settings["kubeconfig"] != path {
		t.Fatal("another kubeconfig must add to, not replace, existing connections")
	}
	selectTree(t, m, second)
	if len(m.current().Connections) != 0 {
		t.Fatal("adding to another workspace changed the active workspace's data")
	}
	press(t, m, "right")
	selectTree(t, m, second+"/add")
	if m.page != "tools" || m.active != second {
		t.Fatal("workspace Add Connection action did not target its workspace")
	}
}

func TestContextRefreshInvalidatesSelectionsAndRecoversFromErrors(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Empty")
	press(t, m, "enter")
	if len(m.contexts) != 0 || m.toolSetup.Err == "" || m.form.Index != 0 {
		t.Fatal("missing default config must show the file field and error")
	}
	valid := contextFile(t, "dev")
	editKubeconfigPath(t, m, valid)
	press(t, m, "enter")
	press(t, m, "enter")
	press(t, m, "space")
	if !m.chosen[0] {
		t.Fatal("valid file did not load")
	}
	bad := filepath.Join(t.TempDir(), "broken")
	if err := os.WriteFile(bad, []byte("contexts: ["), 0600); err != nil {
		t.Fatal(err)
	}
	editKubeconfigPath(t, m, bad)
	if len(m.contexts) != 0 || len(m.chosen) != 0 || m.contextsReady() || !strings.Contains(m.contextPickerBody(70, 20), "Refresh required") {
		t.Fatal("editing the file path must invalidate the previous checklist")
	}
	addSelectedContexts(t, m)
	if len(m.current().Connections) != 0 || !strings.Contains(m.status, "Refresh Contexts") {
		t.Fatal("stale contexts were added before refresh")
	}
	press(t, m, "home")
	press(t, m, "enter")
	press(t, m, "enter")
	if m.form == nil || !strings.Contains(m.toolSetup.Err, "invalid kubeconfig YAML") || len(m.contexts) != 0 || m.contextsReady() {
		t.Fatal("bad file should keep the integrated view open with no stale contexts")
	}
	for _, path := range []string{"", contextFile(t), filepath.Join(t.TempDir(), "missing")} {
		editKubeconfigPath(t, m, path)
		press(t, m, "enter")
		press(t, m, "enter")
		if m.toolSetup.Err == "" || m.contextsReady() {
			t.Fatalf("invalid input %q enabled adding", path)
		}
	}
	editKubeconfigPath(t, m, valid)
	press(t, m, "enter")
	press(t, m, "enter")
	if !m.contextsReady() || len(m.contexts) != 1 || len(m.chosen) != 0 {
		t.Fatal("successful refresh must reset the selection and clear errors")
	}
	press(t, m, "space")
	// Refresh the same path after its file changes externally.
	data, err := os.ReadFile(contextFile(t, "replacement"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, data, 0600); err != nil {
		t.Fatal(err)
	}
	press(t, m, "up")
	press(t, m, "enter")
	if len(m.contexts) != 1 || m.contexts[0].Name != "replacement" || len(m.chosen) != 0 {
		t.Fatal("refresh did not reread the file or reset selected contexts")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if m.settingsOverlay != nil || m.form != nil || len(m.current().Connections) != 0 {
		t.Fatal("Esc must close setup without adding connections")
	}
}

func TestContextPathInputGuardsAndExit(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	installDefaultKubeconfig(t, contextFile(t, "dev"))
	createWorkspace(t, m, "Local")
	press(t, m, "enter")
	press(t, m, "space")
	press(t, m, "home")
	typeText(m, ":q ss cc 11")
	m.Update(tea.PasteMsg{Content: " :quit hh\n"})
	if m.command != nil || m.chatEditing || m.help != nil || m.mode != mode || len(m.contexts) != 0 || len(m.chosen) != 0 || !strings.Contains(m.form.Fields[0].Value.Text, ":quit hh") {
		t.Fatal("path text and paste must stay literal and invalidate the checklist")
	}
	press(t, m, "down")
	form := m.form
	mainCommand(t, m, "3")
	if m.form != form || m.mode != mode {
		t.Fatal("mode command discarded unfinished setup")
	}
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if m.form != form {
		t.Fatal("Help lost the path editor")
	}
	mainCommand(t, m, "chat")
	typeText(m, "preserved draft")
	press(t, m, "esc")
	press(t, m, "alt+left")
	if m.form != form || m.focus != 1 {
		t.Fatal("chat did not preserve and resume setup")
	}
	assertCtrlCQuit(t, m)
	if len(m.current().Connections) != 0 {
		t.Fatal("exit applied unsaved selections")
	}
}

func TestContextPickerFitsSmallTerminalAndTrapsFocus(t *testing.T) {
	m, _ := testModel(t)
	installDefaultKubeconfig(t, contextFile(t, "development", "production", "staging", "test-4", "test-5", "test-6"))
	createWorkspace(t, m, "Local")
	press(t, m, "enter")
	for _, size := range [][2]int{{80, 16}, {120, 30}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(t, m, "home")
		for _, want := range []string{"~/.kube/config", "Refresh Contexts", "development", "production", "staging", "test-4", "test-5", "test-6", "Add Selected Contexts", "Cancel"} {
			press(t, m, "tab")
			press(t, m, "shift+tab")
			screen := ansi.Strip(m.render())
			if m.focus != 1 || !strings.Contains(screen, want) || len(strings.Split(screen, "\n")) != size[1] {
				t.Fatalf("%v hides selected action %q or moves focus\n%s", size, want, screen)
			}
			for _, fixed := range []string{"Kubeconfig File", "~/.kube/config", "Contexts", "Add Selected Contexts", "Cancel"} {
				if !strings.Contains(screen, fixed) {
					t.Fatalf("%v loses fixed section %q while scrolling\n%s", size, fixed, screen)
				}
			}
			press(t, m, "down")
		}
		typeText(m, ":")
		press(t, m, "tab")
		press(t, m, "shift+tab")
		if m.focus != 1 || !strings.Contains(ansi.Strip(m.render()), ":▏") {
			t.Fatal("command prompt must remain visible and modal")
		}
		press(t, m, "esc")
	}
}

func TestAddConnectionOnlyOffersImplementedTools(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Tools")
	m.beginToolSetup(false)
	screen := ansi.Strip(m.connectionPickerBody(100, 30))
	for _, name := range []string{"Kubernetes Cluster"} {
		if !strings.Contains(screen, name) {
			t.Fatalf("missing supported option %s", name)
		}
	}
	for _, name := range []string{"Source", "Code", "Terraform", "Argo", "Kargo", "Other Connections"} {
		if strings.Contains(screen, name) {
			t.Fatalf("unimplemented option remains: %s", name)
		}
	}
	if len(appsettings.ToolTypes) != 1 {
		t.Fatal("unsupported connection can still be selected")
	}
}
