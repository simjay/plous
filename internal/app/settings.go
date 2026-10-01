package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

type settingsOverlay struct {
	page, detail  string
	cursor, focus int
}

func (m *model) beginSettingsOverlay() {
	if m.settingsOverlay == nil {
		m.settingsOverlay = &settingsOverlay{page: m.page, detail: m.detail, cursor: m.cursor, focus: m.focus}
	}
	m.focus = 1
}

func (m *model) closeSettings() {
	m.toolSetup = nil
	if previous := m.settingsOverlay; previous != nil {
		m.page, m.detail, m.cursor, m.focus = previous.page, previous.detail, previous.cursor, previous.focus
		m.settingsOverlay = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) renderSettingsOverlay(background string) string {
	rows := len(m.settingsItems()) + len(m.settings.Configs()) + 7
	if m.current() != nil {
		rows += 2
	}
	switch {
	case m.form != nil && m.form.kind == "kube":
		rows = len(m.contexts) + 9
	case m.form != nil:
		rows = len(m.form.fields)*3 + len(m.form.actions()) + 2
		if m.form.kind == "appsettings" {
			rows += 2
		}
		if m.form.kind == "workspace" {
			rows += 3
		}
	case m.page == "tools":
		rows = len(toolTypes) + 2
	case m.page == "remove" || m.page == "removeprovider":
		rows = 3
	}
	if m.command != nil {
		rows += 3
	}
	width, height := min(84, m.width-8), min(max(10, rows+4), min(28, m.height-6))
	if m.form != nil && m.form.kind == "kube" {
		height = min(max(14, rows+4), min(28, m.height-2))
	}
	contentWidth, contentHeight := width-6, height-4
	title := "Settings"
	if m.toolSetup != nil {
		title = "Add tool"
		if m.toolSetup.contextsOnly {
			title = "Add context"
		}
	}
	if m.form != nil && m.form.kind == "workspace" {
		title = "New workspace"
	}
	if m.form != nil && m.form.kind == "kube" {
		title = "Add Kubernetes contexts"
	}
	heading := overlayTitleStyle.Background(surface).Render(title)
	action := " back"
	if m.form == nil && m.page == "settings" {
		action = " close"
	}
	label := shortcutStyle.Background(surface).Render("Esc") + muted.Background(surface).Render(action)
	heading += lipgloss.NewStyle().Background(surface).Render(strings.Repeat(" ", max(1, contentWidth-lipgloss.Width(heading)-lipgloss.Width(label)))) + label
	body := m.viewContent(contentWidth, contentHeight)
	if m.command != nil {
		body = lipgloss.NewStyle().Width(contentWidth).Height(contentHeight-3).MaxHeight(contentHeight-3).
			Render(m.viewContent(contentWidth, contentHeight-3)) + "\n" + m.commandBox(contentWidth)
	}
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(focusedBorder).
		Foreground(foreground).Background(surface).Padding(0, 2).Width(width).Height(height).MaxWidth(width).MaxHeight(height).
		Render(heading + "\n\n" + body)
	return centeredOverlay(background, window)
}

func (m *model) beginAppSettings() {
	m.beginAISettings(-1)
}

func (m *model) beginAISettings(index int) {
	m.beginSettingsOverlay()
	a := ai.DefaultConfig()
	a.Name, a.KeyEnv = "", ""
	name := ""
	title := "Add AI connection"
	if configs := m.settings.Configs(); index >= 0 && index < len(configs) {
		a = configs[index]
		name = a.Name
		title = "Edit AI connection"
	}
	m.page = ""
	m.command = nil
	m.status = ""
	m.cursor = 0
	m.form = &form{kind: "appsettings", title: title, profile: name, remove: name != "", agent: a}
	m.buildAIFields()
}

func providerLabel(provider string) string {
	switch provider {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "gemini":
		return "Google Gemini"
	case "xai":
		return "xAI (Grok)"
	default:
		return "OpenAI-compatible"
	}
}

func (m *model) buildAIFields() {
	f := m.form
	a := f.agent
	f.fields = []field{
		{key: "provider", label: "Vendor", required: true, value: newEditor(a.Provider), options: ai.Providers()},
		{key: "api_key", label: "API key", secret: true, required: a.CredentialRef == "" && a.KeyEnv == "", value: newEditor(f.apiKey)},
	}
	if a.CredentialRef != "" {
		f.fields[1].label = "API key (saved; blank keeps it)"
	}
	if a.CredentialRef == "" && a.KeyEnv != "" {
		f.fields[1].label = "API key (blank keeps existing environment setup)"
	}
	if !f.advanced {
		return
	}
	f.fields = append(f.fields, field{key: "name", label: "Display name (optional)", value: newEditor(a.Name)})
	f.fields = append(f.fields, field{key: "endpoint", label: "API proxy URL (optional)", value: newEditor(a.Endpoint)})
	f.fields = append(f.fields,
		field{key: "key_env", label: "Environment variable (optional legacy setup)", value: newEditor(a.KeyEnv)},
		field{key: "instructions", label: "Custom instructions (optional)", value: newEditor(a.Instructions)},
	)
}

func applyAIFields(a ai.Config, values map[string]string) ai.Config {
	for key, value := range values {
		switch key {
		case "name":
			a.Name = value
		case "provider":
			a.Provider = value
		case "endpoint":
			a.Endpoint = value
		case "key_env":
			a.KeyEnv = value
		case "instructions":
			a.Instructions = value
		}
	}
	return a
}

func (m *model) captureAIFields() {
	values := map[string]string{}
	for _, field := range m.form.fields {
		values[field.key] = strings.TrimSpace(field.value.text)
		if field.key == "api_key" {
			m.form.apiKey = values[field.key]
		}
	}
	m.form.agent = applyAIFields(m.form.agent, values)
}

func (m *model) changeAIProvider() {
	f := m.form
	m.captureAIFields()
	f.agent.KeyEnv, f.agent.CredentialRef, f.apiKey = "", "", ""
	f.agent.Model = ""
	// Switching vendors must not reuse another vendor's key or endpoint.
	f.agent.Endpoint = ""
	m.buildAIFields()
}

func (m *model) aiCredentialHint() string {
	if m.form.agent.CredentialRef != "" {
		return "API key saved locally · enter a new key to replace it"
	}
	if m.form.agent.KeyEnv != "" {
		return "Existing environment setup retained until a key is saved"
	}
	return "API keys stay in your local Plous data directory"
}

func (m *model) saveAppSettings(values map[string]string) {
	a := applyAIFields(m.form.agent, values).Connection()
	if err := a.SupportedProvider(); err != nil {
		m.status = err.Error()
		return
	}
	if a.Name == "" {
		base := providerLabel(a.Provider)
		a.Name = base
		for suffix := 2; ; suffix++ {
			used := false
			for _, config := range m.settings.Configs() {
				if config.Name != m.form.profile && strings.EqualFold(config.Name, a.Name) {
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
		key = m.form.apiKey
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
		if c.Name == m.form.profile {
			next.Providers[i] = a
			found = true
		}
	}
	if !found {
		next.Providers = append(next.Providers, a)
	}
	if len(next.Providers) == 1 || m.settings.Agent.Name == m.form.profile {
		if m.settings.Agent.Name == m.form.profile && m.settings.Agent.Provider == a.Provider {
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
	m.form = nil
	m.openSettings()
	m.status = "AI connection saved. Use /model in chat to switch."
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
	m.toolSetup = nil
	m.beginSettingsOverlay()
	m.page = "settings"
	m.cursor = 0
	m.command = nil
	m.detail = ""
	m.status = ""
}
func (m *model) cancelForm() {
	if m.toolSetup != nil {
		if m.form.kind == "kube" {
			m.backFromToolSetup()
		} else {
			m.form = nil
			m.status = ""
			m.page, m.cursor = "tools", 0
		}
		return
	}
	m.form = nil
	m.openSettings()
	m.status = "Edits canceled"
}
func (m *model) settingsItems() []string {
	items := []string{}
	for _, c := range m.settings.Configs() {
		items = append(items, c.Name)
	}
	items = append(items, "Add AI connection")
	if w := m.current(); w != nil {
		items = append(items, "Rename workspace", "Add tool")
		for _, c := range w.Connections {
			items = append(items, "Edit "+toolNames[c.Tool]+" / "+c.Name)
		}
	} else {
		items = append(items, "Create workspace")
	}
	return append(items, "Close")
}
func (m *model) updateSettings(key string) {
	items := m.settingsItems()
	count := len(m.settings.Configs())
	m.cursor = max(0, min(m.cursor, len(items)-1))
	switch key {
	case "up":
		m.cursor = max(0, m.cursor-1)
	case "down":
		m.cursor = min(len(items)-1, m.cursor+1)
	case "enter":
		switch {
		case m.cursor < count:
			m.beginAISettings(m.cursor)
		case m.cursor == count:
			m.beginAppSettings()
		case m.cursor == len(items)-1:
			m.closeSettings()
		case m.current() == nil:
			m.beginWorkspace()
		case m.cursor == count+1:
			m.beginRename()
		case m.cursor == count+2:
			m.beginToolSetup(false)
		default:
			c := m.current().Connections[m.cursor-count-3]
			m.beginConnection(c.Tool, &c)
		}
	}
}
func (m *model) settingsBody(width, height int) string {
	items := m.settingsItems()
	configs := m.settings.Configs()
	scope := "Current workspace (none selected)"
	if w := m.current(); w != nil {
		scope = "Current workspace: " + w.Name
	}
	lines := []string{sectionHeadingStyle.Render("Application"), muted.Render("AI connections · shared across workspaces"), ""}
	selectedRow := 0
	for i, item := range items {
		if i == len(configs)+1 {
			lines = append(lines, "", sectionHeadingStyle.Render(clipped(oneLine(scope), width)))
			if m.current() != nil {
				lines = append(lines, workspaceLocation(width)...)
			}
			lines = append(lines, "")
		}
		if i == len(items)-1 {
			lines = append(lines, "")
		}
		if i == m.cursor {
			selectedRow = len(lines)
		}
		label := item
		if i < len(configs) && configs[i].SupportedProvider() != nil {
			label += " · unavailable"
		}
		lines = append(lines, settingsChoiceLine(label, i == m.cursor, width))
		if i < len(configs) {
			vendor := providerLabel(configs[i].Provider)
			if !strings.EqualFold(strings.TrimSpace(configs[i].Name), vendor) {
				lines = append(lines, muted.Render(clipped("  "+oneLine(vendor), width)))
			}
		}
	}
	return scrollSelection(lines, selectedRow, height)
}

func workspaceLocation(width int) []string {
	label := muted.Background(surface)
	return []string{
		clipped(label.Render("Location  ")+shortcutStyle.Background(surface).Render("● Local")+label.Render("   ○ Remote (coming soon)"), width),
		label.Render(clipped("Remote workspaces and sharing need plous-server.", width)),
	}
}

func settingsChoiceLine(label string, active bool, width int) string {
	prefix := "  "
	style := canvasStyle
	if active {
		prefix = "› "
		style = shortcutStyle.Background(highlight)
	}
	return style.Width(width).Render(clipped(prefix+oneLine(label), width))
}

func (m *model) selectSettingsItem(label string) {
	for i, item := range m.settingsItems() {
		if item == label {
			m.cursor = i
			return
		}
	}
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
		m.openSettings()
		m.status = fmt.Sprintf("Removed AI connection %s", m.removeProfile)
		if err := m.store.PruneCredentials(); err != nil {
			m.status += " " + err.Error()
		}
	}
}
