package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
