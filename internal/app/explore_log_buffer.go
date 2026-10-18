package app

import (
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) resetExploreLogs() {
	e := m.explorer()
	e.LogError = ""
	e.WorkloadLogStatus = kubernetes.WorkloadLogStatus{}
	e.LogConnected = false
	m.podLogViewer().Reset()
	m.podLogViewer().SetFormatter(monitor.PodLogFormatter(e.LogTimestamps, e.MergedLogs))
}

func (m *model) appendExploreLogs(text string) {
	text = ui.Plain(text)
	if text == "" {
		return
	}
	m.explorer().LogConnected = true
	m.podLogViewer().Append(text)
}
