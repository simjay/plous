package monitor

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestOverviewCompositionKeepsSourceMeaningAndAvailability(t *testing.T) {
	now := time.Now()
	s := OverviewState{}
	s.Apply(OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now},
		Helm: &helm.Overview{ReadAt: now, Releases: []helm.OverviewRelease{{Name: "checkout", Namespace: "shop", Revision: 3, Status: "failed", Updated: now.Add(-time.Hour)}}},
		Argo: &gitops.Snapshot{State: gitops.ReadDenied, Message: "denied", ReadAt: now},
		Flux: []gitops.Snapshot{{State: gitops.Available, ReadAt: now, Items: []gitops.Item{{Ref: gitops.Reference{Kind: "Kustomization", Name: "apps", Namespace: "flux-system"}, Status: "Not ready", Reason: "source unavailable", ObservedAt: now.Add(-time.Minute)}}}},
	})
	if len(s.Rows[1]) != 2 || s.Rows[1][0].Source != "Flux" || !strings.Contains(s.Rows[1][0].Detail, "Condition changed") {
		t.Fatal(s.Rows[1])
	}
	if len(s.Rows[2]) != 2 || s.Rows[2][0].Severity != 2 {
		t.Fatal(s.Rows[2])
	}
	for _, row := range s.Rows[3] {
		if row.ID == "argocd" {
			t.Fatal("denied integration received counters")
		}
	}
	for _, row := range s.Rows[1] {
		if strings.Contains(row.Detail, "push") || row.Target.Name == "" {
			t.Fatal("fabricated provenance or missing target", row)
		}
	}
	s.Pane = 2
	s.Cursor[0] = 5
	s.Cursor[2] = 1
	chosen := s.Selected().Name
	s.Apply(s.Data)
	if s.Selected().Name != chosen || s.Cursor[0] != 5 {
		t.Fatal("refresh changed logical selection")
	}
}

func TestOverviewLayoutsAreBoundedPureAndKeepLastRowsReachable(t *testing.T) {
	now := time.Now()
	s := OverviewState{Loaded: true, Data: OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now, Reads: map[string]kubernetes.OverviewRead{}}}}
	for p := 1; p < 4; p++ {
		for i := 0; i < 40; i++ {
			s.Rows[p] = append(s.Rows[p], OverviewRow{ID: fmt.Sprint(i), Title: fmt.Sprintf("resource-%02d", i), Source: "Kubernetes", Detail: strings.Repeat("long-namespace ", 9)})
		}
	}
	for _, size := range [][2]int{{36, 8}, {52, 12}, {76, 18}, {90, 24}, {130, 35}, {220, 50}} {
		for pane := 0; pane < 4; pane++ {
			s.Pane = pane
			s.Move("end")
			before := s
			out := ansi.Strip(s.Render("production", size[0], size[1], true, now))
			lines := strings.Split(out, "\n")
			if len(lines) > size[1] {
				t.Fatalf("height %v: %d", size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("width %v: %q", size, line)
				}
			}
			if !reflect.DeepEqual(before, s) {
				t.Fatal("render changed controller state")
			}
			if pane > 0 && !strings.Contains(out, "resource-39") {
				t.Fatalf("last row unreachable at %v pane %d:\n%s", size, pane, out)
			}
		}
	}
	s.Pane = 2
	s.Expanded = false
	out := ansi.Strip(s.Render("production", 130, 35, true, now))
	for _, heading := range OverviewPanes {
		if !strings.Contains(out, heading) {
			t.Fatalf("large view missing %s:\n%s", heading, out)
		}
	}
}

func TestOverviewUnavailableUsageNeverLooksMeasured(t *testing.T) {
	now := time.Now()
	s := OverviewState{Data: OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now, CPU: kubernetes.OverviewMetric{Requests: new(2.0)}, MetricsError: "Unavailable"}}}
	tile := s.tiles(now)[0]
	if tile.value != "Unavailable" || !strings.Contains(strings.Join(tile.lines, " "), "2.00 cores") {
		t.Fatal(tile)
	}
	s.Data.Kubernetes.CPU.Used = new(1.0)
	s.Data.Kubernetes.CPU.Allocatable = new(4.0)
	s.Data.Kubernetes.MetricsAt = now.Add(-2 * time.Minute)
	if s.tiles(now)[0].value != "STALE" || s.tiles(now)[0].lines[0] != "Usage stale" {
		t.Fatal("stale metric displayed as current")
	}
}

func TestOverviewKeepsDifferentControllerKindsInspectable(t *testing.T) {
	s := OverviewState{}
	s.Apply(OverviewData{Flux: []gitops.Snapshot{{State: gitops.Available, Items: []gitops.Item{
		{Ref: gitops.Reference{Kind: "Kustomization", Namespace: "flux-system", Name: "apps"}, Status: "Not ready"},
		{Ref: gitops.Reference{Kind: "GitRepository", Namespace: "flux-system", Name: "apps"}, Status: "Not ready"},
	}}}})
	if len(s.Rows[2]) != 2 || s.Rows[2][0].ID == s.Rows[2][1].ID {
		t.Fatal("different controller resources were grouped into one inspector link", s.Rows[2])
	}
}

func TestOverviewDistinguishesUnavailableFromRetainedObservations(t *testing.T) {
	now := time.Now()
	s := OverviewState{Loaded: true, Error: "All reads failed", Data: OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now}}}
	if view := ansi.Strip(s.Render("test", 80, 24, true, now)); !strings.Contains(view, "Unavailable") || strings.Contains(view, "STALE") {
		t.Fatal("first failed load claimed earlier observations", view)
	}
	s.Retained = true
	if view := ansi.Strip(s.Render("test", 80, 24, true, now)); !strings.Contains(view, "STALE") {
		t.Fatal("retained observations looked current", view)
	}
}
