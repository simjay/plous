package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

type namespaceTransport func(*http.Request) (*http.Response, error)

func (f namespaceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func namespaceClient(t *testing.T, respond namespaceTransport) *kubernetes.Client {
	t.Helper()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: "https://namespace-fixture.invalid", Transport: respond})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func namespaceResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNamespacePickerLoadsSearchesAndAppliesActualNamespaces(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	requests := 0
	e.client = namespaceClient(t, func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Path != "/api/v1/namespaces" || request.Method != http.MethodGet {
			t.Fatalf("unexpected namespace request: %s %s", request.Method, request.URL)
		}
		return namespaceResponse(http.StatusOK, `{"apiVersion":"v1","kind":"NamespaceList","metadata":{"resourceVersion":"1"},"items":[{"metadata":{"name":"prod-west"}},{"metadata":{"name":"shop"}},{"metadata":{"name":"prod-east"}}]}`), nil
	})
	m.updateExplore("n")
	if !strings.Contains(ansi.Strip(m.namespacePickerBody(60, 16)), "Loading namespaces") {
		t.Fatal("picker did not identify loading state")
	}
	command := m.syncNamespacePicker()
	if command == nil {
		t.Fatal("opening picker did not create a namespace request")
	}
	m.updateExploreMessage(command())
	if requests != 1 || !e.namespaces.loaded || e.namespaces.err != "" {
		t.Fatalf("namespace load failed: %#v", e.namespaces)
	}
	choices := m.namespaceChoices()
	if len(choices) != 6 || choices[0].label != "All namespaces" || choices[1].name != "shop" || choices[2].name != "prod-east" || choices[3].name != "prod-west" || !choices[4].reload || !choices[5].manual {
		t.Fatalf("namespace choices missing or duplicated: %#v", choices)
	}
	if !strings.Contains(ansi.Strip(m.namespacePickerBody(60, 16)), "shop  ✓") {
		t.Fatal("current namespace was not marked")
	}
	typeText(m, "prod")
	if len(m.namespaceChoices()) != 3 {
		t.Fatal("namespace search did not filter the discovered list")
	}
	press(t, m, "down")
	press(t, m, "enter")
	if e.preference.Namespace != "prod-west" || e.picker != "" || e.input != nil || e.namespaces != nil {
		t.Fatalf("namespace was not applied and closed: %#v", e.preference)
	}
	if m.currentConnection().Settings["namespace"] != "shop" || m.explorePreferences[m.active+"/"+m.connection].Namespace != "prod-west" {
		t.Fatal("browsing scope changed connection settings or failed to persist")
	}
}

func TestNamespacePickerNeverUsesAnExtensionWithTheCoreResourceName(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.resources = append(e.resources, kubernetes.Resource{Group: "example.io", Name: "namespaces", Kind: "Namespace", Version: "v9", ShortNames: []string{"ns"}})
	requests := 0
	e.client = namespaceClient(t, func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Path != "/api/v1/namespaces" {
			t.Fatalf("namespace picker requested extension instead of core API: %s", request.URL)
		}
		return namespaceResponse(http.StatusOK, `{"apiVersion":"v1","kind":"NamespaceList","items":[{"metadata":{"name":"platform"}}]}`), nil
	})
	m.openNamespacePicker()
	m.updateExploreMessage(m.syncNamespacePicker()())
	if requests != 1 || !e.namespaces.loaded || e.namespaces.err != "" || len(e.namespaces.names) != 1 || e.namespaces.names[0] != "platform" {
		t.Fatalf("core namespace listing did not complete: %#v", e.namespaces)
	}
}

func TestNamespaceReloadReplacesRequestWithoutChangingScope(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.client = namespaceClient(t, func(request *http.Request) (*http.Response, error) {
		return namespaceResponse(http.StatusOK, `{"apiVersion":"v1","kind":"NamespaceList","items":[{"metadata":{"name":"platform"}}]}`), nil
	})
	m.openNamespacePicker()
	p := e.namespaces
	p.loaded, p.names = true, []string{"old"}
	m.updateNamespaceInput(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if !p.loaded || p.run != nil || e.input.text != "" {
		t.Fatal("retired Ctrl+R reloaded namespaces or inserted text")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := &namespaceRequest{cancel: cancel}
	p.run = old
	for i, choice := range m.namespaceChoices() {
		if choice.reload {
			e.pickerCursor = i
			break
		}
	}
	m.updateNamespaceInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.loaded || p.run != nil || ctx.Err() == nil {
		t.Fatal("Reload namespaces did not cancel and replace the existing request")
	}
	command := m.syncNamespacePicker()
	if command == nil {
		t.Fatal("Reload namespaces did not schedule a replacement request")
	}
	m.updateNamespaceResult(namespaceResult{run: old, generation: e.generation, names: []string{"stale"}})
	if p.loaded || len(p.names) != 1 || p.names[0] != "old" {
		t.Fatal("late response replaced the reloading picker")
	}
	m.updateExploreMessage(command())
	if !p.loaded || p.err != "" || len(p.names) != 1 || p.names[0] != "platform" || e.preference.Namespace != "shop" || e.picker != "namespace" {
		t.Fatal("namespace reload changed the browsing scope or lost the fresh result")
	}
	m.updateNamespaceInput(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if e.input.text != "g" {
		t.Fatal("namespace search did not keep plain letters literal")
	}
	for _, choice := range m.namespaceChoices() {
		if choice.reload {
			t.Fatal("reload action should not appear as a namespace search result")
		}
	}
}

func TestNamespaceScopeCommandsPreserveResourceType(t *testing.T) {
	for _, name := range []string{"N", "namespace"} {
		for _, argument := range []string{"", "platform", "*"} {
			t.Run(name+"/"+argument, func(t *testing.T) {
				m := exploreModel(t)
				e := m.explore
				e.resources = append(e.resources, kubernetes.Resource{Name: "namespaces", Kind: "Namespace", Version: "v1", ShortNames: []string{"ns"}})
				mainCommand(t, m, strings.TrimSpace(name+" "+argument))
				if e.preference.Resource != "pods" {
					t.Fatalf("namespace command selected a resource kind: %s", e.preference.Resource)
				}
				if argument == "" {
					if e.picker != "namespace" {
						t.Fatal("namespace command did not open the scope picker")
					}
				} else if e.preference.Namespace != exploreNamespace(argument) || e.picker != "" {
					t.Fatalf("explicit namespace command did not apply scope: %#v", e.preference)
				}
			})
		}
	}
}

func TestExploreInspectionCommandsTakePriorityOverDiscoveredAliases(t *testing.T) {
	for _, item := range []struct {
		command string
		tab     int
	}{
		{"d", 0}, {"describe", 0}, {"y", 1}, {"yaml", 1}, {"l", 4}, {"logs", 4},
	} {
		t.Run(item.command, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.resources = append(e.resources, kubernetes.Resource{Group: "example.io", Name: "widgets", Kind: "Widget", Version: "v1", ShortNames: []string{item.command}})
			mainCommand(t, m, item.command)
			if item.tab == 4 {
				if e.preference.Resource != "pods" || e.detail != nil || !strings.Contains(m.status, "details first") {
					t.Fatal("Logs must remain a detail action, not a discovered alias or list operation")
				}
				return
			}
			if e.preference.Resource != "pods" || e.detail == nil || e.tab != item.tab {
				t.Fatalf("inspection command was captured by discovery: resource=%s detail=%v tab=%d", e.preference.Resource, e.detail, e.tab)
			}
		})
	}
}

func TestNamespacePickerDeniedListKeepsManualAndCurrentScopesUsable(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.client = namespaceClient(t, func(request *http.Request) (*http.Response, error) {
		return namespaceResponse(http.StatusForbidden, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403,"message":"namespaces is forbidden"}`), nil
	})
	m.openNamespacePicker()
	m.updateExploreMessage(m.syncNamespacePicker()())
	view := ansi.Strip(m.namespacePickerBody(70, 16))
	if !strings.Contains(view, "Access denied") || !strings.Contains(view, "shop  ✓") || !strings.Contains(view, "Enter namespace manually") {
		t.Fatalf("denied namespace picker had no recovery: %s", view)
	}
	typeText(m, "allowed-team")
	if !strings.Contains(ansi.Strip(m.namespacePickerBody(70, 16)), "No matching namespaces") {
		t.Fatal("no matching namespace state was not shown")
	}
	press(t, m, "enter")
	if !e.namespaces.manual || e.input.text != "allowed-team" || e.preference.Namespace != "shop" {
		t.Fatal("manual entry was not explicit or lost the typed name")
	}
	press(t, m, "enter")
	if e.preference.Namespace != "allowed-team" || e.picker != "" {
		t.Fatal("manual namespace did not apply when listing was denied")
	}
}

func TestNamespacePickerCancellationAndLateResultsCannotChangeScope(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	m.openExploreSelection(0)
	detail := e.detail
	m.updateExplore("n")
	if e.detail != detail || e.picker != "namespace" {
		t.Fatal("opening namespaces from detail lost the object")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &namespaceRequest{cancel: cancel}
	e.namespaces.run = run
	press(t, m, "esc")
	if ctx.Err() == nil || e.detail != detail || e.preference.Namespace != "shop" {
		t.Fatal("cancel failed to stop request or retain the detail and scope")
	}
	m.openNamespacePicker()
	newRun := &namespaceRequest{cancel: func() {}}
	e.namespaces.run = newRun
	m.updateExploreMessage(namespaceResult{run: run, generation: e.generation, names: []string{"wrong-run"}})
	m.updateExploreMessage(namespaceResult{run: newRun, generation: e.generation - 1, names: []string{"wrong-cluster"}})
	if e.namespaces.loaded || len(e.namespaces.names) != 0 {
		t.Fatal("late namespace result replaced current connection state")
	}
	m.chooseExploreNamespace("")
	if e.detail != nil || len(e.trail) != 0 || e.preference.Namespace != "" {
		t.Fatal("selecting all namespaces did not return to the table")
	}
}

func TestNamespacePickerInputStaysLiteralAndManualEntryCanBeCancelled(t *testing.T) {
	m := exploreModel(t)
	m.openNamespacePicker()
	typeText(m, "bb cc ss :quit")
	m.Update(tea.PasteMsg{Content: " :pods\n"})
	if m.leftHidden || m.chatEditing || m.settingsOverlay != nil || m.command != nil || !strings.Contains(m.explore.input.text, ":pods") {
		t.Fatal("namespace search executed commands or discarded pasted text")
	}
	press(t, m, "tab")
	if m.focus != 1 {
		t.Fatal("Tab escaped the namespace picker")
	}
	press(t, m, "enter")
	press(t, m, "enter")
	if !m.explore.namespaces.manual || m.explore.preference.Namespace != "shop" || m.status == "" {
		t.Fatal("invalid manual namespace was applied")
	}
	press(t, m, "esc")
	if m.explore.picker != "namespace" || m.explore.namespaces.manual {
		t.Fatal("Esc from manual entry did not return to the namespace list")
	}
	press(t, m, "esc")
	if m.explore.picker != "" || m.explore.preference.Namespace != "shop" {
		t.Fatal("cancelling namespace picker changed scope")
	}
}

func TestPodSummaryContainerChoiceCarriesIntoCurrentAndPreviousLogs(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	pod := exploreObject("checkout", "one")
	pod.Object["spec"].(map[string]any)["containers"] = []any{map[string]any{"name": "sidecar"}, map[string]any{"name": "app"}}
	pod.Object["status"].(map[string]any)["containerStatuses"] = []any{
		map[string]any{"name": "sidecar", "ready": true, "state": map[string]any{"running": map[string]any{}}},
		map[string]any{"name": "app", "ready": false, "state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}},
	}
	e.collection.Objects = []unstructured.Unstructured{pod}
	e.container, e.previous = "stale-selection", true
	m.openExploreSelection(0)
	if e.container != "app" || e.previous {
		t.Fatal("new Pod did not prioritize failing container or reset previous-log state")
	}
	m.updateExplore("o")
	if e.picker != "containers" || e.pickerCursor != 1 {
		t.Fatal("Summary did not open a container picker at the current selection")
	}
	press(t, m, "up")
	press(t, m, "enter")
	if e.tab != 0 || e.container != "sidecar" {
		t.Fatal("container selection left Summary or selected the wrong container")
	}
	m.updateExplore("l")
	if e.tab != 4 || e.container != "sidecar" || e.previous {
		t.Fatal("logs ignored Describe's selected container")
	}
	m.updateExplore("d")
	m.updateExplore("p")
	if e.tab != 4 || e.container != "sidecar" || !e.previous {
		t.Fatal("Summary previous-log shortcut lost its selected container")
	}
}

func TestExploreRequestKeysChangeWithServedVersion(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	e.resources[0].Version = "v2"
	old := e.stream
	m.syncExplore()
	if e.stream == old || e.stream.key != "pods/v2/shop/" || old.ctx.Err() == nil {
		t.Fatalf("resource version did not replace list/watch request: %s", e.stream.key)
	}
	object := exploreObject("checkout", "one")
	e.detail = object.DeepCopy()
	e.detailKey = "pods/v1/one/0"
	e.resource.Version = "v2"
	m.syncExploreDetail()
	if e.detailKey != "pods/v2/one/0" {
		t.Fatalf("detail identity omitted served version: %s", e.detailKey)
	}
}

func TestPodContainerPriorityIgnoresCompletedInitAndPrefersFailures(t *testing.T) {
	for _, failure := range []map[string]any{
		{"waiting": map[string]any{"reason": "ImagePullBackOff"}},
		{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}},
		{"terminated": map[string]any{"exitCode": int64(1)}},
	} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			pod := exploreObject("checkout", "one")
			pod.Object["spec"].(map[string]any)["initContainers"] = []any{map[string]any{"name": "init"}}
			pod.Object["spec"].(map[string]any)["containers"] = []any{map[string]any{"name": "starting"}, map[string]any{"name": "app"}}
			pod.Object["status"].(map[string]any)["initContainerStatuses"] = []any{map[string]any{"name": "init", "state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}}}}
			pod.Object["status"].(map[string]any)["containerStatuses"] = []any{
				map[string]any{"name": "starting", "state": map[string]any{"waiting": map[string]any{"reason": "ContainerCreating"}}},
				map[string]any{"name": "app", "state": failure},
			}
			if got := preferredPodContainer(pod); got != "app" {
				t.Fatalf("selected %s instead of failing app", got)
			}
		})
	}
}
