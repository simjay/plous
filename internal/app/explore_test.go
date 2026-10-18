package app

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
