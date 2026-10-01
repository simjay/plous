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
	"github.com/simjay/plous/internal/kubernetes"
)

func TestResourceSectionsShareHeaderAndRefreshWithoutMovingContent(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	e := m.explore
	e.detailValid = true
	for tab := range 4 {
		m.openExploreSelection(tab)
		e.detailValid = true
		before := ansi.Strip(m.exploreBody(100, 40))
		_, cancel := context.WithCancel(context.Background())
		e.detailRun = &exploreDetailRun{cancel: cancel}
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
		e.detailRun = nil
	}
}

func TestDetailSectionCacheRetainsContentAndSeparatesIdentities(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	e := m.explore
	m.syncExploreDetail()
	supplement := []kubernetes.DetailSection{{Title: "Supplement", Fields: []kubernetes.DetailField{{Label: "Evidence", Value: "retained"}}}}
	m.updateExploreMessage(exploreDetailResult{run: e.detailRun, object: e.detail.DeepCopy(), supplement: supplement})
	m.openExploreSelection(2)
	m.syncExploreDetail()
	m.updateExploreMessage(exploreDetailResult{run: e.detailRun, object: e.detail.DeepCopy(), text: "cached event"})
	m.openExploreSelection(0)
	m.syncExploreDetail()
	if !e.detailLoaded || len(e.supplement) != 1 {
		t.Fatal("Describe discarded cached supplement")
	}
	late := e.detailRun
	m.openExploreSelection(2)
	m.syncExploreDetail()
	if !strings.Contains(ansi.Strip(m.exploreBody(100, 40)), "cached event") {
		t.Fatal("Events flashed a loading placeholder")
	}
	m.updateExploreMessage(exploreDetailResult{run: late, text: "late", err: errors.New("late failure")})
	if e.detailText != "cached event" {
		t.Fatal("old request replaced active section")
	}
	e.detail.SetUID("replacement")
	m.syncExploreDetail()
	if e.detailLoaded || e.detailText != "" {
		t.Fatal("cache crossed object UID")
	}
}

func TestEditorLocalSaveCommandsValidateWriteAndQuitScoped(t *testing.T) {
	for _, command := range []string{"w", "wq"} {
		m, a := documentModel(t)
		a.document.Paste("# changed\n")
		if m.execute(command) || a.pending != "validate" || a.closeAfterApply != (command == "wq") {
			t.Fatalf("%s skipped validation", command)
		}
		a.pending = ""
		m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", revision: a.document.Revision, object: a.object.DeepCopy()})
		if a.pending != "apply" || a.stage != "edit" || !a.saveRequested {
			t.Fatal("Save did not proceed after validation")
		}
		a.busy = true
		m.execute("q!")
		if m.explore.action != a {
			t.Fatal("closed a pending operation")
		}
		a.busy = false
		applied := a.object.DeepCopy()
		applied.SetResourceVersion("next")
		m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "apply", object: applied})
		if command == "wq" {
			if m.explore.action != nil {
				t.Fatal("wq did not close after apply")
			}
		} else if m.explore.action != a || a.stage != "edit" || a.document.Dirty() {
			t.Fatal("w did not keep the saved editor")
		}
	}
	m, a := documentModel(t)
	a.document.Paste("change")
	if m.execute("q") || !a.discard {
		t.Fatal("q did not guard modified text")
	}
	m.execute("q!")
	if m.explore.action != nil {
		t.Fatal("q! did not discard")
	}
	m, a = documentModel(t)
	m.focus = 2
	if !m.execute("q") {
		t.Fatal("editor captured another panel's quit")
	}
	m, _ = documentModel(t)
	if !m.execute("quit") {
		t.Fatal("explicit app quit changed")
	}
}

func TestAllPodsDefaultPreservesWorkloadAndContainerControls(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.resource = e.resources[1]
	e.preference.Resource = e.resource.Key()
	e.detail = e.collection.Objects[0].DeepCopy()
	e.detail.SetKind("Deployment")
	e.detail.SetAPIVersion("apps/v1")
	e.detailValid = true
	m.openResourceActions("logs")
	a := e.action
	a.pending = ""
	a.operation = "logs"
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "targets", pods: e.collection.Objects[:1]})
	if a.stage != "pods" || a.cursor != 0 {
		t.Fatal("single-pod workload skipped All Pods choice")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !e.mergedLogs || e.tab != 4 || e.resource.Kind != "Deployment" {
		t.Fatal("All Pods lost workload identity")
	}
	m.openWorkloadLogs()
	if e.logOrigin == nil || e.logOrigin.tab != 0 {
		t.Fatal("reopening All Pods replaced its original return point")
	}
	e.workloadLogStatus = kubernetes.WorkloadLogStatus{Containers: []string{"app", "init"}, Pods: 1, Sources: 1, Failed: 1}
	m.updateExplore("o")
	if e.picker != "containers" || len(m.logContainers()) != 3 {
		t.Fatal("aggregate container chooser unavailable")
	}
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyDown})
	m.updateExploreInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if e.container != "app" {
		t.Fatal("container filter selected wrong source")
	}
	m.updateExplore("esc")
	if e.tab != 0 || e.mergedLogs || e.resource.Kind != "Deployment" {
		t.Fatal("aggregate logs did not restore workload")
	}
}

func TestRelatedGroupsKeepSelectorEvidenceSeparate(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(3)
	e := m.explore
	e.references = sortedReferences([]kubernetes.Reference{
		{Resource: e.resources[0], Name: "selected", Namespace: "shop", Relation: "Selector match"},
		{Resource: e.resources[1], Name: "parent", Namespace: "shop", Relation: "Owner"},
		{Resource: e.resources[0], Name: "child", Namespace: "shop", Relation: "Owned Pod"},
		{Resource: e.resources[3], Name: "worker", Relation: "Owned Pod assigned node"},
	})
	view := ansi.Strip(m.relatedBody(110, 40))
	for _, s := range []string{"Owner", "Managed resources", "Referenced resources", "Selector matches", "Relationship", "Label matches do not establish ownership."} {
		if !strings.Contains(view, s) {
			t.Fatalf("missing %s: %s", s, view)
		}
	}
	if e.references[0].Name != "parent" || e.references[2].Name != "worker" {
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
				e.detailValid = true
				switch scene {
				case "yaml":
					m.openExploreSelection(1)
				case "events":
					m.openExploreSelection(2)
					e.detailText = "Warning BackOff · Retrying container start"
					e.detailLoaded = true
				case "related":
					m.openExploreSelection(3)
					e.references = sortedReferences([]kubernetes.Reference{
						{Resource: e.resources[1], Namespace: "shop", Name: "checkout", Relation: "Owner"},
						{Resource: e.resources[3], Name: "worker-1", Relation: "Assigned node"},
						{Resource: e.resources[2], Name: "checkout-service", Namespace: "shop", Relation: "Service selector match"},
					})
				case "editor":
					m.openResourceActions("edit")
					a := e.action
					a.pending = ""
					a.stage = "edit"
					a.initDocument()
					a.document.Paste("# new configuration\n")
				case "merged-logs":
					e.resource = e.resources[1]
					e.detail.SetKind("Deployment")
					e.tab = 4
					e.mergedLogs = true
					e.logKey = "fixture"
					e.workloadLogStatus = kubernetes.WorkloadLogStatus{Pods: 4, Sources: 35, Active: 30, Failed: 1, Finished: 1, Skipped: 3}
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
	e.detailValid = true
	for i := range 15 {
		e.references = append(e.references, kubernetes.Reference{Resource: e.resources[0], Name: fmt.Sprint("pod-", i), Relation: "Owned Pod"})
	}
	e.detailText = strings.Repeat("Permission unavailable for a related API.\n", 20) + "Last diagnostic"
	m.updateExplore("end")
	if !strings.Contains(ansi.Strip(m.render()), "Last diagnostic") {
		t.Fatal("End cannot reach relationship diagnostics")
	}
	m.updateExplore("up")
	if e.scroll != 0 {
		t.Fatal("row navigation did not resume selection tracking")
	}
}

func TestEditorReviewExitClearsCloseAfterApplyIntent(t *testing.T) {
	m, a := documentModel(t)
	a.document.Paste("# draft\n")
	m.execute("wq")
	if !a.closeAfterApply {
		t.Fatal("wq did not set close intent")
	}
	a.pending = ""
	m.updateResourceActionResult(resourceActionResult{action: a, serial: a.serial, job: "validate", err: errors.New("invalid")})
	if a.closeAfterApply {
		t.Fatal("failed validation kept an earlier close intent")
	}
	a.closeAfterApply = true
	m.validateDocument()
	if a.closeAfterApply || a.saveRequested {
		t.Fatal("normal validation did not keep the document open")
	}
}
