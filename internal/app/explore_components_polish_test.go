package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func assertDetailWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > width {
			t.Fatalf("detail overflow at %d: %q", width, line)
		}
	}
}

func TestSummaryFieldsAlignValuesAndWrapWithoutLosingEvidence(t *testing.T) {
	fields := []kubernetes.DetailField{{Label: "Status", Value: "CrashLoopBackOff"}, {Label: "Namespace", Value: "commerce"}, {Label: "Image", Value: "registry.example.com/platform/checkout@sha256:abc123def456abc123def456"}, {Label: "Message", Value: "first diagnostic line\nsecond diagnostic line"}}
	view := ansi.Strip(strings.Join(monitor.RenderSummaryFields(fields[:2], 60), "\n"))
	lines := strings.Split(view, "\n")
	if len(lines) != 2 || strings.Index(lines[0], "CrashLoopBackOff") != strings.Index(lines[1], "commerce") {
		t.Fatalf("values are not aligned: %s", view)
	}
	for _, width := range []int{28, 48, 80, 120, 160, 200} {
		view := strings.Join(monitor.RenderSummaryFields(fields, width), "\n")
		assertDetailWidth(t, view, width)
		compact := strings.Join(strings.Fields(ansi.Strip(view)), "")
		for _, want := range []string{"CrashLoopBackOff", "commerce", "registry.example.com/platform/checkout@sha256:abc123def456abc123def456", "firstdiagnosticline", "seconddiagnosticline"} {
			if !strings.Contains(compact, want) {
				t.Fatalf("width %d lost %q: %s", width, want, view)
			}
		}
	}
}

func TestDetailTablesWrapLongCellsAndKeepMissingValuesExplicit(t *testing.T) {
	section := kubernetes.DetailSection{Title: "Containers", Columns: []string{"Name", "Image", "State"}, Rows: [][]string{{"checkout", "registry.example.com/platform/checkout@sha256:abc123def456abc123def456", "Running"}, {"sidecar", "", ""}, {"pending"}}}
	for _, width := range []int{28, 48, 80, 120, 160, 200} {
		view := strings.Join(monitor.RenderDetailTable(section, width, "checkout"), "\n")
		assertDetailWidth(t, view, width)
		text := ansi.Strip(view)
		if !strings.Contains(text, "checkout") || !strings.Contains(text, "sidecar") || !strings.Contains(text, "(empty)") || !strings.Contains(text, "Not reported") {
			t.Fatalf("width %d omitted identity or missing value: %s", width, text)
		}
		if width >= 80 && (!strings.Contains(text, " │ ") || !strings.Contains(text, "─┼─")) {
			t.Fatalf("one long value discarded the table at %d: %s", width, text)
		}
		if width >= 80 {
			var image []string
			for _, line := range strings.Split(text, "\n")[2:] {
				if strings.Contains(line, "sidecar") {
					break
				}
				cells := strings.Split(line, " │ ")
				if len(cells) == 3 {
					image = append(image, strings.TrimSpace(cells[1]))
				}
			}
			if strings.Join(image, "") != section.Rows[0][1] {
				t.Fatalf("image reference was truncated or altered at %d: %s", width, text)
			}
		}
	}
}

func TestDetailConditionsDoNotTurnFalseIntoFailure(t *testing.T) {
	section := kubernetes.DetailSection{Title: "Conditions", Columns: []string{"Type", "Status", "Reason", "Message"}, Rows: [][]string{{"Progressing", "False", "Paused", "The workload is intentionally paused."}, {"Available", "True", "MinimumReplicasAvailable", "Deployment has minimum availability."}}}
	for _, width := range []int{48, 80, 120, 160, 200} {
		view := strings.Join(monitor.RenderDetailTable(section, width, ""), "\n")
		assertDetailWidth(t, view, width)
		if strings.Contains(view, ui.RemovedStyle.Render("False")) && ui.RemovedStyle.Render("False") != "False" {
			t.Fatal("False condition was styled as a failure")
		}
		text := strings.Join(strings.Fields(ansi.Strip(view)), "")
		if !strings.Contains(strings.ReplaceAll(text, "│", ""), "Theworkloadisintentionallypaused.") {
			t.Fatal("condition message was lost")
		}
	}
}

func TestSummarySectionsUseOneHeadingAndConsistentSeparation(t *testing.T) {
	m := exploreModel(t)
	object := exploreObject("checkout", "pod-1")
	m.explore.Detail = &object
	for _, width := range []int{28, 48, 80, 120, 160, 200} {
		view := m.resourceSummary(width)
		assertDetailWidth(t, view, width)
		text := ansi.Strip(view)
		if !strings.Contains(text, "▎ Pod overview") || !strings.Contains(text, "\n\n▎ Containers") || strings.Contains(text, "\n\n\n") {
			t.Fatalf("inconsistent summary section structure at %d: %s", width, text)
		}
	}
}

func TestNarrowSummaryKeepsShortFactsOnOneLine(t *testing.T) {
	for _, field := range []kubernetes.DetailField{{Label: "Desired replicas", Value: "3"}, {Label: "Current replicas", Value: "3"}, {Label: "Termination grace seconds", Value: "30"}} {
		width := 28
		if field.Label == "Termination grace seconds" {
			width = 68
		}
		view := ansi.Strip(monitor.SummaryField(field, width))
		if strings.Contains(view, "\n") || !strings.Contains(view, field.Label+": "+field.Value) {
			t.Fatalf("short fact used two lines: %s", view)
		}
	}
}

func TestLongTableProseUsesAvailableWidth(t *testing.T) {
	cases := []kubernetes.DetailSection{
		{Title: "Pod template containers", Columns: []string{"Name", "Type", "Image", "Requests", "Limits"}, Rows: [][]string{{"checkout", "app", "registry.example.com/platform/commerce/checkout-api@sha256:" + strings.Repeat("1234abcd", 8), "cpu=100m, memory=128Mi", "cpu=500m, memory=256Mi"}}},
		{Title: "Conditions", Columns: []string{"Type", "Status", "Reason", "Message", "Transition"}, Rows: [][]string{{"ContainersReady", "False", "ContainersNotReady", "containers with unready status: [checkout]; waiting for database credentials from the checkout-database Secret", "2026-09-30T03:20:00Z"}}},
	}
	for _, section := range cases {
		view := strings.Join(monitor.RenderDetailTable(section, 68, ""), "\n")
		assertDetailWidth(t, view, 68)
		if len(strings.Split(view, "\n")) > 8 {
			t.Fatalf("%s turned one row into a text tower: %s", section.Title, ansi.Strip(view))
		}
		if section.Title == "Conditions" && !strings.Contains(ansi.Strip(view), "Message:") {
			t.Fatal("long condition message lacks a full-width label")
		}
	}
}
