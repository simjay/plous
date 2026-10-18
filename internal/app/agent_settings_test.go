package app

import (
	"strconv"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	appsettings "github.com/simjay/plous/internal/app/settings"
	textview "github.com/simjay/plous/internal/ui/text"
)

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
