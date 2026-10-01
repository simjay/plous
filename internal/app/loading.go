package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

var loadingFrames = []string{
	"●····", "·●···", "··●··", "···●·", "····●",
	"····●", "···●·", "··●··", "·●···", "●····",
}

type loadingTickMsg struct{}

func (m *model) loading(label string) string {
	frame := loadingFrames[m.loadingFrame%len(loadingFrames)]
	return frame + " " + oneLine(label)
}

func (m *model) loadingVisible() bool {
	if m.width < minWidth || m.height < minHeight || m.exploreClosed || m.help != nil || (m.settingsOverlay != nil && m.focus == 1) || m.resourceTerminalVisible() {
		return false
	}
	if m.resourceActionVisible() {
		return m.explore.action.busy
	}
	if m.run != nil && m.run.chat == m.chat() && !m.rightHidden && !m.chat().history && !m.chat().models {
		return true
	}
	if !m.exploreVisible() || m.explore == nil {
		return false
	}
	e := m.explore
	if e.picker == "namespace" && e.namespaces != nil && !e.namespaces.manual && !e.namespaces.loaded {
		return true
	}
	if e.connecting {
		return true
	}
	if e.picker != "" {
		return false
	}
	if e.detail != nil {
		return e.detailRun != nil || (e.tab == 4 && e.log != nil && !e.logConnected && e.logError == "")
	}
	return e.stream != nil && !e.listed && e.err == ""
}

func (m *model) syncLoading() tea.Cmd {
	if m.loadingTickPending || !m.loadingVisible() {
		return nil
	}
	m.loadingTickPending = true
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return loadingTickMsg{} })
}
