package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func TestAgentEntryCommandDraftsSaveIndependently(t *testing.T) {
	m, _ := testModel(t)
	m.beginAgentSettings()
	press(t, m, "right")
	if len(m.form.Fields) != 2 || m.form.Fields[1].Value.Text != "claude" {
		t.Fatal("Claude Code entry command default missing")
	}
	m.form.Fields[1].Value = textview.NewField("celai claude")
	press(t, m, "right")
	if m.form.Fields[1].Value.Text != "codex" {
		t.Fatal("Codex entry command default missing")
	}
	m.form.Fields[1].Value = textview.NewField("celai codex")
	press(t, m, "left")
	if m.form.Fields[1].Value.Text != "celai claude" || m.settings.MainAgent.Effective() != agent.Plous || m.settings.AgentCommands != (agent.Commands{}) {
		t.Fatal("browsing agents lost a draft or applied settings")
	}
	m.submitForm()
	saved, err := m.store.ReadSettings()
	if err != nil || saved.MainAgent != agent.ClaudeCode || saved.AgentCommands != (agent.Commands{ClaudeCode: "celai claude", Codex: "celai codex"}) {
		t.Fatal("entry commands were not saved independently", saved, err)
	}
	m.beginAgentSettings()
	if m.form.Fields[1].Value.Text != "celai claude" {
		t.Fatal("Agent settings did not restore the command")
	}
	m.form.Fields[1].Value = textview.NewField("different claude")
	m.cancelForm()
	if m.settings.AgentCommands != saved.AgentCommands {
		t.Fatal("cancel changed the entry command")
	}
	m.beginAgentSettings()
	m.form.Fields[1].Value = textview.NewField("")
	m.submitForm()
	if m.settings.AgentCommands.For(agent.ClaudeCode) != "claude" || m.settings.AgentCommands.For(agent.Codex) != "celai codex" {
		t.Fatal("clearing a command did not restore only its default")
	}
}

func TestEntryCommandChangeWaitsForNativeEvidence(t *testing.T) {
	for _, action := range []string{"save", "edit", "paste", "cancel", "invalid", "evidence failure"} {
		t.Run(action, func(t *testing.T) {
			m, n := nativeModel(t)
			m.beginAgentSettings()
			m.form.Fields[1].Value = textview.NewField("celai claude")
			if action == "invalid" {
				m.form.Fields[1].Value = textview.NewField(`celai "claude`)
			}
			m.submitForm()
			if action == "invalid" {
				if n.Input.Ctx.Err() != nil || m.nativeChange != nil || !strings.Contains(m.status, "quotes") {
					t.Fatal("invalid command closed the session or hid the problem", m.status)
				}
				return
			}
			if m.settings.AgentCommands != (agent.Commands{}) || n.Input.Ctx.Err() != context.Canceled || m.nativeChange == nil {
				t.Fatal("entry command was applied before closing the session")
			}
			result := nativeAgentResult{session: n}
			switch action {
			case "edit":
				// Cover even changes made without the normal key dispatch path.
				m.form.Fields[1].Value = textview.NewField("other claude")
			case "paste":
				m.form.Index = 1
				m.Update(tea.PasteMsg{Content: " changed"})
			case "cancel":
				m.cancelForm()
			case "evidence failure":
				result.saveErr = context.Canceled
			}
			m.Update(result)
			if action == "save" {
				if m.settings.AgentCommands.For(agent.ClaudeCode) != "celai claude" || m.nativeAgent != nil || m.form != nil {
					t.Fatal("confirmed command change was not applied", m.status)
				}
			} else if m.settings.AgentCommands != (agent.Commands{}) || m.nativeChange != nil {
				t.Fatal("closure applied a canceled, changed, or failed command")
			}
		})
	}
}

func agentRowIndex(t *testing.T, m *model, kind appsettings.RowKind) int {
	t.Helper()
	for i, row := range m.form.Rows {
		if row.Kind == kind {
			return i
		}
	}
	t.Fatalf("agent row %d missing", kind)
	return -1
}

func TestAgentProviderSetupKeepsActiveSessionAndPendingChoice(t *testing.T) {
	m, native := nativeModel(t)
	m.openSettings()
	root := m.settingsBody(80, 30)
	if strings.Contains(root, "provider") || strings.Contains(root, "OpenAI") || m.settingsRows()[0].Kind != appsettings.AgentRow {
		t.Fatal("root Settings exposes provider setup or hides the agent entry")
	}
	m.updateSettings("enter")
	if strings.Contains(m.formBody(80, 30), "Provider connections") || len(m.form.Rows) != 2 {
		t.Fatal("external agent offers Plous provider controls")
	}
	press(t, m, "left")
	parent := m.form
	if parent.Fields[0].Value.Text != "Plous" || !strings.Contains(m.formBody(80, 30), "Provider connections") {
		t.Fatal("Plous preview does not show its provider setup")
	}
	add := agentRowIndex(t, m, appsettings.AddProviderRow)
	clickMouse(t, m, "main", "formaction/"+strconv.Itoa(add))
	if m.form.Parent != parent || m.form.Kind != "appsettings" {
		t.Fatal("mouse did not open provider setup with its parent")
	}
	m.form.Fields[1].Value = textview.NewField("fixture-provider-key")
	m.form.Index = len(m.form.Fields)
	press(t, m, "enter")
	if m.form != parent || m.form.Fields[0].Value.Text != "Plous" || len(m.settings.Configs()) != 1 {
		t.Fatal("provider Save lost the pending agent choice", m.status)
	}
	if m.settings.MainAgent != agent.ClaudeCode || native.Input.Ctx.Err() != nil || m.nativeChange != nil {
		t.Fatal("previewing or configuring Plous changed the active native session")
	}
	press(t, m, "esc")
	if m.form != nil || m.page != "settings" || m.settings.MainAgent != agent.ClaudeCode {
		t.Fatal("canceling the agent preview changed the selection")
	}
	m.beginAgentSettings()
	if strings.Contains(m.formBody(80, 30), "OpenAI") {
		t.Fatal("saved provider leaked into external-agent setup")
	}
	press(t, m, "left")
	if !strings.Contains(m.formBody(80, 30), "OpenAI") {
		t.Fatal("saved provider was lost when canceling the agent preview")
	}
}

func TestProviderRemovalReturnsThroughDraftAndAgent(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "Saved", "openai")
	openProviderItem(t, m, "Saved")
	provider, parent := m.form, m.form.Parent
	provider.Fields[1].Value = textview.NewField("unsaved-replacement")
	provider.Index = len(provider.Fields) + 3
	press(t, m, "enter")
	m.execute("settings")
	if m.removeForm != provider || m.page != "removeprovider" {
		t.Fatal("Settings command discarded the provider draft during removal")
	}
	press(t, m, "esc")
	if m.form != provider || provider.Fields[1].Value.Text != "unsaved-replacement" {
		t.Fatal("canceling removal lost the provider draft")
	}
	press(t, m, "enter")
	press(t, m, "down")
	press(t, m, "enter")
	if m.form != parent || len(m.settings.Configs()) != 0 || m.removeForm != nil {
		t.Fatal("removal did not restore the pending Agent view")
	}
	if m.form.Index >= len(m.form.Fields)+len(m.form.Rows) {
		t.Fatal("removed provider left an unreachable selection")
	}
}

func TestProviderSetupCancelsPendingNativeAgentChange(t *testing.T) {
	m, native := nativeModel(t)
	m.beginAgentSettings()
	press(t, m, "left")
	parent := m.form
	m.submitForm()
	if m.nativeChange == nil {
		t.Fatal("native closure did not queue the agent change")
	}
	m.beginAppSettings()
	m.saveAppSettings(map[string]string{"api_key": "fixture-key"})
	m.Update(nativeAgentResult{session: native})
	if m.form != parent || m.settings.MainAgent != agent.ClaudeCode || m.nativeChange != nil {
		t.Fatal("finishing provider setup applied an earlier pending agent change")
	}
}

func TestAgentProviderRowsKeepSaveDistinctFromProviderNames(t *testing.T) {
	m, _ := testModel(t)
	config := ai.DefaultConfig()
	config.Name = "save-agent"
	m.settings.Providers = []ai.Config{config}
	m.beginAgentSettings()
	m.form.Index = len(m.form.Fields) + agentRowIndex(t, m, appsettings.SaveAgentRow)
	m.form.RefreshAgent(m.settings.Configs())
	press(t, m, "enter")
	if m.form != nil || m.page != "settings" {
		t.Fatal("a provider name replaced the Save action")
	}
}

func TestKeyInputIsMaskedAndOnlySavedOnSave(t *testing.T) {
	m, root := testModel(t)
	m.beginAppSettings()
	press(t, m, "down")
	typeText(m, "SECRET-NEW-KEY")
	if strings.Contains(m.render(), "SECRET") || !strings.Contains(m.render(), "•") {
		t.Fatal("API Key was not masked")
	}
	press(t, m, "esc")
	if _, err := os.Stat(filepath.Join(root, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("cancel wrote a key")
	}
	m.beginAppSettings()
	completeAIForm(t, m, map[string]string{"api_key": "SECRET-NEW-KEY"})
	key, err := m.store.Credential(m.settings.Agent)
	if err != nil || key != "SECRET-NEW-KEY" || m.settings.Agent.KeyEnv != "" {
		t.Fatal("key did not save separately", err)
	}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if !entry.IsDir() && entry.Name() != "credentials.json" {
			data, _ := os.ReadFile(path)
			if strings.Contains(string(data), "SECRET-NEW-KEY") {
				t.Fatalf("key leaked to %s", entry.Name())
			}
		}
		return nil
	})
	m.beginAISettings(0)
	if m.form.Fields[1].Value.Text != "" {
		t.Fatal("saved secret was loaded into the editor")
	}
	press(t, m, "right")
	if m.form.Agent.CredentialRef != "" || m.form.ApiKey != "" || m.form.Agent.KeyEnv != "" {
		t.Fatal("changing provider retained its old credentials")
	}
}

func TestChatCatalogUsesEveryModelFromConnectedVendors(t *testing.T) {
	m, _ := testModel(t)
	addAIConnection(t, m, "Work", "openai")
	choices := m.chatModelChoices()
	if len(choices) != 2 {
		t.Fatal("one OpenAI key must offer both chat models")
	}
	connection := m.settings.Configs()[0]
	for _, choice := range choices {
		if choice.Connection() != connection {
			t.Fatal("models did not share one connection")
		}
	}
	startChat(t, m)
	typeText(m, "/model")
	if cmd := press(t, m, "enter"); cmd != nil || !m.chat().Models {
		t.Fatal("picker must open without a network call")
	}
	before := m.settings.Agent
	typeText(m, "unknown-model")
	m.Update(tea.PasteMsg{Content: "unknown-model"})
	press(t, m, "down")
	press(t, m, "esc")
	if m.settings.Agent != before {
		t.Fatal("Esc, text, or paste changed model selection")
	}
	typeText(m, "/model")
	press(t, m, "enter")
	press(t, m, "down")
	press(t, m, "enter")
	if m.settings.Agent.Model != "gpt-5.4" || m.settings.Agent.Connection() != connection {
		t.Fatal("chat model did not switch on the same connection")
	}
	configs := m.settings.Configs()
	if len(configs) != 1 || configs[0].Model != "" {
		t.Fatal("chat choice was written into the vendor connection")
	}
	addAIConnection(t, m, "Personal", "anthropic")
	choices = m.chatModelChoices()
	if len(choices) != 5 || choices[0].Provider != "openai" || choices[2].Provider != "anthropic" {
		t.Fatal("picker did not group all connected vendors")
	}
	startChat(t, m)
	m.openModelPicker()
	body := m.modelPickerBody(80, 30)
	for _, label := range []string{"OpenAI · Work", "Anthropic · Personal", "GPT-5.4", "Claude Haiku", "Claude Sonnet", "Claude Opus"} {
		if !strings.Contains(body, label) {
			t.Fatalf("missing model group or name: %s", label)
		}
	}
}

func TestUnsupportedSavedConnectionsStayReadableButCannotRun(t *testing.T) {
	for _, provider := range []string{"openai", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			m, root := testModel(t)
			old := ai.DefaultConfig()
			old.Provider, old.Model, old.Endpoint = provider, "old-model", "https://example.com/v1"
			ref, err := m.store.SaveCredential("preserved-key")
			if err != nil {
				t.Fatal(err)
			}
			old.CredentialRef = ref
			// Seed a revision written by a previous build.
			if _, err = m.store.SaveSettings(workspace.Settings{Agent: old, Providers: []ai.Config{old}}); err != nil {
				t.Fatal(err)
			}
			m, err = newModel(m.store, m.history, "0.0.1")
			if err != nil || m.settings.Agent != old {
				t.Fatal("legacy settings were changed or could not load", err)
			}
			m.beginAgentSettings()
			if old.SupportedProvider() != nil && !strings.Contains(m.formBody(90, 30), "unavailable") {
				t.Fatal("missing unavailable label")
			}
			m.cancelForm()
			startChat(t, m)
			typeText(m, "Keep my draft")
			if cmd := press(t, m, "enter"); cmd != nil || m.run != nil || m.draft().Text != "Keep my draft" {
				t.Fatal("unsupported connection started a request or lost the draft")
			}
			paths, _ := filepath.Glob(filepath.Join(root, "conversations", "*", "conversation", "*.jsonl"))
			if len(paths) != 0 {
				t.Fatal("unsupported request was persisted")
			}
			press(t, m, "esc")
			if old.SupportedProvider() != nil {
				m.beginAISettings(0)
				completeAIForm(t, m, map[string]string{"provider": "openai"})
			}
			startChat(t, m)
			press(t, m, "ctrl+u")
			typeText(m, "/model")
			press(t, m, "enter")
			press(t, m, "home")
			press(t, m, "enter")
			if _, err := m.store.ReadSettings(); err != nil {
				t.Fatal("editing broke old revision history", err)
			}
			if m.settings.Agent.Supported() != nil {
				t.Fatal("legacy connection could not be repaired")
			}
		})
	}
}

func TestDisallowedVendorCannotSaveCredentials(t *testing.T) {
	for _, provider := range []string{"gemini", "xai", "openai-compatible"} {
		m, root := testModel(t)
		m.beginAppSettings()
		m.saveAppSettings(map[string]string{"provider": provider, "api_key": "not-saved"})
		if m.form == nil || m.settings.Revision != 0 || !strings.Contains(m.status, "unavailable") {
			t.Fatal("unsupported config saved")
		}
		if _, err := os.Stat(filepath.Join(root, "credentials.json")); !os.IsNotExist(err) {
			t.Fatal("unsupported config saved its key")
		}
	}
}

func TestBasicSetupNeedsOnlyVendorAndKey(t *testing.T) {
	for _, provider := range ai.Providers() {
		t.Run(provider, func(t *testing.T) {
			m, _ := testModel(t)
			m.beginAppSettings()
			if provider == "anthropic" {
				press(t, m, "right")
			}
			press(t, m, "enter")
			typeText(m, "setup-only-key")
			press(t, m, "enter")
			if m.form.Index != len(m.form.Fields) {
				t.Fatal("key must advance directly to Save")
			}
			press(t, m, "enter")
			if m.form == nil || m.form.Kind != "mainagent" || m.settings.Agent.Provider != provider {
				t.Fatal("basic setup failed", m.status)
			}
			if err := m.settings.Agent.Ready("setup-only-key"); err != nil {
				t.Fatal("default chat model is not ready", err)
			}
			if len(m.settings.Configs()) != 1 || m.settings.Providers[0].Model != "" {
				t.Fatal("vendor connection has a model selection")
			}
			key, err := m.store.Credential(m.settings.Agent)
			if err != nil || key != "setup-only-key" {
				t.Fatal("key not shared by default model", err)
			}
			m.beginAISettings(0)
			m.form.Index = len(m.form.Fields) + 2
			press(t, m, "enter")
			for _, field := range m.form.Fields {
				if field.Key == "model" {
					t.Fatal("Advanced must not contain model selection")
				}
			}
		})
	}
}
