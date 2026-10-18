package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"time"
)

func (m *model) chartsAvailable() bool {
	e := m.explore
	return m.exploreVisible() && e != nil && e.Usage != nil && e.Action == nil && e.Terminal == nil && (e.Detail == nil || e.Tab == 0 && e.DetailValid)
}

func (m *model) chartsActive() bool { return m.chartsAvailable() && m.explore.Usage.Charts.Inspect }

func (m *model) chartCommand(command string) bool {
	switch command {
	case "charts", "chartrange", "chartlatest":
	default:
		return false
	}
	if m.form != nil || m.settingsOverlay != nil || m.explore != nil && (m.explore.Input != nil || m.explore.Picker != "") {
		m.status = "Finish or cancel the open input first."
		return true
	}
	if !m.chartsAvailable() {
		m.status = "Open Pod or Node lists, or a resource's usage in Describe, to use charts."
		return true
	}
	m.closeHelp()
	m.focus, m.status = 1, ""
	m.status = m.explore.Usage.ChartCommand(command, m.explore.Detail != nil, time.Now())
	return true
}

func (m *model) updateChartKey(key string) bool {
	if !m.chartsActive() {
		return false
	}
	if key == "/" {
		m.explore.Usage.Charts.Inspect = false
		return false
	}
	width := max(1, m.panelWidths()[1]-4)
	layout := m.explore.ChartLayout(width, m.exploreContentHeight(width, m.mainBodyHeight()), m.viewExpanded(), time.Now())
	m.explore.Usage.ChartKey(key, m.explore.Detail != nil, layout.Height, layout.Limit, time.Now())
	return true
}

func (m *model) chartSampleClick(value string) {
	if !m.chartsAvailable() {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, value)
	if err == nil && m.explore.Usage.SelectSample(at, time.Now()) {
		m.explore.Usage.Charts.Inspect = true
		m.explore.Usage.Charts.Scroll = 0
	}
}

func (m *model) chartsBody(width, height int) string {
	return m.explore.ChartLayout(width, height, m.viewExpanded(), time.Now()).View(m.explore.Usage.Charts.Scroll)
}

func (m *model) mainBodyHeight() int {
	height := m.height - 6
	if m.command != nil {
		height -= 3
	}
	return max(0, height)
}

func (m *model) exploreContentHeight(width, height int) int {
	gap := 1
	if height >= 14 {
		gap = 2
	}
	return max(0, height-lipgloss.Height(m.exploreHeader(width))-gap+1)
}
