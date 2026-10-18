package app

func (m *model) openCodeView(row treeRow) bool {
	if s := m.savedCodeState(row.workspace, row.connection); s != nil && (s.Input != nil || s.Discard) {
		m.selectScope(row.workspace, s.View, row.connection)
		m.mode = 1
		m.status = "Finish or cancel the open source input first."
		m.focus = 1
		return false
	}
	m.selectScope(row.workspace, row.tool, row.connection)
	if m.active != row.workspace || m.connection != row.connection || m.tool != row.tool {
		return false
	}
	m.mode, m.focus = 1, 1
	s := m.coder()
	s.BrowserDetail = false
	s.Group, s.NodeGraph = "", ""
	s.Details = nil
	s.ReturnView = ""
	s.Preference.Browser = s.View
	s.SyncListScope()
	if row.resource == "render" {
		s.Details = nil
		s.Menu = ""
		s.ChooseSection(0)
		s.InspectorArea = 0
	}
	m.rememberCode(s)
	m.queueTreeSources(row)
	return true
}

func (m *model) openCodeDirectory(directory string) {
	m.tool = "files"
	s := m.coder()
	s.Filter, s.FileRoot = "", ""
	if directory != "." {
		s.FileRoot = directory
	}
	s.RevealFile(directory + "/_")
	s.Cursor = 0
	s.RememberSelection()
	m.rememberCode(s)
}
