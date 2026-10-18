package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
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
