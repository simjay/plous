package app

import (
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/source"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceReentryAndExpansionRescanCategories(t *testing.T) {
	m, s := codeModel(t)
	id := m.connection
	if err := os.Remove(filepath.Join(s.Config.Path, "infra/main.tf")); err != nil {
		t.Fatal(err)
	}
	m.selectScope(m.active, "", "")
	m.selectScope(m.active, "files", id)
	finishCode(t, m)
	if m.codeHas(m.active, id, "terraform") {
		t.Fatal("returning to the source retained a removed category")
	}
	if err := os.WriteFile(filepath.Join(s.Config.Path, "added.tf"), []byte("terraform {}"), 0600); err != nil {
		t.Fatal(err)
	}
	m.expanded[id] = false
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, id)
	m.updateTree("right")
	if m.connection != id || m.focus != 0 || !m.expanded[id] {
		t.Fatal("expansion changed navigation")
	}
	finishCode(t, m)
	if !m.codeHas(m.active, id, "terraform") {
		t.Fatal("expansion did not discover a newly added category")
	}
	m.updateTree("right")
	if m.syncCode() != nil {
		t.Fatal("Right on an already expanded source repeated the scan")
	}
}

func TestSidebarRefreshTargetsHighlightedSourceAndPreservesDraft(t *testing.T) {
	for _, command := range []string{"g", ":g", ":refresh"} {
		t.Run(command, func(t *testing.T) {
			m, s := codeModel(t)
			first := m.connection
			s.Preference.File = "README.md"
			s.RestoreCursor()
			m.openCodeFile(true)
			s.Document.Paste("unsaved changes\n")
			draft, text, revision, baseline := s.Document, s.Document.Text(), s.Document.Revision, string(s.File.Baseline)
			w := *m.current()
			w.Connections = append(w.Connections, workspace.Connection{Tool: "code", Name: "Second", Settings: map[string]string{"path": t.TempDir()}})
			if !m.saveWorkspace(w) {
				t.Fatal(m.status)
			}
			second := m.current().Connections[1]
			finishCode(t, m)
			if err := os.WriteFile(filepath.Join(second.Settings["path"], "new.tf"), []byte("terraform {}"), 0600); err != nil {
				t.Fatal(err)
			}
			m.expanded[second.ID] = false
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, second.ID)
			if command == "g" {
				m.updateTree(command)
			} else {
				m.execute(strings.TrimPrefix(command, ":"))
			}
			finishCode(t, m)
			if !m.codeHas(m.active, second.ID, "terraform") || m.connection != first || m.focus != 0 || m.expanded[second.ID] {
				t.Fatal("sidebar refresh targeted the main view or changed its scope/folds")
			}
			m.treeCursor = treeIndexFor(t, m, first)
			m.updateTree("g")
			finishCode(t, m)
			if s.Document != draft || draft.Text() != text || draft.Revision != revision || string(s.File.Baseline) != baseline {
				t.Fatal("rescanning overwrote the open draft or its save baseline")
			}
		})
	}
}

func TestSourceRefreshKeepsCapturedDocumentOrigin(t *testing.T) {
	m, s := codeModel(t)
	before := sourceTextFixture(t, s.Inventory.Source, "README.md")
	s.ShowDocument("Captured Trace", "captured")
	s.DocumentSource = source.Discover(changedSourceFixture(t, s.Inventory.Source, "README.md", []byte(before)))
	s.DocumentKind, s.DocumentPath = "trace", "README.md"
	if err := os.WriteFile(filepath.Join(s.Config.Path, "README.md"), []byte("Changed on disk"), 0600); err != nil {
		t.Fatal(err)
	}
	m.queueTreeSources(treeRow{kind: "codeview", workspace: m.active, connection: m.connection})
	finishCode(t, m)
	m.openCodeDocumentSource()
	if s.Document.Text() != before || sourceTextFixture(t, s.Inventory.Source, "README.md") != "Changed on disk" {
		t.Fatal("source refresh substituted new bytes into a captured document's origin")
	}
}

func TestSourceRefreshFailureMarksCachedCategoriesStale(t *testing.T) {
	m, s := codeModel(t)
	m.expanded[m.connection] = true
	if err := os.Rename(s.Config.Path, s.Config.Path+"-moved"); err != nil {
		t.Fatal(err)
	}
	m.queueTreeSources(treeRow{kind: "workspace", workspace: m.active})
	cmd := m.syncCode()
	if cmd == nil || m.syncCode() != nil {
		t.Fatal("refresh did not schedule exactly one scan")
	}
	applyCodeResult(t, m, cmd().(appcode.CodeResult))
	if m.codeScanStatus(m.active, m.connection) != "scan failed" || s.Inventory == nil || m.syncCode() != nil {
		t.Fatal("failed refresh hid its failure, discarded cached data, or retried")
	}
	for _, row := range m.treeRows() {
		if row.kind != "codeview" || row.workspace != m.active || row.tool == "files" {
			continue
		}
		want := ""
		for _, item := range s.Inventory.Catalog {
			if item.Technology == row.resource {
				want = "stale"
			}
		}
		if row.badge != want {
			t.Fatal("freshness badge does not match cached items", row)
		}
	}
}

func TestCodeEmptyViewsHaveNoFreshnessBadges(t *testing.T) {
	m, s := codeModel(t)
	m.expanded[m.connection] = true
	for _, inventory := range []*source.Inventory{nil, source.Discover(&source.Snapshot{Files: map[string][]byte{}})} {
		s.Inventory = inventory
		for _, state := range []string{"not scanned", "scan queued", "scanning", "scan failed", "scan cancelled", "loaded"} {
			s.LoadAttempted = state != "not scanned"
			s.Rescan = state == "scan queued"
			s.Busy, s.Running = state == "scanning", "load"
			s.ScanFailed, s.ScanCancelled = state == "scan failed", state == "scan cancelled"
			for _, row := range m.treeRows() {
				if row.kind == "codeview" && row.workspace == m.active && row.tool != "files" && row.badge != "" {
					t.Fatalf("empty view labeled %q during %s: %s", row.badge, state, row.label)
				}
			}
		}
	}
}

func TestSourceRefreshRemovedFormatKeepsViewCursor(t *testing.T) {
	m, s := codeModel(t)
	m.expanded[m.connection] = true
	m.treeCursor = treeIndexFor(t, m, m.connection+"/helm")
	if err := os.Remove(filepath.Join(s.Config.Path, "infra/main.tf")); err != nil {
		t.Fatal(err)
	}
	m.refreshTree()
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("refresh was not scheduled")
	}
	initial := cmd().(appcode.CodeResult)
	m.Update(initial)
	if initial.Continue != nil {
		m.Update(initial.Continue())
	}
	if m.treeRows()[m.treeCursor].key != m.connection+"/helm" || m.codeHas(m.active, m.connection, "terraform") {
		t.Fatal("format discovery changed navigation or retained the removed format")
	}
}

func TestSidebarCodeRefreshQueuesEverySourceWithoutSelectingIt(t *testing.T) {
	m, _ := codeModel(t)
	active, selected := m.active, m.connection
	w := *m.current()
	w.Connections = append(w.Connections, workspace.Connection{Tool: "code", Name: "Other", Settings: map[string]string{"path": t.TempDir()}})
	if !m.saveWorkspace(w) {
		t.Fatal(m.status)
	}
	finishCode(t, m)
	m.focus, m.treeCursor = 0, treeIndexFor(t, m, m.active+"/code")
	m.updateTree("g")
	seen := map[string]bool{}
	for range w.Connections {
		cmd := m.syncCode()
		if cmd == nil || m.syncCode() != nil {
			t.Fatal("Code refresh must serialize every source scan")
		}
		r := cmd().(appcode.CodeResult)
		seen[r.State.Connection] = true
		applyCodeResult(t, m, r)
	}
	if len(seen) != 2 || m.syncCode() != nil || m.active != active || m.connection != selected || m.focus != 0 {
		t.Fatal("Code refresh lost a source, changed selection, or restarted")
	}
}

func TestSidebarRefreshDuringScanQueuesOneFreshPass(t *testing.T) {
	m, s := codeModel(t)
	row := treeRow{kind: "codeview", workspace: m.active, connection: m.connection}
	m.queueTreeSources(row)
	cmd := m.syncCode()
	if cmd == nil {
		t.Fatal("scan did not start")
	}
	older := cmd().(appcode.CodeResult)
	if err := os.WriteFile(filepath.Join(s.Config.Path, "after-scan.txt"), []byte("New"), 0600); err != nil {
		t.Fatal(err)
	}
	m.queueTreeSources(row)
	m.queueTreeSources(row)
	if m.syncCode() != nil {
		t.Fatal("refresh started a competing scan")
	}
	applyCodeResult(t, m, older)
	finishCode(t, m)
	if sourceTextFixture(t, s.Inventory.Source, "after-scan.txt") != "New" || m.syncCode() != nil {
		t.Fatal("refresh requests did not coalesce into one fresh pass")
	}
}
