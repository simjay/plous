package app

import (
	tea "charm.land/bubbletea/v2"
	ai "github.com/simjay/plous/internal/ai/model"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
