package app

import (
	"github.com/simjay/plous/internal/app/monitor"
)

func (m *model) helmActive() bool {
	c := m.connectionView(m.selectedConnection())
	return m.mode == 1 && c != nil && c.Tool == "helm"
}

func (m *model) helmVisible() bool { return m.helmActive() && m.page == "" && m.form == nil }

func (m *model) helmer() *monitor.HelmState {
	c := m.currentConnection()
	if c == nil || c.Tool != "helm" {
		return nil
	}
	if back := m.overviewBack; back != nil && back.Helm != nil && back.Helm.Connection == c.ID && back.Helm.Workspace == m.active {
		return back.Helm
	}
	config := monitor.HelmConfig(c)
	key := m.active + "/" + c.ID + "/" + config.Fingerprint()
	if m.helmStates == nil {
		m.helmStates = map[string]*monitor.HelmState{}
	}
	if m.helmPreferences == nil {
		m.helmPreferences = map[string]monitor.HelmPreference{}
	}
	h := m.helmStates[key]
	if h == nil {
		p := m.helmPreferences[m.active+"/"+c.ID]
		if !monitor.ContainsString(monitor.HelmSections, p.Section) {
			p.Section = "Releases"
		}
		if p.Namespace == "" {
			p.Namespace = config.Namespace
		}
		h = &monitor.HelmState{Key: key, Workspace: m.active, Connection: c.ID, Config: config, Preference: p}
		m.helmStates[key] = h
	}
	return h
}

func (m *model) rememberHelm(h *monitor.HelmState) {
	if h.Temporary || m.overviewBack != nil {
		return
	}
	m.helmPreferences[h.Workspace+"/"+h.Connection] = h.Preference
	m.persist()
}

func (m *model) chooseHelmSection(section string) bool {
	h := m.helmer()
	if h == nil || !monitor.ContainsString(monitor.HelmSections, section) {
		return false
	}
	if h.Document != nil && !h.Document.ReadOnly {
		m.status = "Close or save the values editor before switching Helm sections."
		return false
	}
	if h.Busy {
		if h.Running == "execute" {
			m.status = "Wait for the Helm operation before changing this view."
			return false
		}
		if h.Cancel != nil {
			h.Cancel()
		}
		h.Run++
		h.Busy = false
		h.Cancel = nil
	}
	h.Pending = ""
	if h.Workflow != nil {
		h.Workflow.Active = false
	}
	h.SavePosition()
	h.Preference.Section = section
	h.Subcursor = 0
	h.Cursor = h.Positions[section]
	h.StopTimelineLoad()
	h.Timeline = nil
	h.Find = nil
	h.Detail = nil
	h.Menu = nil
	h.Document = nil
	h.Comparison = nil
	h.Confirm = false
	h.Input = nil
	h.StopNamespacePicker()
	h.Err = ""
	m.rememberHelm(h)
	return true
}

func (m *model) stopHelm() {
	for _, h := range m.helmStates {
		h.Stop()
	}
}
