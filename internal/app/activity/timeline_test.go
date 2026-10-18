package activity

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func TestTimelineGroupsExactRevisionAndOrdersBySourceTime(t *testing.T) {
	source := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	observed := source.Add(48 * time.Hour)
	base := workspace.Operation{ID: "first", WorkspaceID: "workspace", ConnectionID: "connection", ConnectionFingerprint: "config", Tool: "helm", Context: "local", Namespace: "apps", Release: "checkout", Kind: "revision", Revision: 1, Status: "deployed", SourceTime: source.Format(time.RFC3339), ObservedAt: observed}
	next := base
	next.ID = "second"
	next.Status = "superseded"
	next.ObservedAt = observed.Add(time.Hour)
	revision := next
	revision.ID = "rev2"
	revision.Revision = 2
	revision.SourceTime = source.Add(time.Hour).Format(time.RFC3339)
	changedConfig := base
	changedConfig.ID = "config2"
	changedConfig.ConnectionFingerprint = "other"
	changedConnection := base
	changedConnection.ID = "connection2"
	changedConnection.ConnectionID = "other"
	legacy := base
	legacy.ID = "legacy"
	legacy.ConnectionFingerprint = ""
	legacy2 := legacy
	legacy2.ID = "legacy2"
	missing := base
	missing.ID = "missing"
	missing.Release = "missing-time"
	missing.SourceTime = "not a timestamp"
	a := Controller{Operations: []workspace.Operation{base, next, revision, changedConfig, changedConnection, legacy, legacy2, missing}}
	original := append([]workspace.Operation(nil), a.Operations...)
	entries := a.Entries("workspace", "", "", "")
	if len(entries) != 7 {
		t.Fatalf("grouped distinct revisions or identities: %+v", entries)
	}
	if entries[0].Operation.ID != "missing" || entries[0].SourceTime || !strings.Contains(entries[0].Metadata(), "observation time") {
		t.Fatal("fallback must be explicit", entries[0])
	}
	if entries[1].Operation.ID != "rev2" {
		t.Fatal("newest source event was not first", entries[1])
	}
	found := false
	for _, e := range entries {
		if e.Operation.ID == "second" {
			found = true
			if len(e.Observations) != 2 || e.Outcome() != "superseded" || !e.At.Equal(source) || !e.SourceTime {
				t.Fatal("incorrect grouped status/time", e)
			}
			if !strings.Contains(e.Detail(true), "first") || !strings.Contains(e.Detail(true), "second") {
				t.Fatal("observations missing")
			}
		}
	}
	if !found || !reflect.DeepEqual(original, a.Operations) {
		t.Fatal("lost group or mutated saved operations")
	}
	a.Filter = HelmHistory
	matches := a.Entries("workspace", "helm", "connection", "deployed")
	if len(matches) != 5 {
		t.Fatalf("search must include older observations within connection scope: %d", len(matches))
	}
}

func TestActivityTypesSummariesAndUnknownOutcomes(t *testing.T) {
	before := workspace.Workspace{Name: "Demo"}
	connection := workspace.Connection{ID: "repo", Name: "Remote Test", Tool: "repository"}
	after := before
	after.Connections = []workspace.Connection{connection}
	added := workspace.Change{ID: "add", Before: &before, After: after, Revision: 2}
	removed := workspace.Change{Before: &after, After: before}
	if ChangeTitle(added) != "Added Remote Test" || ChangeTitle(removed) != "Removed Remote Test" {
		t.Fatal("summaries omit affected names")
	}
	batch := after
	batch.Connections = append(append([]workspace.Connection(nil), after.Connections...), workspace.Connection{ID: "local", Name: "Local source"})
	if ChangeTitle(workspace.Change{Before: &before, After: batch}) != "Added Remote Test + 1 more" {
		t.Fatal("batch additions must retain their action and affected target")
	}
	renamed := after
	renamed.Connections = []workspace.Connection{{ID: "repo", Name: "Prod", Tool: "repository"}}
	if ChangeTitle(workspace.Change{Before: &after, After: renamed}) != "Updated Prod" {
		t.Fatal("connection rename misclassified")
	}
	a := Controller{Changes: []workspace.Change{added}, Operations: []workspace.Operation{{ID: "install", Kind: "install", Tool: "helm", Release: "checkout", Status: "submitted"}, {ID: "revision", Kind: "revision", Tool: "helm", Release: "checkout", Status: "deployed"}}}
	for filter, count := range map[Filter]int{All: 3, PlousActions: 1, WorkspaceChanges: 1, HelmHistory: 1} {
		a.Filter = filter
		entries := a.Entries("workspace", "", "", "")
		if len(entries) != count {
			t.Fatalf("%s: %d", filter, len(entries))
		}
		if filter == PlousActions && (entries[0].Outcome() != "unknown" || !strings.Contains(entries[0].Preview(), "Outcome unknown")) {
			t.Fatal("submission shown as success")
		}
	}
	a.Filter = WorkspaceChanges
	if len(a.Entries("workspace", "", "", "Remote Test")) != 1 {
		t.Fatal("named target not searchable")
	}
}

func TestTimelineBoundsAndSelectedVisibility(t *testing.T) {
	entries := []ActivityEntry{}
	for i := 0; i < 60; i++ {
		entries = append(entries, ActivityEntry{Change: &workspace.Change{ID: fmt.Sprint(i), After: workspace.Workspace{Name: fmt.Sprintf("item-%02d", i)}}, At: time.Date(2026, 10, 1+i/3, 12, 0, 0, 0, time.UTC)})
	}
	for _, width := range []int{24, 45, 46, 60, 100} {
		for _, height := range []int{1, 2, 3, 5, 16} {
			for _, selected := range []int{0, 1, 7, 59} {
				view := Timeline(entries, selected, width, height)
				clean, _ := textview.MouseLayout(view)
				rows := strings.Split(ansi.Strip(clean), "\n")
				if len(rows) > height {
					t.Fatalf("height overflow %d", height)
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > width {
						t.Fatalf("width overflow: %q", row)
					}
				}
				if !strings.Contains(clean, "›") {
					t.Fatalf("selection clipped %dx%d at %d", width, height, selected)
				}
			}
		}
	}
	e := ActivityEntry{Operation: &workspace.Operation{Status: "failed", Release: "broken", Kind: "install"}}
	if !strings.Contains(timelineRow(e, true, 60), ui.RemovedStyle.Render("failed")) {
		t.Fatal("selection erased outcome color")
	}
	for _, width := range []int{24, 80} {
		if !strings.Contains(timelineRow(entries[0], true, width), ui.AddedStyle.Render("+")) {
			t.Fatal("selection erased change color")
		}
	}
	// A clipped viewport must never place a new day's entry under the preceding date.
	entries = []ActivityEntry{entries[0], entries[3]}
	clean, _ := textview.MouseLayout(Timeline(entries, 0, 80, 4))
	if strings.Contains(ansi.Strip(clean), "item-03") {
		t.Fatal("entry shown without its date group")
	}
	clean, _ = textview.MouseLayout(Timeline(entries, 0, 80, 5))
	if !strings.Contains(ansi.Strip(clean), "Fri, Oct 2") {
		t.Fatal("new group lost its date")
	}
}
