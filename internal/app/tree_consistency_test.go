package app

import (
	"github.com/simjay/plous/internal/workspace"
	"maps"
	"testing"
)

func TestSidebarBranchesKeepSiblingConnectionsCollapsed(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm", "code", "kustomize"} {
		t.Run(tool, func(t *testing.T) {
			m, _ := testModel(t)
			if !m.saveWorkspace(workspace.Workspace{Name: "Folders", Connections: []workspace.Connection{
				{Tool: tool, Name: "First", Settings: map[string]string{"context": "first", "path": t.TempDir()}},
				{Tool: tool, Name: "Second", Settings: map[string]string{"context": "second", "path": t.TempDir()}},
			}}) {
				t.Fatal(m.status)
			}
			first, second := m.current().Connections[0].ID, m.current().Connections[1].ID
			m.focus, m.mode = 0, 0
			m.setTreeExpanded(m.active, false)
			m.treeCursor = treeIndexFor(t, m, m.active)
			press(t, m, "right")
			for _, row := range m.treeRows() {
				if row.kind == "connection" && row.open || row.kind == "capability" || row.kind == "codeview" {
					t.Fatal("opening a workspace exposed a connection's children", row.label)
				}
			}
			m.treeCursor = treeIndexFor(t, m, first)
			press(t, m, "right")
			area := m.active + "/" + connectionArea(tool)
			m.treeCursor = treeIndexFor(t, m, area)
			press(t, m, "left")
			press(t, m, "right")
			if !m.treeRows()[treeIndexFor(t, m, first)].open || m.treeRows()[treeIndexFor(t, m, second)].open {
				t.Fatal("reopening a section changed its connections' fold choices")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range restored.treeRows() {
				if row.kind == "connection" && row.open || row.kind == "capability" || row.kind == "codeview" {
					t.Fatal("restart exposed a connection's children", row.label)
				}
			}
		})
	}
}

func TestCodeViewsAndRefreshPreserveNestedFolderFolds(t *testing.T) {
	m, s := codeModel(t)
	selectCodeFile(t, s, "chart/templates/deployment.yaml")
	s.FileTreeKey("left")
	s.FileTreeKey("left")
	before := maps.Clone(s.FileExpanded)
	for _, view := range []string{"helm", "files"} {
		openSourceView(t, m, view)
		finishCode(t, m)
		if !maps.Equal(s.FileExpanded, before) {
			t.Fatal("source view or refresh changed manual folder folds", view)
		}
	}
	var parent, child bool
	for _, row := range s.FileRows() {
		switch row.Path {
		case "chart":
			parent = row.Open
		case "chart/templates":
			child = !row.Open
		case "chart/templates/deployment.yaml":
			t.Fatal("view navigation exposed a file inside a folded folder")
		}
	}
	if !parent || !child {
		t.Fatal("Files did not restore its nested fold state")
	}
}
