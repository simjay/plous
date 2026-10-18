package app

import (
	appcode "github.com/simjay/plous/internal/app/code"

	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) codeBody(width, height int) string {
	s := m.coder()
	var actions []textview.ActionGroup
	if s.Document != nil {
		actions = m.codeDocumentActions()
	}
	chrome := appcode.Presentation{ConnectionName: m.currentConnection().Name, Expand: m.expandAction(), DocumentActions: actions, Loading: m.loading}
	if m.codeOverviewVisible() && m.searches[1] != nil {
		chrome.FindText = func(text string, width, height, offset int) string {
			return m.findText(1, text, width, height, offset)
		}
	}
	return s.Body(width, height, chrome)
}

func (m *model) codeOverviewVisible() bool {
	if !m.codeVisible() {
		return false
	}
	s := m.savedCodeState(m.active, m.connection)
	return s != nil && m.tool == "code-kustomize" && s.BrowserDetail && s.Inventory != nil && s.Document == nil && s.Review == nil && s.Menu == "" && s.Input == nil
}
