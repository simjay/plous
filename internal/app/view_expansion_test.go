package app

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/diffview"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/textview"
)

func TestViewExpansionRestoresSavedLayoutsAcrossFrameworks(t *testing.T) {
	for _, kind := range []string{"yaml", "editor", "diff", "logs", "activity"} {
		for _, hidden := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
			t.Run(kind+string(rune('0'+boolInt(hidden[0])*2+boolInt(hidden[1]))), func(t *testing.T) {
				m := yamlViewModel(t)
				switch kind {
				case "editor", "diff":
					m, _ = documentModel(t)
					if kind == "diff" {
						a := m.explore.action
						a.stage = "review"
						a.review = diffview.New("old\n", "new\n", a.document.Target, a.document.Revision, true)
					}
				case "logs":
					m.explore.tab = 4
					m.podLogViewer().Append(strings.Repeat("retained output\n", 100))
					m.podLogViewer().Pause()
				case "activity":
					m.mode = 2
					changes, err := m.store.History(m.active)
					if err != nil || len(changes) == 0 {
						t.Fatal("missing workspace history", err)
					}
					m.setActivityDetail(changes[0])
					m.openActivityReview()
				}
				m.leftHidden, m.rightHidden = hidden[0], hidden[1]
				widths := m.panelWidths()
				source, _, _, _ := m.expandableView()
				m.execute("zoom")
				if !m.viewExpanded() || m.panelWidths() != [3]int{0, m.width, 0} {
					t.Fatal("view did not expand", m.status)
				}
				if m.leftHidden != hidden[0] || m.rightHidden != hidden[1] {
					t.Fatal("expansion changed saved visibility")
				}
				if err := m.saveSession(); err != nil {
					t.Fatal(err)
				}
				data, err := m.store.ReadSession()
				if err != nil {
					t.Fatal(err)
				}
				var saved session
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				if saved.LeftHidden != hidden[0] || saved.RightHidden != hidden[1] || strings.Contains(string(data), "viewExpansion") {
					t.Fatal("temporary layout persisted")
				}
				m.render()
				m.execute("z")
				if m.viewExpanded() || widths != m.panelWidths() {
					t.Fatal("layout not restored")
				}
				after, _, _, _ := m.expandableView()
				if source != after {
					t.Fatal("component replaced during expansion")
				}
			})
		}
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestExpandedEscapeClosesLocalLayersBeforeRestoring(t *testing.T) {
	m := yamlViewModel(t)
	mainCommand(t, m, "z")
	d := m.explore.yamlDocument.document
	typeText(m, "z/")
	m.Update(tea.PasteMsg{Content: "kind"})
	press(t, m, "esc")
	if !m.viewExpanded() || d.Searching || d.Mode != docedit.Select {
		t.Fatal("search Escape restored too soon")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || d.Mode != docedit.Normal {
		t.Fatal("selection Escape restored too soon")
	}
	press(t, m, "esc")
	if m.viewExpanded() || m.explore.detail == nil {
		t.Fatal("restore Escape left resource")
	}
	press(t, m, "esc")
	if m.explore.detail != nil {
		t.Fatal("next Escape failed to leave resource")
	}
}

func TestExpandedDocumentKeepsDraftThroughCommandsAndOverlays(t *testing.T) {
	m, a := documentModel(t)
	mainCommand(t, m, "zoom")
	typeText(m, "i")
	m.Update(tea.PasteMsg{Content: "# draft\n"})
	text := a.document.Text()
	press(t, m, "esc")
	if !m.viewExpanded() || a.document.Mode != docedit.Normal {
		t.Fatal("Insert Escape restored layout")
	}
	mainCommand(t, m, "h")
	if m.help == nil || m.viewExpansion == nil {
		t.Fatal("Help lost temporary view")
	}
	press(t, m, "esc")
	if !m.viewExpanded() {
		t.Fatal("Help failed to resume view")
	}
	mainCommand(t, m, "L")
	if a.document.LineNumbers {
		t.Fatal("line numbers not toggled")
	}
	mainCommand(t, m, "c")
	if m.focus != 2 || !m.chatEditing || m.viewExpansion != nil {
		t.Fatal("chat did not restore panels")
	}
	if a.document.Text() != text || m.explore.action != a {
		t.Fatal("chat discarded draft")
	}
}

func TestExpandRestoresOnPanelNavigationAndKeepsReviewUnconfirmed(t *testing.T) {
	m, a := documentModel(t)
	a.stage = "review"
	a.review = diffview.New("old\n", "new\n", a.document.Target, a.document.Revision, true)
	a.review.Reader.Selecting = true
	mainCommand(t, m, "z")
	press(t, m, "tab")
	if m.focus != 2 || m.viewExpansion != nil || !a.review.Reader.Selecting {
		t.Fatal("Tab lost review state or trapped focus")
	}
	press(t, m, "shift+tab")
	mainCommand(t, m, "z")
	press(t, m, "esc")
	press(t, m, "esc")
	if a.stage != "review" || a.pending != "" || a.confirmedID != "" || m.viewExpansion != nil {
		t.Fatal("restore confirmed or left review")
	}
	mainCommand(t, m, "z")
	mainCommand(t, m, "b")
	if m.viewExpansion != nil || !m.leftHidden {
		t.Fatal("explicit sidebar command did not restore then toggle")
	}
}

func TestExpandedFramesFitAndKeepReaderPosition(t *testing.T) {
	m := yamlViewModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, command := range []string{"zoom", "z"} {
			m.execute(command)
			view := m.render()
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatalf("%v height overflow", size)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%v width overflow", size)
				}
			}
		}
	}
	m.explore.tab = 4
	m.podLogViewer().Pause()
	m.podLogViewer().Append(strings.Repeat(strings.Repeat("x", 90)+"\n", 100))
	m.width, m.height = 120, 30
	m.render()
	v := m.podLogViewer()
	v.Reader.Cursor = textview.Point{Row: 30, Col: 45}
	m.render()
	before, left, cursor := v.Reader.Top, v.Reader.Left, v.Reader.Cursor
	typeText(m, "z")
	if !m.viewExpanded() || m.command != nil {
		t.Fatal("log shortcut did not expand immediately")
	}
	m.render()
	typeText(m, "z")
	m.render()
	if m.viewExpanded() || v.Reader.Top != before || v.Reader.Left != left || v.Reader.Cursor != cursor || m.podLogViewer().Following {
		t.Fatal("expansion lost reading position")
	}
	typeText(m, "z")
	m.render()
	v.Reader.Key("pgdown", "")
	m.render()
	cursor = v.Reader.Cursor
	m.execute("z")
	m.render()
	if v.Reader.Cursor != cursor {
		t.Fatal("restore rewound navigation in expanded view")
	}
}

func TestExpandedExecReviewKeepsSnapshotAndRestoresOnResume(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 30, 5)
	m.explore.terminal = tm
	writeTerminalLines(t, tm, 0, 100)
	m.execute("zoom")
	if m.viewExpansion != nil {
		t.Fatal("live Exec was expanded")
	}
	tm.beginReview()
	mainCommand(t, m, "z")
	r := tm.review
	if !m.viewExpanded() || tm.captured {
		t.Fatal("output review did not expand")
	}
	typeText(m, "b")
	press(t, m, "esc")
	if !m.viewExpanded() || m.shortcutPrefix != "" {
		t.Fatal("prefix Escape restored view too soon")
	}
	m.render()
	if tm.review != r {
		t.Fatal("expansion refreshed the held output")
	}
	typeText(m, "r")
	if !tm.captured || m.viewExpansion != nil {
		t.Fatal("resuming input did not restore layout")
	}
}

func TestExpandedDiagnosticsAndDraftConfirmationStayLocal(t *testing.T) {
	m, a := documentModel(t)
	mainCommand(t, m, "z")
	a.showDiagnostic("invalid document")
	press(t, m, "esc")
	if !m.viewExpanded() || a.diagnostic != nil {
		t.Fatal("diagnostic Escape also restored view")
	}
	a.document.Paste("# draft\n")
	mainCommand(t, m, "q")
	if !a.discard || !m.viewExpanded() {
		t.Fatal("close lost expansion before discard decision")
	}
	press(t, m, "esc")
	if a.discard || !m.viewExpanded() || !a.document.Dirty() {
		t.Fatal("cancel discard lost document")
	}
	press(t, m, "esc")
	if m.viewExpanded() || !a.document.Dirty() {
		t.Fatal("restore discarded draft")
	}
}
