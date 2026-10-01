package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

func completeAIForm(t *testing.T, m *model, values map[string]string) {
	t.Helper()
	setField := func(key, value string) {
		t.Helper()
		for i, field := range m.form.fields {
			if field.key != key {
				continue
			}
			m.form.index = i
			if len(field.options) > 0 {
				for range len(field.options) {
					if m.form.fields[i].value.text == value {
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
	if _, ok := values["api_key"]; !ok && m.form.fields[1].required {
		setField("api_key", "test-api-key")
	}
	m.form.index = len(m.form.fields) + 2
	press(t, m, "enter")
	for key, value := range values {
		if key != "provider" {
			setField(key, value)
		}
	}
	m.form.index = len(m.form.fields)
	press(t, m, "enter")
	if m.form != nil {
		t.Fatalf("AI form did not save: %s", m.status)
	}
}

func TestAISetupDefaultsAndAdvancedPreservation(t *testing.T) {
	m, _ := testModel(t)
	m.beginAppSettings()
	if len(m.form.fields) != 2 {
		t.Fatal("initial setup should ask only for vendor and key")
	}
	m.Update(tea.PasteMsg{Content: "invalid-provider"})
	typeText(m, "invalid-provider")
	press(t, m, "right")
	if m.form.fields[0].value.text != "anthropic" || m.form.agent.KeyEnv != "" {
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
	m.form.index = 2
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
			for _, label := range []string{"Settings", "Application", "AI connections", "Current workspace"} {
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
	for _, item := range []string{"Add AI connection", "Rename workspace"} {
		for _, action := range []string{"esc", "cancel", "save"} {
			openSettingsItem(t, m, item)
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
				for i := 0; i < len(form.fields)+1; i++ {
					press(t, m, "down")
				}
				press(t, m, "enter")
			case "save":
				if form.kind == "appsettings" {
					form.fields[1].value = newEditor("test-key")
				}
				for range form.fields {
					press(t, m, "down")
				}
				press(t, m, "enter")
			}
			if m.form != nil || m.page != "settings" {
				t.Fatalf("%s %s did not return to Settings", item, action)
			}
		}
	}
	openSettingsItem(t, m, "Add tool")
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
	openSettingsItem(t, m, "Create workspace")
	completeForm(t, m, "First")
	if m.page != "tools" || m.toolSetup == nil {
		t.Fatal("creation did not offer adding a tool")
	}
}

func TestRenderingDoesNotRecoverChatOnDisk(t *testing.T) {
	m, root := testModel(t)
	id := createWorkspace(t, m, "Payments")
	c, err := m.store.NewConversation(id, "Interrupted chat")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.store.StartTurn(id, c.ID, workspace.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "workspaces", id, "conversations", c.ID, "conversation.jsonl")
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
	m.selectScope(id, "", "")
	if m.chat().turns[0].Response.Status != "interrupted" {
		t.Fatal("scope selection did not recover chat")
	}
}

func TestSettingsOverlayFitsTerminal(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.openSettings()
		for range len(m.settingsItems()) {
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
	press(t, m, "down")
	typeText(m, "unsaved-model")
	form := m.form
	for _, size := range [][2]int{{120, 30}, {60, 10}, {80, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, key := range []string{"tab", "shift+tab"} {
			press(t, m, key)
			if m.focus != 1 || m.form != form || form.fields[1].value.text != "unsaved-model" || form.index != 1 {
				t.Fatal("Tab must leave modal focus and input unchanged")
			}
		}
	}
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
		for i, field := range m.form.fields {
			m.form.index = i
			screen := ansi.Strip(m.render())
			if !strings.Contains(screen, field.label) || (!strings.Contains(screen, "▏") && len(field.options) == 0) {
				t.Fatalf("%dx%d hides current field %q or its cursor", size[0], size[1], field.label)
			}
		}
		m.form.index = len(m.form.fields)
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
