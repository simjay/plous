package app

import (
	tea "charm.land/bubbletea/v2"

	appcode "github.com/simjay/plous/internal/app/code"

	"github.com/simjay/plous/internal/workspace"
)

func (m *model) codeActive() bool {
	c := m.selectedConnection()
	return m.mode == 1 && c != nil && sourceConnection(c.Tool) && (appcode.IsBrowser(m.tool) || c.Tool == "code" || m.tool == "files" || m.tool == "manifests" || m.tool == "tfcode")
}

func (m *model) codeVisible() bool {
	return m.codeActive() && m.page == "" && m.form == nil && !(m.explore != nil && m.explore.Picker == "connections")
}

func (m *model) coder() *appcode.CodeState {
	c := m.savedConnection()
	if c == nil || !sourceConnection(c.Tool) {
		return nil
	}
	s := m.codeStateFor(m.active, c)
	s.View = m.tool
	s.SyncListScope()
	return s
}

func (m *model) codeStateFor(workspace string, c *workspace.Connection) *appcode.CodeState {
	var legacy *appcode.LegacySelection
	if old, ok := m.kustomizePreferences[workspace+"/"+c.ID]; ok {
		legacy = &appcode.LegacySelection{Open: old.Open, Revision: old.Revision}
	}
	config := appcode.ConfigForConnection(c)
	if m.store != nil {
		config.ScanRules = m.store.SourceIgnore
	}
	return m.codeUI.StateFor(workspace, c.ID, config, legacy)
}

func (m *model) savedCodeState(workspace, id string) *appcode.CodeState {
	for _, w := range m.treeWorkspaces() {
		if w.ID != workspace {
			continue
		}
		for _, c := range w.Connections {
			if c.ID == id && sourceConnection(c.Tool) {
				return m.codeUI.States[workspace+"/"+id+"/"+appcode.ConfigForConnection(&c).Fingerprint()]
			}
		}
	}
	return nil
}

func (m *model) codeScanStatus(workspace, id string) string {
	s := m.savedCodeState(workspace, id)
	if s != nil && s.Rescan {
		return "scan queued"
	}
	if s == nil || !s.LoadAttempted {
		if workspace == m.active {
			return "scan queued"
		}
		return "not scanned"
	}
	if s.Busy && s.Running == "load" {
		return "scanning"
	}
	if s.ScanFailed {
		return "scan failed"
	}
	if s.ScanCancelled {
		return "scan cancelled"
	}
	return ""
}

func (m *model) codeViewScanStatus(workspace, id, view string) string {
	status := m.codeScanStatus(workspace, id)
	if view == "files" || status == "" {
		return status
	}
	s := m.savedCodeState(workspace, id)
	if s == nil || s.Inventory == nil {
		return ""
	}
	for _, item := range s.Inventory.Catalog {
		if item.Technology != view {
			continue
		}
		switch status {
		case "scanning", "scan queued":
			return "refreshing"
		case "scan failed", "scan cancelled":
			return "stale"
		}
	}
	return ""
}

func (m *model) rememberCode(s *appcode.CodeState) {
	m.codeUI.Remember(s)
	m.persist()
}

func (m *model) stopCode() { m.codeUI.Stop() }

func (m *model) syncCode() tea.Cmd {
	if m.exploreClosed {
		return nil
	}
	loading := false
	for _, s := range m.codeUI.States {
		if m.savedCodeState(s.Workspace, s.Connection) != s {
			if s.Busy || s.Pending != "" {
				s.Stop()
			}
			continue
		}
		loading = loading || s.Busy && s.Running == "load"
	}
	if m.codeActive() {
		s := m.coder()
		if s.Pending != "" && !s.Busy {
			return m.startCode(s)
		}
		if m.codeVisible() && !m.helpVisible() && m.settingsOverlay == nil && m.tool == "manifests" && !s.Rescan && !s.ScanFailed && !s.ScanCancelled &&
			!(s.ReturnView == "code-kustomize" && s.Preference.Section == 1) &&
			!s.Busy && s.Document == nil && s.Review == nil && s.Menu == "" && s.Input == nil && (s.Preference.Section == 1 || s.Preference.Section == 2) && s.NeedsPreview() {
			s.Pending = "preview"
			return m.startCode(s)
		}
	}
	// Discovery belongs to the workspace, independent of the selected view.
	// Scan one saved source at a time and never retry a failed scan implicitly.
	if !loading {
		for _, w := range m.treeWorkspaces() {
			for i := range w.Connections {
				c := &w.Connections[i]
				if !sourceConnection(c.Tool) {
					continue
				}
				s := m.savedCodeState(w.ID, c.ID)
				if w.ID == m.active {
					s = m.codeStateFor(w.ID, c)
				}
				if s != nil && !s.Busy && s.Pending == "" && (s.Rescan || w.ID == m.active && !s.LoadAttempted && s.Inventory == nil) {
					s.Pending = "load"
					return m.startCode(s)
				}
			}
		}
	}
	return nil
}

func (m *model) startCode(s *appcode.CodeState) tea.Cmd { return s.Start() }

func (m *model) updateCodeResult(r appcode.CodeResult) tea.Cmd {
	s := r.State
	if s == nil || m.savedCodeState(s.Workspace, s.Connection) != s {
		return nil
	}
	if s.Apply(r) {
		if r.Kind == "paths" && r.Inventory != nil {
			_ = m.store.AddCloser(r.Inventory.Source.Cleanup())
			return r.Continue
		}
		m.rememberCode(s)
	}
	return nil
}
