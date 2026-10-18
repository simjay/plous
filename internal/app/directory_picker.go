package app

import (
	tea "charm.land/bubbletea/v2"

	appsettings "github.com/simjay/plous/internal/app/settings"

	"strconv"
)

func (m *model) openDirectoryPicker() { m.form.OpenDirectory() }

func (m *model) syncDirectoryPicker() tea.Cmd { return m.form.SyncDirectory() }

func (m *model) applyDirectoryResult(r appsettings.DirectoryResult) { m.form.ApplyDirectoryResult(r) }

func (m *model) updateDirectoryPicker(msg tea.KeyPressMsg) { m.form.UpdateDirectory(msg) }

func (m *model) directoryPickerBody(width, height int) string {
	return m.form.DirectoryBody(width, height)
}

func (m *model) mouseDirectory(value string, double bool) tea.Cmd {
	if m.form == nil || m.form.Directory == nil {
		return nil
	}
	p := m.form.Directory
	if p.Loading {
		return nil
	}
	if value == "path" {
		p.Editing = true
		return nil
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= len(p.Entries)+2 {
		return nil
	}
	p.Editing, p.Cursor = false, index
	if index < 2 || double {
		m.updateDirectoryPicker(localMouseKey("enter"))
	}
	return nil
}
