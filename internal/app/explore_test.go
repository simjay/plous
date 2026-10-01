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
	"github.com/simjay/plous/internal/kubernetes"
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
	client, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	resources := []kubernetes.Resource{{Name: "pods", Kind: "Pod", Version: "v1", Namespaced: true, Verbs: []string{"list", "watch"}, ShortNames: []string{"po"}}, {Name: "deployments", Kind: "Deployment", Group: "apps", Version: "v1", Namespaced: true}, {Name: "services", Kind: "Service", Version: "v1", Namespaced: true}, {Name: "nodes", Kind: "Node", Version: "v1"}}
	ctx, cancel := context.WithCancel(context.Background())
	m.explore = &exploreState{key: strings.Join([]string{m.active, c.ID, c.Settings["kubeconfig"], c.Settings["context"], c.Settings["namespace"]}, "\x00"), generation: 1, client: client, resources: resources, resource: resources[0], preference: explorePreference{Resource: "pods", Namespace: "shop"}, collection: kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("checkout", "one"), exploreObject("catalog", "two")}}, selected: "one", status: "Live", stream: &exploreStream{key: "pods/v1/shop/", ctx: ctx, cancel: cancel, events: make(chan kubernetes.Snapshot)}}
	m.exploreGeneration = 1
	m.explore.listed = true
	t.Cleanup(m.stopExplore)
	return m
}

func TestExploreCancelsRequestsWhenLeavingActiveConnection(t *testing.T) {
	for _, action := range []string{"overview", "activity", "connection", "workspace", "quit"} {
		t.Run(action, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			discovery, cancelDiscovery := context.WithCancel(context.Background())
			detail, cancelDetail := context.WithCancel(context.Background())
			logs, cancelLogs := context.WithCancel(context.Background())
			defer cancelDiscovery()
			defer cancelDetail()
			defer cancelLogs()
			e.connectionCancel = cancelDiscovery
			e.detailRun = &exploreDetailRun{cancel: cancelDetail}
			e.log = &exploreLogRun{ctx: logs, cancel: cancelLogs}
			watch := e.stream.ctx
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
				press(t, m, "ctrl+c")
			}
			for name, ctx := range map[string]context.Context{"discovery": discovery, "watch": watch, "detail": detail, "logs": logs} {
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
	e.collection = kubernetes.Collection{}
	e.preference.Namespace = "default"
	e.stream.key = "pods/v1/default/"
	e.listed = false
	view := ansi.Strip(m.exploreTableBody(70, 10))
	if !strings.Contains(view, "Loading Pods") || strings.Contains(view, "No Pods") {
		t.Fatalf("pending request looked empty: %s", view)
	}
	m.Update(exploreUpdate{stream: e.stream, snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	view = ansi.Strip(m.exploreTableBody(70, 10))
	if !e.listed || !strings.Contains(view, "No Pods in namespace default.") || !strings.Contains(view, ":N *  Show all namespaces") {
		t.Fatalf("empty namespace lacks scope and recovery: %s", view)
	}
	mainCommand(t, m, "N *")
	if e.listed || e.preference.Namespace != "" {
		t.Fatal("namespace change did not reset loading state")
	}
	m.Update(exploreUpdate{stream: e.stream, snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{}, Status: "Live"}})
	view = ansi.Strip(m.exploreTableBody(70, 10))
	if !strings.Contains(view, "No Pods across all namespaces.") {
		t.Fatalf("all-namespace scope unclear: %s", view)
	}
}

func TestExploreCommandsRouteGloballyWithoutLosingChat(t *testing.T) {
	for _, focus := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(focus), func(t *testing.T) {
			m := exploreModel(t)
			m.focus = focus
			m.draft().insert("Keep this draft", true)
			typeText(m, ":deployments")
			press(t, m, "enter")
			if m.mode != 1 || m.focus != 1 || m.explore.preference.Resource != "deployments.apps" || m.draft().text != "Keep this draft" {
				t.Fatal("resource command lost scope or draft")
			}
		})
	}
}

func TestExploreCommandFromOverviewPreservesExplicitNamespace(t *testing.T) {
	m := exploreModel(t)
	client, resources := m.explore.client, m.explore.resources
	mainCommand(t, m, "1")
	mainCommand(t, m, "pods platform")
	mainCommand(t, m, "chat")
	m.Update(exploreConnected{generation: m.explore.generation, client: client, resources: resources})
	if m.explore.preference.Resource != "pods" || m.explore.preference.Namespace != "platform" {
		t.Fatalf("wrong scope: %#v", m.explore.preference)
	}
	if m.focus != 2 || !m.chatEditing {
		t.Fatal("discovery completion stole chat focus")
	}
}

func TestExploreConnectionPickerRetainsCommandAndContextNamespace(t *testing.T) {
	for _, command := range []string{"pods platform", "pods", "namespace platform", "kinds"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			client, resources := m.explore.client, m.explore.resources
			client.Namespace = "context-default"
			m.currentConnection().Settings["namespace"] = ""
			m.selectScope(m.active, "", "")
			mainCommand(t, m, command)
			press(t, m, "enter")
			m.Update(exploreConnected{generation: m.explore.generation, client: client, resources: resources})
			want := "context-default"
			if strings.Contains(command, "platform") {
				want = "platform"
			}
			if m.explore.preference.Namespace != want {
				t.Fatalf("namespace = %q, want %q", m.explore.preference.Namespace, want)
			}
			if command == "kinds" && m.explore.picker != "kinds" {
				t.Fatal("kind picker command was lost during connection selection")
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
	e.collection.Objects = []unstructured.Unstructured{older, newer}
	for _, column := range []int{4, 5} {
		e.preference.Sort = column
		e.preference.Descending = false
		_, rows := m.exploreRows()
		if rows[0].Object.GetName() != "newer" {
			t.Fatalf("column %d sorted text instead of values", column)
		}
		e.preference.Descending = true
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
	e.detailValid = true
	e.container = "app"
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

	m.updateExploreMessage(exploreDetailResult{run: e.detailRun, text: "This object was deleted and replaced."})
	m.exploreDetailBody(50, 12)
	m.podLogViewer().Key("end", "")
	if view := m.exploreDetailBody(50, 12); !strings.Contains(view, "deleted and replaced") || !strings.Contains(view, "Disconnected") {
		t.Fatalf("replacement error hidden in Logs: %s", ansi.Strip(view))
	}
	e.resource.Group = "example.io"
	e.tab = 0
	m.openExploreSelection(4)
	if e.tab == 4 {
		t.Fatal("custom resource named pods exposed core Pod logs")
	}
}

func TestExploreEditorsKeepCommandsAndPastesLiteral(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "/")
	typeText(m, "bb cc ss :quit")
	m.Update(tea.PasteMsg{Content: " :deployments\n"})
	if m.explore.input == nil || m.command != nil || m.leftHidden || m.rightHidden || m.chatEditing || m.settingsOverlay != nil {
		t.Fatal("filter text executed navigation")
	}
	if !strings.Contains(m.explore.input.text, ":deployments") {
		t.Fatal("filter paste was lost")
	}
	press(t, m, "esc")
	if m.explore.preference.Filter != "" {
		t.Fatal("cancel applied unfinished filter")
	}
	mainCommand(t, m, "kinds")
	typeText(m, "bb")
	if m.leftHidden || m.explore.input.text != "bb" {
		t.Fatal("kind search ran sidebar shortcut")
	}
	press(t, m, "tab")
	if m.focus != 1 {
		t.Fatal("Tab escaped resource picker")
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
	if m.form != form || form.fields[0].value.text != "Unfinished" || !strings.Contains(m.status, "Finish or cancel") {
		t.Fatal("resource command discarded an unfinished form")
	}
}

func TestExploreRejectsStaleConnectionAndWatchResults(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	client := e.client
	e.connecting = true
	m.Update(exploreConnected{generation: 0, client: nil})
	if e.client != client || !e.connecting {
		t.Fatal("stale connection replaced current client")
	}
	e.connecting = false
	stale := &exploreStream{}
	m.Update(exploreUpdate{stream: stale, snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("wrong-cluster", "wrong")}}}})
	if e.collection.Objects[0].GetName() == "wrong-cluster" {
		t.Fatal("stale watch crossed connection scope")
	}
	m.Update(exploreUpdate{stream: e.stream, snapshot: kubernetes.Snapshot{Collection: &kubernetes.Collection{Objects: []unstructured.Unstructured{exploreObject("catalog", "two"), exploreObject("checkout", "one")}}, Status: "Live"}})
	if e.selected != "one" {
		t.Fatal("live sorting lost selection by UID")
	}
}

func TestExploreDetailsAndBackRetainSelectionAndFilter(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.preference.Filter = "checkout"
	press(t, m, "enter")
	if e.detail == nil || e.detail.GetUID() != "one" {
		t.Fatal("Enter did not open selected object")
	}
	typeText(m, "y")
	if e.tab != 1 {
		t.Fatal("YAML shortcut failed")
	}
	press(t, m, "esc")
	if e.detail == nil || e.tab != 0 || e.detail.GetUID() != "one" {
		t.Fatal("YAML did not return to the original Summary")
	}
	press(t, m, "esc")
	if e.detail != nil || e.selected != "one" || e.preference.Filter != "checkout" {
		t.Fatal("back lost table state")
	}
}

func TestExploreConnectionPickerDoesNotChooseAnUnrelatedCluster(t *testing.T) {
	m := exploreModel(t)
	m.selectScope(m.active, "", "")
	mainCommand(t, m, "pods")
	if m.connection != "" || m.explore.picker != "connections" {
		t.Fatal("resource command silently chose a connection")
	}
	press(t, m, "enter")
	if m.currentConnection() == nil || m.mode != 1 || m.explore.preference.Resource != "pods" {
		t.Fatal("connection selection did not open requested resources")
	}
}

func TestExplorePreferencesPersistWithoutObjectsOrLogs(t *testing.T) {
	m := exploreModel(t)
	mainCommand(t, m, "N platform")
	m.podLogViewer().Append("PRIVATE_LOG_CONTENT")
	m.explore.collection.Objects = []unstructured.Unstructured{exploreObject("PRIVATE_RESOURCE_OBJECT", "private")}
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
	ctx := m.explore.stream.ctx
	m.draft().insert("draft", true)
	cmd := press(t, m, "ctrl+c")
	if cmd == nil {
		t.Fatal("exit unavailable")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not exit")
	}
	if ctx.Err() == nil || m.explore.stream != nil {
		t.Fatal("exit retained a watch")
	}
	if m.draft().text != "draft" {
		t.Fatal("exit discarded chat draft")
	}
}

func TestExploreLayoutsStayInsideThreePanels(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 32}, {160, 40}} {
		for _, view := range []string{"table", "filter-input", "filter-applied", "loading", "empty", "failure", "kinds", "detail", "yaml", "command"} {
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
					m.explore.collection = kubernetes.Collection{}
					m.explore.listed = false
				case "empty":
					m.explore.collection = kubernetes.Collection{}
				case "failure":
					m.explore.collection = kubernetes.Collection{}
					m.explore.err = "Kubernetes authentication failed. Renew your credentials, then refresh."
				case "kinds":
					mainCommand(t, m, "kinds")
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
