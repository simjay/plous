package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/app/navigation"
	helm "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestUIToolRoundtripThroughApplication(t *testing.T) {
	m, s := codeModel(t)
	m.focus, m.chatEditing = 2, true
	m.draft().Text = "keep my question"
	connections := m.assistantConnections()
	m.captureSourceReads(connections)
	if err := m.ensureConversationScope(); err != nil {
		t.Fatal(err)
	}
	tools, err := ai.OpenExternalTools(t.Context(), ai.Options{Agent: agent.ClaudeCode, Store: m.store, History: m.history, WorkspaceID: m.conversationScope(), Connections: connections, UI: m.uiTools(connections)})
	if err != nil {
		t.Fatal(err)
	}
	defer tools.Close()
	path := s.Inventory.Source.Paths()[0]
	args, _ := json.Marshal(map[string]any{"purpose": "show source", "connection_id": connections[0].ConnectionID, "path": path, "line": 1})
	done := make(chan error, 1)
	go func() {
		body, err := tools.Call(t.Context(), agent.ToolCall{ID: "show", Name: "source_show", Args: args}, 65536)
		if err == nil && !strings.Contains(string(body), `"status":"opened"`) {
			err = errors.New(string(body))
		}
		done <- err
	}()
	request := <-m.uiRequests
	command := m.receiveUI(request)
	if command == nil {
		t.Fatal("UI load did not start")
	}
	m.applyUI(command().(uiLoaded))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !m.presentedDocumentVisible() || m.uiDocument.view.Path != path || m.focus != 2 || m.draft().Text != "keep my question" {
		t.Fatal("tool did not present the requested source safely")
	}
}

func uiTestRequest(m *model, q agent.ViewRequest) *uiRequest {
	return &uiRequest{port: m.uiTools(m.assistantConnections()).(*uiPort), ctx: context.Background(), show: &q, reply: make(chan agent.UIResult, 1), version: m.uiInputVersion, location: m.uiLocation()}
}
func TestUIShowPreservesFocusDraftAndResourceIdentity(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.focus, m.chatEditing, m.assistantSize = 2, true, assistantDouble
	m.draft().Text = "unfinished question"
	resource := kubecommon.Resource{Group: "apps", Version: "v1", Name: "deployments", Kind: "Deployment", Namespaced: true}
	object := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "api", "namespace": "production", "uid": "exact-object"}, "spec": map[string]any{"replicas": int64(3)}}}
	q := agent.ViewRequest{View: "resource", Resource: resource.Key(), Namespace: "production", Name: "api", UID: "exact-object", Tab: "yaml"}
	r := uiTestRequest(m, q)
	m.uiPending = r
	m.applyUI(uiLoaded{request: r, prepared: &navigation.Prepared{Request: q, Resource: resource, Catalog: []kubecommon.Resource{resource}, Object: object, Origin: "live"}})
	result := <-r.reply
	if result.Status != "opened" || m.explore.Detail.GetUID() != "exact-object" || m.explore.Tab != 1 || m.focus != 2 || !m.chatEditing || m.draft().Text != "unfinished question" || m.assistantSize != assistantDouble {
		t.Fatalf("incorrect open: %+v / %+v", result, m.explore)
	}
	if !strings.Contains(ansi.Strip(m.mainBody(90, 25)), "api") {
		t.Fatal("requested object is not displayed")
	}
}

func TestUIShowBlocksEditsAndLateNavigation(t *testing.T) {
	m, s := codeModel(t)
	m.focus = 2
	s.Document = docedit.New("original")
	s.Document.Key("i", "i")
	s.Document.Key("x", "x")
	before := s.Document.Text()
	r := uiTestRequest(m, agent.ViewRequest{View: "source", Path: "README.md"})
	if command := m.receiveUI(r); command != nil {
		t.Fatal("started a read while editor was open")
	}
	if result := <-r.reply; result.Status != "blocked" || s.Document.Text() != before {
		t.Fatal("draft was not preserved", result)
	}
	s.Document = nil
	r = uiTestRequest(m, agent.ViewRequest{View: "source", Path: "README.md"})
	m.uiPending = r
	m.uiInputVersion++
	m.applyUI(uiLoaded{request: r, prepared: &navigation.Prepared{Request: *r.show, Text: "new"}})
	if result := <-r.reply; result.Status != "blocked" || m.uiDocument != nil {
		t.Fatal("late request replaced navigation", result)
	}
}

func TestUIShowSourceEvidenceAndComparisonUseExistingViewers(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.focus = 2
	for _, q := range []agent.ViewRequest{{View: "source", Path: "deployment.yaml", Line: 3}, {View: "evidence", Ref: "record:fixture:saved"}} {
		text := "first\nsecond\nthird\n"
		origin := "source"
		if q.View == "evidence" {
			text, origin = `{"coverage":"partial","original_time":"2026-10-10"}`, "saved"
		}
		m.presentUI(&navigation.Prepared{Request: q, Text: text, Origin: origin})
		if !m.presentedDocumentVisible() || !m.uiDocument.model.ReadOnly || m.focus != 2 || !strings.Contains(ansi.Strip(m.mainBody(90, 25)), origin) {
			t.Fatal("document was not displayed read-only")
		}
		if q.Line == 3 && m.uiDocument.model.Cursor != len("first\nsecond\n") {
			t.Fatal("wrong source line")
		}
		m.focus = 1
		m.presentedDocumentKey(localMouseKey("esc"))
		if m.uiDocument != nil {
			t.Fatal("document did not close")
		}
		m.focus = 2
	}
	q := agent.ViewRequest{View: "compare", Namespace: "apps", Name: "web", Revision: 3, CompareRevision: 2, Tab: "values"}
	m.presentUI(&navigation.Prepared{Request: q, Origin: "recorded", Snapshot: &helm.Snapshot{Name: "web", Namespace: "apps", Version: 3}, Before: "replicas: 2", After: "replicas: 3"})
	if m.helmer().Comparison == nil || m.helmer().Comparison.Name != "values: revision 2 → 3" || m.helmer().Detail.Version != 3 || !strings.Contains(ansi.Strip(m.mainBody(90, 25)), "replicas") || m.focus != 2 {
		t.Fatal("comparison not presented")
	}
}

func TestUIReadScopeAndFailureDoNotChangeScreen(t *testing.T) {
	m, _ := gitOpsModel(t)
	p := m.uiTools(m.assistantConnections()).(*uiPort)
	p.connections[0].Driver = "changed-after-capture"
	if result := m.uiState(p); result.Status != "blocked" || result.View != nil {
		t.Fatal("accepted changed connection configuration", result)
	}
	p = m.uiTools(m.assistantConnections()).(*uiPort)
	p.scope = "foreign"
	result := m.uiState(p)
	if result.Status != "blocked" || result.View != nil {
		t.Fatal("leaked another scope", result)
	}
	r := uiTestRequest(m, agent.ViewRequest{View: "resource"})
	m.uiPending = r
	before := m.uiLocation()
	m.applyUI(uiLoaded{request: r, err: errors.New("not found")})
	if result = <-r.reply; result.Status != "failed" || before != m.uiLocation() {
		t.Fatal("failure changed the screen", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ctx, m.uiPending = ctx, r
	m.applyUI(uiLoaded{request: r})
	if len(r.reply) != 0 || before != m.uiLocation() {
		t.Fatal("cancelled request opened")
	}
}

func TestUIDocumentUsesGlobalAndLocalControls(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.mode, m.focus = 0, 1
	m.presentUI(&navigation.Prepared{Request: agent.ViewRequest{View: "evidence"}, Text: `{"saved":"original evidence"}`, Origin: "saved"})
	d := m.uiDocument.model
	m.Update(localMouseKey("/"))
	m.pasteText("original")
	if !d.Searching || d.Query != "original" {
		t.Fatal("document search/paste lost input ownership")
	}
	press(t, m, "alt+right")
	press(t, m, "alt+left")
	if m.uiDocument.model != d || !d.Searching {
		t.Fatal("panel travel lost document state")
	}
	press(t, m, "esc")
	m.execute("fullscreen")
	source, _, _, _ := m.expandableView()
	if !m.viewExpanded() || source != d {
		t.Fatal("document did not use shared fullscreen")
	}
	m.execute("help")
	if !strings.Contains(m.viewHelp(), "Assistant document") {
		t.Fatal("help lost document controls")
	}
	m.closeHelp()
	m.execute("q")
	if m.quitting || m.uiDocument != nil || m.page != "" || m.mode != 0 {
		t.Fatal("document close did not restore the original view")
	}
}

func TestUIListAndLogsUseExistingViewers(t *testing.T) {
	m, _ := gitOpsModel(t)
	m.focus = 2
	resource := kubecommon.Resource{Version: "v1", Name: "pods", Kind: "Pod", Namespaced: true}
	object := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "web", "namespace": "apps", "uid": "pod-one"},
		"spec":     map[string]any{"containers": []any{map[string]any{"name": "app"}}},
		"status":   map[string]any{"phase": "Running"},
	}}
	q := agent.ViewRequest{View: "list", Resource: "pods", Namespace: "*", Filter: "Running", Labels: "app=web"}
	m.presentUI(&navigation.Prepared{Request: q, Resource: resource, Catalog: []kubecommon.Resource{resource}, Collection: kubernetes.Collection{Objects: []unstructured.Unstructured{object}}, Origin: "live"})
	m.explore.Selected = kubernetes.Identity(object)
	result := m.uiState(m.uiTools(m.assistantConnections()).(*uiPort))
	if result.View.Namespace != "*" || result.View.Filter != "Running" || result.View.Labels != "app=web" || result.Selected == nil || result.Selected.UID != "pod-one" || result.Selected.Namespace != "apps" {
		t.Fatalf("list selection changed applied filters: %+v", result)
	}
	if !strings.Contains(ansi.Strip(m.mainBody(90, 25)), "web") {
		t.Fatal("filtered table did not render the prepared resource")
	}
	q = agent.ViewRequest{View: "logs", Resource: "pods", Namespace: "apps", Name: "web", Container: "app", Highlight: "connection refused"}
	m.presentUI(&navigation.Prepared{Request: q, Resource: resource, Catalog: []kubecommon.Resource{resource}, Object: &object, Text: "started\nconnection refused\n", Origin: "live"})
	if !m.logWorkspaceActive() || m.podLogViewer().Reader.Query != q.Highlight || m.explore.Log != nil || m.focus != 2 {
		t.Fatal("logs did not preserve the snapshot, highlight and focus")
	}
	if !strings.Contains(ansi.Strip(m.mainBody(90, 25)), "connection refused") {
		t.Fatal("prepared logs did not render")
	}
}

func TestUIWaitsEndWhenApplicationCloses(t *testing.T) {
	p := &uiPort{done: make(chan struct{}), requests: make(chan *uiRequest)}
	readDone, waitDone := make(chan error, 1), make(chan struct{})
	go func() {
		_, err := p.Read(context.Background())
		readDone <- err
	}()
	go func() {
		waitUI(p.requests, p.done)()
		close(waitDone)
	}()
	close(p.done)
	select {
	case err := <-readDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("UI caller outlived application closure")
	}
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("UI receiver outlived application closure")
	}
}
