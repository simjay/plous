package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

func overviewModel(t *testing.T) (*model, *overviewView) {
	m, g := gitOpsModel(t)
	m.closeSettings()
	m.mode, m.tool, m.focus = 0, "kubernetes", 1
	g.Cluster.HelmChecked = true
	v := m.overviewCurrent()
	v.State.Apply(monitor.OverviewData{Kubernetes: kubernetes.Overview{ReadAt: time.Now()}})
	v.State.Rows[2] = []monitor.OverviewRow{{ID: "first", Title: "broken-pod", Target: monitor.OverviewTarget{Tool: "kubernetes", Name: "broken-pod"}}, {ID: "second", Title: "another-pod"}}
	return m, v
}
func TestOverviewKeyboardSearchAndResponsivePanels(t *testing.T) {
	m, v := overviewModel(t)
	m.width, m.height = 80, 24
	m.Update(localMouseKey("right"))
	if v.State.Pane != 0 || v.State.Cursor[0] != 1 {
		t.Fatal("Right did not select Memory")
	}
	if hint := m.hint(); !strings.Contains(hint, "←→ Tiles") || !strings.Contains(hint, "↑↓ Rows/sections") {
		t.Fatal("Overview footer describes the old navigation", hint)
	}
	m.Update(localMouseKey("down"))
	m.Update(localMouseKey("down"))
	m.Update(localMouseKey("down"))
	if v.State.Pane != 2 || v.State.Cursor[2] != 1 {
		t.Fatal("section navigation failed", v.State)
	}
	m.Update(localMouseKey("/"))
	for _, key := range []string{"f", "f", ":", "c", "c"} {
		msg := localMouseKey(key)
		msg.Text = key
		m.Update(msg)
	}
	if v.State.Input == nil || v.State.Input.Text != "ff:cc" || m.viewExpanded() || m.chatEditing || m.command != nil {
		t.Fatal("search lost literal ownership")
	}
	m.pasteText("/model")
	if !strings.Contains(v.State.Input.Text, "/model") {
		t.Fatal("paste did not reach Overview search")
	}
	m.Update(localMouseKey("esc"))
	m.Update(localMouseKey("e"))
	m.execute("fullscreen")
	m.Update(localMouseKey("esc"))
	if v.State.Expanded || !m.viewExpanded() {
		t.Fatal("Esc restored global layout before local expansion")
	}
	m.Update(localMouseKey("esc"))
	if m.viewExpanded() {
		t.Fatal("second Esc did not restore main layout")
	}
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 30}, {180, 45}, {240, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		before := v.State.Cursor
		out := ansi.Strip(m.render())
		if !strings.Contains(out, ":1") || strings.Contains(out, "Resize terminal") {
			t.Fatalf("Overview unavailable at %v", size)
		}
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Fatalf("height %v = %d", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow %v %q", size, line)
			}
		}
		if before != v.State.Cursor {
			t.Fatal("resize changed independent positions")
		}
	}
	m.width, m.height = 150, 40
	view := ansi.Strip(m.render())
	if m.panelWidths()[1]-4 < 78 || !strings.Contains(view, "Deployments") || !strings.Contains(view, "Attention") {
		t.Fatal("150-column Overview lost readable two-column layout")
	}
	m.width = 80
	if m.panelWidths() != [3]int{28, 24, 28} || m.leftHidden || m.rightHidden {
		t.Fatal("compact layout changed saved visibility")
	}
	m.cycleFocus(false)
	if !m.panelVisible(2) {
		t.Fatal("assistant unreachable on compact terminal")
	}
}
func TestOverviewRejectsLateResultsAndStopsReads(t *testing.T) {
	m, v := overviewModel(t)
	cancelled := false
	v.Cancel = func() { cancelled = true }
	v.Run = 5
	v.State.Loading = true
	m.mode = 1
	m.syncOverview()
	if !cancelled || v.Cancel != nil || v.State.Loading {
		t.Fatal("hidden Overview kept reading")
	}
	m.applyOverview(overviewResult{View: v, Run: 5, Data: monitor.OverviewData{Kubernetes: kubernetes.Overview{PodsFailed: 999}}})
	if v.State.Data.Kubernetes.PodsFailed == 999 {
		t.Fatal("late data replaced current scope")
	}
}

func TestOverviewControllerLinkStartsAtSelectedResource(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 180
	m.tool = string(gitops.ArgoCD)
	g := m.gitOpsCurrent()
	g.State.ApplyDetail(gitops.Item{Ref: gitops.Reference{Kind: "Application", Name: "old-parent"}})
	g.State.ApplyDetail(gitops.Item{Ref: gitops.Reference{Kind: "Application", Name: "old-child"}})
	m.tool = "kubernetes"
	ref := gitops.Reference{Kind: "Application", Namespace: "argocd", Name: "new-app"}
	m.openOverviewTarget(v, monitor.OverviewTarget{Tool: string(gitops.ArgoCD), Name: ref.Name, Namespace: ref.Namespace, Ref: ref})
	g = m.gitOpsCurrent()
	if g.State.Pending == nil || g.State.Pending.Ref != ref {
		t.Fatal("selected target was lost")
	}
	g.State.ApplyDetail(gitops.Item{Ref: ref})
	m.updateGitOps("esc")
	if m.mode != 0 || m.overviewBack != nil || m.tool != "kubernetes" {
		t.Fatal("Overview inspection returned to a previous controller navigation trail")
	}
}

func TestOverviewNestedKubernetesInspectorReturnsThroughController(t *testing.T) {
	m, v := overviewModel(t)
	m.mode = 1
	m.overviewBack = &overviewReturn{View: v, Tool: "kubernetes"}
	m.gitOpsReturn = &gitOpsReturn{Workspace: m.active, Connection: m.connection, Tool: string(gitops.ArgoCD)}
	m.explore = &monitor.ExploreState{}
	m.backExploreLocation()
	if m.mode != 1 || m.tool != string(gitops.ArgoCD) || m.overviewBack == nil {
		t.Fatal("nested inspector skipped its controller parent")
	}
}

func TestOverviewInventoryEmptyCounterCanRenderAndReturn(t *testing.T) {
	m, v := overviewModel(t)
	m.width, m.height = 220, 50
	data := v.State.Data
	data.Helm = &helm.Overview{ReadAt: data.Kubernetes.ReadAt, Releases: []helm.OverviewRelease{{Name: "metrics", Namespace: "monitoring", Status: "deployed", Revision: 2}}}
	v.State.Apply(data)
	v.State.Pane, v.State.Cursor[3] = 3, 1
	for _, key := range []string{"down", "enter", "esc", "down", "down", "down", "enter", "esc"} {
		msg := localMouseKey(key)
		m.Update(msg)
		m.render()
	}
	if v.State.InventoryTool != "" || v.State.Counter != 4 {
		t.Fatal("counter back changed selection")
	}
}

func TestOverviewRegressionInspectAfterFailedRediscovery(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 180
	m.overviewCommand("refresh")
	v.Cluster.Cancel = func() {}
	m.applyClusterDiscovery(clusterDiscoveryResult{Cluster: v.Cluster, Run: v.Cluster.Run, Target: v.Cluster.Connection.Target, Err: errors.New("client setup failed")})
	m.syncOverview()
	defer func() {
		if got := recover(); got != nil {
			t.Errorf("opening retained row panicked after failed rediscovery: %v", got)
		}
	}()
	cmd := m.openOverviewTarget(v, monitor.OverviewTarget{Tool: "kubernetes", Resource: kubecommon.Resource{Version: "v1", Name: "pods", Namespaced: true}, Namespace: "shop", Name: "broken-pod"})
	if cmd != nil {
		cmd()
	}
}

func TestOverviewRegressionPreservesSavedKubernetesScope(t *testing.T) {
	m, v := overviewModel(t)
	old := monitor.ExplorePreference{Resource: "deployments.apps", Namespace: "original", Filter: "keep", Selector: "team=one"}
	m.explorePreferences = map[string]monitor.ExplorePreference{m.active + "/" + m.connection: old}
	v.Cancel = func() {}
	object := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "broken-pod", "namespace": "another", "uid": "p1"}}}
	m.applyOverviewInspect(overviewInspectResult{View: v, Run: v.Run, Target: monitor.OverviewTarget{Tool: "kubernetes", Resource: kubecommon.Resource{Version: "v1", Name: "pods", Namespaced: true}, Namespace: "another", Name: "broken-pod", UID: "p1"}, Object: object})
	m.rememberExplore()
	if got := m.explorePreferences[m.active+"/"+m.connection]; got != old {
		t.Errorf("inspection overwrote saved scope: %+v", got)
	}
}

func TestOverviewRegressionPreservesControllerScope(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 180
	key := m.active + "/" + m.connection + "/argocd"
	old := gitOpsPreference{Section: gitops.Applications, Namespace: "original", Selected: "keep"}
	m.gitOpsPreferences = map[string]gitOpsPreference{key: old}
	m.openOverviewTarget(v, monitor.OverviewTarget{Tool: "argocd", Name: "new-app", Namespace: "another", Ref: gitops.Reference{Group: "argoproj.io", Kind: "Application", Name: "new-app", Namespace: "another"}})
	if got := m.gitOpsPreferences[key]; got != old {
		t.Errorf("inspection overwrote saved scope: %+v", got)
	}
}

func TestOverviewRegressionCompactEvidenceContainsSelectedWarning(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 60
	v.State.Pane = 2
	m.openOverviewTarget(v, *v.State.Selected())
	if !strings.Contains(m.detail, "broken-pod") {
		t.Error("selected warning is absent from compact evidence")
	}
}

func TestOverviewRegressionInspectorErrorVisible(t *testing.T) {
	m, v := overviewModel(t)
	m.width, m.height = 180, 40
	v.Cancel = func() {}
	m.applyOverviewInspect(overviewInspectResult{View: v, Run: v.Run, Err: errors.New("failed")})
	if !strings.Contains(m.render(), m.status) {
		t.Errorf("inspector error is hidden: %s", m.status)
	}
}

func TestOverviewRegressionCompactChatRemainsVisible(t *testing.T) {
	m, _ := overviewModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m.execute("chat")
	if m.focus == 2 && m.chatEditing && !m.panelVisible(2) {
		t.Error("chat accepts input in a hidden panel at 60 columns")
	}
}

func TestOverviewRegressionEscapeCancelsInspectorRead(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 180
	cancelled := false
	v.Cancel = func() { cancelled = true }
	v.State.Loading = true
	v.Inspecting = true
	run := v.Run
	m.Update(localMouseKey("esc"))
	object := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "late", "namespace": "shop"}}}
	m.applyOverviewInspect(overviewInspectResult{View: v, Run: run, Target: monitor.OverviewTarget{Tool: "kubernetes", Resource: kubecommon.Resource{Version: "v1", Name: "pods", Namespaced: true}, Namespace: "shop", Name: "late"}, Object: object})
	if !cancelled || m.mode == 1 {
		t.Errorf("Esc did not cancel inspector: cancelled=%t, mode=%d, focus=%d", cancelled, m.mode, m.focus)
	}
}

func TestOverviewFullscreenInspectorBackPreservesGroup(t *testing.T) {
	m, v := overviewModel(t)
	m.width, m.height = 180, 40
	v.State.Pane, v.State.Group = 2, "group"
	m.viewCommand("fullscreen")
	v.Cancel = func() {}
	object := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "broken-pod", "namespace": "services", "uid": "p1"}}}
	m.applyOverviewInspect(overviewInspectResult{View: v, Run: v.Run, Target: monitor.OverviewTarget{Tool: "kubernetes", Resource: kubecommon.Resource{Version: "v1", Name: "pods", Namespaced: true}, Namespace: "services", Name: "broken-pod", UID: "p1"}, Object: object})
	m.Update(localMouseKey("esc"))
	if m.mode != 0 || v.State.Group != "group" || !m.viewExpanded() {
		t.Fatal("inspector Escape restored layout before its parent group")
	}
	m.Update(localMouseKey("esc"))
	if v.State.Group != "" || !m.viewExpanded() {
		t.Fatal("group Escape lost main expansion")
	}
	m.Update(localMouseKey("esc"))
	if m.viewExpanded() {
		t.Fatal("root Escape failed to restore layout")
	}
}

func TestOverviewHelmInspectionKeepsOriginalState(t *testing.T) {
	m, v := overviewModel(t)
	m.width = 180
	m.tool = "helm"
	original := m.helmer()
	original.Preference = monitor.HelmPreference{Namespace: "saved", Filter: "keep", Section: "Charts"}
	m.rememberHelm(original)
	before := original.Preference
	m.tool = "kubernetes"
	m.openOverviewTarget(v, monitor.OverviewTarget{Tool: "helm", Namespace: "another", Name: "release", Revision: 4})
	if m.overviewBack == nil || m.helmer() == original {
		t.Fatal("inspection reused the original state")
	}
	m.rememberHelm(m.helmer())
	if original.Preference != before || m.helmPreferences[original.Workspace+"/"+original.Connection] != before {
		t.Fatal("inspection replaced saved Helm scope")
	}
	temporary := m.helmer()
	m.returnToOverview()
	m.rememberHelm(temporary)
	if m.helmPreferences[original.Workspace+"/"+original.Connection] != before {
		t.Fatal("late inspection result replaced saved scope")
	}
	m.tool = "helm"
	if m.helmer() != original {
		t.Fatal("return lost original Helm view")
	}
}

func TestOverviewWidthsGrowWithoutBreakpointCollapse(t *testing.T) {
	m, _ := overviewModel(t)
	m.focus = 1
	m.leftHidden, m.rightHidden = false, false
	for _, size := range []assistantSize{assistantNormal, assistantDouble} {
		for _, focus := range []int{0, 1, 2} {
			m.assistantSize, m.focus = size, focus
			previous := 0
			for width := 80; width <= 240; width++ {
				m.width = width
				w := m.panelWidths()
				if w[1] < previous || w[0]+w[1]+w[2] != width {
					t.Fatalf("%s focus %d width %d collapsed main: %v after %d", size, focus, width, w, previous)
				}
				previous = w[1]
			}
		}
	}
	m.assistantSize = assistantNormal
	for _, focus := range []int{0, 2} {
		m.width, m.focus = 110, focus
		if m.panelWidths()[focus] < 28 {
			t.Fatal("side panel is too narrow")
		}
	}
}

func TestOverviewNarrowAttentionKeepsCauseAndIdentity(t *testing.T) {
	m, v := overviewModel(t)
	m.width, m.height = 80, 24
	v.State.Pane = 2
	v.State.Rows[2] = []monitor.OverviewRow{{
		ID: "broken", Title: "broken-checkout", Source: "Kubernetes", Reason: "CrashLoopBackOff", Severity: 2,
		Target: monitor.OverviewTarget{Tool: "kubernetes", Namespace: "shop", Name: "broken-checkout"},
	}}
	for _, focus := range []int{1, 2, 0} {
		m.focus = focus
		view := ansi.Strip(m.render())
		for _, fact := range []string{"Needs Attention", "× CrashLoopBackOff", "broken-checkout", "K8s · shop"} {
			if !strings.Contains(view, fact) {
				t.Fatalf("focus %d lost %q:\n%s", focus, fact, view)
			}
		}
		if m.panelWidths() != [3]int{28, 24, 28} || m.leftHidden || m.rightHidden {
			t.Fatal("reading attention changed the panel layout")
		}
		rows := strings.Split(view, "\n")
		if len(rows) != 24 {
			t.Fatal("narrow attention changed frame height")
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != 80 {
				t.Fatal("narrow attention overflowed the frame", row)
			}
		}
	}
}

func overviewMetricModel(t *testing.T) (*model, *overviewView) {
	t.Helper()
	m, v := overviewModel(t)
	m.width, m.height = 180, 45
	now := time.Now()
	k := kubernetes.Overview{ReadAt: now, ActivePods: 321, Reads: map[string]kubernetes.OverviewRead{"pods": {ReadAt: now, Count: 345}, "nodes": {ReadAt: now, Count: 13}}}
	k.CPU = kubernetes.OverviewMetric{Requests: new(70.12), Limits: new(79.68), RequestsMissing: 33, LimitsMissing: 108}
	k.Memory = kubernetes.OverviewMetric{Requests: new(275.4 * (1 << 30)), Limits: new(298.7 * (1 << 30)), RequestsMissing: 59, LimitsMissing: 73}
	v.State.Apply(monitor.OverviewData{Kubernetes: k})
	for i := range 60 {
		at := now.Add(-time.Duration(59-i) * 5 * time.Second)
		v.State.ApplyUsage(kubernetes.OverviewUsage{ReadAt: at, SourceStart: at, SourceEnd: at, Observed: 13, Expected: 13,
			CPU: new(6.91 + float64(i%7)*0.2), Memory: new((183.4 + float64(i%5)*2) * (1 << 30)), CPUAllocatable: new(118.79), MemoryAllocatable: new(569.5 * (1 << 30))})
	}
	return m, v
}

func TestOverviewMetricClickSearchSourcesAndBack(t *testing.T) {
	m, v := overviewMetricModel(t)
	v.State.Cursor[0] = 0
	v.State.Cursor[2], v.State.Query = 1, "keep filter"
	clickMouse(t, m, "main", "overview-tile/1")
	d := v.State.MetricDetail
	if d == nil || d.Index != 1 || m.detail != "" {
		t.Fatal("Memory click did not open its own chart")
	}
	screen := ansi.Strip(m.render())
	for _, want := range []string{"Memory · All namespaces", "GiB", "Coverage: 13/13 Nodes", "73 observed active Pods have missing memory limits."} {
		if !strings.Contains(screen, want) {
			t.Fatalf("metric view missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "Overview evidence") || strings.Contains(screen, "CPU requests") {
		t.Fatal("chart still includes unrelated evidence")
	}
	m.Update(localMouseKey("/"))
	m.pasteText("ff:cc")
	if d.Reader().Query != "ff:cc" || m.viewExpanded() || m.chatEditing {
		t.Fatal("metric search lost literal input")
	}
	m.Update(localMouseKey("esc"))
	m.Update(localMouseKey("end"))
	m.render()
	position := d.Reader().Cursor
	clickMouse(t, m, "main", "action/enter")
	if !d.Sources {
		t.Fatal("source details action did not open")
	}
	if text := ansi.Strip(m.render()); !strings.Contains(text, "metrics.k8s.io Node observations") || strings.Contains(text, "HPA current replica") {
		t.Fatal("sources are not scoped to the metric", text)
	}
	m.Update(localMouseKey("esc"))
	if d.Sources || d.Reader().Cursor != position {
		t.Fatal("returning from sources lost chart position")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != nil || v.State.Cursor[0] != 1 || v.State.Cursor[2] != 1 || v.State.Query != "keep filter" {
		t.Fatal("returning to Overview lost selection or filter")
	}
}

func TestOverviewMetricLayersAndPolling(t *testing.T) {
	m, v := overviewMetricModel(t)
	m.Update(localMouseKey("enter"))
	d := v.State.MetricDetail
	if d == nil {
		t.Fatal("Enter did not open CPU")
	}
	cancelled := false
	v.UsageCancel = func() { cancelled = true }
	m.syncOverview()
	if cancelled || !m.overviewVisible() {
		t.Fatal("metric detail stopped Overview polling")
	}
	usage := *v.State.Usage
	usage.ReadAt = usage.ReadAt.Add(time.Second)
	usage.SourceStart, usage.SourceEnd, usage.CPU = usage.ReadAt, usage.ReadAt, new(59.395)
	v.State.ApplyUsage(usage)
	if !strings.Contains(ansi.Strip(m.render()), "50% used") || v.State.MetricDetail != d {
		t.Fatal("new sample did not update the same detail view")
	}
	m.execute("fullscreen")
	m.execute("help")
	if !strings.Contains(ansi.Strip(m.render()), "CPU") {
		t.Fatal("Help lost metric context")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != d || !m.viewExpanded() {
		t.Fatal("Help did not preserve detail and fullscreen")
	}
	m.Update(localMouseKey("enter"))
	m.Update(localMouseKey("esc"))
	if d.Sources || !m.viewExpanded() {
		t.Fatal("source Escape restored layout too early")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != nil || !m.viewExpanded() {
		t.Fatal("detail Escape restored layout too early")
	}
	m.Update(localMouseKey("esc"))
	if m.viewExpanded() {
		t.Fatal("Overview Escape did not restore layout")
	}
}

func TestOverviewPulseResourceListsPreserveSelectionAndPreferences(t *testing.T) {
	for index, resource := range map[int]string{2: "nodes", 3: "pods", 4: "horizontalpodautoscalers.autoscaling"} {
		for _, width := range []int{60, 180} {
			m, v := overviewMetricModel(t)
			m.width = width
			client, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
			if err != nil {
				t.Fatal(err)
			}
			v.Cluster.Client = client
			old := monitor.ExplorePreference{Resource: "deployments.apps", Namespace: "saved", Filter: "keep", Selector: "team=one"}
			key := m.active + "/" + m.connection
			m.explorePreferences = map[string]monitor.ExplorePreference{key: old}
			v.State.Cursor[0] = index
			m.Update(localMouseKey("enter"))
			if m.explore == nil || m.mode != 1 || m.explore.Resource.Key() != resource || !m.explore.Temporary || m.explore.Preference.Namespace != "" {
				t.Fatalf("tile %d at width %d did not open its cluster-wide list", index, width)
			}
			m.rememberExplore()
			m.Update(localMouseKey("esc"))
			if m.mode != 0 || v.State.Cursor[0] != index || m.explorePreferences[key] != old {
				t.Fatalf("tile %d at width %d did not return with saved preferences", index, width)
			}
		}
	}
}

func TestOverviewMetricScreenMatrix(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {150, 40}, {180, 45}, {240, 60}} {
		for _, index := range []int{0, 1} {
			m, v := overviewMetricModel(t)
			m.width, m.height = size[0], size[1]
			v.State.Cursor[0] = index
			m.Update(localMouseKey("enter"))
			for _, full := range []bool{false, true} {
				if full {
					m.execute("fullscreen")
				}
				name := fmt.Sprintf("overview-metric-%d-%dx%d-full-%t", index, size[0], size[1], full)
				captureOverviewScreen(t, m, name)
				m.Update(localMouseKey("end"))
				if text := ansi.Strip(m.render()); !strings.Contains(text, "unbounded") {
					t.Fatalf("%s cannot reach the last warning:\n%s", name, text)
				}
				m.Update(localMouseKey("home"))
			}
		}
	}
	m, v := overviewMetricModel(t)
	v.State.OpenMetric()
	v.State.Data.Kubernetes.MetricsAt = time.Now().Add(-2 * time.Minute)
	v.State.Data.Kubernetes.Reads["pods"] = kubernetes.OverviewRead{ReadAt: time.Now(), Partial: true}
	text := ansi.Strip(m.render())
	for _, want := range []string{"STALE", "Partial Pod inventory", "budget totals unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing limitation %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "70.12") || strings.Contains(text, "79.68") {
		t.Fatal("partial budget was presented as a complete total")
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	captureOverviewScreen(t, m, "overview-metric-stale-partial")
}

func TestOverviewUsageArrivesWhileInventoryAndDiscoveryWait(t *testing.T) {
	m, v := overviewModel(t)
	v.State = monitor.OverviewState{Loading: true, Pane: 2, Cursor: [4]int{0, 0, 3, 0}}
	v.Cluster.HelmChecked = false
	v.Cluster.Client = &kubernetes.Client{}
	v.Cancel = func() {}
	v.Run = 12
	if m.syncOverview() == nil || v.UsageCancel == nil {
		t.Fatal("usage waited for Helm detection or the inventory request")
	}
	defer v.stop()
	run := v.UsageRun
	m.syncOverview()
	if v.UsageRun != run {
		t.Fatal("started overlapping usage reads")
	}
	now := time.Now()
	data := kubernetes.OverviewUsage{ReadAt: now, SourceStart: now.Add(-time.Second), SourceEnd: now.Add(-time.Second), CPU: new(2.0), CPUAllocatable: new(8.0), Memory: new(1.0), MemoryAllocatable: new(4.0), Observed: 1, Expected: 1}
	m.applyOverviewUsage(overviewUsageResult{v, run, data})
	if v.State.Loaded || !v.State.Loading || *v.State.Data.Kubernetes.CPU.Used != 2 || v.State.Cursor[2] != 3 || v.Cancel == nil {
		t.Fatal("metrics refresh changed inventory state or navigation", v.State)
	}
	if delay := time.Until(v.UsageNext); delay <= 0 || delay > 5*time.Second {
		t.Fatal("usage did not schedule a fast follow-up", delay)
	}
	if m.syncOverviewUsage(v) == nil || !v.UsagePoll || m.syncOverviewUsage(v) != nil {
		t.Fatal("usage poll was missing or scheduled twice")
	}
	m.applyOverview(overviewResult{View: v, Run: 12, Data: monitor.OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now, CPU: kubernetes.OverviewMetric{Requests: new(3.0), Used: new(99.0)}}}})
	if *v.State.Data.Kubernetes.CPU.Used != 2 || *v.State.Data.Kubernetes.CPU.Requests != 3 || len(v.State.Samples[0]) != 1 {
		t.Fatal("late inventory replaced usage, erased requests, or duplicated history")
	}
}

func TestOverviewUsageStopsOnExitAndRejectsLateResults(t *testing.T) {
	m, v := overviewModel(t)
	cancelled := false
	v.UsageCancel = func() { cancelled = true }
	v.UsageRun = 10
	v.UsagePoll = true
	m.mode = 1
	m.syncOverview()
	if !cancelled || v.UsageCancel != nil || v.UsagePoll {
		t.Fatal("usage continued after leaving Overview")
	}
	m.applyOverviewUsage(overviewUsageResult{v, 10, kubernetes.OverviewUsage{ReadAt: time.Now(), CPU: new(999.0)}})
	if v.State.Usage != nil {
		t.Fatal("a late usage response crossed the view lifetime")
	}
}

func TestOverviewFullApplicationComposition(t *testing.T) {
	m, v := overviewModel(t)
	now := time.Now()
	k := kubernetes.Overview{ReadAt: now, MetricsAt: now, MetricsEnd: now, MetricsObserved: 13, MetricsExpected: 13, Reads: map[string]kubernetes.OverviewRead{}, NodesReady: 13, PodsRunning: 323, PodsPending: 8, PodsFailed: 3, PodsSucceeded: 13, Restarts: 82, HPACurrent: 28, HPADesired: 31, HPAUp: 1, HPAStable: 5, HPALastScale: now.Add(-2 * time.Minute)}
	k.CPU = kubernetes.OverviewMetric{Used: new(49.9), Allocatable: new(118.79), Requests: new(70.12), Limits: new(79.68)}
	k.Memory = kubernetes.OverviewMetric{Used: new(178.3 * (1 << 30)), Allocatable: new(569.5 * (1 << 30)), Requests: new(275.4 * (1 << 30)), Limits: new(298.7 * (1 << 30))}
	for key, count := range map[string]int{"nodes": 13, "pods": 347, "deployments.apps": 73, "statefulsets.apps": 21, "daemonsets.apps": 8, "horizontalpodautoscalers.autoscaling": 6} {
		k.Reads[key] = kubernetes.OverviewRead{Count: count, ReadAt: now}
	}
	dep := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true}
	pod := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	for i, name := range []string{"payment-service", "user-api", "frontend", "analytics", "notification", "postgresql", "redis", "ingress-nginx"} {
		k.Changes = append(k.Changes, kubernetes.OverviewEntry{Resource: dep, Namespace: "production", Name: name, UID: name, Revision: fmt.Sprint(72 - i), Outcome: "Available", At: now.Add(-time.Duration(3+i*4) * time.Minute)})
	}
	for i, cause := range []string{"CrashLoopBackOff", "ImagePullBackOff", "Readiness gate blocked"} {
		k.Attention = append(k.Attention, kubernetes.OverviewEntry{Resource: pod, Namespace: "production", Name: []string{"user-api-676d9c87-xy212", "notification-worker-78f99", "payment-service-56fc2f"}[i], Summary: cause, Reason: cause, UID: fmt.Sprintf("failing-pod-%d", i), Severity: 2, At: now.Add(-time.Duration(2+i*3) * time.Minute)})
	}
	data := monitor.OverviewData{Kubernetes: k, Helm: &helm.Overview{ReadAt: now, Releases: []helm.OverviewRelease{{Name: "analytics", Namespace: "production", Status: "failed", Revision: 4, Updated: now.Add(-12 * time.Minute)}, {Name: "postgresql", Namespace: "infra", Status: "deployed", Revision: 9, Updated: now.Add(-time.Hour)}}}, Argo: &gitops.Snapshot{ReadAt: now, State: gitops.Available, Items: []gitops.Item{{Ref: gitops.Reference{Kind: "Application", Name: "payments", Namespace: "argocd"}, Sync: "Synced", Health: "Healthy", Status: "Synced / Healthy", Fields: []gitops.Field{{Label: "Latest operation", Value: "Succeeded"}, {Label: "Operation finished", Value: now.Add(-time.Minute).Format(time.RFC3339)}, {Label: "Successful operation revision", Value: "a13bf92"}}}}}, Flux: []gitops.Snapshot{{ReadAt: now, State: gitops.Available, Items: []gitops.Item{{Ref: gitops.Reference{Kind: "Kustomization", Name: "platform", Namespace: "flux-system"}, Status: "Not ready", Reason: "HealthCheckFailed", ObservedAt: now.Add(-5 * time.Minute)}}}, {ReadAt: now, State: gitops.Empty}, {ReadAt: now, State: gitops.Empty}}, Notices: []string{"Network: no telemetry provider configured"}}
	m.width, m.height, m.focus = 220, 50, 1
	saved := v.State
	v.State = monitor.OverviewState{}
	v.State.Apply(data)
	captureOverviewScreen(t, m, "overview-visual-first-load-220x50")
	v.State = monitor.OverviewState{}
	for i := 0; i < 4; i++ {
		sample := data
		at := now.Add(-time.Duration(3-i) * 15 * time.Second)
		sample.Kubernetes.ReadAt, sample.Kubernetes.MetricsAt = at, at
		sample.Kubernetes.CPU.Used = new([]float64{5.5, 6.8, 5.9, 6.5}[i])
		v.State.Apply(sample)
	}
	for _, size := range [][2]int{{150, 40}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-startup-%dx%d", size[0], size[1]))
	}
	v.State = saved
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(59-i) * 15 * time.Second)
		data.Kubernetes.ReadAt, data.Kubernetes.MetricsAt = at, at
		data.Kubernetes.Reads = map[string]kubernetes.OverviewRead{}
		for key, read := range k.Reads {
			read.ReadAt = at
			data.Kubernetes.Reads[key] = read
		}
		restarts, replicas := int64(79), int64(26)
		data.Kubernetes.Attention = k.Attention[:2]
		if i == 59 {
			restarts, replicas = 82, 28
			data.Kubernetes.Attention = k.Attention
		}
		data.Kubernetes.Restarts = int(restarts)
		data.Kubernetes.HPACurrent = replicas
		data.Kubernetes.RestartsByContainer = map[string]int64{"observed-pod/containers/worker": restarts}
		data.Kubernetes.ReplicasByHPA = map[string]int64{"observed-hpa": replicas}
		data.Kubernetes.CPU.Used = new(40.0 + float64(i%9)*2)
		data.Kubernetes.Memory.Used = new((150 + float64(i%13)*3) * (1 << 30))
		v.State.Apply(data)
	}
	for _, size := range [][2]int{{80, 24}, {109, 40}, {110, 40}, {149, 40}, {150, 40}, {180, 45}, {220, 50}, {260, 60}} {
		m.width, m.height = size[0], size[1]
		m.focus = 1
		m.leftHidden, m.rightHidden = false, false
		v.State.Pane = 0
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-%dx%d", size[0], size[1]))
		if size[0] >= 150 {
			plain := ansi.Strip(m.render())
			for _, label := range []string{"Pods", "Autoscaling", "Infrastructure Snapshot"} {
				if !strings.Contains(plain, label) {
					t.Fatalf("normal %v hides %s", size, label)
				}
			}
		}
		if size[0] >= 220 {
			view := ansi.Strip(m.render())
			for _, text := range []string{"Recent Deployments", "Needs Attention", "Infrastructure Snapshot", "Autoscaling", "Helm", "Argo CD", "1 kustomization", "0 Helm releases", "4 integrations"} {
				if !strings.Contains(view, text) {
					t.Fatalf("normal composition %v missing %s", size, text)
				}
			}
		}
		for _, pane := range []int{2, 3} {
			v.State.Pane = pane
			captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-%dx%d-pane%d", size[0], size[1], pane))
		}
	}
	m.width, m.height = 220, 50
	m.execute("fullscreen")
	captureOverviewScreen(t, m, "overview-visual-fullscreen")
	m.execute("fullscreen")
	v.State = monitor.OverviewState{}
	onlyKube := data
	onlyKube.Helm, onlyKube.Argo, onlyKube.Flux = nil, nil, nil
	onlyKube.Notices = []string{"Argo CD API not detected here", "Flux APIs not detected in this cluster", "Network: no telemetry provider configured"}
	v.State.Apply(onlyKube)
	captureOverviewScreen(t, m, "overview-visual-kubernetes-only-220x50")
	partial := data
	partial.Flux = append([]gitops.Snapshot(nil), data.Flux...)
	partial.Flux[1] = gitops.Snapshot{ReadAt: now, State: gitops.ReadDenied}
	partial.Notices = []string{"Flux HelmReleases: read denied", "Network: no telemetry provider configured"}
	v.State = monitor.OverviewState{}
	v.State.Apply(partial)
	captureOverviewScreen(t, m, "overview-visual-partial-220x50")
	v.State = monitor.OverviewState{}
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(59-i) * 15 * time.Second)
		data.Kubernetes.ReadAt, data.Kubernetes.MetricsAt = at, at
		data.Kubernetes.CPU.Used = new(3.5 + float64(i%7)*0.28)
		data.Kubernetes.Memory.Used = new((165 + float64(i%9)*2) * (1 << 30))
		v.State.Apply(data)
	}
	for _, size := range [][2]int{{150, 40}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		captureOverviewScreen(t, m, fmt.Sprintf("overview-visual-low-usage-%dx%d", size[0], size[1]))
	}
}

func captureOverviewScreen(t *testing.T, m *model, name string) {
	t.Helper()
	out := m.render()
	plain := ansi.Strip(out)
	rows := strings.Split(plain, "\n")
	if len(rows) != m.height {
		t.Fatalf("%s height=%d", name, len(rows))
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > m.width {
			t.Fatalf("%s overflow", name)
		}
	}
	if directory := os.Getenv("PLOUS_REVIEW_DIR"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		for suffix, content := range map[string]string{"ansi": out, "txt": plain} {
			if err := os.WriteFile(filepath.Join(directory, name+"."+suffix), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
