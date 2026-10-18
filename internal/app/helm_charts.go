package app

import (
	"github.com/simjay/plous/internal/app/monitor"
)

func (m *model) helmChartCardKey(h *monitor.HelmState, key string) bool {
	columns := monitor.HelmChartColumnsForWidth(max(1, m.panelWidths()[1]-4))
	start := 0
	groups := h.ChartGroups()
	for g, group := range groups {
		if h.Cursor >= start+len(group.Releases) {
			start += len(group.Releases)
			continue
		}
		local := h.Cursor - start
		col := local % columns
		switch key {
		case "left":
			if col > 0 {
				h.Cursor--
			}
		case "right":
			if col < columns-1 && local+1 < len(group.Releases) {
				h.Cursor++
			}
		case "up", "k":
			if local >= columns {
				h.Cursor -= columns
			} else if g > 0 {
				previous := len(groups[g-1].Releases)
				row := (previous - 1) / columns * columns
				h.Cursor = start - previous + min(previous-1, row+col)
			}
		case "down", "j":
			if local/columns < (len(group.Releases)-1)/columns {
				h.Cursor = start + min(len(group.Releases)-1, local+columns)
			} else if g+1 < len(groups) {
				h.Cursor = start + len(group.Releases) + min(col, len(groups[g+1].Releases)-1)
			}
		default:
			return false
		}
		h.Preference.Selected = h.CollectionSelectionKey()
		m.rememberHelm(h)
		return true
	}
	return false
}
