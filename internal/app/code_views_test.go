package app

import (
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"

	docedit "github.com/simjay/plous/internal/ui/editor"
	"slices"
	"strings"
	"testing"
)

func openSourceView(t *testing.T, m *model, id string) {
	t.Helper()
	if id == "render" {
		if !m.openCodeView(treeRow{workspace: m.active, connection: m.connection, tool: "manifests", resource: "render"}) {
			t.Fatal("render detail unavailable")
		}
		return
	}
	m.expanded[m.active], m.expanded[m.connection] = true, true
	m.treeCursor = treeIndexFor(t, m, m.connection+"/"+id)
	m.focus = 0
	m.updateTree("enter")
}

func TestSourceSidebarViewsDoNotDependOnDetection(t *testing.T) {
	m, s := codeModel(t)
	m.expanded[m.connection] = true
	for _, loaded := range []bool{true, false} {
		if !loaded {
			s.Inventory = nil
		}
		var labels []string
		for _, row := range m.treeRows() {
			if row.kind == "codeview" && row.connection == m.connection {
				labels = append(labels, row.label)
			}
		}
		if !slices.Equal(labels, []string{"Files", "Helm", "Kustomize", "Argo", "Flux"}) {
			t.Fatal("source views changed with detection", labels)
		}
	}
}

func TestSourceConnectionOpensFirstViewAndKeepsSidebarFocus(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		name := "enter"
		if mouse {
			name = "click"
		}
		t.Run(name, func(t *testing.T) {
			m, s := codeModel(t)
			connection := m.connection
			s.Preference.Section = 3
			m.selectScope(m.active, "", "")
			m.width, m.height = 200, 50
			m.expanded[connection] = false
			m.treeCursor = treeIndexFor(t, m, connection)
			m.focus = 0
			activate := func() {
				if mouse {
					clickMouse(t, m, "tree", "tree/"+connection)
				} else {
					m.updateTree("enter")
				}
			}
			for _, open := range []bool{true, false, true} {
				activate()
				if m.connection != connection || m.tool != "files" || s.ViewID() != "files" || m.mode != 1 {
					t.Fatal("source branch did not open or retain Files", m.tool, s.ViewID())
				}
				if m.focus != 0 || m.treeRows()[m.treeCursor].key != connection || m.expanded[connection] != open {
					t.Fatal("source branch moved the cursor or did not toggle", open)
				}
			}
			for _, row := range m.treeRows() {
				if active, _ := m.treeRowState(row); row.kind == "codeview" && active != (row.resource == "files") {
					t.Fatal("wrong source view highlighted", row)
				}
			}

			for _, id := range []string{"helm", "kustomize", "argo", "flux", "files"} {
				openSourceView(t, m, id)
				if id == "files" {
					selectCodeFile(t, s, "README.md")
					m.openCodeFile(true)
					s.Document.Paste("unfinished file edit\n")
				}
				document := s.Document
				draft := ""
				if document != nil {
					draft = document.Text()
				}
				m.treeCursor = treeIndexFor(t, m, connection)
				m.focus = 0
				for _, open := range []bool{false, true} {
					activate()
					if s.ViewID() != id || m.focus != 0 || m.treeRows()[m.treeCursor].key != connection || m.expanded[connection] != open {
						t.Fatal("folding the active source changed its view or focus", id, open)
					}
					if s.Document != document || document != nil && document.Text() != draft {
						t.Fatal("folding the source replaced the unfinished editor")
					}
				}
			}
		})
	}
}

func TestSourceDisclosureAndArrowExpansionKeepWelcome(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		m, _ := codeModel(t)
		connection, area := m.connection, m.active+"/code"
		m.selectScope(m.active, "", "")
		m.width, m.height = 200, 50
		m.expanded[area], m.expanded[connection] = false, false
		for _, key := range []string{area, connection} {
			m.treeCursor = treeIndexFor(t, m, key)
			m.focus = 0
			if mouse {
				clickMouse(t, m, "tree", "fold/"+key)
			} else {
				m.updateTree("right")
			}
			if m.connection != "" || m.tool != "" || m.focus != 0 || m.treeRows()[m.treeCursor].key != key || !m.expanded[key] {
				t.Fatal("expansion selected a source view or moved the cursor", mouse, key)
			}
		}
		finishCode(t, m)
		if m.connection != "" || m.treeRows()[m.treeCursor].key != connection {
			t.Fatal("source discovery changed the welcome page or sidebar cursor")
		}
	}
}

func TestSourceConnectionSelectionPreservesUnfinishedInput(t *testing.T) {
	m, s := codeModel(t)
	connection := m.connection
	s.Prompt("Helm Release Name", "unfinished")
	input := s.Input
	m.selectScope(m.active, "", "")
	m.expanded[connection] = false
	m.treeCursor = treeIndexFor(t, m, connection)
	m.focus = 0
	m.updateTree("enter")
	if s.Input != input || s.Input.Text != "unfinished" || m.connection != connection || m.coder().ViewID() != "files" || m.mode != 1 || m.expanded[connection] || m.focus != 1 {
		t.Fatal("source selection did not resume the unfinished input in its original view")
	}
}

func TestSourceSidebarNavigationPreservesScopeAndHighlightsOneView(t *testing.T) {
	m, s := codeModel(t)
	s.Preference.Selected = "plain:plain/service.yaml"
	m.selectScope(m.active, "manifests", m.connection)
	s.Pending = "render"
	finishCode(t, m)
	s.RememberSelection()
	resource := s.Preference.Resource
	for _, id := range []string{"files", "helm", "kustomize", "argo", "flux"} {
		openSourceView(t, m, id)
		if s.ViewID() != id || s.Preference.Selected != "plain:plain/service.yaml" || s.Preference.Resource != resource {
			t.Fatal("view navigation changed the selected scope", id, s.Preference)
		}
		active := 0
		for _, row := range m.treeRows() {
			if open, _ := m.treeRowState(row); row.kind == "codeview" && open {
				active++
				if row.resource != id {
					t.Fatal("wrong source view highlighted", row)
				}
			}
		}
		if active != 1 || m.mode != 1 || m.focus != 1 {
			t.Fatal("sidebar activation did not focus exactly one Explore view", id)
		}
		body := ansi.Strip(m.codeBody(100, 24))
		if !strings.Contains(body, "platform / "+s.ViewLabel()) || strings.Contains(body, "[Sources]") || strings.Contains(body, "K8S Manifests") {
			t.Fatal("view title or retired tabs are incorrect", body)
		}
		m.focusRootSidebar()
		if m.treeRows()[m.treeCursor].resource != id {
			t.Fatal("Back did not return to the current sidebar view")
		}
	}
	openSourceView(t, m, "render")
	for _, key := range []string{"[", "]", "left", "right"} {
		m.updateCode(key)
		if s.ViewID() != "files" {
			t.Fatal("main navigation switched sidebar views", key)
		}
	}
}

func TestSourceViewsPreserveDraftAndReviewWithoutCreatingAnotherEditor(t *testing.T) {
	m, s := codeModel(t)
	selectCodeFile(t, s, "README.md")
	m.openCodeFile(true)
	d := s.Document
	d.Paste("draft kept across views\n")
	draft, revision := d.Text(), d.Revision
	m.codeDocumentCommand("v")
	review := s.Review
	openSourceView(t, m, "render")
	if s.Document != nil || s.Review != nil {
		t.Fatal("suspended editor obscures Render")
	}
	s.Preference.File = "plain/service.yaml"
	m.openCodeFile(true)
	if s.Document != nil || !strings.Contains(m.status, "finish the open editor") {
		t.Fatal("opening a second editor replaced the suspended draft")
	}
	openSourceView(t, m, "files")
	if s.Document != d || s.Review != review || d.Text() != draft || d.Revision != revision {
		t.Fatal("returning to Files lost the exact editor or review")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, _ := m.store.ReadSession()
	if strings.Contains(string(data), "draft kept across views") {
		t.Fatal("navigation persisted source contents")
	}
	s.Review = nil
	m.codeDocumentCommand("u")
	if d.Text() == draft || d.Mode != docedit.Normal {
		t.Fatal("undo history was lost while the document was suspended")
	}
}

func TestSourceViewInputAndLateRenderKeepNavigation(t *testing.T) {
	m, s := codeModel(t)
	openSourceView(t, m, "render")
	s.Prompt("Helm Release Name", "unfinished")
	input := s.Input
	openSourceView(t, m, "files")
	if s.Input != input || s.Input.Text != "unfinished" || s.ViewID() != "files" {
		t.Fatal("sidebar discarded an unfinished source input")
	}
	s.Input = nil
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending = "render"
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("render did not start")
	}
	openSourceView(t, m, "files")
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.ViewID() != "files" || s.Builds[s.Preference.Selected] == nil {
		t.Fatal("completed render changed the newly selected view or lost its output")
	}
}

func TestSourceViewsExposeTerraformWithoutExecutingIt(t *testing.T) {
	m, s := codeModel(t)
	openSourceView(t, m, "render")
	s.Rescan = false
	s.Preference.Selected = "terraform:infra"
	s.RestoreCursor()
	if s.NeedsPreview() {
		t.Fatal("Terraform was treated as renderable")
	}
	for _, command := range []string{"render", "review"} {
		m.executeCode(command)
		if s.Pending != "" || s.Menu != "" || !strings.Contains(s.Err, "not available") {
			t.Fatal("unsupported entry offers execution", command)
		}
	}
	if m.syncCode() != nil || !strings.Contains(ansi.Strip(m.codeBody(90, 24)), "Files only") {
		t.Fatal("unsupported entry hid its limitation")
	}
	codeAction(t, m, "Repository Findings")
	if !strings.Contains(ansi.Strip(m.codeBody(90, 24)), "unsupported Terraform") {
		t.Fatal("findings omitted unsupported coverage")
	}
	m.updateCode("esc")
	m.updateCode("enter")
	if m.tool != "files" || s.FileRoot != "infra" {
		t.Fatal("Terraform entry did not open its files")
	}
}

func TestSourceViewLateComparisonReturnsToOriginatingView(t *testing.T) {
	m, s := codeModel(t)
	openSourceView(t, m, "render")
	s.Preference.Selected = "plain:plain/service.yaml"
	s.Pending, s.Other = "compare", s.Preference.Selected
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("comparison did not start")
	}
	openSourceView(t, m, "files")
	selectCodeFile(t, s, "README.md")
	m.openCodeFile(true)
	document, file := s.Document, s.File
	document.Paste("draft during comparison\n")
	draft := document.Text()
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if s.Document != document || s.File != file || s.Review != nil || document.Text() != draft {
		t.Fatal("late comparison replaced the active editor")
	}
	openSourceView(t, m, "render")
	if s.Comparison == nil || s.ComparisonScope != s.Preference.Selected || s.File != nil {
		t.Fatal("comparison was not retained in its originating view")
	}
	openSourceView(t, m, "files")
	if s.Document != document || s.File != file || document.Text() != draft {
		t.Fatal("returning from comparison lost the file draft")
	}
}

func TestSourceViewSessionCompatibility(t *testing.T) {
	for _, tool := range []string{"manifests", "tfcode"} {
		t.Run(tool, func(t *testing.T) {
			m, s := codeModel(t)
			m.tool = tool
			s.Preference.Selected = "plain:plain/service.yaml"
			s.Preference.Section = 3
			m.rememberCode(s)
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			reopened, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			reopened.selectScope(m.active, "manifests", m.connection)
			want := "files"
			if reopened.tool != "manifests" || reopened.coder().ViewID() != want || reopened.coder().Preference.Selected != s.Preference.Selected {
				t.Fatal("legacy view did not retain compatible navigation")
			}
			data, _ := reopened.store.ReadSession()
			var stored session
			if json.Unmarshal(data, &stored) != nil || stored.Tool != tool {
				t.Fatal("opening rewrote the saved session")
			}
		})
	}
}

func TestSourcesEmptyRepositoryHasUsefulViews(t *testing.T) {
	m, s := codeModel(t)
	snapshot := source.Snapshot{Files: map[string][]byte{}}
	s.Inventory = source.Discover(&snapshot)
	s.Preference.Selected = ""
	for _, id := range []string{"render"} {
		openSourceView(t, m, id)
		body := ansi.Strip(m.codeBody(90, 24))
		if strings.Contains(body, "Preparing Preview") || !strings.Contains(body, "Files") {
			t.Fatal("empty repository implies unavailable work is running", id, body)
		}
	}
	if slices.ContainsFunc(s.Entries(), func(e source.Entry) bool { return e.Renderable() }) {
		t.Fatal("empty source invented a renderable entry")
	}
}
