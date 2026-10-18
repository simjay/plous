package app

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLogsEscapeRestoresEachOriginSectionBeforeTheList(t *testing.T) {
	for tab := range 4 {
		for _, key := range []string{"l", "p"} {
			if key == "p" && tab != 0 {
				continue
			}
			t.Run(fmt.Sprintf("section%d/%s", tab, key), func(t *testing.T) {
				m := exploreModel(t)
				e := m.explore
				e.Preference.Filter, e.Preference.Selector, e.Preference.Sort = "checkout", "app=checkout", 2
				e.Preference.Descending = true
				m.openExploreSelection(tab)
				e.Scroll, e.RelatedCursor = 17, 3
				preference, uid := e.Preference, e.Detail.GetUID()
				m.updateExplore(key)
				if e.Tab != 4 || e.LogOrigin == nil {
					t.Fatal("Logs did not capture the detail origin")
				}
				ctx, cancel := context.WithCancel(context.Background())
				e.Log = &monitor.ExploreLogRun{Ctx: ctx, Cancel: cancel}
				e.Scroll = 500
				m.updateExplore("esc")
				if ctx.Err() == nil || e.Log != nil || e.LogOrigin != nil {
					t.Fatal("leaving Logs kept the stream or its return state alive")
				}
				if e.Detail == nil || e.Detail.GetUID() != uid || e.Tab != tab || e.Scroll != 17 || e.RelatedCursor != 3 || e.Preference != preference || m.focus != 1 {
					t.Fatalf("Esc did not restore the exact originating detail: %#v", e)
				}
				m.updateExplore("esc")
				if e.Detail != nil || e.Preference != preference || m.focus != 1 {
					t.Fatal("second Esc did not return to the preserved resource list")
				}
			})
		}
	}
}

func TestWorkloadLogActionsReturnDirectlyToOriginalDetail(t *testing.T) {
	for _, entry := range []string{"l", "logs", "actions"} {
		t.Run(entry, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			pod := e.Collection.Objects[0].DeepCopy()
			workload := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "deployment-uid"}}}
			e.Resource, e.Preference.Resource = e.Resources[1], e.Resources[1].Key()
			e.Detail, e.DetailValid, e.Tab, e.Scroll, e.RelatedCursor = workload, true, 3, 12, 2
			e.Selected = kubernetes.Identity(*workload)
			e.Preference.Filter = "checkout"
			origin := e.Location()
			if entry == "l" {
				m.updateExplore(entry)
			} else {
				m.runExploreCommand(entry, true)
			}
			a := e.Action
			if a == nil {
				t.Fatal("Logs entry did not open target selection")
			}
			a.Pending, a.Operation, a.Pod, a.Container, a.TargetChecked = "", "logs", pod, "app", true
			m.chooseActionContainer()
			if e.Resource.Key() != "pods" || e.Detail.GetUID() != pod.GetUID() || e.Tab != 4 || e.Action != nil || len(e.Trail) != 0 {
				t.Fatal("workload Logs did not open as one action subview")
			}
			m.updateExplore("esc")
			if e.Resource.Key() != origin.Resource.Key() || e.Detail.GetUID() != workload.GetUID() || e.Tab != origin.Tab || e.Scroll != origin.Scroll || e.RelatedCursor != origin.RelatedCursor || e.Preference != origin.Preference || e.Selected != origin.Selected {
				t.Fatal("Logs Esc inserted a Pod Describe or lost workload browsing state")
			}
			m.updateExplore("esc")
			if e.Detail != nil || e.Resource.Key() != "deployments.apps" {
				t.Fatal("second Esc did not reach the workload list")
			}
		})
	}
}

func TestPodLogActionAndContainerSwitchKeepTheOriginalSection(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	e.DetailValid, e.Scroll = true, 13
	m.runExploreCommand("logs", true)
	a := e.Action
	if a == nil {
		t.Fatal("Logs command did not open the action")
	}
	a.Pending, a.Operation, a.Pod, a.Container, a.TargetChecked = "", "logs", e.Detail.DeepCopy(), "app", true
	m.chooseActionContainer()
	m.openPodLogs(e.Resource, e.Detail, "sidecar")
	m.updateExplore("esc")
	if e.Detail == nil || e.Tab != 2 || e.Scroll != 13 || e.Container != "app" || len(e.Trail) != 0 {
		t.Fatal("Pod log action or container switch replaced its origin")
	}
}

func TestPodAllContainersKeepsSelectionAcrossRefreshAndRestoresOrigin(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	_ = unstructured.SetNestedSlice(e.Detail.Object, []any{map[string]any{"name": "app"}, map[string]any{"name": "proxy"}}, "spec", "containers")
	e.DetailValid, e.Scroll = true, 13
	m.openResourceActions("logs")
	a := e.Action
	a.Pending, a.Operation, a.Pod, a.TargetChecked = "", "logs", e.Detail.DeepCopy(), true
	m.chooseActionPod()
	if a.Stage != "containers" || a.Containers[0] != monitor.AllApplicationContainers {
		t.Fatal("Pod log chooser has no All application containers option")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "targetaccess"})
	if !e.MergedLogs || e.Container != "" || e.Tab != 4 || len(m.logContainers()) != 3 {
		t.Fatal("All containers did not open the Pod's merged log view")
	}
	m.syncExploreDetail()
	run := e.DetailRun
	run.Cancel()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: e.Detail.DeepCopy()})
	if e.Container != "" {
		t.Fatal("detail refresh replaced All containers with an individual container")
	}
	m.syncExploreDetail()
	logs := e.Log
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.syncExploreDetail()
	if e.MergedLogs || e.Container != "proxy" || logs.Ctx.Err() == nil || e.Log == logs {
		t.Fatal("container switch kept merged sources alive or chose the wrong container")
	}
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !e.MergedLogs || e.Container != "" {
		t.Fatal("could not return to All containers")
	}
	m.updateExplore("esc")
	if e.Tab != 2 || e.Scroll != 13 || e.Container != "app" || e.MergedLogs {
		t.Fatal("container selection lost the original detail or selected container")
	}
}

func TestLogControlsAndSiblingSectionsDoNotAccumulateHistory(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(1)
	e.Scroll = 9
	m.updateExplore("l")
	for _, key := range []string{"l", "p", "p", "w", "t", "space", "space"} {
		m.updateExplore(key)
	}
	m.updateExplore("esc")
	if e.Tab != 1 || e.Scroll != 9 || len(e.Trail) != 0 {
		t.Fatal("log controls replaced or accumulated the return origin")
	}
	for range 3 {
		for _, key := range []string{"d", "y", "e", "r", "]", "["} {
			m.updateExplore(key)
			if e.Tab == 4 || e.LogOrigin != nil || len(e.Trail) != 0 {
				t.Fatal("sibling navigation created action history or entered Logs")
			}
		}
	}
	m.updateExplore("l")
	m.updateExplore("y")
	if e.Tab != 1 || e.LogOrigin != nil {
		t.Fatal("explicit sibling navigation did not leave Logs")
	}
	m.updateExplore("esc")
	if e.Detail == nil || e.Tab != 3 {
		t.Fatal("YAML did not return to Related after leaving Logs")
	}
	m.updateExplore("esc")
	if e.Detail != nil {
		t.Fatal("Related did not return to the list")
	}
}

func TestLogsBackPreservesRelatedTrailAndCapturedIdentity(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(3)
	parent := e.Location()
	e.Trail = []monitor.ExploreLocation{parent}
	e.Detail = e.Collection.Objects[1].DeepCopy()
	e.Tab, e.Scroll = 2, 8
	childUID := e.Detail.GetUID()
	m.updateExplore("l")
	e.Detail.SetUID("replacement")
	m.updateExplore("esc")
	if e.Detail.GetUID() != childUID || e.Tab != 2 || e.Scroll != 8 || len(e.Trail) != 1 {
		t.Fatal("Logs lost the captured child or consumed its Related parent")
	}
	m.updateExplore("esc")
	if e.Detail.GetUID() != parent.Object.GetUID() || e.Tab != parent.Tab || len(e.Trail) != 0 {
		t.Fatal("next Esc did not return to the Related parent")
	}
}

func TestLogsEscapeRespectsTransientLayersAndHiddenSidebar(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.Scroll = 11
	m.updateExplore("l")
	typeText(m, "b")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.shortcutPrefix != "" {
		t.Fatal("prefix Esc also left Logs")
	}
	typeText(m, ":")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.command != nil {
		t.Fatal("command Esc also left Logs")
	}
	typeText(m, "/")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || m.searches[1] != nil {
		t.Fatal("search Esc also left Logs")
	}
	m.updateExplore("o")
	press(t, m, "esc")
	if e.Tab != 4 || e.LogOrigin == nil || e.Picker != "" {
		t.Fatal("container picker Esc also left Logs")
	}
	m.leftHidden = true
	press(t, m, "esc")
	if e.Detail == nil || e.Tab != 0 || e.Scroll != 11 || m.focus != 1 {
		t.Fatal("Logs Esc did not restore detail while the sidebar was hidden")
	}
	press(t, m, "esc")
	if e.Detail != nil || m.focus != 1 || !m.leftHidden {
		t.Fatal("detail Esc did not return to the table alone")
	}
	press(t, m, "esc")
	if m.focus != 0 || m.leftHidden {
		t.Fatal("root Esc did not reveal the resource sidebar")
	}
}

func TestCancellingResourceActionRetainsOriginSection(t *testing.T) {
	for _, stage := range []string{"menu", "done", "confirm", "scale", "edit", "pods", "containers", "execcommand"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			m.openExploreSelection(2)
			e.DetailValid, e.Scroll = true, 14
			m.openResourceActions("")
			e.Action.Stage, e.Action.Pending = stage, ""
			for range 4 {
				if e.Action != nil {
					m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				}
			}
			if e.Action != nil || e.Detail == nil || e.Tab != 2 || e.Scroll != 14 || e.Detail.GetUID() != "one" {
				t.Fatal("cancelling an action lost its originating detail")
			}
		})
	}
}
