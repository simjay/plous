package tui

import (
	"context"
	"encoding/json"
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
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
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

func TestServiceReadinessWatchRefreshesOnlyCapturedUIDAndNewerObservation(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Resource = kubecommon.Resource{Name: "services", Kind: "Service", Version: "v1", Namespaced: true}
	e.Detail = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "checkout", "namespace": "shop", "uid": "captured"}}}
	e.DetailValid = true
	observed := time.Now()
	update := func(uid string, ready int, at time.Time) {
		object := e.Detail.DeepCopy()
		object.SetUID(types.UID(uid))
		collection := kubernetes.Collection{Objects: []unstructured.Unstructured{*object}, ServiceEndpoints: map[string]kubernetes.EndpointReadiness{uid: {Ready: ready, Total: 2, ObservedAt: at}}}
		m.updateExploreMessage(monitor.ExploreUpdate{Stream: e.Stream, Snapshot: kubernetes.Snapshot{Collection: &collection}})
	}
	update("captured", 1, observed)
	if e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("Service watch did not refresh endpoint readiness")
	}
	update("replacement", 2, observed.Add(time.Second))
	if e.Detail.GetUID() != "captured" || e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("replacement Service retargeted the open detail supplement")
	}
	update("captured", 0, observed.Add(-time.Second))
	if e.Supplement[0].Fields[0].Value != "1/2" {
		t.Fatal("older list snapshot overwrote newer readiness")
	}
	update("captured", 2, observed.Add(2*time.Second))
	if e.Supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("newer readiness observation was not displayed")
	}
	run := &monitor.ExploreDetailRun{}
	e.DetailRun = run
	older, _ := kubernetes.EndpointReadinessSummary(kubernetes.EndpointReadiness{Ready: 0, Total: 2, ObservedAt: observed})
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: e.Detail.DeepCopy(), Supplement: older, SupplementAt: observed})
	if e.Supplement[0].Fields[0].Value != "2/2" {
		t.Fatal("an older detail query overwrote newer watch readiness")
	}
}

func TestReadOnlyActivityRoutesLiteralSearchAndGlobalNavigation(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	mainCommand(t, m, "3")
	press(t, m, "enter")
	m.cursor = 3
	summary := m.detail
	typeText(m, "d")
	if !m.activityReviewVisible() {
		t.Fatal("visible View changes shortcut did not open shared comparison")
	}
	r := m.activityReview.Comparison
	typeText(m, "/bbii33:q")
	m.Update(tea.PasteMsg{Content: "\n:settings"})
	if r.Reader.Query != "bbii33:q:settings" || m.mode != 2 || m.leftHidden || m.rightHidden || m.command != nil || m.settingsOverlay != nil {
		t.Fatal("comparison search escaped into application navigation")
	}
	press(t, m, "esc")
	typeText(m, "b")
	if !strings.Contains(ansi.Strip(m.footer()), "b → b") {
		t.Fatal("comparison hid the pending global shortcut")
	}
	press(t, m, "esc")
	if !m.activityReviewVisible() {
		t.Fatal("clearing a pending prefix also left comparison")
	}
	typeText(m, "v")
	press(t, m, "alt+right")
	if m.focus != 2 || !r.Reader.Selecting {
		t.Fatal("panel change lost comparison selection")
	}
	press(t, m, "alt+left")
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if !m.activityReviewVisible() || !r.Reader.Selecting {
		t.Fatal("Help lost comparison reading state")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if !m.activityDetailActive() || m.detail != summary || m.cursor != 3 {
		t.Fatal("comparison did not return to the exact summary position")
	}
	mainCommand(t, m, "v")
	if !m.activityReviewVisible() {
		t.Fatal("named View changes command did not reopen comparison")
	}
	m.execute("applychange")
	if r.CanConfirm || m.explore != nil && m.explore.Action != nil {
		t.Fatal("Activity exposed a resource write operation")
	}
}

func TestReadOnlyYAMLRoutesSearchSelectionAndExplicitEdit(t *testing.T) {
	m := yamlViewModel(t)
	m.syncExploreDetail()
	run := m.explore.DetailRun
	run.Cancel()
	m.Update(monitor.ExploreDetailResult{Run: run, Object: m.explore.Detail.DeepCopy()})
	m.exploreDetailBody(84, 28)
	d := m.explore.YamlDocument
	original := d.Text
	typeText(m, "/bbii33:q")
	m.Update(tea.PasteMsg{Content: "\n:settings"})
	if d.Document.Query != "bbii33:q:settings" || m.mode != 1 || m.leftHidden || m.rightHidden || m.command != nil {
		t.Fatal("YAML search escaped into application navigation")
	}
	press(t, m, "esc")
	typeText(m, "z")
	press(t, m, "alt+right")
	press(t, m, "alt+left")
	if d.Document.Mode != docedit.Select || m.explore.YamlDocument != d {
		t.Fatal("panel focus lost YAML selection")
	}
	mainCommand(t, m, "help")
	press(t, m, "esc")
	if d.Document.Mode != docedit.Select || m.explore.YamlDocument != d {
		t.Fatal("Help replaced the YAML reader")
	}
	press(t, m, "esc")
	m.Update(tea.PasteMsg{Content: "kind: Secret\n"})
	typeText(m, "ii")
	if m.assistantSize != assistantDouble || d.Text != original || m.explore.Action != nil {
		t.Fatal("read-only navigation changed the document or blocked a global shortcut")
	}
	_, copied := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if copied == nil || !strings.Contains(ansi.Strip(m.footer()), "terminal clipboard") {
		t.Fatal("YAML copy feedback was hidden")
	}
	typeText(m, "E")
	if m.explore.Action == nil || !m.explore.Action.Requested || m.explore.Action.Items[m.explore.Action.Cursor].Id != "edit" {
		t.Fatal("explicit Edit did not enter the resource edit workflow")
	}
}

func readOnlyPersistenceModel(t *testing.T, view string) *model {
	t.Helper()
	if view == "YAML" {
		return yamlViewModel(t)
	}
	m, _ := testModel(t)
	createWorkspace(t, m, "Commerce")
	mainCommand(t, m, "3")
	press(t, m, "enter")
	typeText(m, "d")
	if !m.activityReviewVisible() {
		t.Fatal("Activity comparison did not open")
	}
	return m
}

func TestReadOnlyGlobalShortcutsAutosaveSession(t *testing.T) {
	for _, view := range []string{"Activity", "YAML"} {
		for _, shortcut := range []string{"bb", "ii", "11", "22", "33"} {
			t.Run(view+"/"+shortcut, func(t *testing.T) {
				m := readOnlyPersistenceModel(t, view)
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				initialMode := m.mode
				typeText(m, shortcut[:1])
				m.Update(autosaveMsg{})
				m.drainPersistence()
				if m.dirty {
					t.Fatal("autosave between prefix keys should clear dirty state")
				}
				typeText(m, shortcut[1:])
				if !m.dirty {
					t.Fatal("completed global shortcut must mark the session for autosave")
				}
				m.Update(autosaveMsg{})
				m.drainPersistence()
				data, err := m.store.ReadSession()
				if err != nil {
					t.Fatal(err)
				}
				var saved session
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				wantMode := initialMode
				if shortcut[0] >= '1' && shortcut[0] <= '3' {
					wantMode = int(shortcut[0] - '1')
				}
				if saved.Mode != wantMode || saved.LeftHidden != (shortcut == "bb") || saved.RightHidden || (saved.AssistantSize == assistantDouble) != (shortcut == "ii") {
					t.Fatalf("autosave lost completed shortcut: mode=%d, sidebar hidden=%t, AI hidden=%t", saved.Mode, saved.LeftHidden, saved.RightHidden)
				}
			})
		}
	}
}

func TestReadOnlySearchDoesNotRunOrPersistGlobalShortcuts(t *testing.T) {
	for _, view := range []string{"Activity", "YAML"} {
		t.Run(view, func(t *testing.T) {
			m := readOnlyPersistenceModel(t, view)
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			before, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			mode := m.mode
			typeText(m, "/bbii112233:settings")
			m.Update(tea.PasteMsg{Content: "\n:quit"})
			query := ""
			if view == "Activity" {
				query = m.activityReview.Comparison.Reader.Query
			} else {
				query = m.explore.YamlDocument.Document.Query
			}
			if query != "bbii112233:settings:quit" || m.mode != mode || m.leftHidden || m.rightHidden || m.command != nil || m.settingsOverlay != nil || m.dirty {
				t.Fatal("literal search input reached global shortcuts or session persistence")
			}
			m.Update(autosaveMsg{})
			m.drainPersistence()
			after, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("transient reader search changed the persisted session")
			}
		})
	}
}
