package app

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
						a := m.explore.Action
						a.Stage = "review"
						a.Review = diffview.New("old\n", "new\n", a.Document.Target, a.Document.Revision, true)
					}
				case "logs":
					m.explore.Tab = 4
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
				m.execute("fullscreen")
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
				m.execute("f")
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
	typeText(m, "ff")
	if !m.viewExpanded() {
		t.Fatal("read-only YAML did not accept the fullscreen shortcut")
	}
	d := m.explore.YamlDocument.Document
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
	if m.viewExpanded() || m.explore.Detail == nil {
		t.Fatal("restore Escape left resource")
	}
	press(t, m, "esc")
	if m.explore.Detail != nil {
		t.Fatal("next Escape failed to leave resource")
	}
}

func TestExpandedDocumentKeepsDraftThroughCommandsAndOverlays(t *testing.T) {
	m, a := documentModel(t)
	mainCommand(t, m, "fullscreen")
	typeText(m, "i")
	m.Update(tea.PasteMsg{Content: "# draft\n"})
	text := a.Document.Text()
	press(t, m, "esc")
	if !m.viewExpanded() || a.Document.Mode != docedit.Normal {
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
	if a.Document.LineNumbers {
		t.Fatal("line numbers not toggled")
	}
	mainCommand(t, m, "c")
	if m.focus != 2 || !m.chatEditing || m.viewExpansion != nil {
		t.Fatal("chat did not restore panels")
	}
	if a.Document.Text() != text || m.explore.Action != a {
		t.Fatal("chat discarded draft")
	}
}

func TestExpandRestoresOnPanelNavigationAndKeepsReviewUnconfirmed(t *testing.T) {
	m, a := documentModel(t)
	a.Stage = "review"
	a.Review = diffview.New("old\n", "new\n", a.Document.Target, a.Document.Revision, true)
	a.Review.Reader.Selecting = true
	mainCommand(t, m, "f")
	press(t, m, "alt+right")
	if m.focus != 2 || m.viewExpansion != nil || !a.Review.Reader.Selecting {
		t.Fatal("Alt+Right lost review state or trapped focus")
	}
	press(t, m, "alt+left")
	mainCommand(t, m, "f")
	press(t, m, "esc")
	press(t, m, "esc")
	if a.Stage != "review" || a.Pending != "" || a.ConfirmedID != "" || m.viewExpansion != nil {
		t.Fatal("restore confirmed or left review")
	}
	mainCommand(t, m, "f")
	mainCommand(t, m, "b")
	if m.viewExpansion != nil || !m.leftHidden {
		t.Fatal("explicit sidebar command did not restore then toggle")
	}
}

func TestExpandedFramesFitAndKeepReaderPosition(t *testing.T) {
	m := yamlViewModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m.width, m.height = size[0], size[1]
		for _, command := range []string{"fullscreen", "f"} {
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
	m.explore.Tab = 4
	m.podLogViewer().Pause()
	m.podLogViewer().Append(strings.Repeat(strings.Repeat("x", 90)+"\n", 100))
	m.width, m.height = 120, 30
	m.render()
	v := m.podLogViewer()
	v.Reader.Cursor = textview.Point{Row: 30, Col: 45}
	m.render()
	before, left, cursor := v.Reader.Top, v.Reader.Left, v.Reader.Cursor
	typeText(m, "ff")
	if !m.viewExpanded() || m.command != nil {
		t.Fatal("log shortcut did not expand on the second key")
	}
	m.render()
	typeText(m, "ff")
	m.render()
	if m.viewExpanded() || v.Reader.Top != before || v.Reader.Left != left || v.Reader.Cursor != cursor || m.podLogViewer().Following {
		t.Fatal("expansion lost reading position")
	}
	typeText(m, "ff")
	m.render()
	v.Reader.Key("pgdown", "")
	m.render()
	cursor = v.Reader.Cursor
	m.execute("f")
	m.render()
	if v.Reader.Cursor != cursor {
		t.Fatal("restore rewound navigation in expanded view")
	}
}

func TestExpandedExecReviewKeepsSnapshotAndRestoresOnResume(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 30, 5)
	m.explore.Terminal = tm
	writeTerminalLines(t, tm, 0, 100)
	m.execute("fullscreen")
	if m.viewExpansion != nil {
		t.Fatal("live Exec was expanded")
	}
	tm.BeginReview()
	mainCommand(t, m, "f")
	r := tm.Review
	if !m.viewExpanded() || tm.Captured {
		t.Fatal("output review did not expand")
	}
	typeText(m, "b")
	press(t, m, "esc")
	if !m.viewExpanded() || m.shortcutPrefix != "" {
		t.Fatal("prefix Escape restored view too soon")
	}
	m.render()
	if tm.Review != r {
		t.Fatal("expansion refreshed the held output")
	}
	typeText(m, "r")
	if !tm.Captured || m.viewExpansion != nil {
		t.Fatal("resuming input did not restore layout")
	}
}

func TestExpandedDiagnosticsAndDraftConfirmationStayLocal(t *testing.T) {
	m, a := documentModel(t)
	mainCommand(t, m, "f")
	a.ShowDiagnostic("invalid document")
	press(t, m, "esc")
	if !m.viewExpanded() || a.Diagnostic != nil {
		t.Fatal("diagnostic Escape also restored view")
	}
	a.Document.Paste("# draft\n")
	mainCommand(t, m, "q")
	if !a.Discard || !m.viewExpanded() {
		t.Fatal("close lost expansion before discard decision")
	}
	press(t, m, "esc")
	if a.Discard || !m.viewExpanded() || !a.Document.Dirty() {
		t.Fatal("cancel discard lost document")
	}
	press(t, m, "esc")
	if m.viewExpanded() || !a.Document.Dirty() {
		t.Fatal("restore discarded draft")
	}
}

func TestGlobalExpansionSurvivesMainNavigationAndLocalSearch(t *testing.T) {
	m := exploreModel(t)
	for _, command := range []string{"1", "2", "3"} {
		mainCommand(t, m, command)
		mainCommand(t, m, "f")
		if !m.viewExpanded() || m.panelWidths() != [3]int{0, m.width, 0} {
			t.Fatalf("main view %s did not expand", command)
		}
		mainCommand(t, m, "fullscreen")
	}
	mainCommand(t, m, "1")
	typeText(m, "ff22")
	if !m.viewExpanded() || m.mode != 1 {
		t.Fatal("switching main tabs restored the layout")
	}
	// Switching tabs stops the fixture's stream; use a populated table for opening details.
	m = exploreModel(t)
	typeText(m, "ff")
	press(t, m, "enter")
	if !m.viewExpanded() || m.explore.Detail == nil {
		t.Fatal("opening resource details restored the layout")
	}
	typeText(m, "/ff")
	if !m.viewExpanded() || m.searches[1] == nil || m.searches[1].input.Text != "ff" {
		t.Fatal("search did not keep literal shortcut keys")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || m.searches[1] != nil {
		t.Fatal("search Escape restored layout")
	}
	press(t, m, "esc")
	if m.viewExpanded() || m.explore.Detail == nil {
		t.Fatal("restore Escape closed resource details")
	}
	press(t, m, "esc")
	if m.explore.Detail != nil {
		t.Fatal("next Escape failed to close details")
	}
}

func TestGlobalExpansionRetainsPickersAndHiddenPanels(t *testing.T) {
	m := exploreModel(t)
	typeText(m, "ff/")
	typeText(m, "ff")
	if !m.viewExpanded() || m.explore.Input == nil || m.explore.Input.Text != "ff" {
		t.Fatal("resource filter triggered fullscreen")
	}
	press(t, m, "esc")
	if !m.viewExpanded() || m.explore.Input != nil {
		t.Fatal("closing resource filter restored layout")
	}
	mainCommand(t, m, "K")
	press(t, m, "esc")
	if !m.viewExpanded() || m.explore.Picker != "" {
		t.Fatal("closing the connection picker restored layout")
	}
	m.leftHidden, m.rightHidden = true, true
	press(t, m, "alt+right")
	if !m.viewExpanded() || m.focus != 1 {
		t.Fatal("Alt+Right restored layout without another visible panel")
	}
	m.leftHidden = false
	press(t, m, "alt+left")
	if m.viewExpansion != nil || m.focus != 0 || !m.rightHidden {
		t.Fatal("panel switch did not restore the saved visibility")
	}
}

func TestGlobalExpansionKeepsOverlaysAndDocumentOwnership(t *testing.T) {
	m, a := documentModel(t)
	before := a.Document.Text()
	typeText(m, "ff")
	if m.viewExpansion != nil || a.Document.Text() != before {
		t.Fatal("document keys triggered global expansion")
	}
	mainCommand(t, m, "f")
	mainCommand(t, m, "q")
	if !m.viewExpanded() || m.explore.Action != nil {
		t.Fatal("closing a clean editor restored the layout")
	}
	mainCommand(t, m, "s")
	if m.settingsOverlay == nil || m.viewExpansion == nil {
		t.Fatal("Settings discarded expansion")
	}
	typeText(m, "ff")
	if m.settingsOverlay == nil || m.viewExpansion == nil || a.Document.Text() != before {
		t.Fatal("fullscreen changed an overlay or discarded the draft")
	}
	press(t, m, "esc")
	if !m.viewExpanded() {
		t.Fatal("closing Settings did not resume expansion")
	}
	mainCommand(t, m, "f")
	mainCommand(t, m, "c")
	typeText(m, "ff:fullscreen")
	if m.viewExpansion != nil || m.draft().Text != "ff:fullscreen" {
		t.Fatal("chat interpreted fullscreen shortcuts")
	}
}

func TestGlobalExpansionHelmDocumentsKeepLocalInput(t *testing.T) {
	m, h := helmModel(t)
	typeText(m, "ff")
	m.openHelmDocument(h, "Values", "key: value\n")
	m.syncViewExpansion()
	if !m.viewExpanded() {
		t.Fatal("opening Helm values restored layout")
	}
	typeText(m, "/ff")
	press(t, m, "esc")
	if !m.viewExpanded() || h.Document.Searching {
		t.Fatal("Helm search Escape restored layout")
	}
	press(t, m, "esc")
	if m.viewExpanded() || h.Document == nil {
		t.Fatal("restore closed Helm values")
	}
	press(t, m, "esc")
	if h.Document != nil {
		t.Fatal("second Escape failed to close Helm values")
	}
}

func TestGlobalExpansionMainFramesFit(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 50}} {
		for mode := range modes {
			m := exploreModel(t)
			m.mode, m.width, m.height, m.status = mode, size[0], size[1], ""
			for _, expanded := range []bool{false, true} {
				if expanded {
					m.execute("f")
				}
				view := m.render()
				if len(strings.Split(view, "\n")) != m.height {
					t.Fatalf("%s expanded=%t: wrong screen height", modes[mode], expanded)
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) != m.width {
						t.Fatalf("%s expanded=%t: row width %d, expected %d", modes[mode], expanded, ansi.StringWidth(line), m.width)
					}
				}
				if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(dir, fmt.Sprintf("fullscreen-%s-%d-%t.ansi", modes[mode], m.width, expanded))
					if err := os.WriteFile(path, []byte(view), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}
