package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"

	"os"
	"path/filepath"
	"strings"
	"testing"
)

func addAIConnection(t *testing.T, m *model, name, provider string) {
	t.Helper()
	openProviderItem(t, m, "Add provider connection")
	completeAIForm(t, m, map[string]string{"name": name, "provider": provider})
}

func TestModelCheckmarkFollowsSelectionInsteadOfCursor(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "First", "openai")
	addAIConnection(t, m, "Second", "anthropic")
	if body := m.settingsBody(80, 30); strings.Contains(body, "First") || strings.Contains(body, "Second") {
		t.Fatal("root Settings exposes Plous providers")
	}
	m.beginAgentSettings()
	body := ansi.Strip(m.formBody(80, 30))
	if !strings.Contains(body, "First") || !strings.Contains(body, "Second") || strings.Contains(body, " · chat") || strings.Contains(body, "✓") {
		t.Fatal("Settings must list connections without a model selection badge")
	}
	m.cancelForm()
	startChat(t, m)
	m.openModelPicker()
	press(t, m, "down")
	body = ansi.Strip(m.modelPickerBody(80, 30))
	if !strings.Contains(body, "GPT-5.4 mini ✓") || strings.Contains(body, "GPT-5.4 ✓") {
		t.Fatal("moving the picker cursor changed the current model's marker")
	}
	press(t, m, "enter")
	m.openModelPicker()
	body = ansi.Strip(m.modelPickerBody(80, 30))
	if !strings.Contains(body, "GPT-5.4 ✓") || strings.Contains(body, "GPT-5.4 mini ✓") {
		t.Fatal("model selection did not move the checkmark")
	}
}

func TestMultipleConnectionsSelectEditRemoveAndRestart(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "First", "openai")
	addAIConnection(t, m, "Second", "anthropic")
	if len(m.settings.Configs()) != 2 || m.settings.Agent.Name != "First" {
		t.Fatal("adding a connection replaced the current selection")
	}
	startChat(t, m)
	typeText(m, "/model")
	if cmd := press(t, m, "enter"); cmd != nil || !m.chat().Models || m.run != nil || m.draft().Text != "" {
		t.Fatal("model command sent a request or failed to open the picker")
	}
	press(t, m, "down")
	press(t, m, "down")
	press(t, m, "enter")
	if m.settings.Agent.Name != "Second" || !m.chatEditing || m.chat().Models {
		t.Fatal("selection did not return to typing")
	}
	typeText(m, "Keep this draft")
	press(t, m, "esc")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newModel(m.store, m.history, "0.0.1")
	if err != nil || restarted.settings.Agent.Model != "claude-haiku-4-5-20251001" || restarted.draft().Text != "Keep this draft" {
		t.Fatal("selection or draft did not survive restart", err)
	}
	openProviderItem(t, m, "Second")
	completeAIForm(t, m, map[string]string{"name": "Renamed"})
	if m.settings.Agent.Model != "claude-haiku-4-5-20251001" || m.settings.Agent.Name != "Renamed" {
		t.Fatal("editing the selected connection lost its selection")
	}
	for _, name := range []string{"Renamed", "First"} {
		openProviderItem(t, m, name)
		m.form.Index = len(m.form.Fields) + 3
		press(t, m, "enter")
		if m.page != "removeprovider" {
			t.Fatal("removal must show a confirmation")
		}
		press(t, m, "down")
		press(t, m, "enter")
		press(t, m, "esc")
	}
	if len(m.settings.Configs()) != 0 || m.settings.Agent != ai.DefaultConfig() {
		t.Fatal("removing the last connection did not clear the selection")
	}
	if _, err := m.store.ReadSettings(); err != nil {
		t.Fatal("settings audit chain is invalid", err)
	}
}

func TestModelSwitchKeepsInflightRequestAndHistory(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "First", "openai")
	addAIConnection(t, m, "Second", "anthropic")
	startChat(t, m)
	c := m.chat()
	conversation, err := m.history.NewConversation("", "Running")
	if err != nil {
		t.Fatal(err)
	}
	c.Conversation = conversation
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: m.settings.Agent, Prompt: "Inspect"})
	if err != nil {
		t.Fatal(err)
	}
	c.Turns = []history.Turn{turn}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &appchat.ChatRun{Chat: c, Cancel: cancel}
	m.run = run
	typeText(m, "/m")
	press(t, m, "enter")
	press(t, m, "down")
	press(t, m, "down")
	press(t, m, "enter")
	if m.run != run || ctx.Err() != nil || c.Turns[0].Request.Agent.Name != "First" || m.settings.Agent.Name != "Second" {
		t.Fatal("switching models mutated or cancelled an in-flight request")
	}
	m.updateChatEvent(appchat.ChatEvent{Run: run, Done: true, Result: ai.Result{Text: "Done"}})
	m.drainPersistence()
	turns, err := m.history.Turns("", conversation.ID)
	if err != nil || turns[0].Request.Agent.Model != "gpt-5.4-mini" || turns[0].Response.Text != "Done" {
		t.Fatal("running response lost its original model", err)
	}
}

func TestModelCommandScopeAndLiteralText(t *testing.T) {
	for _, command := range []string{"/m", "/model"} {
		m, _ := testModel(t)
		startChat(t, m)
		typeText(m, command)
		press(t, m, "enter")
		if !m.chat().Models {
			t.Fatal("picker must open without a configured model")
		}
		press(t, m, "esc")
		if !m.chatEditing || m.chat().Models {
			t.Fatal("Esc must return to typing")
		}
		m.Update(tea.PasteMsg{Content: command})
		press(t, m, "enter")
		if m.chat().Models || m.draft().Text != command {
			t.Fatal("pasted model command executed")
		}
		press(t, m, "ctrl+u")
		typeText(m, command)
		press(t, m, "enter")
		if !m.chat().Models {
			t.Fatal("clearing pasted text did not restore command input")
		}
		press(t, m, "enter")
		if m.form == nil || m.form.Kind != "mainagent" || m.form.Actions()[m.form.Index-len(m.form.Fields)] != "Add provider connection" {
			t.Fatal("empty picker must offer Settings")
		}
	}
	for _, text := range []string{"/model explanation", "line\n/model", "//model"} {
		m, _ := testModel(t)
		startChat(t, m)
		*m.draft() = textview.NewField(text)
		press(t, m, "enter")
		if m.chat().Models || m.draft().Text == "" {
			t.Fatal("ordinary or escaped text became a command")
		}
	}
}

func TestAIConnectionValidationPreservesForm(t *testing.T) {
	m, root := testModel(t)
	addAIConnection(t, m, "First", "openai")
	openProviderItem(t, m, "Add provider connection")
	values := map[string]string{"name": "FIRST", "provider": "openai", "api_key": "rejected-secret"}
	m.saveAppSettings(values)
	if m.form == nil || m.settings.Revision != 1 || !strings.Contains(m.status, "unique") {
		t.Fatal("duplicate names must retain the form without changing settings")
	}
	data, err := os.ReadFile(filepath.Join(root, "credentials.json"))
	if err != nil || strings.Contains(string(data), "rejected-secret") {
		t.Fatal("failed settings save left a new secret behind", err)
	}
	key, err := m.store.Credential(m.settings.Agent)
	if err != nil || key != "test-api-key" {
		t.Fatal("failed settings save changed the active key", err)
	}
}

func TestModelPickerAndSettingsFitWithManyConnections(t *testing.T) {
	m, _ := testModel(t)
	for i := range 12 {
		config := ai.DefaultConfig()
		config.Name, config.Model = fmt.Sprintf("Connection %d", i), strings.Repeat("model-", 12)
		m.settings.Providers = append(m.settings.Providers, config)
	}
	m.settings.Agent = m.settings.Providers[0]
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, settings := range []bool{false, true} {
			if settings {
				m.beginAgentSettings()
			} else {
				startChat(t, m)
				m.openModelPicker()
			}
			count := len(m.chatModelChoices())
			if settings {
				count = 12
			}
			for i := range count + 1 {
				m.chat().ModelCursor = i
				if settings {
					m.form.Index = len(m.form.Fields) + i
				}
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatalf("%v: incorrect height", size)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != size[0] {
						t.Fatalf("%v: row overflow: %q", size, line)
					}
				}
				label := ""
				if i < count {
					label = "› " + appchat.ChatModelName(m.chatModelChoices()[i])
					if settings {
						label = fmt.Sprintf("› Connection %d", i)
					}
				}
				if label != "" && !strings.Contains(strings.Join(lines, "\n"), label) {
					t.Fatalf("%v: selected connection %d is hidden", size, i)
				}
			}
		}
		m.cancelForm()
		m.closeSettings()
	}
}

func TestSidebarSeparatesWorkspacesAndApplication(t *testing.T) {
	m, _ := testModel(t)
	for _, row := range m.treeRows() {
		if row.kind == "general" {
			t.Fatal("General chat must not be a sidebar item")
		}
	}
	for _, height := range []int{10, 24, 54} {
		body := ansi.Strip(m.treeBody(24, height))
		lines := strings.Split(body, "\n")
		if len(lines) != height || !strings.Contains(lines[height-1], "Settings") || strings.Contains(body, "General chat") {
			t.Fatal("only Settings belongs at the bottom of the sidebar")
		}
		if strings.Index(body, "+ New Workspace") > strings.Index(body, "Application") {
			t.Fatal("workspace creation is in the application section")
		}
	}
}
