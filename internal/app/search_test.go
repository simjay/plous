package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/workspace"
	"maps"
	"strings"
	"testing"
)

func TestTreeSearchRevealsCollapsedPathsWithoutChangingFolds(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	m.setTreeExpanded(m.active, false)
	m.focus, m.treeCursor = 0, 0
	before := maps.Clone(m.expanded)
	typeText(m, "/dEpLoY")
	rows := m.treeRows()
	if len(rows) != 8 || rows[6].resource != "deployments.apps" || m.treeCursor != 6 {
		t.Fatalf("unexpected filtered path: %#v", rows)
	}
	if !maps.Equal(before, m.expanded) {
		t.Fatal("search changed saved folds")
	}
	press(t, m, "esc")
	if m.focus != 0 || m.searches[0] != nil || !maps.Equal(before, m.expanded) || m.treeCursor != 0 {
		t.Fatal("Esc did not restore the tree")
	}
	typeText(m, "/deploy")
	press(t, m, "enter")
	if m.focus != 1 || m.explore.Preference.Resource != "deployments.apps" || m.explore.Preference.Namespace != "shop" || m.searches[0] != nil {
		t.Fatal("search opened the wrong resource or scope")
	}
	if m.treeRows()[m.treeCursor].resource != "deployments.apps" {
		t.Fatal("opened result was hidden")
	}
}

func TestTreeSearchNoMatchAndCategoryActivation(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	m.focus = 0
	typeText(m, "/missing kind")
	press(t, m, "enter")
	if m.searches[0] == nil || m.settingsOverlay != nil || !strings.Contains(ansi.Strip(m.treeBody(30, 15)), "No matches") {
		t.Fatal("empty search activated a pinned action")
	}
	press(t, m, "esc")
	m.setTreeExpanded(m.connection, false)
	typeText(m, "/network")
	press(t, m, "enter")
	if !m.expanded[categoryTreeKey(m.connection, "Network")] || !m.expanded[m.connection] || m.searches[0] != nil {
		t.Fatal("opening a matched group did not expand its path")
	}
}

func TestAutomaticExpansionRetiresOnlyAutomaticBranches(t *testing.T) {
	m := exploreModel(t)
	m.syncExploreTree()
	workloads := categoryTreeKey(m.connection, "Workloads")
	network := categoryTreeKey(m.connection, "Network")
	cluster := categoryTreeKey(m.connection, "Cluster")
	m.chooseExploreResource("nodes")
	m.syncExploreTree()
	if m.expanded[workloads] || !m.expanded[cluster] {
		t.Fatal("old automatic group stayed open")
	}
	m.setTreeExpanded(network, true)
	m.setTreeExpanded(cluster, false)
	m.chooseExploreResource("pods")
	m.syncExploreTree()
	if !m.expanded[network] || m.expanded[cluster] || !m.expanded[workloads] {
		t.Fatal("manual expansion or fold was lost")
	}
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test")
	if err != nil {
		t.Fatal(err)
	}
	restored.selectScope(m.active, m.tool, m.connection)
	restored.explore = m.explore
	restored.mode = 1
	restored.syncExploreTree()
	restored.chooseExploreResource("nodes")
	restored.syncExploreTree()
	if restored.expanded[workloads] || !restored.expanded[network] || restored.expanded[cluster] {
		t.Fatal("restart lost automatic/manual expansion ownership")
	}
}

func TestPanelSearchKeepsCommandsAndPasteLiteral(t *testing.T) {
	m := exploreModel(t)
	m.focus = 0
	typeText(m, "/cc:ss11")
	m.Update(tea.PasteMsg{Content: ":q\n/model"})
	if m.searches[0].input.Text != "cc:ss11:q/model" || m.command != nil || m.chatEditing || m.mode != 1 || m.settingsOverlay != nil {
		t.Fatal("search text ran a command")
	}
	press(t, m, "alt+right")
	typeText(m, ":h")
	press(t, m, "enter")
	press(t, m, "alt+right")
	if m.focus != 2 || m.helpVisible() || m.help == nil {
		t.Fatal("Help did not pause for panel travel")
	}
	press(t, m, "alt+left")
	press(t, m, "esc")
	press(t, m, "alt+left")
	if m.focus != 0 || m.searches[0] == nil {
		t.Fatal("Alt+Right lost suspended search")
	}
	press(t, m, "esc")
	typeText(m, ":c")
	press(t, m, "enter")
	typeText(m, "/model")
	if m.searches[2] != nil || m.draft().Text != "/model" {
		t.Fatal("chat slash was captured by search")
	}
	press(t, m, "enter")
	if !m.chat().Models {
		t.Fatal("model command stopped working")
	}
}

func TestDetailFindMatchesAcrossWrapAndKeepsEscLocal(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(1)
	m.explore.DetailValid = true
	m.explore.DetailRun = nil
	m.explore.Detail.Object["longValue"] = strings.Repeat("x", 46) + "FindMe" + strings.Repeat("x", 100) + "findme"
	typeText(m, "/findme")
	view := m.exploreDetailBody(48, 13)
	if !m.yamlDocumentSearching() || !strings.Contains(m.explore.YamlDocument.Document.SearchStatus(), "1/2") || !strings.Contains(ansi.Strip(view), "1/2") {
		t.Fatalf("did not search the full YAML: %s", view)
	}
	press(t, m, "enter")
	typeText(m, "n")
	view = m.exploreDetailBody(48, 13)
	if !strings.Contains(ansi.Strip(view), "2/2") || m.explore.Detail == nil {
		t.Fatal("n did not move to the next match")
	}
	typeText(m, "N")
	if !strings.Contains(m.explore.YamlDocument.Document.SearchStatus(), "1/2") {
		t.Fatal("N did not find the previous match")
	}
	press(t, m, "esc")
	if m.yamlDocumentSearching() || m.explore.YamlDocument.Document.Query != "" || m.explore.Detail == nil || m.explore.Tab != 1 || m.focus != 1 {
		t.Fatal("Esc also exited the detail view")
	}
	press(t, m, "esc")
	if m.explore.Detail != nil {
		t.Fatal("second Esc did not exit details")
	}
}

func TestConversationSearchAndHistoryOpenMatchingIdentity(t *testing.T) {
	m, _ := testModel(t)
	c := m.chat()
	c.Turns = []history.Turn{{Request: history.Request{Prompt: "earlier needle"}, Response: history.Response{Text: strings.Repeat("later response\n", 40) + "second NEEDLE", Status: "completed"}}}
	m.focus = 2
	typeText(m, "/needle")
	view := ansi.Strip(m.chatTranscript(38, 14))
	if m.searches[2].total != 2 || !strings.Contains(view, "earlier needle") {
		t.Fatalf("first match hidden: %s", view)
	}
	press(t, m, "down")
	view = ansi.Strip(m.chatTranscript(38, 14))
	if !strings.Contains(view, "second NEEDLE") {
		t.Fatalf("next match hidden: %s", view)
	}
	press(t, m, "esc")
	if m.chatEditing || m.chat().History {
		t.Fatal("closing find changed chat mode")
	}
	first, err := m.history.NewConversation("", "Keep first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.history.NewConversation("", "Needle investigation")
	if err != nil {
		t.Fatal(err)
	}
	c.History, c.Choices, c.Cursor = true, []history.Conversation{first, second}, 0
	typeText(m, "/needle")
	press(t, m, "enter")
	if c.Conversation.ID != second.ID || c.History || m.searches[2] != nil {
		t.Fatal("filtered history opened the unfiltered index")
	}
}

func TestActivitySearchOpensMatchingRevision(t *testing.T) {
	m := exploreModel(t)
	m.mode = 2
	m.activityUI.Changes = []workspace.Change{{Revision: 8, Before: &workspace.Workspace{Name: "Before"}, After: workspace.Workspace{Name: "After"}}, {Revision: 3, After: workspace.Workspace{Name: "Shop"}}}
	typeText(m, "/created")
	if len(m.activityEntries()) != 1 {
		t.Fatal("Activity did not filter")
	}
	press(t, m, "enter")
	if !strings.Contains(ansi.Strip(m.detail), "Revision 3") || m.searches[1] != nil {
		t.Fatal("wrong revision opened")
	}
	press(t, m, "esc")
	if m.positions[m.viewKey()] != 0 || m.searches[1] == nil || m.searches[1].input.Text != "created" || len(m.activityEntries()) != 1 {
		t.Fatal("return lost the filtered selection")
	}
}

func TestSearchGeometryAndSessionIsolation(t *testing.T) {
	for _, panel := range []int{0, 1, 2} {
		m := exploreModel(t)
		if panel == 1 {
			m.openExploreSelection(1)
		}
		m.focus = panel
		typeText(m, "/"+strings.Repeat("検索", 60))
		for _, size := range [][2]int{{80, 16}, {120, 30}, {180, 45}} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			lines := strings.Split(ansi.Strip(m.render()), "\n")
			if len(lines) != size[1] {
				t.Fatalf("panel %d height %d: got %d", panel, size[1], len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("panel %d overflow: %s", panel, line)
				}
			}
		}
		if err := m.saveSession(); err != nil {
			t.Fatal(err)
		}
		data, err := m.store.ReadSession()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "検索") {
			t.Fatal("transient query was saved")
		}
	}
}

func TestFindLongLogBufferAndWideCharacters(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	m.syncExploreDetail()
	run := m.explore.DetailRun
	run.Cancel()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: m.explore.Detail.DeepCopy()})
	m.syncExploreDetail()
	m.podLogViewer().Append(strings.Repeat("xy", 64*1024))
	m.podLogViewer().Reader.Wrap = false
	typeText(m, "/x")
	m.exploreDetailBody(48, 15)
	if m.podLogViewer().Following || m.podLogViewer().Reader.Wrap || m.podLogViewer().Reader.Status() != "1/65536 matches" {
		t.Fatal("log find lost matches or changed the wrap preference")
	}
	typeText(m, "z")
	if m.viewExpanded() || m.podLogViewer().Reader.Query != "xz" {
		t.Fatal("typing z in log search ran the expansion shortcut")
	}
	press(t, m, "esc")
	if m.explore.Detail == nil || m.explore.Tab != 4 || m.podLogViewer().Following {
		t.Fatal("closing search left logs or resumed the stream")
	}
	rows := searchMatchRows("abc界界界", [][]int{{3, 6}, {6, 9}, {9, 12}}, 4)
	if len(rows) != 3 || rows[0] != 1 || rows[1] != 1 || rows[2] != 2 {
		t.Fatalf("wide-character wrap positions: %v", rows)
	}
}
