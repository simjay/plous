package app

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/kubernetes"
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
				e.preference.Filter, e.preference.Selector, e.preference.Sort = "checkout", "app=checkout", 2
				e.preference.Descending = true
				m.openExploreSelection(tab)
				e.scroll, e.relatedCursor = 17, 3
				preference, uid := e.preference, e.detail.GetUID()
				m.updateExplore(key)
				if e.tab != 4 || e.logOrigin == nil {
					t.Fatal("Logs did not capture the detail origin")
				}
				ctx, cancel := context.WithCancel(context.Background())
				e.log = &exploreLogRun{ctx: ctx, cancel: cancel}
				e.scroll = 500
				m.updateExplore("esc")
				if ctx.Err() == nil || e.log != nil || e.logOrigin != nil {
					t.Fatal("leaving Logs kept the stream or its return state alive")
				}
				if e.detail == nil || e.detail.GetUID() != uid || e.tab != tab || e.scroll != 17 || e.relatedCursor != 3 || e.preference != preference || m.focus != 1 {
					t.Fatalf("Esc did not restore the exact originating detail: %#v", e)
				}
				m.updateExplore("esc")
				if e.detail != nil || e.preference != preference || m.focus != 1 {
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
			pod := e.collection.Objects[0].DeepCopy()
			workload := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "deployment-uid"}}}
			e.resource, e.preference.Resource = e.resources[1], e.resources[1].Key()
			e.detail, e.detailValid, e.tab, e.scroll, e.relatedCursor = workload, true, 3, 12, 2
			e.selected = kubernetes.Identity(*workload)
			e.preference.Filter = "checkout"
			origin := e.location()
			if entry == "l" {
				m.updateExplore(entry)
			} else {
				m.runExploreCommand(entry, true)
			}
			a := e.action
			if a == nil {
				t.Fatal("Logs entry did not open target selection")
			}
			a.pending, a.operation, a.pod, a.container, a.targetChecked = "", "logs", pod, "app", true
			m.chooseActionContainer()
			if e.resource.Key() != "pods" || e.detail.GetUID() != pod.GetUID() || e.tab != 4 || e.action != nil || len(e.trail) != 0 {
				t.Fatal("workload Logs did not open as one action subview")
			}
			m.updateExplore("esc")
			if e.resource.Key() != origin.resource.Key() || e.detail.GetUID() != workload.GetUID() || e.tab != origin.tab || e.scroll != origin.scroll || e.relatedCursor != origin.relatedCursor || e.preference != origin.preference || e.selected != origin.selected {
				t.Fatal("Logs Esc inserted a Pod Describe or lost workload browsing state")
			}
			m.updateExplore("esc")
			if e.detail != nil || e.resource.Key() != "deployments.apps" {
				t.Fatal("second Esc did not reach the workload list")
			}
		})
	}
}

func TestPodLogActionAndContainerSwitchKeepTheOriginalSection(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	e.detailValid, e.scroll = true, 13
	m.runExploreCommand("logs", true)
	a := e.action
	if a == nil {
		t.Fatal("Logs command did not open the action")
	}
	a.pending, a.operation, a.pod, a.container, a.targetChecked = "", "logs", e.detail.DeepCopy(), "app", true
	m.chooseActionContainer()
	m.openPodLogs(e.resource, e.detail, "sidecar")
	m.updateExplore("esc")
	if e.detail == nil || e.tab != 2 || e.scroll != 13 || e.container != "app" || len(e.trail) != 0 {
		t.Fatal("Pod log action or container switch replaced its origin")
	}
}

func TestPodAllContainersKeepsSelectionAcrossRefreshAndRestoresOrigin(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(2)
	_ = unstructured.SetNestedSlice(e.detail.Object, []any{map[string]any{"name": "app"}, map[string]any{"name": "proxy"}}, "spec", "containers")
	e.detailValid, e.scroll = true, 13
	m.openResourceActions("logs")
	a := e.action
	a.pending, a.operation, a.pod, a.targetChecked = "", "logs", e.detail.DeepCopy(), true
	m.chooseActionPod()
	if a.stage != "containers" || a.containers[0] != allApplicationContainers {
		t.Fatal("Pod log chooser has no All application containers option")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "targetaccess"})
	if !e.mergedLogs || e.container != "" || e.tab != 4 || len(m.logContainers()) != 3 {
		t.Fatal("All containers did not open the Pod's merged log view")
	}
	m.syncExploreDetail()
	run := e.detailRun
	run.cancel()
	m.updateExploreMessage(exploreDetailResult{run: run, object: e.detail.DeepCopy()})
	if e.container != "" {
		t.Fatal("detail refresh replaced All containers with an individual container")
	}
	m.syncExploreDetail()
	logs := e.log
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.syncExploreDetail()
	if e.mergedLogs || e.container != "proxy" || logs.ctx.Err() == nil || e.log == logs {
		t.Fatal("container switch kept merged sources alive or chose the wrong container")
	}
	m.updateExplore("o")
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyUp})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !e.mergedLogs || e.container != "" {
		t.Fatal("could not return to All containers")
	}
	m.updateExplore("esc")
	if e.tab != 2 || e.scroll != 13 || e.container != "app" || e.mergedLogs {
		t.Fatal("container selection lost the original detail or selected container")
	}
}

func TestLogControlsAndSiblingSectionsDoNotAccumulateHistory(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(1)
	e.scroll = 9
	m.updateExplore("l")
	for _, key := range []string{"l", "p", "p", "w", "t", "space", "space"} {
		m.updateExplore(key)
	}
	m.updateExplore("esc")
	if e.tab != 1 || e.scroll != 9 || len(e.trail) != 0 {
		t.Fatal("log controls replaced or accumulated the return origin")
	}
	for range 3 {
		for _, key := range []string{"d", "y", "e", "r", "]", "["} {
			m.updateExplore(key)
			if e.tab == 4 || e.logOrigin != nil || len(e.trail) != 0 {
				t.Fatal("sibling navigation created action history or entered Logs")
			}
		}
	}
	m.updateExplore("l")
	m.updateExplore("y")
	if e.tab != 1 || e.logOrigin != nil {
		t.Fatal("explicit sibling navigation did not leave Logs")
	}
	m.updateExplore("esc")
	if e.detail == nil || e.tab != 3 {
		t.Fatal("YAML did not return to Related after leaving Logs")
	}
	m.updateExplore("esc")
	if e.detail != nil {
		t.Fatal("Related did not return to the list")
	}
}

func TestLogsBackPreservesRelatedTrailAndCapturedIdentity(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(3)
	parent := e.location()
	e.trail = []exploreLocation{parent}
	e.detail = e.collection.Objects[1].DeepCopy()
	e.tab, e.scroll = 2, 8
	childUID := e.detail.GetUID()
	m.updateExplore("l")
	e.detail.SetUID("replacement")
	m.updateExplore("esc")
	if e.detail.GetUID() != childUID || e.tab != 2 || e.scroll != 8 || len(e.trail) != 1 {
		t.Fatal("Logs lost the captured child or consumed its Related parent")
	}
	m.updateExplore("esc")
	if e.detail.GetUID() != parent.object.GetUID() || e.tab != parent.tab || len(e.trail) != 0 {
		t.Fatal("next Esc did not return to the Related parent")
	}
}

func TestLogsEscapeRespectsTransientLayersAndHiddenSidebar(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	e.scroll = 11
	m.updateExplore("l")
	typeText(m, "b")
	press(t, m, "esc")
	if e.tab != 4 || e.logOrigin == nil || m.shortcutPrefix != "" {
		t.Fatal("prefix Esc also left Logs")
	}
	typeText(m, ":")
	press(t, m, "esc")
	if e.tab != 4 || e.logOrigin == nil || m.command != nil {
		t.Fatal("command Esc also left Logs")
	}
	typeText(m, "/")
	press(t, m, "esc")
	if e.tab != 4 || e.logOrigin == nil || m.searches[1] != nil {
		t.Fatal("search Esc also left Logs")
	}
	m.updateExplore("o")
	press(t, m, "esc")
	if e.tab != 4 || e.logOrigin == nil || e.picker != "" {
		t.Fatal("container picker Esc also left Logs")
	}
	m.leftHidden = true
	press(t, m, "esc")
	if e.detail == nil || e.tab != 0 || e.scroll != 11 || m.focus != 1 {
		t.Fatal("Logs Esc did not restore detail while the sidebar was hidden")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if e.detail != nil || m.focus != 1 || !m.leftHidden {
		t.Fatal("root Esc focused or revealed the hidden sidebar")
	}
}

func TestCancellingResourceActionRetainsOriginSection(t *testing.T) {
	for _, stage := range []string{"menu", "done", "confirm", "scale", "edit", "pods", "containers", "execcommand"} {
		t.Run(stage, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			m.openExploreSelection(2)
			e.detailValid, e.scroll = true, 14
			m.openResourceActions("")
			e.action.stage, e.action.pending = stage, ""
			for range 4 {
				if e.action != nil {
					m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
				}
			}
			if e.action != nil || e.detail == nil || e.tab != 2 || e.scroll != 14 || e.detail.GetUID() != "one" {
				t.Fatal("cancelling an action lost its originating detail")
			}
		})
	}
}
