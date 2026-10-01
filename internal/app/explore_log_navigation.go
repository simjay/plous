package app

import (
	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (e *exploreState) location() exploreLocation {
	return exploreLocation{
		yamlOrigin: e.yamlOrigin, preference: e.preference, resource: e.resource, selected: e.selected,
		object: e.detail.DeepCopy(), tab: e.tab, scroll: e.scroll,
		relatedCursor: e.relatedCursor, container: e.container,
	}
}

func (e *exploreState) restoreLocation(location exploreLocation) {
	e.yamlOrigin = location.yamlOrigin
	e.preference, e.selected, e.detail, e.tab = location.preference, location.selected, location.object, location.tab
	e.scroll, e.relatedCursor, e.container = location.scroll, location.relatedCursor, location.container
	if location.resource.Name != "" {
		e.resource = location.resource
	} else if resource, ok := exactResource(e.resources, e.preference.Resource); ok {
		e.resource = resource
	}
	e.detailValid = false
}

func (m *model) openPodLogs(resource kubernetes.Resource, pod *unstructured.Unstructured, container string) {
	e := m.explorer()
	origin := e.logOrigin
	if e.detail != nil && e.tab != 4 {
		location := e.location()
		origin = &location
	}
	trail := e.trail
	m.stopResourceAction()
	if e.resource.Key() != "pods" || e.detail == nil || e.detail.GetUID() != pod.GetUID() {
		m.chooseExploreResource("pods")
		e.resource, e.detail = resource, pod.DeepCopy()
		e.preference.Namespace = pod.GetNamespace()
		e.selected = kubernetes.Identity(*pod)
		m.rememberExplore()
	}
	e.trail, e.logOrigin = trail, origin
	e.mergedLogs = container == ""
	e.container, e.tab, e.detailKey, e.logKey = container, 4, "", ""
	e.scroll, e.previous, e.detailValid = 0, false, false
	m.podLogViewer().Follow()
}

func (m *model) returnFromExploreLogs() bool {
	e := m.explorer()
	if e.tab != 4 || e.logOrigin == nil {
		return false
	}
	origin, trail := *e.logOrigin, e.trail
	m.clearExploreDetail()
	e.restoreLocation(origin)
	e.trail = trail
	m.rememberExplore()
	return true
}
