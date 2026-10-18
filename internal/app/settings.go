package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	"strings"
)

func (m *model) beginSettingsOverlay() {
	if m.settingsOverlay == nil {
		m.settingsOverlay = &appsettings.SettingsOverlay{Page: m.page, Detail: m.detail, Cursor: m.cursor, Focus: m.focus}
	}
	m.focus = 1
}

func (m *model) closeSettings() {
	m.removeForm = nil
	m.toolSetup = nil
	if previous := m.settingsOverlay; previous != nil {
		m.page, m.detail, m.cursor, m.focus = previous.Page, previous.Detail, previous.Cursor, previous.Focus
		m.settingsOverlay = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) renderSettingsOverlay(background string) string {
	width := min(84, m.width-8)
	rows := len(m.settingsRows()) + 7
	if m.current() != nil {
		rows += 2
	}
	switch {
	case m.form != nil && m.form.Directory != nil:
		rows = 24
	case m.form != nil && m.form.Kind == "kube":
		rows = len(m.contexts) + 9
	case m.form != nil:
		rows = len(m.form.Fields)*3 + len(m.form.Actions()) + 2
		if m.form.Note != "" {
			rows += strings.Count(ansi.Wrap(m.form.Note, max(1, width-6), ""), "\n") + 2
		}
		if m.form.LocalDirectory() {
			rows += 5
		}
		if m.form.Kind == "appsettings" {
			rows += 2
		}
		if m.form.Kind == "mainagent" {
			rows += 2
			for _, row := range m.form.Rows {
				if row.Detail != "" {
					rows++
				}
			}
		}
		if m.form.Kind == "workspace" {
			rows += 3
		}
	case m.page == "tools":
		rows = len(appsettings.ToolTypes) + 8
	case m.page == "remove" || m.page == "removeprovider":
		rows = 3
	}
	if m.command != nil {
		rows += 3
	}
	height := min(max(10, rows+4), min(28, m.height-6))
	if m.form != nil && m.form.Kind == "kube" {
		height = min(max(14, rows+4), min(28, m.height-2))
	}
	contentWidth, contentHeight := width-6, height-4
	title := "Settings"
	if m.toolSetup != nil {
		title = "Add Connection"
		if m.toolSetup.ContextsOnly {
			title = "Add Context"
		}
	}
	if m.form != nil && m.form.Kind == "workspace" {
		title = "New Workspace"
	}
	if m.form != nil && m.form.Kind == "kube" {
		title = "Add Context"
	}
	if m.form != nil && m.form.Kind == "contextname" {
		title = "Add Context"
	}
	if m.form != nil && m.form.Source != nil {
		title = m.form.Title
	}
	heading := ui.OverlayTitleStyle.Background(ui.Surface).Render(title)
	action := " back"
	if m.form == nil && m.page == "settings" {
		action = " close"
	}
	label := ui.MouseAction(ui.ShortcutStyle.Background(ui.Surface).Render("Esc")+ui.Muted.Background(ui.Surface).Render(action), "Esc")
	heading += lipgloss.NewStyle().Background(ui.Surface).Render(strings.Repeat(" ", max(1, contentWidth-lipgloss.Width(heading)-lipgloss.Width(label)))) + label
	body := m.viewContent(contentWidth, contentHeight)
	if m.command != nil {
		body = lipgloss.NewStyle().Width(contentWidth).Height(contentHeight-3).MaxHeight(contentHeight-3).
			Render(m.viewContent(contentWidth, contentHeight-3)) + "\n" + m.commandBox(contentWidth)
	}
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.FocusedBorder).
		Foreground(ui.Foreground).Background(ui.Surface).Padding(0, 2).Width(width).Height(height).MaxWidth(width).MaxHeight(height).
		Render(heading + "\n\n" + body)
	return ui.CenteredOverlay(background, window)
}

func (m *model) beginAppSettings() {
	m.beginAISettings(-1)
}

func (m *model) beginAISettings(index int) {
	m.beginSettingsOverlay()
	m.nativeChange = nil
	parent := m.form
	if parent == nil || parent.Kind != "mainagent" {
		parent = appsettings.AgentForm(agent.Plous)
		parent.RefreshAgent(m.settings.Configs())
	}
	a := ai.DefaultConfig()
	a.Name, a.KeyEnv = "", ""
	name := ""
	title := "Add provider connection"
	if configs := m.settings.Configs(); index >= 0 && index < len(configs) {
		a = configs[index]
		name = a.Name
		title = "Edit provider connection"
	}
	m.page = ""
	m.command = nil
	m.status = ""
	m.cursor = 0
	m.form = &appsettings.Form{Kind: "appsettings", Title: title, Profile: name, Remove: name != "", Agent: a, Parent: parent}
	m.buildAIFields()
}

func (m *model) buildAIFields() {
	f := m.form
	a := f.Agent
	f.Fields = []appsettings.Field{
		{Key: "provider", Label: "Vendor", Required: true, Value: textview.NewField(a.Provider), Options: ai.Providers()},
		{Key: "api_key", Label: "API Key", Secret: true, Required: a.CredentialRef == "" && a.KeyEnv == "", Value: textview.NewField(f.ApiKey)},
	}
	if a.CredentialRef != "" {
		f.Fields[1].Label = "API Key (saved; blank keeps it)"
	}
	if a.CredentialRef == "" && a.KeyEnv != "" {
		f.Fields[1].Label = "API Key (blank keeps existing environment setup)"
	}
	if !f.Advanced {
		return
	}
	f.Fields = append(f.Fields, appsettings.Field{Key: "name", Label: "Display Name (Optional)", Value: textview.NewField(a.Name)})
	f.Fields = append(f.Fields, appsettings.Field{Key: "endpoint", Label: "API Proxy URL (Optional)", Value: textview.NewField(a.Endpoint)})
	f.Fields = append(f.Fields,
		appsettings.Field{Key: "key_env", Label: "Environment Variable (optional)", Value: textview.NewField(a.KeyEnv)},
		appsettings.Field{Key: "instructions", Label: "Custom Instructions (Optional)", Value: textview.NewField(a.Instructions)},
	)
}

func (m *model) captureAIFields() {
	values := map[string]string{}
	for _, field := range m.form.Fields {
		values[field.Key] = strings.TrimSpace(field.Value.Text)
		if field.Key == "api_key" {
			m.form.ApiKey = values[field.Key]
		}
	}
	m.form.Agent = appsettings.ApplyAIFields(m.form.Agent, values)
}

func (m *model) changeAIProvider() {
	f := m.form
	m.captureAIFields()
	f.Agent.KeyEnv, f.Agent.CredentialRef, f.ApiKey = "", "", ""
	f.Agent.Model = ""
	// Switching vendors must not reuse another vendor's key or endpoint.
	f.Agent.Endpoint = ""
	m.buildAIFields()
}

func (m *model) aiCredentialHint() string {
	if m.form.Agent.CredentialRef != "" {
		return "API Key saved locally · enter a new key to replace it"
	}
	if m.form.Agent.KeyEnv != "" {
		return "Existing environment setup retained until a key is saved"
	}
	return "API Keys stay in your local Plous data directory"
}

func (m *model) saveAppSettings(values map[string]string) {
	a := appsettings.ApplyAIFields(m.form.Agent, values).Connection()
	if err := a.SupportedProvider(); err != nil {
		m.status = err.Error()
		return
	}
	if a.Name == "" {
		base := appsettings.ProviderLabel(a.Provider)
		a.Name = base
		for suffix := 2; ; suffix++ {
			used := false
			for _, config := range m.settings.Configs() {
				if config.Name != m.form.Profile && strings.EqualFold(config.Name, a.Name) {
					used = true
					break
				}
			}
			if !used {
				break
			}
			a.Name = fmt.Sprintf("%s (%d)", base, suffix)
		}
	}
	initial, err := a.WithDefaultModel()
	if err != nil {
		m.status = err.Error()
		return
	}
	key := strings.TrimSpace(values["api_key"])
	if _, exists := values["api_key"]; !exists {
		key = m.form.ApiKey
	}
	if key != "" {
		ref, err := m.store.SaveCredential(key)
		if err != nil {
			m.status = err.Error()
			return
		}
		a.CredentialRef, a.KeyEnv = ref, ""
	}
	next := m.settings
	next.Providers = m.settings.Configs()
	found := false
	for i, c := range next.Providers {
		if c.Name == m.form.Profile {
			next.Providers[i] = a
			found = true
		}
	}
	if !found {
		next.Providers = append(next.Providers, a)
	}
	if len(next.Providers) == 1 || m.settings.Agent.Name == m.form.Profile {
		if m.settings.Agent.Name == m.form.Profile && m.settings.Agent.Provider == a.Provider {
			next.Agent = a
			next.Agent.Model = m.settings.Agent.Model
		} else {
			next.Agent = a
			next.Agent.Model = initial.Model
		}
	}
	if !m.saveAISettings(next) {
		if err := m.store.PruneCredentials(); err != nil {
			m.status += " " + err.Error()
		}
		return
	}
	m.returnToAgent(m.form.Parent)
	m.status = "Provider connection saved. Use /model in Plous chat to switch models."
	if err := m.store.PruneCredentials(); err != nil {
		m.status += " " + err.Error()
	}
}

func (m *model) saveAISettings(next workspace.Settings) bool {
	settings, err := m.store.SaveSettings(next)
	if err != nil {
		if current, loadErr := m.store.ReadSettings(); loadErr == nil {
			m.settings = current
		}
		m.status = "Settings save could not be confirmed: " + err.Error()
		return false
	}
	m.settings = settings
	return true
}

func (m *model) openSettings() {
	m.removeForm = nil
	m.toolSetup = nil
	m.beginSettingsOverlay()
	m.page = "settings"
	m.cursor = 0
	m.command = nil
	m.detail = ""
	m.status = ""
}

func (m *model) cancelForm() {
	m.nativeChange = nil
	if m.form.Kind == "contextname" && m.toolSetup != nil {
		m.finishContextName(false)
		return
	}
	if m.toolSetup != nil {
		if m.form.Kind == "kube" || m.toolSetup.RepositoryOnly {
			m.backFromToolSetup()
		} else {
			m.form = nil
			m.status = ""
			m.page = "tools"
		}
		return
	}
	if m.form.Parent != nil {
		m.returnToAgent(m.form.Parent)
	} else {
		m.form = nil
		m.openSettings()
	}
	m.status = "Edits canceled"
}

func (m *model) removeAIConnection() {
	next := m.settings
	next.Providers = nil
	for _, c := range m.settings.Configs() {
		if c.Name != m.removeProfile {
			next.Providers = append(next.Providers, c)
		}
	}
	if next.Agent.Name == m.removeProfile {
		next.Agent = ai.DefaultConfig()
		if len(next.Providers) > 0 {
			next.Agent = next.Providers[0]
			for _, connection := range next.Providers {
				if agent, err := connection.WithDefaultModel(); err == nil {
					next.Agent = agent
					break
				}
			}
		}
	}
	if m.saveAISettings(next) {
		var parent *appsettings.Form
		if m.removeForm != nil {
			parent = m.removeForm.Parent
		}
		m.removeForm = nil
		m.returnToAgent(parent)
		m.status = fmt.Sprintf("Removed provider connection %s", m.removeProfile)
		if err := m.store.PruneCredentials(); err != nil {
			m.status += " " + err.Error()
		}
	}
}

func (m *model) cancelProviderRemoval() {
	m.form, m.removeForm = m.removeForm, nil
	m.page, m.status = "", ""
	if m.form == nil {
		m.beginAgentSettings()
	}
}
