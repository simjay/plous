package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/activity"
)

func (m *model) stopActivity() { m.activityUI.Stop() }
func (m *model) loadHistory()  { m.activityUI.Reload(m.active, m.current() != nil && m.store != nil) }
func (m *model) syncActivity() tea.Cmd {
	if m.mode != 2 || m.page != "" || m.form != nil || m.store == nil || m.exploreClosed {
		return nil
	}
	return m.activityUI.Start(m.store)
}

func (m *model) applyActivity(result activity.ActivityResult) {
	if !m.activityUI.Matches(result, m.active) {
		return
	}
	selected := ""
	entries := m.activityEntries()
	if pos := m.activityPosition(); pos >= 0 && pos < len(entries) {
		selected = entries[pos].Identity()
	}
	if !m.activityUI.Apply(result) {
		return
	}
	m.rebaseActivitySearch(m.searches[1])
	m.rebaseActivitySearch(m.activityListSearch)
	m.setActivityPosition(min(max(0, m.activityPosition()), max(0, len(m.activityEntries())-1)))
	for i, entry := range m.activityEntries() {
		if entry.Identity() == selected {
			m.setActivityPosition(i)
			break
		}
	}
}
