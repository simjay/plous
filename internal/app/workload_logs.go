package app

import (
	"slices"

	"github.com/simjay/plous/internal/kubernetes"
)

const allApplicationContainers = "All application containers"

func (a *resourceActionState) allPodsChoice() bool {
	return a.operation == "logs" && a.resource.Key() != "pods" && kubernetes.HasPodTargets(a.resource)
}

func (m *model) openWorkloadLogs() {
	e := m.explorer()
	origin := e.logOrigin
	if e.detail != nil && e.tab != 4 {
		location := e.location()
		origin = &location
	}
	e.logOrigin = origin
	m.stopResourceAction()
	e.mergedLogs = true
	e.container, e.tab, e.detailKey, e.logKey = "", 4, "", ""
	e.scroll, e.previous, e.detailValid = 0, false, false
	m.podLogViewer().Follow()
}

func (m *model) logContainers() []string {
	e := m.explorer()
	if e.mergedLogs && e.resource.Key() != "pods" {
		names := slices.Clone(e.workloadLogStatus.Containers)
		if e.container != "" && !slices.Contains(names, e.container) {
			names = append(names, e.container)
		}
		return append([]string{allApplicationContainers}, names...)
	}
	if e.detail == nil {
		return nil
	}
	names := kubernetes.Containers(*e.detail)
	if e.tab == 4 {
		return append([]string{allApplicationContainers}, names...)
	}
	return names
}
