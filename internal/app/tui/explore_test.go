package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

func exploreObject(name, uid string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{"kind": "Pod", "apiVersion": "v1", "metadata": map[string]any{"name": name, "namespace": "shop", "uid": uid}, "spec": map[string]any{"nodeName": "worker-1", "containers": []any{map[string]any{"name": "app", "image": "example/app:v1"}}}, "status": map[string]any{"phase": "Running", "containerStatuses": []any{map[string]any{"name": "app", "ready": true, "restartCount": int64(1)}}}}}
}

func exploreModel(t *testing.T) *model {
	t.Helper()
	m, _ := testModel(t)
	if !m.saveWorkspace(workspace.Workspace{Name: "Shop", Connections: []workspace.Connection{{Tool: "kubernetes", Name: "test-context", Settings: map[string]string{"context": "test-context", "namespace": "shop", "kubeconfig": "/nonexistent-test-config"}}}}) {
		t.Fatal(m.status)
	}
	c := m.current().Connections[0]
	m.connection = c.ID
	m.tool = "kubernetes"
	m.mode = 1
	m.page = ""
	m.focus = 1
	m.settingsOverlay = nil
	m.form = nil
	m.loadChat()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	resources := []kubecommon.Resource{{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true, Verbs: []string{"list", "watch"}, ShortNames: []string{"po"}}, {Name: "deployments", Kind: "Deployment", Group: "apps", Version: "v1", Namespaced: true}, {Name: "services", Kind: "Service", Version: "v1", Namespaced: true}, {Name: "nodes", Kind: "Node", Version: "v1"}}
	ctx, cancel := context.WithCancel(context.Background())
	m.explore = &monitor.ExploreState{Key: strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00"), Generation: 1, Client: client, Resources: resources, Resource: resources[0], Preference: monitor.ExplorePreference{Resource: "pods", Namespace: "shop"}, Collection: kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("checkout", "one"), exploreObject("catalog", "two")}}, Selected: "one", Status: "Live", Stream: &monitor.ExploreStream{Key: "pods/v1/shop/", Ctx: ctx, Cancel: cancel, Events: make(chan kubernetes.Snapshot)}}
	m.exploreGeneration = 1
	m.explore.Listed = true
	t.Cleanup(m.stopExplore)
	return m
}

func TestExploreCancelsRequestsWhenLeavingActiveConnection(t *testing.T) {
	for _, action := range []string{"overview", "activity", "connection", "workspace", "quit"} {
		t.Run(action, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			detail, cancelDetail := context.WithCancel(context.Background())
			logs, cancelLogs := context.WithCancel(context.Background())
			defer cancelDetail()
			defer cancelLogs()
			e.DetailRun = &monitor.ExploreDetailRun{Cancel: cancelDetail}
			e.Log = &monitor.ExploreLogRun{Ctx: logs, Cancel: cancelLogs}
			watch := e.Stream.Ctx
			switch action {
			case "overview":
				typeText(m, "11")
			case "activity":
				typeText(m, "33")
			case "connection":
				m.workspaces[0].Connections = append(m.workspaces[0].Connections, workspace.Connection{ID: "other", Tool: "kubernetes", Name: "other", Settings: map[string]string{"kubeconfig": "/nonexistent-test-config", "context": "other"}})
				m.selectScope(m.active, "kubernetes", "other")
			case "workspace":
				m.selectScope(m.active, "", "")
			case "quit":
				assertCtrlCQuit(t, m)
			}
			for name, ctx := range map[string]context.Context{"watch": watch, "detail": detail, "logs": logs} {
				if ctx.Err() == nil {
					t.Fatalf("%s still runs after %s", name, action)
				}
			}
		})
	}
}

func TestExploreLoadingAndEmptyNamespaceHaveDistinctMessages(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Collection = kubernetes.Collection{}
	e.Preference.Namespace = "default"
	e.Stream.Key = "pods/v1/default/"
	e.Listed = false
	view := ansi.Strip(m.exploreTableBody(70, 10))
	if !strings.Contains(view, "Loading Pods") || strings.Contains(view, "No Pods") {
		t.Fatalf("pending request looked empty: %s", view)
	}
	m.Update(monitor.ExploreUpdate{Stream: e.Stream, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	view = ansi.Strip(m.exploreTableBody(70, 10))
	if !e.Listed || !strings.Contains(view, "No Pods in namespace default.") || !strings.Contains(view, "[n] Choose namespace") || !strings.Contains(view, "All namespaces") {
		t.Fatalf("empty namespace lacks scope and recovery: %s", view)
	}
	mainCommand(t, m, "N *")
	if e.Listed || e.Preference.Namespace != "" {
		t.Fatal("namespace change did not reset loading state")
	}
	m.Update(monitor.ExploreUpdate{Stream: e.Stream, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	view = ansi.Strip(m.exploreTableBody(70, 10))
	if !strings.Contains(view, "No Pods across all namespaces.") {
		t.Fatalf("all-namespace scope unclear: %s", view)
	}
}

func TestExploreEmptyRecoveryControls(t *testing.T) {
	for _, input := range []string{"keyboard", "mouse"} {
		t.Run(input, func(t *testing.T) {
			m := exploreModel(t)
			m.width, m.height = 160, 40
			e := m.explore
			e.Collection.Objects = nil
			if input == "mouse" {
				clickMouse(t, m, "main", "action/n")
			} else {
				typeText(m, "n")
			}
			if e.Picker != "namespace" || e.Preference.Namespace != "shop" {
				t.Fatal("namespace action must open the picker without changing scope")
			}
			press(t, m, "esc")
			e.Preference.Filter, e.Preference.Selector = "no-match", "app=checkout"
			view := ansi.Strip(m.exploreTableBody(70, 20))
			if !strings.Contains(view, "No matching Pods.") || !strings.Contains(view, "[Backspace] Clear text filter") || !strings.Contains(view, "[L] Edit label selector") {
				t.Fatalf("filtered result lacks targeted recovery: %s", view)
			}
			if input == "mouse" {
				clickMouse(t, m, "footer", "action/Backspace")
				clickMouse(t, m, "main", "action/L")
			} else {
				press(t, m, "backspace")
				typeText(m, "L")
			}
			if e.Preference.Filter != "" || e.Preference.Selector != "app=checkout" || e.InputKind != "selector" || e.Input == nil {
				t.Fatal("filter actions must preserve labels and open their editor")
			}
			if strings.Contains(ansi.Strip(m.exploreBody(70, 20)), "[L] Edit label selector") {
				t.Fatal("a text field advertised navigation actions")
			}
		})
	}
}

func TestExploreEmptyScopesAndCompactLayout(t *testing.T) {
	for _, scope := range []string{"namespace", "all namespaces", "cluster"} {
		m := exploreModel(t)
		e := m.explore
		e.Collection.Objects = nil
		if scope == "all namespaces" {
			e.Preference.Namespace = ""
		} else if scope == "cluster" {
			e.Resource = e.Resources[3]
		}
		footer := ansi.Strip(m.footer())
		if strings.Contains(footer, "Enter Open") || strings.Contains(footer, "/ Filter") {
			t.Fatalf("empty footer offers actions without rows: %s", footer)
		}
		for _, size := range [][2]int{{24, 4}, {32, 8}, {70, 20}} {
			view := ansi.Strip(m.exploreTableBody(size[0], size[1]))
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatalf("empty state overflows %v: %s", size, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("empty state is too wide at %v: %s", size, view)
				}
			}
			if scope == "namespace" && !strings.Contains(view, "[n] Choose namespace") || scope != "namespace" && !strings.Contains(view, "[g] Refresh") {
				t.Fatalf("missing primary action at %v: %s", size, view)
			}
			if strings.Contains(view, ":k") || strings.Contains(view, "/ Filter resources") || scope == "cluster" && strings.Contains(view, "namespace") {
				t.Fatalf("irrelevant empty-state instructions: %s", view)
			}
			if size[1] == 20 && !strings.Contains(view, "+------+ |") {
				t.Fatalf("roomy empty state lacks illustration: %s", view)
			}
		}
	}
}

func TestExploreCommandsRouteGloballyWithoutLosingChat(t *testing.T) {
	for _, focus := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(focus), func(t *testing.T) {
			m := exploreModel(t)
			m.focus = focus
			m.draft().Insert("Keep this draft", true)
			typeText(m, ":deployments")
			press(t, m, "enter")
			if m.mode != 1 || m.focus != 1 || m.explore.Preference.Resource != "deployments.apps" || m.draft().Text != "Keep this draft" {
				t.Fatal("resource command lost scope or draft")
			}
		})
	}
}

func TestExploreCommandFromOverviewPreservesExplicitNamespace(t *testing.T) {
	m := exploreModel(t)
	client, resources := m.explore.Client, m.explore.Resources
	mainCommand(t, m, "1")
	mainCommand(t, m, "pods platform")
	mainCommand(t, m, "chat")
	m.Update(monitor.ExploreConnected{Generation: m.explore.Generation, Client: client, Resources: resources})
	if m.explore.Preference.Resource != "pods" || m.explore.Preference.Namespace != "platform" {
		t.Fatalf("wrong scope: %#v", m.explore.Preference)
	}
	if m.focus != 2 || !m.chatEditing {
		t.Fatal("discovery completion stole chat focus")
	}
}

func TestExploreConnectionPickerRetainsCommandAndContextNamespace(t *testing.T) {
	for _, command := range []string{"pods platform", "pods", "namespace platform"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			client, resources := m.explore.Client, m.explore.Resources
			client.Namespace = "context-default"
			m.currentConnection().Settings["namespace"] = ""
			m.selectScope(m.active, "", "")
			mainCommand(t, m, command)
			press(t, m, "enter")
			m.Update(monitor.ExploreConnected{Generation: m.explore.Generation, Client: client, Resources: resources})
			want := "context-default"
			if strings.Contains(command, "platform") {
				want = "platform"
			}
			if m.explore.Preference.Namespace != want {
				t.Fatalf("namespace = %q, want %q", m.explore.Preference.Namespace, want)
			}
		})
	}
}

func TestExploreSortsNumbersAndAgesByValue(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	newer := exploreObject("newer", "newer")
	older := exploreObject("older", "older")
	newer.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-2 * time.Hour)))
	older.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-10 * time.Hour)))
	older.Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)["restartCount"] = int64(10)
	e.Collection.Objects = []unstructured.Unstructured{older, newer}
	for _, column := range []int{4, 5} {
		e.Preference.Sort = column
		e.Preference.Descending = false
		_, rows := m.exploreRows()
		if rows[0].Object.GetName() != "newer" {
			t.Fatalf("column %d sorted text instead of values", column)
		}
		e.Preference.Descending = true
		_, rows = m.exploreRows()
		if rows[0].Object.GetName() != "older" {
			t.Fatalf("column %d did not reverse", column)
		}
	}
}

func TestExploreLogsKeepContainerHeaderAndReplacementErrorsVisible(t *testing.T) {
	m := exploreModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	e := m.explore
	m.openExploreSelection(4)
	e.DetailValid = true
	e.Container = "app"
	m.podLogViewer().Append(strings.Repeat("example log line\n", 100))
	for _, wrap := range []bool{true, false} {
		m.podLogViewer().Reader.Wrap = wrap
		view := ansi.Strip(m.exploreDetailBody(50, 12))
		if !strings.Contains(view, "Logs · app") || !strings.Contains(view, "example log line") {
			t.Fatalf("log header disappeared while following: %s", view)
		}
	}
	v := m.podLogViewer()
	cursor := v.Reader.Cursor
	press(t, m, "up")
	if v.Following || v.Reader.Cursor.Row != cursor.Row-1 {
		t.Fatal("Up did not leave the followed tail")
	}

	m.updateExploreMessage(monitor.ExploreDetailResult{Run: e.DetailRun, Text: "This object was deleted and replaced."})
	m.exploreDetailBody(50, 12)
	m.podLogViewer().Key("end", "")
	if view := m.exploreDetailBody(50, 12); !strings.Contains(view, "deleted and replaced") || !strings.Contains(view, "Disconnected") {
		t.Fatalf("replacement error hidden in Logs: %s", ansi.Strip(view))
	}
	e.Resource.Group = "example.io"
	e.Tab = 0
	m.openExploreSelection(4)
	if e.Tab == 4 {
		t.Fatal("custom resource named pods exposed core Pod logs")
	}
}

func TestExploreEditorsKeepCommandsAndPastesLiteral(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "/")
	typeText(m, "bb cc ss :quit")
	m.Update(tea.PasteMsg{Content: " :deployments\n"})
	if m.explore.Input == nil || m.command != nil || m.leftHidden || m.rightHidden || m.chatEditing || m.settingsOverlay != nil {
		t.Fatal("filter text executed navigation")
	}
	if !strings.Contains(m.explore.Input.Text, ":deployments") {
		t.Fatal("filter paste was lost")
	}
	press(t, m, "esc")
	if m.explore.Preference.Filter != "" {
		t.Fatal("cancel applied unfinished filter")
	}
	mainCommand(t, m, "connections")
	typeText(m, "bb")
	if m.leftHidden || m.explore.Input.Text != "bb" {
		t.Fatal("connection search ran sidebar shortcut")
	}
	press(t, m, "tab")
	if m.focus != 1 {
		t.Fatal("Tab escaped connection picker")
	}
	press(t, m, "esc")
	typeText(m, "bb")
	if !m.leftHidden {
		t.Fatal("navigation shortcut stopped working")
	}
}

func TestResourceCommandsCannotDiscardUnsavedForms(t *testing.T) {
	m := exploreModel(t)
	mainCommand(t, m, "n")
	typeText(m, "Unfinished")
	form := m.form
	press(t, m, "enter")
	typeText(m, ":pods")
	press(t, m, "enter")
	if m.form != form || form.Fields[0].Value.Text != "Unfinished" || !strings.Contains(m.status, "Finish or cancel") {
		t.Fatal("resource command discarded an unfinished form")
	}
}

func TestExploreRejectsStaleConnectionAndWatchResults(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	client := e.Client
	e.Connecting = true
	m.Update(monitor.ExploreConnected{Generation: 0, Client: nil})
	if e.Client != client || !e.Connecting {
		t.Fatal("stale connection replaced current client")
	}
	e.Connecting = false
	stale := &monitor.ExploreStream{}
	m.Update(monitor.ExploreUpdate{Stream: stale, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("wrong-cluster", "wrong")}}}})
	if e.Collection.Objects[0].GetName() == "wrong-cluster" {
		t.Fatal("stale watch crossed connection scope")
	}
	m.Update(monitor.ExploreUpdate{Stream: e.Stream, Snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("catalog", "two"), exploreObject("checkout", "one")}}, Status: "Live"}})
	if e.Selected != "one" {
		t.Fatal("live sorting lost selection by UID")
	}
}

func TestExploreDetailsAndBackRetainSelectionAndFilter(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.Preference.Filter = "checkout"
	press(t, m, "enter")
	if e.Detail == nil || e.Detail.GetUID() != "one" {
		t.Fatal("Enter did not open selected object")
	}
	typeText(m, "y")
	if e.Tab != 1 {
		t.Fatal("YAML shortcut failed")
	}
	press(t, m, "esc")
	if e.Detail == nil || e.Tab != 0 || e.Detail.GetUID() != "one" {
		t.Fatal("YAML did not return to the original Summary")
	}
	press(t, m, "esc")
	if e.Detail != nil || e.Selected != "one" || e.Preference.Filter != "checkout" {
		t.Fatal("back lost table state")
	}
}

func TestExploreConnectionPickerDoesNotChooseAnUnrelatedCluster(t *testing.T) {
	m := exploreModel(t)
	m.selectScope(m.active, "", "")
	mainCommand(t, m, "pods")
	if m.connection != "" || m.explore.Picker != "connections" {
		t.Fatal("resource command silently chose a connection")
	}
	press(t, m, "enter")
	if m.currentConnection() == nil || m.mode != 1 || m.explore.Preference.Resource != "pods" {
		t.Fatal("connection selection did not open requested resources")
	}
}

func TestExplorePreferencesPersistWithoutObjectsOrLogs(t *testing.T) {
	m := exploreModel(t)
	mainCommand(t, m, "N platform")
	m.podLogViewer().Append("PRIVATE_LOG_CONTENT")
	m.explore.Collection.Objects = []unstructured.Unstructured{exploreObject("PRIVATE_RESOURCE_OBJECT", "private")}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, err := m.store.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE_") {
		t.Fatal("resource data or logs leaked into session")
	}
	var saved session
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Explore[m.active+"/"+m.connection].Namespace != "platform" {
		t.Fatal("browsing namespace was not persisted")
	}
	if m.currentConnection().Settings["namespace"] != "shop" {
		t.Fatal("browsing namespace changed saved connection")
	}
}

func TestExploreExitCancelsStreamsAndKeepsDraft(t *testing.T) {
	m := exploreModel(t)
	ctx := m.explore.Stream.Ctx
	m.draft().Insert("draft", true)
	press(t, m, "ctrl+c")
	m.drainPersistence()
	cmd := m.syncQuit()
	if cmd == nil {
		t.Fatal("exit unavailable")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not exit")
	}
	if ctx.Err() == nil || m.explore.Stream != nil {
		t.Fatal("exit retained a watch")
	}
	if m.draft().Text != "draft" {
		t.Fatal("exit discarded chat draft")
	}
}

func TestExploreLayoutsStayInsideThreePanels(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 32}, {160, 40}} {
		for _, view := range []string{"table", "filter-input", "filter-applied", "loading", "empty", "failure", "connections", "detail", "yaml", "command"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], view), func(t *testing.T) {
				m := exploreModel(t)
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				switch view {
				case "filter-input", "filter-applied":
					typeText(m, "/"+strings.Repeat("検索", 30))
					if view == "filter-applied" {
						press(t, m, "enter")
					}
				case "loading":
					m.explore.Collection = kubernetes.Collection{}
					m.explore.Listed = false
				case "empty":
					m.explore.Collection = kubernetes.Collection{}
				case "failure":
					m.explore.Collection = kubernetes.Collection{}
					m.explore.Err = "Kubernetes authentication failed. Renew your credentials, then refresh."
				case "connections":
					mainCommand(t, m, "connections")
				case "detail":
					press(t, m, "enter")
				case "yaml":
					typeText(m, "y")
				case "command":
					typeText(m, ":pods")
				}
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != size[1] {
					t.Fatalf("height %d != %d", len(lines), size[1])
				}
				for i, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("row %d overflows: %s", i, line)
					}
				}
				if strings.Count(lines[len(lines)-2], "╰") != 3 {
					t.Fatal("Explore changed the three-panel frame")
				}
			})
		}
	}
}

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

func TestExploreSidebarOpensNativeResourceAndKeepsScope(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	var deployment int = -1
	for i, row := range m.treeRows() {
		if row.kind == "resource" && row.resource == "deployments.apps" {
			deployment = i
		}
	}
	if deployment < 0 {
		t.Fatal("native resource missing from expanded Workloads sidebar")
	}
	m.treeCursor = deployment
	m.focus = 0
	m.draft().Insert("Keep investigation draft", true)
	m.updateTree("enter")
	if m.explore.Preference.Resource != "deployments.apps" || m.explore.Preference.Namespace != "shop" || m.focus != 1 || m.draft().Text != "Keep investigation draft" {
		t.Fatal("sidebar resource selection lost namespace, focus, or draft")
	}
	m.syncExploreTree()
	view := m.treeBody(36, 30)
	if !strings.Contains(ansi.Strip(view), "Deployments") || !strings.Contains(view, "\x1b[") {
		t.Fatal("current resource lacks persistent presentation")
	}
}

func TestExploreSidebarExpansionAndAPIIdentity(t *testing.T) {
	m := exploreModel(t)
	m.explore.Resources = append(m.explore.Resources, kubecommon.Resource{Group: "example.io", Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true})
	m.syncExploreTree()
	key := categoryTreeKey(m.connection, "Other APIs")
	m.expanded[key] = true
	for _, row := range m.treeRows() {
		if row.resource == "pods.example.io" && row.parent != key {
			t.Fatal("extension Pod inherited native Workloads category")
		}
	}
	for i, row := range m.treeRows() {
		if row.key == categoryTreeKey(m.connection, "Workloads") {
			m.treeCursor = i
			break
		}
	}
	m.updateTree("enter")
	for _, row := range m.treeRows() {
		if row.resource == "pods" {
			t.Fatal("collapsed category still contains resources")
		}
	}
	if m.explore.Preference.Resource != "pods" {
		t.Fatal("folding changed resource selection")
	}
}

func TestPodSummaryShowsFailureEvidenceBeforeRawMetadata(t *testing.T) {
	m := exploreModel(t)
	object := exploreObject("checkout-6d8f9-x2k7m", "pod-uid")
	state := object.Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)
	state["ready"] = false
	state["restartCount"] = int64(12)
	state["state"] = map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}
	state["lastState"] = map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}}
	m.explore.Detail = &object
	m.explore.DetailValid = true
	m.explore.Container = "app"
	for _, width := range []int{62, 80, 110} {
		view := ansi.Strip(m.exploreDetailBody(width, 23))
		for _, want := range []string{"CrashLoopBackOff", "Containers", "OOMKilled", "137", "Esc back"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d missing first-screen evidence %q:\n%s", width, want, view)
			}
		}
		if strings.Contains(view, `{"`) {
			t.Fatal("Summary contains a serialized JSON object")
		}
	}
}

func TestPodListPreservesStatusAcrossWidths(t *testing.T) {
	m := exploreModel(t)
	state := m.explore.Collection.Objects[0].Object["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)
	state["ready"] = false
	state["state"] = map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}
	for _, width := range []int{34, 60, 90, 140} {
		view := ansi.Strip(m.exploreTableBody(width, 15))
		if !strings.Contains(view, "CrashLoopBackOff") || !strings.Contains(view, "Filter resources") {
			t.Fatalf("essential list context missing at width %d:\n%s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("list overflow at %d: %q", width, line)
			}
		}
	}
}

func TestSummaryPreservesMultilineAndEmptyConfigValues(t *testing.T) {
	section := kubernetes.DetailSection{Title: "Data", Columns: []string{"Key", "Value"}, Rows: [][]string{{"settings", "first\nsecond"}, {"optional", ""}}}
	for _, width := range []int{28, 100} {
		view := ansi.Strip(strings.Join(monitor.RenderDetailTable(section, width, ""), "\n"))
		first, second := strings.Index(view, "first"), strings.Index(view, "second")
		if first < 0 || second <= first || !strings.Contains(view[first:second], "\n") || !strings.Contains(view, "optional") || !strings.Contains(view, "(empty)") {
			t.Fatalf("width %d lost configuration data structure:\n%s", width, view)
		}
	}
}

func TestSummaryColorsOperationalValuesOnly(t *testing.T) {
	if monitor.SummaryValue("Type", "Ready") != "Ready" || monitor.SummaryValue("Value", "Error") != "Error" || monitor.SummaryValue("Status", "False") != "False" {
		t.Fatal("non-operational text gained a misleading status color")
	}
	if monitor.SummaryValue("State", "CrashLoopBackOff") == "CrashLoopBackOff" {
		t.Fatal("container failure lost its status color")
	}
}

func TestExploreResourceHeadingSurvivesLongConnectionStatus(t *testing.T) {
	m := exploreModel(t)
	m.explore.Status = "Watch denied; refreshing every 5s"
	view := ansi.Strip(m.exploreBody(34, 20))
	if !strings.Contains(view, "Pods") {
		t.Fatalf("refresh status hid the resource identity:\n%s", view)
	}
}

func TestBackspaceControlByteDoesNotBecomeNavigation(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "/checkout")
	press(t, m, "enter")
	key := tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}
	m.Update(key)
	if m.explore.Preference.Filter != "checkout" || m.focus != 1 || m.help != nil {
		t.Fatal("Ctrl+H changed navigation or cleared the applied filter")
	}
	typeText(m, "/")
	m.Update(key)
	if m.explore.Input == nil || m.explore.Input.Text != "checkou" || m.explore.Preference.Filter != "checkout" {
		t.Fatal("BS-style Backspace did not stay inside the filter editor")
	}
}

func TestExploreBackspaceClearsOnlyAppliedTextFilter(t *testing.T) {
	for _, query := range []string{"checkout", "no-match"} {
		t.Run(query, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.Preference.Selector = "app=checkout"
			e.Preference.Sort, e.Preference.Descending = 2, true
			e.Stream.Key += e.Preference.Selector
			stream := e.Stream
			want := e.Preference
			typeText(m, "/"+query)
			press(t, m, "enter")
			selected := e.Selected
			for _, width := range []int{24, 45, 80} {
				view := ansi.Strip(m.exploreTableBody(width, 15))
				if !strings.Contains(view, "Backspace clear") {
					t.Fatalf("missing clear hint at width %d: %s", width, view)
				}
			}
			press(t, m, "backspace")
			_, rows := m.exploreRows()
			if e.Preference != want || len(rows) != 2 || e.Selected != selected || e.Stream != stream || m.focus != 1 {
				t.Fatalf("clearing changed browsing state: %#v, selection %q, rows %d", e.Preference, e.Selected, len(rows))
			}
			if saved := m.explorePreferences[m.active+"/"+m.connection]; saved != want {
				t.Fatalf("cleared filter was not remembered: %#v", saved)
			}
			if strings.Contains(ansi.Strip(m.exploreTableBody(80, 15)), "Backspace clear") {
				t.Fatal("empty filter still shows a clear action")
			}
			press(t, m, "backspace")
			if e.Selected != selected || e.Preference != want {
				t.Fatal("Backspace without a filter changed browsing state")
			}
		})
	}
}

func TestTableFullscreenAndLabelSelectorDoNotConflict(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "f")
	if m.shortcutPrefix != "f" || m.explore.Input != nil || m.viewExpanded() {
		t.Fatal("fullscreen prefix opened a filter or changed the layout")
	}
	typeText(m, "fLapp=ff")
	if !m.viewExpanded() || m.explore.InputKind != "selector" || m.explore.Input == nil || m.explore.Input.Text != "app=ff" {
		t.Fatal("label selector is unreachable or interprets fullscreen keys")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || m.explore.Input != nil {
		t.Fatal("closing the selector restored the layout")
	}
	typeText(m, "ff")
	if m.viewExpanded() {
		t.Fatal("fullscreen shortcut did not restore the layout")
	}
}

func TestExploreBackspaceEditsFilterWithoutClearingCommittedQuery(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	typeText(m, "/checkout")
	press(t, m, "enter")
	typeText(m, "/")
	press(t, m, "backspace")
	if e.Input == nil || e.Input.Text != "checkou" || e.Preference.Filter != "checkout" {
		t.Fatal("Backspace in the editor cleared the applied filter")
	}
	if strings.Contains(ansi.Strip(m.exploreBody(80, 20)), "Backspace clear") {
		t.Fatal("editor advertises navigation-only clear action")
	}
	press(t, m, "esc")
	if e.Preference.Filter != "checkout" || e.Input != nil {
		t.Fatal("Esc did not cancel only the unfinished edit")
	}
	press(t, m, "esc")
	if m.focus != 0 || e.Preference.Filter != "checkout" {
		t.Fatal("root Esc cleared the committed filter")
	}
}

func TestExploreBackspaceDoesNotClearFilterFromOtherControls(t *testing.T) {
	for _, control := range []string{"sidebar", "tree search", "chat", "command", "selector", "connections", "detail", "help", "settings"} {
		t.Run(control, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.Preference.Filter = "checkout"
			var input *textview.Field
			switch control {
			case "sidebar", "tree search":
				m.focus = 0
				if control == "tree search" {
					typeText(m, "/pods")
					input = &m.searches[0].input
				}
			case "chat":
				mainCommand(t, m, "chat")
				typeText(m, "draft")
				input = m.draft()
			case "command":
				typeText(m, ":pods")
				input = m.command
			case "selector":
				typeText(m, "Lapp=checkout")
				input = e.Input
			case "connections":
				mainCommand(t, m, "connections")
				typeText(m, "pods")
				input = e.Input
			case "detail":
				press(t, m, "enter")
			case "help", "settings":
				mainCommand(t, m, control)
			}
			want := ""
			if input != nil {
				want = input.Text[:len(input.Text)-1]
			}
			press(t, m, "backspace")
			if e.Preference.Filter != "checkout" {
				t.Fatal("Backspace escaped its focused control and cleared the table filter")
			}
			if input != nil && input.Text != want {
				t.Fatalf("Backspace did not edit normally: %q, want %q", input.Text, want)
			}
		})
	}
}

func TestResourceHeadingAndEscapeRevealSidebar(t *testing.T) {
	for _, input := range []string{"keyboard", "mouse"} {
		for _, hidden := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/hidden=%t", input, hidden), func(t *testing.T) {
				m := exploreModel(t)
				m.width, m.height = 160, 40
				m.leftHidden = hidden
				m.explore.Preference.Filter = "checkout"
				before, connection := m.explore.Preference, m.connection
				if input == "mouse" {
					clickMouse(t, m, "main", "resource-types/")
				} else {
					press(t, m, "esc")
				}
				if m.leftHidden || m.focus != 0 || m.connection != connection || m.explore.Preference != before {
					t.Fatal("resource browsing must reveal the sidebar without changing the connection or filters")
				}
				typeText(m, "/deployments")
				press(t, m, "enter")
				if m.explore.Preference.Resource != "deployments.apps" {
					t.Fatal("sidebar search did not open the selected resource")
				}
			})
		}
	}
}
