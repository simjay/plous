package app

import (
	"slices"

	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) openWorkloadLogs() {
	e := m.explorer()
	origin := e.LogOrigin
	if e.Detail != nil && e.Tab != 4 {
		location := e.Location()
		origin = &location
	}
	e.LogOrigin = origin
	m.stopResourceAction()
	e.MergedLogs = true
	e.Container, e.Tab, e.DetailKey, e.LogKey = "", 4, "", ""
	e.Scroll, e.Previous, e.DetailValid = 0, false, false
	m.podLogViewer().Follow()
}

func (m *model) logContainers() []string {
	e := m.explorer()
	if e.MergedLogs && e.Resource.Key() != "pods" {
		names := slices.Clone(e.WorkloadLogStatus.Containers)
		if e.Container != "" && !slices.Contains(names, e.Container) {
			names = append(names, e.Container)
		}
		return append([]string{monitor.AllApplicationContainers}, names...)
	}
	if e.Detail == nil {
		return nil
	}
	names := kubernetes.Containers(*e.Detail)
	if e.Tab == 4 {
		return append([]string{monitor.AllApplicationContainers}, names...)
	}
	return names
}
