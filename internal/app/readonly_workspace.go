package app

import tea "charm.land/bubbletea/v2"

func (m *model) readOnlyWorkspaceKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	key := msg.String()
	if m.activityReviewVisible() || m.yamlDocumentActive() || m.valueDocumentActive() {
		searching := m.activityReviewSearching() || m.yamlDocumentSearching()
		if m.valueDocumentActive() {
			searching = m.explore.action.valueDocument.Searching
		}
		if !searching {
			if key == ":" || key == "tab" || key == "shift+tab" {
				return false, nil
			}
			if m.updateShortcut(msg) {
				m.persist()
				return true, nil
			}
		}
		m.shortcutPrefix = ""
		m.status = ""
		if m.activityReviewVisible() {
			return m.activityReviewKey(msg)
		}
		if m.valueDocumentActive() {
			return true, m.valueDocumentKey(msg)
		}
		return m.yamlDocumentKey(msg)
	}
	if m.activityDetailActive() && m.searches[1] == nil && key == "d" {
		m.shortcutPrefix = ""
		m.status = ""
		return m.openActivityReview(), nil
	}
	return false, nil
}
