package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/ai"
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

func chatModelName(config ai.Config) string {
	if model, ok := ai.LookupModel(config.Provider, config.Model); ok {
		return model.Name
	}
	return config.Model
}

func (m *model) openModelPicker() {
	c := m.chat()
	c.models, c.history, c.modelCursor = true, false, 0
	m.chatEditing = false
	for i, config := range m.chatModelChoices() {
		if config == m.settings.Agent {
			c.modelCursor = i
		}
	}
	m.status = ""
}

func (m *model) updateModelPicker(key string) {
	c := m.chat()
	choices := m.chatModelChoices()
	c.modelCursor = max(0, min(c.modelCursor, len(choices)))
	switch key {
	case "up":
		c.modelCursor = max(0, c.modelCursor-1)
	case "down":
		c.modelCursor = min(len(choices), c.modelCursor+1)
	case "home":
		c.modelCursor = 0
	case "end":
		c.modelCursor = len(choices)
	case "esc":
		c.models = false
		m.chatEditing = true
		m.status = ""
	case "enter":
		if c.modelCursor == len(choices) {
			if m.form != nil {
				m.status = "Finish or cancel the open Settings form first."
				return
			}
			c.models = false
			m.chatEditing = true
			m.openSettings()
			m.selectSettingsItem("Add AI connection")
			return
		}
		config := choices[c.modelCursor]
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
		c.models = false
		m.chatEditing = true
		m.status = "Next message: " + chatModelName(config) + " · " + config.Name
	}
}

func (m *model) modelPickerBody(width, height int) string {
	choices := m.chatModelChoices()
	lines := []string{titleStyle.Render("Choose a model"), muted.Render("✓ Current model"), ""}
	selectedRow := 0
	group := ""
	for i, config := range choices {
		if group != config.Name {
			group = config.Name
			heading := providerLabel(config.Provider)
			if group != heading {
				heading += " · " + group
			}
			lines = append(lines, titleStyle.Render(clipped(oneLine(heading), width)))
		}
		label := chatModelName(config)
		if config == m.settings.Agent {
			label += " ✓"
		}
		if i == m.chat().modelCursor {
			selectedRow = len(lines)
		}
		lines = append(lines, choiceLine(label, i == m.chat().modelCursor, width))
		if i+1 == len(choices) || choices[i+1].Name != group {
			lines = append(lines, "")
		}
	}
	if len(choices) == 0 {
		lines = append(lines, "Connect a supported vendor in Settings.", "")
	}
	if m.chat().modelCursor == len(choices) {
		selectedRow = len(lines)
	}
	lines = append(lines, choiceLine("Manage in Settings", m.chat().modelCursor == len(choices), width))
	return scrollSelection(lines, selectedRow, height)
}

func choiceLine(label string, active bool, width int) string {
	prefix := "  "
	style := lipgloss.NewStyle().Width(width)
	if active {
		prefix = "› "
		style = selected.Background(highlight).Width(width)
	}
	return style.Render(clipped(prefix+oneLine(label), width))
}

func scrollSelection(lines []string, selectedRow, height int) string {
	if height <= 0 {
		return ""
	}
	start := max(0, selectedRow-height+min(3, height))
	start = min(start, max(0, len(lines)-height))
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}
