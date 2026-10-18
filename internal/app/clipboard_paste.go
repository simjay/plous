package app

import "slices"

func (m *model) pasteText(text string) {
	m.mouse.reset()
	m.shortcutPrefix = ""
	if m.nativeAgentCaptured() {
		m.nativeAgent.Follow()
		m.nativeAgent.Screen.Paste(text)
		return
	}
	owner := m.activeInteraction().owner
	if owner == interactionHelp || owner == interactionCommand || owner == interactionUnavailable {
		return
	}
	if owner == interactionSources {
		if s := m.chatSources; s.ShowReply && s.Reply.Reader.Searching && m.command == nil {
			s.Reply.Reader.Paste(text)
		}
		if d := m.chatSources.Document; d != nil && d.Searching && m.command == nil {
			d.Paste(text)
		}
		return
	}
	if m.focus == 1 && m.overviewVisible() {
		if d := m.overviewCurrent().State.MetricDetail; d != nil {
			d.Reader().Paste(text)
		} else if input := m.overviewCurrent().State.Input; input != nil {
			input.Insert(text, false)
			m.overviewCurrent().State.Cursor = [4]int{}
		}
		return
	}
	if m.focus == 1 && m.presentedDocumentVisible() {
		m.uiDocument.model.Paste(text)
		return
	}
	if m.codePaste(text) {
		return
	}

	if m.gitOpsPaste(text) {
		return
	}
	if m.helmPaste(text) {
		return
	}
	if m.workspacePaste(text) {
		return
	}
	if m.resourceTerminalVisible() && m.explore.Terminal.Captured {
		m.explore.Terminal.Emulator.Paste(text)
		return
	}
	if m.resourceActionVisible() {
		a := m.explore.Action
		if !a.Busy && (slices.Contains([]string{"scale", "export"}, a.Stage) || a.Stage == "execcommand" && a.ExecEditing) {
			a.Input.Insert(text, a.Stage == "edit")
		}
		return
	}
	// Handle paste before key dispatch so pasted commands stay literal.
	if search := m.activePanelSearch(m.focus); search != nil {
		if !search.finished {
			search.input.Insert(text, false)
			m.searchChanged()
		}
	} else if m.focus == 2 && m.chatEditing && !m.chat().History && !m.chat().Models {
		m.draft().Insert(text, true)
		m.draft().Literal = true
		m.persist()
	} else if m.focus == 1 && m.form != nil {
		before := m.form.Fields[0].Value.Text
		m.form.Paste(text)
		if m.form.Kind == "kube" && m.form.Fields[0].Value.Text != before {
			m.invalidateContexts()
		}
	} else if m.exploreEditing() && m.explore.Input != nil {
		m.explore.Input.Insert(text, false)
	}
}
