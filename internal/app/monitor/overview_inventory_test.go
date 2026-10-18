package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
)

func TestOverviewCounterDrilldownUsesExactObservedStatus(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	data.Helm = &helm.Overview{ReadAt: now, Releases: []helm.OverviewRelease{
		{Name: "failed-in-name", Namespace: "shop", Status: "deployed", Revision: 3},
		{Name: "actual-failure", Namespace: "shop", Status: "failed", Revision: 4},
	}}
	s.Apply(data)
	s.Pane, s.Cursor[3], s.Query = 3, 1, "Helm"
	s.Cursor[3] = 0
	for range 4 {
		s.Move("down")
	}
	if !s.OpenInventory() || s.InventoryCounter != "Failed" {
		t.Fatal("counter not selected")
	}
	rows := s.Filtered(3)
	if len(rows) != 1 || rows[0].Title != "actual-failure" || s.Selected().Revision != 4 {
		t.Fatal("counter matched text rather than status", rows)
	}
	s.Apply(data)
	if s.Selected().Name != "actual-failure" {
		t.Fatal("refresh changed counter selection")
	}
	if !s.CloseInventory() || s.Query != "Helm" || s.Counter != 4 {
		t.Fatal("back lost card position or query")
	}
	s.Move("up")
	if !s.OpenInventory() || len(s.Filtered(3)) != 0 || s.inventoryCoverage() != "Observed" {
		t.Fatal("empty counter not distinguished")
	}
}

func TestOverviewUnavailableCounterAndKubernetesTargets(t *testing.T) {
	s, now := overviewDesignFixture()
	s.Pane = 3
	for _, resource := range []string{"pods", "deployments", "statefulsets", "nodes"} {
		s.Move("down")
		if s.OpenInventory() || s.Selected().Resource.Name != resource {
			t.Fatal("wrong Kubernetes counter target", resource)
		}
	}
	data := s.Data
	data.Flux = []gitops.Snapshot{{ReadAt: now, State: gitops.Empty}, {ReadAt: now, State: gitops.ReadDenied}, {ReadAt: now, State: gitops.Empty}}
	s.Apply(data)
	s.Query, s.Cursor[3], s.Counter = "Flux", 0, 2
	if !s.OpenInventory() || s.inventoryCoverage() != "Unavailable" {
		t.Fatal("denied category looked empty")
	}
	out := ansi.Strip(s.Render("test", 100, 35, true, now))
	if !strings.Contains(out, "unavailable") {
		t.Fatal("denied source not visible", out)
	}
}

func TestOverviewStatusTimeAndCompactCoverage(t *testing.T) {
	s, now := overviewDesignFixture()
	data := s.Data
	data.Kubernetes.Attention[0].At = now.Add(-time.Minute)
	data.Kubernetes.Attention[1].At = now.Add(-10 * time.Minute)
	s.Apply(data)
	if !s.Rows[2][0].At.Equal(now.Add(-time.Minute)) {
		t.Fatal("group lost latest source time")
	}
	out := ansi.Strip(s.Render("test", 92, 34, true, now))
	for _, label := range []string{"CPU", "Memory", "Nodes", "Pods", "Autoscaling", "Recent Deployments", "Needs Attention", "Infrastructure Snapshot", "coverage notices"} {
		if !strings.Contains(out, label) {
			t.Fatal("medium composition lost", label)
		}
	}
	out = ansi.Strip(s.Render("test", 92, 34, true, now.Add(time.Minute)))
	if !strings.Contains(out, "Stale health") {
		t.Fatal("stale health looked current")
	}
	data.Kubernetes.Changes[0].Revision = "new"
	data.Kubernetes.Changes[0].At = now.Add(time.Second)
	s.Apply(data)
	s.Pane, s.Cursor[1] = 1, 1
	if !strings.Contains(ansi.Strip(strings.Join(s.rowLines(s.Rows[1][1], 1, 60, true, now), "\n")), "History") {
		t.Fatal("older observation not labeled before opening")
	}
}
