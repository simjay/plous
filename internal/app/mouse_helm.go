package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	"strconv"
)

func (m *model) mouseHelmItem(kind, value string, double bool) tea.Cmd {
	if !m.helmVisible() {
		return nil
	}
	h := m.helmer()
	if h.Busy || h.Confirm || h.Discard || h.Menu != nil || h.Document != nil || h.Comparison != nil {
		return nil
	}
	switch kind {
	case "helmrow":
		index := 0
		for _, group := range h.ChartGroups() {
			for _, release := range group.Releases {
				if release.Key() == value {
					h.Cursor = index
					h.Preference.Selected = release.Key()
					m.rememberHelm(h)
					if double {
						m.updateHelm("enter")
					}
					return nil
				}
				index++
			}
		}
	case "helmrevision":
		if h.Timeline == nil {
			return nil
		}
		for i, r := range h.Timeline.Revisions {
			if strconv.Itoa(r.Revision) == value {
				h.Timeline.Cursor, h.Timeline.Scroll = i, 0
				if double {
					m.helmTimelineKey(h, "enter")
				}
				break
			}
		}
	case "helmresource":
		rs, _ := h.InspectorResources()
		for i, r := range rs {
			if r.Kind+"/"+r.Namespace+"/"+r.Name == value {
				h.Subcursor = i
				if double {
					m.updateHelm("enter")
				}
				break
			}
		}
	case "helmtab":
		i, err := strconv.Atoi(value)
		if err == nil {
			for _, tab := range monitor.HelmInspectorTabs {
				if tab == i {
					h.SaveInspectorPosition()
					h.Tab = i
					h.Subcursor = h.InspectorPositions[h.InspectorPositionKey()]
					h.Find = nil
					break
				}
			}
		}
	}
	return nil
}
