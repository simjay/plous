package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/ui"
	"time"
)

var loadingFrames = []string{
	"●····", "·●···", "··●··", "···●·", "····●",
	"····●", "···●·", "··●··", "·●···", "●····",
}

type loadingTickMsg struct{}

func (m *model) loading(label string) string {
	frame := loadingFrames[m.loadingFrame%len(loadingFrames)]
	return frame + " " + ui.OneLine(label)
}

func (m *model) loadingVisible() bool {
	if m.width < minWidth || m.height < minHeight || m.exploreClosed || m.helpVisible() || (m.settingsOverlay != nil && m.focus == 1) || m.resourceTerminalVisible() {
		return false
	}
	if m.resourceActionVisible() {
		return m.explore.Action.Busy
	}
	if m.chat().Restore != nil && !m.rightHidden && !m.chat().History && !m.chat().Models {
		return true
	}
	if m.run != nil && m.run.Chat == m.chat() && !m.rightHidden && !m.chat().History && !m.chat().Models {
		return true
	}
	if m.mode == 2 && m.page == "" && (m.activityUI.Pending || m.activityUI.Load != nil) {
		return true
	}
	if m.codeVisible() {
		return m.coder().Busy
	}

	if m.gitOpsVisible() {
		v := m.gitOpsCurrent()
		return !v.Cluster.Loaded || v.State.Loading || v.Namespaces != nil && !v.Namespaces.Loaded
	}
	if m.helmVisible() {
		h := m.helmer()
		return h.Busy || h.Namespaces != nil && !h.Namespaces.Manual && !h.Namespaces.Loaded
	}
	if !m.exploreVisible() || m.explore == nil {
		return false
	}
	e := m.explore
	if e.Picker == "namespace" && e.Namespaces != nil && !e.Namespaces.Manual && !e.Namespaces.Loaded {
		return true
	}
	if e.Connecting {
		return true
	}
	if e.Picker != "" {
		return false
	}
	if e.Detail != nil {
		return e.DetailRun != nil || (e.Tab == 4 && e.Log != nil && !e.LogConnected && e.LogError == "")
	}
	return e.Stream != nil && !e.Listed && e.Err == ""
}

func (m *model) syncLoading() tea.Cmd {
	if m.loadingTickPending || !m.loadingVisible() {
		return nil
	}
	m.loadingTickPending = true
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return loadingTickMsg{} })
}
