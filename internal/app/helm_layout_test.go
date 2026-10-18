package app

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
)

func TestHelmTimelineSummariesKeepFullComparisonAndLargeChartDetails(t *testing.T) {
	m, h := helmModel(t)
	after := inspectorFixture()
	after.History[1].Updated = "2026-09-30T16:35:05.006943-04:00"
	after.History[1].Chart = "demo-2.0.0"
	before := *after
	before.Version = 1
	before.Manifest = strings.ReplaceAll(before.Manifest, "replicas: 2", "replicas: 1")
	changes, err := helmtool.CompareRevisions(&before, after)
	if err != nil {
		t.Fatal(err)
	}
	h.OpenTimeline(after)
	h.Timeline.Entries[2] = monitor.HelmTimelineEntry{Changes: changes}
	view := ansi.Strip(monitor.HelmTimelineBody(h, 110, 36))
	for _, want := range []string{"Revision 2 · latest", "2026-09-30 16:35 -04:00", "Changes from revision 1", "Resources  1 changed", "Deployment / apps/web", "Revision 1", "Initial installation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	for _, unwanted := range []string{"replicas:", "apiVersion:", "r2 ·", ".006943"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("timeline contains %q: %s", unwanted, view)
		}
	}
	m.reviewHelmRevision(h)
	if h.Comparison == nil || h.Comparison.AfterLabel != "Recorded revision 2" {
		t.Fatal("summary lost the exact full comparison")
	}
	h.Comparison = nil
	changes.Unavailable = "Full diff is too large. Inspect each revision to read its configuration."
	changes.Before, changes.After, changes.Diff = "", "", ""
	m.reviewHelmRevision(h)
	if h.Comparison != nil || h.Document == nil || !strings.Contains(h.Document.Text(), "Deployment / apps/web") || !strings.Contains(h.Document.Text(), changes.Unavailable) {
		t.Fatal("large comparison opened an empty diff or lost its summary")
	}
	h.Document = nil
	typeText(m, "/")
	typeText(m, "Deployment")
	if target, matched := h.FindTarget(); !matched || target != 0 {
		t.Fatal("large-chart summary is not searchable")
	}
	h.Timeline.Entries[2] = monitor.HelmTimelineEntry{Changes: &helmtool.RevisionChanges{}}
	m.reviewHelmRevision(h)
	if h.Comparison != nil || h.Document != nil || !strings.Contains(h.Notice, "No recorded configuration changes") {
		t.Fatal("unchanged revision opened an empty comparison")
	}
}

func TestHelmResourceGroupSpacingKeepsEverySelectionReachable(t *testing.T) {
	m, h := helmModel(t)
	h.ApplyInspectorSnapshot(inspectorFixture())
	h.Detail.Manifest += "---\napiVersion: v1\nkind: PersistentVolumeClaim\nmetadata: {name: data}\n"
	view := ansi.Strip(monitor.HelmResourceInventory(h, 120, 28))
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Networking") || strings.Contains(line, "Storage") {
			if i == 0 || strings.TrimSpace(lines[i-1][:min(58, len(lines[i-1]))]) != "" {
				t.Fatalf("resource sections have no whitespace: %s", view)
			}
		}
	}
	resources, _ := h.InspectorResources()
	for _, size := range [][2]int{{28, 5}, {50, 12}, {88, 18}, {120, 28}} {
		for cursor, r := range resources {
			h.Subcursor = cursor
			view := ansi.Strip(monitor.HelmResourceInventory(h, size[0], size[1]))
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(view, "› "+r.Kind) {
				t.Fatalf("selection lost at %v: %s", size, view)
			}
			m.updateHelm("enter")
			if h.Document == nil || !strings.Contains(h.Document.Text(), "kind: "+r.Kind) {
				t.Fatal("spacing changed resource selection")
			}
			h.Document = nil
		}
	}
}
