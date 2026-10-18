package app

import (
	"github.com/simjay/plous/internal/workspace"
	"testing"
)

func TestStartupClosesSavedToolBranchesUnderCollapsedConnections(t *testing.T) {
	for _, tool := range []string{"kubernetes", "helm"} {
		t.Run(tool, func(t *testing.T) {
			m := exploreModel(t)
			w := *m.current()
			w.Connections[0].Tool = tool
			if !m.saveWorkspace(w) {
				t.Fatal(m.status)
			}
			connection := m.connection
			m.expanded[connection] = false
			m.expanded[connection+"/view/kubernetes"] = true
			m.expanded[connection+"/view/helm"] = true
			category := categoryTreeKey(connection, "Workloads")
			m.expanded[category] = true
			m.autoExpanded = nil
			m.draft().Text = "Keep this draft"
			m.focus, m.treeCursor = 0, treeIndexFor(t, m, connection)
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			restored.updateTree("right")
			for _, row := range restored.treeRows() {
				if row.kind == "capability" && row.open || row.kind == "helmsection" || row.kind == "category" {
					t.Fatal("restored connection exposed saved tool children", row.label)
				}
			}
			if !restored.expanded[category] || restored.connection != "" || restored.drafts[m.conversationScope()].Text != "Keep this draft" {
				t.Fatal("closing tool branches changed the deeper folds, scope, or draft")
			}
		})
	}
}

func TestStartupCollapsesEveryConnectionAndKeepsWorkspacePreferences(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	active, connection := m.active, m.connection
	m.explore.Preference.Filter = "checkout"
	m.rememberExplore()
	m.draft().Insert("Keep this draft", true)
	if !m.saveWorkspace(workspace.Workspace{Name: "Other", Connections: []workspace.Connection{
		{Tool: "code", Name: "Source", Settings: map[string]string{"path": t.TempDir()}},
		{Tool: "terraform", Name: "Terraform"},
	}}) {
		t.Fatal(m.status)
	}
	for _, w := range m.workspaces {
		for _, c := range w.Connections {
			m.expanded[c.ID], m.autoExpanded[c.ID] = true, true
		}
	}
	m.active, m.connection, m.tool = active, connection, "kubernetes"
	m.treeCursor = treeIndexFor(t, m, connection+"/resource/pods")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.explore = m.explore
	restored.syncExploreTree()
	for _, w := range restored.workspaces {
		for _, c := range w.Connections {
			if open, saved := restored.expanded[c.ID]; open || !saved || restored.autoExpanded[c.ID] {
				t.Fatal("startup or rediscovery reopened a connection", c.Name)
			}
		}
	}
	if restored.active != "" || restored.connection != "" || restored.tool != "" || restored.mode != 0 || restored.drafts[m.conversationScope()].Text != "Keep this draft" {
		t.Fatal("startup collapse changed the selected scope or draft")
	}
	if restored.treeRows()[restored.treeCursor].key != connection || !restored.expanded[categoryTreeKey(connection, "Workloads")] {
		t.Fatal("startup lost the visible cursor ancestor or nested fold preference")
	}
	if restored.explorePreferences[active+"/"+connection].Filter != "checkout" {
		t.Fatal("startup changed the selected resource filter")
	}
}

func TestStartupSourceScanKeepsConnectionCollapsed(t *testing.T) {
	for _, collapsedSection := range []bool{false, true} {
		t.Run(map[bool]string{false: "Source Cursor", true: "Section Cursor"}[collapsedSection], func(t *testing.T) {
			m, _ := codeModel(t)
			m.expanded[m.connection] = true
			m.treeCursor = treeIndexFor(t, m, m.connection+"/helm")
			if collapsedSection {
				m.expanded[m.active+"/code"] = false
				m.treeCursor = treeIndexFor(t, m, m.active+"/code")
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			restored, err := newModel(m.store, m.history, "test")
			if err != nil {
				t.Fatal(err)
			}
			if restored.syncCode() != nil {
				t.Fatal("startup scanned a saved workspace")
			}
			restored.selectScope(m.active, "", "")
			finishCode(t, restored)
			if restored.expanded[m.connection] || !restored.codeHas(m.active, m.connection, "terraform") {
				t.Fatal("startup scan did not detect categories behind the collapsed source")
			}
			for _, row := range restored.treeRows() {
				if row.kind == "codeview" {
					t.Fatal("startup scan exposed source children")
				}
			}
			want := m.connection
			if collapsedSection {
				want = m.active + "/code"
			}
			if restored.treeRows()[restored.treeCursor].key != want {
				t.Fatal("startup did not restore the closest visible ancestor")
			}
		})
	}
}
