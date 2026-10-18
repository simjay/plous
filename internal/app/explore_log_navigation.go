package app

import (
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (m *model) openPodLogs(resource kubecommon.Resource, pod *unstructured.Unstructured, container string) {
	e := m.explorer()
	origin := e.LogOrigin
	if e.Detail != nil && e.Tab != 4 {
		location := e.Location()
		origin = &location
	}
	trail := e.Trail
	m.stopResourceAction()
	if e.Resource.Key() != "pods" || e.Detail == nil || e.Detail.GetUID() != pod.GetUID() {
		m.chooseExploreResource("pods")
		e.Resource, e.Detail = resource, pod.DeepCopy()
		e.Preference.Namespace = pod.GetNamespace()
		e.Selected = kubernetes.Identity(*pod)
		m.rememberExplore()
	}
	e.Trail, e.LogOrigin = trail, origin
	e.MergedLogs = container == ""
	e.Container, e.Tab, e.DetailKey, e.LogKey = container, 4, "", ""
	e.Scroll, e.Previous, e.DetailValid = 0, false, false
	m.podLogViewer().Follow()
}

func (m *model) returnFromExploreLogs() bool {
	e := m.explorer()
	if e.Tab != 4 || e.LogOrigin == nil {
		return false
	}
	origin, trail := *e.LogOrigin, e.Trail
	m.clearExploreDetail()
	e.RestoreLocation(origin)
	e.Trail = trail
	m.rememberExplore()
	return true
}
