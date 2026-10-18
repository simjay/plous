package app

import (
	"github.com/simjay/plous/internal/ai/agent"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	appsettings "github.com/simjay/plous/internal/app/settings"
	"github.com/simjay/plous/internal/ui"
)

// Each vendor connection makes its full supported chat catalog available.
func (m *model) chatModelChoices() []ai.Config {
	var choices []ai.Config
	for _, provider := range ai.Providers() {
		for _, connection := range m.settings.Configs() {
			if connection.Provider != provider {
				continue
			}
			for _, model := range ai.Models(provider) {
				config := connection
				config.Model = model.ID
				choices = append(choices, config)
			}
		}
	}
	return choices
}

func (m *model) openModelPicker() {
	if m.settings.MainAgent.Effective() != agent.Plous {
		m.status = "The selected agent uses its CLI model configuration."
		return
	}
	c := m.chat()
	c.Models, c.History, c.ModelCursor = true, false, 0
	m.chatEditing = false
	for i, config := range m.chatModelChoices() {
		if config == m.settings.Agent {
			c.ModelCursor = i
		}
	}
	m.status = ""
}

func (m *model) updateModelPicker(key string) {
	c := m.chat()
	choices := m.chatModelChoices()
	c.ModelCursor = max(0, min(c.ModelCursor, len(choices)))
	switch key {
	case "up":
		c.ModelCursor = max(0, c.ModelCursor-1)
	case "down":
		c.ModelCursor = min(len(choices), c.ModelCursor+1)
	case "home":
		c.ModelCursor = 0
	case "end":
		c.ModelCursor = len(choices)
	case "esc":
		c.Models = false
		m.chatEditing = true
		m.status = ""
	case "enter":
		if c.ModelCursor == len(choices) {
			if m.form != nil || m.removeForm != nil {
				m.status = "Finish or cancel the open Settings form first."
				return
			}
			c.Models = false
			m.chatEditing = true
			m.beginAgentSettings()
			for i, row := range m.form.Rows {
				if row.Kind == appsettings.AddProviderRow {
					m.form.Index = len(m.form.Fields) + i
					break
				}
			}
			return
		}
		config := choices[c.ModelCursor]
		if err := config.Supported(); err != nil {
			m.status = err.Error()
			return
		}
		if config != m.settings.Agent {
			next := m.settings
			next.Providers, next.Agent = m.settings.Configs(), config
			if !m.saveAISettings(next) {
				return
			}
		}
		c.Models = false
		m.chatEditing = true
		m.status = "Next message: " + appchat.ChatModelName(config) + " · " + config.Name
	}
}

func (m *model) modelPickerBody(width, height int) string {
	choices := m.chatModelChoices()
	lines := []string{ui.TitleStyle.Render("Choose a model"), ui.Muted.Render("✓ Current model"), ""}
	selectedRow := 0
	group := ""
	for i, config := range choices {
		if group != config.Name {
			group = config.Name
			heading := appsettings.ProviderLabel(config.Provider)
			if group != heading {
				heading += " · " + group
			}
			lines = append(lines, ui.TitleStyle.Render(ui.Clipped(ui.OneLine(heading), width)))
		}
		label := appchat.ChatModelName(config)
		if config == m.settings.Agent {
			label += " ✓"
		}
		if i == m.chat().ModelCursor {
			selectedRow = len(lines)
		}
		lines = append(lines, ui.MouseChoice(ui.ChoiceLine(label, i == m.chat().ModelCursor, width), i, config.Name+"/"+config.Model))
		if i+1 == len(choices) || choices[i+1].Name != group {
			lines = append(lines, "")
		}
	}
	if len(choices) == 0 {
		lines = append(lines, "Connect a supported vendor in Settings.", "")
	}
	if m.chat().ModelCursor == len(choices) {
		selectedRow = len(lines)
	}
	lines = append(lines, ui.MouseChoice(ui.ChoiceLine("Manage in Settings", m.chat().ModelCursor == len(choices), width), len(choices), "settings"))
	return ui.ScrollSelection(lines, selectedRow, height)
}
