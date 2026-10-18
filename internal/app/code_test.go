package app

import (
	"context"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"

	"github.com/simjay/plous/internal/source"
	docedit "github.com/simjay/plous/internal/ui/editor"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codeModel(t *testing.T) (*model, *appcode.CodeState) {
	t.Helper()
	m, _ := testModel(t)
	createWorkspace(t, m, "Mixed")
	snapshot, err := source.Load(context.Background(), source.Config{Path: "../../tests/fixtures/code/mixed"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "repo")
	if err = snapshot.Copy(dir); err != nil {
		t.Fatal(err)
	}
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "code", Name: "platform", Settings: map[string]string{"path": dir, "source_type": "Local directory"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	c := m.current().Connections[0]
	m.selectScope(m.active, "files", c.ID)
	m.mode, m.focus = 1, 1
	s := m.coder()
	s.Pending = "load"
	finishCode(t, m)
	return m, s
}

func finishCode(t *testing.T, m *model) {
	t.Helper()
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("no pending operation")
	}
	result, ok := cmd().(appcode.CodeResult)
	if !ok {
		t.Fatal("wrong operation result")
	}
	applyCodeResult(t, m, result)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
}

func TestCodeSidebarMixedDetectionAndScopedViews(t *testing.T) {
	m, s := codeModel(t)
	m.expanded[m.active] = true
	m.expanded[m.connection] = true
	rows := m.treeRows()
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.key] = true
	}
	for _, key := range []string{m.active + "/monitor", m.active + "/code", m.connection + "/files", m.connection + "/helm", m.connection + "/kustomize", m.connection + "/argo", m.connection + "/flux"} {
		if !seen[key] {
			t.Fatal("missing sidebar view", key)
		}
	}
	m.tool = "manifests"
	if !m.codeVisible() || m.exploreActive() {
		t.Fatal("Code routed to an unrelated tool")
	}
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending = "render"
	finishCode(t, m)
	if s.Builds[s.Preference.Selected] == nil || s.Preference.Section != 1 {
		t.Fatal("render not retained")
	}
	for _, size := range [][2]int{{32, 12}, {80, 24}, {120, 40}} {
		text := ansi.Strip(m.codeBody(size[0], size[1]))
		if !strings.Contains(text, "Final output") {
			t.Fatal(text)
		}
	}
	s.Preference.Section = 2
	m.openCodeTrace()
	for n, field := range s.CurrentFields() {
		if field.Pointer == "/metadata/name" {
			s.FieldCursor = n
		}
	}
	evidence := s.FieldEvidence(100, 20)
	if !strings.Contains(evidence, "plain/service.yaml:4") || !strings.Contains(evidence, "Exact source field") {
		t.Fatal(evidence)
	}
}

func TestCodeEditorPreservesDraftAndRequiresExactReview(t *testing.T) {
	m, s := codeModel(t)
	s.Preference.File = "README.md"
	m.tool = "manifests"
	m.openCodeFile(true)
	d := s.Document
	d.Paste("new text\n")
	draft := d.Text()
	m.execute("v")
	if s.Review == nil || !d.Dirty() {
		t.Fatal("review saved or disappeared")
	}
	d.Paste("changed after review\n")
	m.execute("w")
	if !d.Dirty() || !strings.Contains(d.Diagnostic, "changed after review") {
		t.Fatal("stale review wrote file")
	}
	s.Review = nil
	m.execute("1")
	m.execute("2")
	m.execute("help")
	m.closeHelp()
	if s.Document != d || d.Text() == draft {
		t.Fatal("navigation discarded draft")
	}
	m.execute("w")
	if d.Dirty() || d.SavedStatus != "Saved to local file" {
		t.Fatal(d.Diagnostic)
	}
	saved, _ := os.ReadFile(filepath.Join(s.Config.Path, "README.md"))
	if string(saved) != d.Text() {
		t.Fatal("wrong file bytes")
	}
	d.Paste("do-not-persist-buffer")
	m.rememberCode(s)
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	session, _ := m.store.ReadSession()
	if strings.Contains(string(session), "do-not-persist-buffer") {
		t.Fatal("persisted source buffer")
	}
	m.execute("q")
	if !s.Discard {
		t.Fatal("dirty close did not offer choice")
	}
	m.execute("q!")
	if s.Document != nil {
		t.Fatal("discard failed")
	}
}

func TestCodeSelectionFollowsScopeAndSurvivesRestart(t *testing.T) {
	m, s := codeModel(t)
	m.updateCode("end")
	m.updateCode("enter")
	file := s.Document.Name
	m.codeDocumentCommand("q")
	m.selectScope(m.active, "manifests", m.connection)
	m.updateCode("end")
	selected := s.Preference.Selected
	s.ChooseSection(1)
	s.ChooseSection(0)
	if s.Entries()[s.Cursor].ID != selected {
		t.Fatal("Sources highlighted a different entry from the selected scope")
	}
	m.selectScope(m.active, "files", m.connection)
	s = m.coder()
	if s.FileRows()[s.Cursor].Path != file {
		t.Fatal("Files lost its selection when changing source views")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	reopened.selectScope(m.active, m.tool, m.connection)
	restored := reopened.coder()
	if restored == nil || restored.Preference.File != file || restored.Preference.Selected != selected {
		t.Fatal("saved source selection was not restored")
	}
}

func TestCodeLateResultsStayBoundToCapturedSource(t *testing.T) {
	m, s := codeModel(t)
	s.Pending = "load"
	cmd := m.syncCode()
	s.Stop()
	result := cmd().(appcode.CodeResult)
	before := s.Inventory
	applyCodeResult(t, m, result)
	if s.Inventory != before {
		t.Fatal("cancelled operation replaced current source")
	}
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Inputs[s.Preference.Selected] = source.Inputs{Namespace: "old"}
	s.Pending = "render"
	cmd = m.syncCode()
	s.Inputs[s.Preference.Selected] = source.Inputs{Namespace: "new"}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Status(s.Preference.Selected) != "stale" {
		t.Fatal("changed inputs claimed fresh output")
	}
}

func TestSharedMonitorKeepsIDsHistoryAndOptions(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Monitor")
	path := contextFile(t, "production")
	w := *m.current()
	w.Connections = []workspace.Connection{{Tool: "kubernetes", Name: "production", Settings: map[string]string{"kubeconfig": path, "context": "production", "namespace": "team", "driver": "configmap", "future": "preserved"}}}
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	id := m.current().Connections[0].ID
	history, _ := m.store.History(m.active)
	before, _ := json.Marshal(history)
	for _, tool := range []string{"kubernetes", "helm"} {
		m.selectScope(m.active, tool, id)
		c := m.currentConnection()
		if c.Tool != tool || c.ID != id || c.Settings["driver"] != "configmap" {
			t.Fatal(c)
		}
	}
	captured := m.assistantConnections()
	if len(captured) != 2 || captured[1].ConnectionID != id+"/helm" || captured[1].Driver != "configmap" {
		t.Fatal(captured)
	}
	if m.savedConnection().Tool != "kubernetes" {
		t.Fatal("rewrote storage identity")
	}
	m.mode = 1
	m.persist()
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.selectScope(m.active, "helm", id)
	if restored.currentConnection().Tool != "helm" || restored.savedConnection().ID != id || restored.savedConnection().Settings["future"] != "preserved" {
		t.Fatal("restart lost capability selection or settings")
	}
	history, _ = m.store.History(m.active)
	after, _ := json.Marshal(history)
	if string(before) != string(after) {
		t.Fatal("navigation rewrote history")
	}
}

func TestCodeEditorKeysKeepLiteralInputAndUndo(t *testing.T) {
	m, s := codeModel(t)
	m.tool = "manifests"
	s.Preference.File = "README.md"
	m.openCodeFile(true)
	d := s.Document
	d.Mode = docedit.Insert
	typeText(m, "bbii:sscc")
	if m.command != nil || m.focus != 1 || !strings.Contains(d.Text(), "bbii:sscc") {
		t.Fatal("document keys dispatched app commands")
	}
	press(t, m, "esc")
	m.execute("u")
	if strings.Contains(d.Text(), "bbii:sscc") {
		t.Fatal("undo failed")
	}
}
