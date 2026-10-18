package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestResourceSectionsShareHeaderAndRefreshWithoutMovingContent(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	e := m.explore
	e.DetailValid = true
	for tab := range 4 {
		m.openExploreSelection(tab)
		e.DetailValid = true
		before := ansi.Strip(m.exploreBody(100, 40))
		_, cancel := context.WithCancel(context.Background())
		e.DetailRun = &monitor.ExploreDetailRun{Cancel: cancel}
		after := ansi.Strip(m.exploreBody(100, 40))
		for _, s := range []string{"ctx test-context", "Workloads / Pods", "checkout", "d Describe", "e Events", "r Related"} {
			if !strings.Contains(before, s) || !strings.Contains(after, s) {
				t.Fatalf("tab %d missing %s: %s", tab, s, after)
			}
		}
		if tab == 1 && strings.Contains(after, "Plous Editor") {
			t.Fatal("YAML duplicated the resource identity")
		}
		if strings.Index(before, "d Describe") != strings.Index(after, "d Describe") {
			// A status label can change the preceding row's length, but not its row number.
			line := func(s string) int { return strings.Count(s[:strings.Index(s, "d Describe")], "\n") }
			if line(before) != line(after) {
				t.Fatal("refresh moved section controls")
			}
		}
		cancel()
		e.DetailRun = nil
	}
}

func TestDetailSectionCacheRetainsContentAndSeparatesIdentities(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	e := m.explore
	m.syncExploreDetail()
	supplement := []kubernetes.DetailSection{{Title: "Supplement", Fields: []kubernetes.DetailField{{Label: "Evidence", Value: "retained"}}}}
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: e.DetailRun, Object: e.Detail.DeepCopy(), Supplement: supplement})
	m.openExploreSelection(2)
	m.syncExploreDetail()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: e.DetailRun, Object: e.Detail.DeepCopy(), Text: "cached event"})
	m.openExploreSelection(0)
	m.syncExploreDetail()
	if !e.DetailLoaded || len(e.Supplement) != 1 {
		t.Fatal("Describe discarded cached supplement")
	}
	late := e.DetailRun
	m.openExploreSelection(2)
	m.syncExploreDetail()
	if !strings.Contains(ansi.Strip(m.exploreBody(100, 40)), "cached event") {
		t.Fatal("Events flashed a loading placeholder")
	}
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: late, Text: "late", Err: errors.New("late failure")})
	if e.DetailText != "cached event" {
		t.Fatal("old request replaced active section")
	}
	e.Detail.SetUID("replacement")
	m.syncExploreDetail()
	if e.DetailLoaded || e.DetailText != "" {
		t.Fatal("cache crossed object UID")
	}
}

func TestEditorLocalSaveCommandsValidateWriteAndQuitScoped(t *testing.T) {
	for _, command := range []string{"w", "wq"} {
		m, a := documentModel(t)
		a.Document.Paste("# changed\n")
		if m.execute(command) || a.Pending != "validate" || a.CloseAfterApply != (command == "wq") {
			t.Fatalf("%s skipped validation", command)
		}
		a.Pending = ""
		m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Revision: a.Document.Revision, Object: a.Object.DeepCopy()})
		if a.Pending != "apply" || a.Stage != "edit" || !a.SaveRequested {
			t.Fatal("Save did not proceed after validation")
		}
		a.Busy = true
		m.execute("q!")
		if m.explore.Action != a {
			t.Fatal("closed a pending operation")
		}
		a.Busy = false
		applied := a.Object.DeepCopy()
		applied.SetResourceVersion("next")
		m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "apply", Object: applied})
		if command == "wq" {
			if m.explore.Action != nil {
				t.Fatal("wq did not close after apply")
			}
		} else if m.explore.Action != a || a.Stage != "edit" || a.Document.Dirty() {
			t.Fatal("w did not keep the saved editor")
		}
	}
	m, a := documentModel(t)
	a.Document.Paste("change")
	if m.execute("q") || !a.Discard {
		t.Fatal("q did not guard modified text")
	}
	m.execute("q!")
	if m.explore.Action != nil {
		t.Fatal("q! did not discard")
	}
	m, _ = documentModel(t)
	m.focus = 2
	m.execute("q")
	if !m.quitting {
		t.Fatal("editor captured another panel's quit")
	}
	m, _ = documentModel(t)
	m.execute("quit")
	if !m.quitting {
		t.Fatal("explicit app quit changed")
	}
}

func TestAllPodsDefaultPreservesWorkloadAndContainerControls(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Resource = e.Resources[1]
	e.Preference.Resource = e.Resource.Key()
	e.Detail = e.Collection.Objects[0].DeepCopy()
	e.Detail.SetKind("Deployment")
	e.Detail.SetAPIVersion("apps/v1")
	e.DetailValid = true
	m.openResourceActions("logs")
	a := e.Action
	a.Pending = ""
	a.Operation = "logs"
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "targets", Pods: e.Collection.Objects[:1]})
	if a.Stage != "pods" || a.Cursor != 0 {
		t.Fatal("single-pod workload skipped All Pods choice")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !e.MergedLogs || e.Tab != 4 || e.Resource.Kind != "Deployment" {
		t.Fatal("All Pods lost workload identity")
	}
	m.openWorkloadLogs()
	if e.LogOrigin == nil || e.LogOrigin.Tab != 0 {
		t.Fatal("reopening All Pods replaced its original return point")
	}
	e.WorkloadLogStatus = kubernetes.WorkloadLogStatus{Containers: []string{"app", "init"}, Pods: 1, Sources: 1, Failed: 1}
	m.updateExplore("o")
	if e.Picker != "containers" || len(m.logContainers()) != 3 {
		t.Fatal("aggregate container chooser unavailable")
	}
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if e.Container != "app" {
		t.Fatal("container filter selected wrong source")
	}
	m.updateExplore("esc")
	if e.Tab != 0 || e.MergedLogs || e.Resource.Kind != "Deployment" {
		t.Fatal("aggregate logs did not restore workload")
	}
}

func TestRelatedGroupsKeepSelectorEvidenceSeparate(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(3)
	e := m.explore
	e.References = monitor.SortedReferences([]kubernetes.Reference{
		{Resource: e.Resources[0], Name: "selected", Namespace: "shop", Relation: "Selector match"},
		{Resource: e.Resources[1], Name: "parent", Namespace: "shop", Relation: "Owner"},
		{Resource: e.Resources[0], Name: "child", Namespace: "shop", Relation: "Owned Pod"},
		{Resource: e.Resources[3], Name: "worker", Relation: "Owned Pod assigned node"},
	})
	view := ansi.Strip(m.relatedBody(110, 40))
	for _, s := range []string{"Owner", "Managed resources", "Referenced resources", "Selector matches", "Relationship", "Label matches do not establish ownership."} {
		if !strings.Contains(view, s) {
			t.Fatalf("missing %s: %s", s, view)
		}
	}
	if e.References[0].Name != "parent" || e.References[2].Name != "worker" {
		t.Fatal("misclassified ownership")
	}
}

func TestDetailUpdateFramesFitAcrossTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}, {160, 40}, {200, 48}} {
		for _, scene := range []string{"describe", "yaml", "events", "related", "editor", "merged-logs"} {
			t.Run(fmt.Sprintf("%s/%d", scene, size[0]), func(t *testing.T) {
				m := exploreModel(t)
				m.width, m.height = size[0], size[1]
				m.openExploreSelection(0)
				e := m.explore
				e.DetailValid = true
				switch scene {
				case "yaml":
					m.openExploreSelection(1)
				case "events":
					m.openExploreSelection(2)
					e.DetailText = "Warning BackOff · Retrying container start"
					e.DetailLoaded = true
				case "related":
					m.openExploreSelection(3)
					e.References = monitor.SortedReferences([]kubernetes.Reference{
						{Resource: e.Resources[1], Namespace: "shop", Name: "checkout", Relation: "Owner"},
						{Resource: e.Resources[3], Name: "worker-1", Relation: "Assigned node"},
						{Resource: e.Resources[2], Name: "checkout-service", Namespace: "shop", Relation: "Service selector match"},
					})
				case "editor":
					m.openResourceActions("edit")
					a := e.Action
					a.Pending = ""
					a.Stage = "edit"
					a.InitDocument()
					a.Document.Paste("# new configuration\n")
				case "merged-logs":
					e.Resource = e.Resources[1]
					e.Detail.SetKind("Deployment")
					e.Tab = 4
					e.MergedLogs = true
					e.LogKey = "fixture"
					e.WorkloadLogStatus = kubernetes.WorkloadLogStatus{Pods: 4, Sources: 35, Active: 30, Failed: 1, Finished: 1, Skipped: 3}
					m.podLogViewer().Append("[checkout-blue/app] 2026-09-30T12:00:00Z INFO request completed\n[checkout-green/app] 2026-09-30T12:00:01Z WARN retrying database\n")
				}
				m.status = ""
				view := m.render()
				if scene == "merged-logs" && size[0] == 80 {
					if !strings.Contains(ansi.Strip(view), "checkout-green/app") || !strings.Contains(ansi.Strip(view), "database") || !strings.Contains(ansi.Strip(view), "3 omitted") {
						t.Fatalf("compact log header hides output or incomplete coverage:\n%s", ansi.Strip(view))
					}
				}
				if len(strings.Split(view, "\n")) != m.height {
					t.Fatal("frame height changed")
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("row width %d expected %d", ansi.StringWidth(line), m.width)
					}
				}
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.ansi", scene, m.width)), []byte(view), 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestRelatedEndReachesUnavailableNotes(t *testing.T) {
	m := exploreModel(t)
	m.width, m.height = 120, 30
	m.openExploreSelection(3)
	e := m.explore
	e.DetailValid = true
	for i := range 15 {
		e.References = append(e.References, kubernetes.Reference{Resource: e.Resources[0], Name: fmt.Sprint("pod-", i), Relation: "Owned Pod"})
	}
	e.DetailText = strings.Repeat("Permission unavailable for a related API.\n", 20) + "Last diagnostic"
	m.updateExplore("end")
	if !strings.Contains(ansi.Strip(m.render()), "Last diagnostic") {
		t.Fatal("End cannot reach relationship diagnostics")
	}
	m.updateExplore("up")
	if e.Scroll != 0 {
		t.Fatal("row navigation did not resume selection tracking")
	}
}

func TestEditorReviewExitClearsCloseAfterApplyIntent(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("# draft\n")
	m.execute("wq")
	if !a.CloseAfterApply {
		t.Fatal("wq did not set close intent")
	}
	a.Pending = ""
	m.updateResourceActionResult(monitor.ResourceActionResult{Action: a, Serial: a.Serial, Job: "validate", Err: errors.New("invalid")})
	if a.CloseAfterApply {
		t.Fatal("failed validation kept an earlier close intent")
	}
	a.CloseAfterApply = true
	m.validateDocument()
	if a.CloseAfterApply || a.SaveRequested {
		t.Fatal("normal validation did not keep the document open")
	}
}
