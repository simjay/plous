package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
)

func (m *model) helmFindKey(h *monitor.HelmState, msg tea.KeyPressMsg) bool {
	f, key := h.Find, msg.String()
	if !f.Reader.Searching && key != "esc" && key != "n" && key != "N" && key != "/" {
		if h.Detail != nil && h.Tab != 5 && monitor.ContainsString([]string{"up", "down", "pgup", "pgdown", "home", "end"}, key) {
			f.Reader.Key(key, msg.Text)
			return true
		}
		return false
	}
	m.shortcutPrefix = ""
	if key == "esc" {
		if h.Detail != nil && h.Tab != 5 {
			h.Subcursor = f.Reader.Top
		}
		h.Find = nil
		return true
	}
	f.Reader.Key(key, msg.Text)
	h.PositionFind()
	return true
}
