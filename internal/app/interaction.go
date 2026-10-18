package app

import tea "charm.land/bubbletea/v2"

type interactionOwner uint8

const (
	interactionNavigation interactionOwner = iota
	interactionHelp
	interactionSources
	interactionCommand
	interactionSettings
	interactionForm
	interactionChatInput
	interactionFeature
	interactionCharts
	interactionTerminal
	interactionUnavailable
)

// Composition resolves ownership; feature controllers retain their local state.
// Panel resolution also supplies the underlying context shown by Help.
type interaction struct {
	owner        interactionOwner
	panelSearch  bool
	localEscape  bool
	resumeSearch func()
}

func (m *model) panelInteraction(panel int) interaction {
	if panel == 0 {
		return interaction{owner: interactionNavigation, panelSearch: true}
	}
	if panel == 2 {
		if m.externalAgentSelected() {
			if m.nativeAgent != nil && m.nativeAgent.Captured && !m.nativeAgent.Finished {
				return interaction{owner: interactionTerminal}
			}
			return interaction{owner: interactionNavigation}
		}
		if m.chat().Models || m.chatEditing && !m.chat().History {
			return interaction{owner: interactionChatInput}
		}
		return interaction{owner: interactionNavigation, panelSearch: true}
	}
	if m.form != nil {
		return interaction{owner: interactionForm, localEscape: true}
	}
	if m.presentedDocumentVisible() {
		return interaction{owner: interactionFeature, localEscape: true}
	}
	if m.settingsOverlay != nil || m.page != "" {
		return interaction{owner: interactionSettings, localEscape: true}
	}
	if m.mainTerminalCaptured() {
		return interaction{owner: interactionTerminal, localEscape: true}
	}
	if m.overviewVisible() {
		v := m.overviewRead()
		if v == nil {
			return interaction{owner: interactionNavigation}
		}
		if v.State.MetricDetail != nil {
			return interaction{owner: interactionFeature, localEscape: true}
		}
		return interaction{owner: interactionNavigation, localEscape: v.State.Input != nil || v.State.Query != "" || v.State.Expanded || v.State.Group != "" || v.Inspecting}
	}
	if m.codeOverviewVisible() {
		return interaction{owner: interactionNavigation, panelSearch: true}
	}
	if m.codeVisible() || m.helmVisible() || m.gitOpsVisible() || m.documentActive() || m.valueDocumentActive() || m.yamlDocumentActive() || m.activityReviewVisible() || m.logWorkspaceActive() || m.resourceTerminalVisible() {
		return interaction{owner: interactionFeature}
	}
	if m.exploreVisible() {
		e := m.explore
		if e.Input != nil || e.Picker != "" || e.Action != nil {
			return interaction{owner: interactionFeature, localEscape: true}
		}
		if m.chartsActive() {
			return interaction{owner: interactionCharts, localEscape: true, resumeSearch: func() { e.Usage.Charts.Inspect = false }}
		}
		return interaction{owner: interactionNavigation, panelSearch: e.Detail != nil}
	}
	return interaction{owner: interactionNavigation, panelSearch: true}
}

func (m *model) activeInteraction() interaction {
	if m.commandFocused() {
		return interaction{owner: interactionCommand}
	}
	if m.helpVisible() {
		return interaction{owner: interactionHelp}
	}
	if m.chatSourcesVisible() {
		return interaction{owner: interactionSources}
	}
	if m.mainTerminalCaptured() && m.focus == 1 {
		return m.panelInteraction(1)
	}
	if m.tooSmall() {
		return interaction{owner: interactionUnavailable}
	}
	return m.panelInteraction(m.focus)
}

// Keyboard and action buttons enter feature handlers through the same boundary.
func (m *model) featureKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	owner := m.activeInteraction().owner
	if owner != interactionFeature && owner != interactionTerminal && owner != interactionNavigation {
		return false, nil
	}
	if handled, cmd := m.presentedDocumentKey(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.overviewKey(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.codeKey(msg); handled {
		return true, cmd
	}
	if m.gitOpsInputKey(msg) {
		return true, nil
	}
	if handled, cmd := m.helmKey(msg); handled {
		return true, cmd
	}
	return m.workspaceKey(msg)
}
