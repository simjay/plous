package app

import "github.com/simjay/plous/internal/kubernetes"

const exploreLogBufferLimit = 128 * 1024

func (m *model) resetExploreLogs() {
	e := m.explorer()
	e.logError = ""
	e.workloadLogStatus = kubernetes.WorkloadLogStatus{}
	e.logConnected = false
	m.podLogViewer().Reset()
	m.podLogViewer().SetFormatter(podLogFormatter(e.logTimestamps, e.mergedLogs))
}

func (m *model) appendExploreLogs(text string) {
	text = plain(text)
	if text == "" {
		return
	}
	m.explorer().logConnected = true
	m.podLogViewer().Append(text)
}
