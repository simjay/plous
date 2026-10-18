package app

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/simjay/plous/internal/app/monitor"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
	"time"
)

func mousePoint(t *testing.T, m *model, scope, target string) textview.MouseSpan {
	t.Helper()
	v := m.View()
	if strings.Contains(v.Content, "plous-mouse:") {
		t.Fatal("mouse metadata leaked to terminal output")
	}
	for _, span := range m.mouse.spans {
		if span.Scope == scope && span.Target == target {
			return span
		}
	}
	t.Fatalf("missing target %s/%s; spans: %+v", scope, target, m.mouse.spans)
	return textview.MouseSpan{}
}

func clickMouse(t *testing.T, m *model, scope, target string) tea.Cmd {
	t.Helper()
	p := mousePoint(t, m, scope, target)
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.View()
	return m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
}

func TestMouseTabsFocusAndLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, _ := testModel(t)
			m.width, m.height = size[0], size[1]
			if m.View().MouseMode != tea.MouseModeCellMotion {
				t.Fatal("mouse not enabled")
			}
			widths := m.panelWidths()
			for _, span := range m.mouse.spans {
				left, right := 0, widths[0]
				switch span.Scope {
				case "main":
					left, right = widths[0], widths[0]+widths[1]
				case "chat":
					left, right = widths[0]+widths[1], m.width
				case "footer":
					continue
				}
				if span.X < left || span.X+span.Width > right {
					t.Fatalf("target crossed panel boundary: %+v", span)
				}
			}
			clickMouse(t, m, "main", "command/2")
			if m.mode != 1 || m.focus != 1 {
				t.Fatal("tab did not activate")
			}
			clickMouse(t, m, "chat", "composer")
			if m.focus != 2 || !m.chatEditing {
				t.Fatal("composer did not focus")
			}
			clickMouse(t, m, "tree", "")
			if m.focus != 0 {
				t.Fatal("panel did not focus")
			}
			m.leftHidden = true
			m.View()
			for _, s := range m.mouse.spans {
				if s.Scope == "tree" {
					t.Fatal("hidden tree has targets")
				}
			}
		})
	}
}

func TestMouseOverlayAndDraftProtection(t *testing.T) {
	m, _ := testModel(t)
	m.beginWorkspace()
	m.form.Fields[0].Value = textview.NewField("unsaved workspace")
	form := m.form
	clickMouse(t, m, "main", "field/name")
	if m.form != form || len(m.workspaces) != 0 {
		t.Fatal("field click submitted form")
	}
	m.execute("help")
	m.View()
	for _, s := range m.mouse.spans {
		if s.Scope == "tree" || s.Target == "formaction/0" {
			t.Fatal("background targets escaped overlay")
		}
	}
	clickMouse(t, m, "footer", "action/Esc")
	if m.help != nil || m.form != form || m.form.Fields[0].Value.Text != "unsaved workspace" {
		t.Fatal("Help lost form")
	}
	clickMouse(t, m, "main", "formaction/0")
	if len(m.workspaces) != 1 || m.workspaces[0].Name != "unsaved workspace" {
		t.Fatal("explicit form save failed", m.status)
	}
}

func TestMouseResourceSelectionDoubleClickAndStaleGesture(t *testing.T) {
	m := exploreModel(t)
	m.width = 200
	now := time.Unix(1000, 0)
	m.mouse.now = func() time.Time { return now }
	_, rows := m.exploreRows()
	target := "resource/" + mouseResourceID(rows[0].Object)
	clickMouse(t, m, "main", target)
	if m.explore.Detail != nil {
		t.Fatal("single click opened row")
	}
	now = now.Add(100 * time.Millisecond)
	clickMouse(t, m, "main", target)
	if m.explore.Detail == nil || m.explore.Detail.GetUID() != rows[0].Object.GetUID() {
		t.Fatal("double click did not open exact resource")
	}
	m.backExploreLocation()
	p := mousePoint(t, m, "main", target)
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.explore.Collection.Objects = nil
	m.View()
	m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.explore.Detail != nil {
		t.Fatal("stale row gesture opened an item")
	}
}

func TestMouseDragResizeAndKeyboardCancelGestures(t *testing.T) {
	m, _ := testModel(t)
	m.mode = 0
	p := mousePoint(t, m, "main", "command/2")
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseMotionMsg{X: p.X + 2, Y: p.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.mode != 0 {
		t.Fatal("drag activated a tab")
	}
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	m.View()
	m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.mode != 0 {
		t.Fatal("resize activated stale tab")
	}
	if m.mouse.press != nil || m.mouse.last != nil {
		t.Fatal("resize retained gesture")
	}
}

func TestMouseSettingsPersistsAndKeepsKeyboardAvailable(t *testing.T) {
	m, _ := testModel(t)
	m.openSettings()
	m.selectSettingsItem("Mouse support: On")
	m.View()
	var target string
	for _, span := range m.mouse.spans {
		if strings.HasPrefix(span.Target, fmt.Sprintf("choice/%d/", m.cursor)) {
			target = span.Target
			break
		}
	}
	clickMouse(t, m, "main", target)
	if !m.settings.MouseDisabled || m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("mouse Off did not apply")
	}
	next, err := newModel(m.store, m.history, "test")
	if err != nil || !next.settings.MouseDisabled {
		t.Fatal("mouse preference did not survive restart", err)
	}
	m.updateSettings("enter")
	if m.settings.MouseDisabled {
		t.Fatal("keyboard could not turn mouse on")
	}
}

func TestMouseWheelPreservesChatDraftAndFocus(t *testing.T) {
	m := chatReadingModel(t)
	m.focus, m.chatEditing = 0, true
	m.draft().Text = "keep typing here"
	m.View()
	var p textview.MouseSpan
	for _, span := range m.mouse.spans {
		if span.Scope == "chat" {
			p = span
			break
		}
	}
	m.updateMouse(tea.MouseWheelMsg{X: p.X, Y: p.Y, Button: tea.MouseWheelUp})
	if m.focus != 0 || !m.chatEditing || m.draft().Text != "keep typing here" {
		t.Fatal("wheel changed typing ownership")
	}
}

func TestMouseDocumentSaveAndScrollPreserveDraft(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.Document.Paste("# draft\n")
	a.Document.Mode = docedit.Normal
	before, revision := a.Document.Text(), a.Document.Revision
	clickMouse(t, m, "main", "documentaction/1")
	if a.Pending != "validate" || !a.SaveRequested || a.SaveRevision != revision {
		t.Fatal("Save did not establish exact validation intent")
	}
	if a.Document.Text() != before {
		t.Fatal("Save replaced the draft")
	}
	a.Pending, a.SaveRequested = "", false
	a.Document.Mode = docedit.Insert
	m.View()
	var p textview.MouseSpan
	for _, s := range m.mouse.spans {
		if s.Scope == "main" && s.Target == "" {
			p = s
			break
		}
	}
	m.updateMouse(tea.MouseWheelMsg{X: p.X, Y: p.Y, Button: tea.MouseWheelDown})
	if a.Document.Text() != before || a.Document.Revision != revision || a.Document.Mode != docedit.Insert {
		t.Fatal("wheel changed editor state")
	}
}

func TestMouseCommandDraftResumesAfterContentClick(t *testing.T) {
	m, _ := testModel(t)
	m.beginCommand()
	m.command.Text = "settings"
	clickMouse(t, m, "main", "")
	if m.command == nil || m.commandFocused() || m.command.Text != "settings" {
		t.Fatal("content click discarded or kept typing into command")
	}
	clickMouse(t, m, "main", "command")
	if !m.commandFocused() || m.command.Text != "settings" {
		t.Fatal("command did not resume")
	}
	clickMouse(t, m, "footer", "action/Enter")
	if m.page != "settings" || m.command != nil {
		t.Fatal("command footer did not execute saved draft")
	}
}

func TestMouseSearchEscapeAndExpansion(t *testing.T) {
	m := exploreModel(t)
	m.width = 200
	m.status = ""
	clickMouse(t, m, "footer", "action/:f")
	if !m.viewExpanded() {
		t.Fatal("fullscreen footer did not expand main")
	}
	clickMouse(t, m, "footer", "action/Esc")
	if m.viewExpanded() {
		t.Fatal("footer did not restore layout")
	}
	m.focus = 0
	m.beginPanelSearch()
	m.searches[0].input.Text = "pod"
	clickMouse(t, m, "footer", "action/Esc")
	if m.searches[0] != nil || m.focus != 0 {
		t.Fatal("search footer did not close search first")
	}
}

func TestMouseDisabledAndCapturedIgnoreResidualInput(t *testing.T) {
	m := exploreModel(t)
	p := mousePoint(t, m, "main", "command/3")
	m.settings.MouseDisabled = true
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.mode != 1 {
		t.Fatal("disabled mouse dispatched")
	}
	m.settings.MouseDisabled = false
	m.explore.Terminal = &monitor.ResourceTerminal{Captured: true}
	m.updateMouse(tea.MouseClickMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	m.updateMouse(tea.MouseReleaseMsg{X: p.X, Y: p.Y, Button: tea.MouseLeft})
	if m.mode != 1 || m.mouse.press != nil {
		t.Fatal("captured mouse dispatched")
	}
	m.explore.Terminal = nil
}

func TestMouseKeyboardCancelsDoubleClick(t *testing.T) {
	m := exploreModel(t)
	m.width = 200
	_, rows := m.exploreRows()
	target := "resource/" + mouseResourceID(rows[0].Object)
	clickMouse(t, m, "main", target)
	m.Update(localMouseKey("down"))
	clickMouse(t, m, "main", target)
	if m.explore.Detail != nil {
		t.Fatal("keyboard did not cancel double click")
	}
}

func TestMouseFieldLabelAndOptionHaveSeparateIntent(t *testing.T) {
	m, _ := testModel(t)
	m.beginAppSettings()
	before := m.form.Fields[0].Value.Text
	clickMouse(t, m, "main", "field/provider")
	if m.form.Fields[0].Value.Text != before {
		t.Fatal("label changed option")
	}
	clickMouse(t, m, "main", "option/provider")
	if m.form.Fields[0].Value.Text == before {
		t.Fatal("option value did not advance")
	}
}

func TestMouseSourcesKeepsDraftAndRestoresReply(t *testing.T) {
	m := sourceChatModel(t, 2)
	m.width, m.height = 200, 50
	m.draft().Text = "preserved draft"
	turn := m.chat().Turns[0]
	runSourceCommand(t, m, clickMouse(t, m, "chat", "sources/"+turn.Request.ID))
	if m.chatSources == nil || !m.chatSources.List {
		t.Fatal("Sources did not open saved list")
	}
	var target string
	m.View()
	for _, s := range m.mouse.spans {
		if s.Scope == "sources" && strings.HasPrefix(s.Target, "choice/1/") {
			target = s.Target
			break
		}
	}
	clickMouse(t, m, "sources", target)
	if !m.chatSources.List || m.chatSources.Index != 1 {
		t.Fatal("source single click did not select")
	}
	clickMouse(t, m, "sources", target)
	if m.chatSources.List {
		t.Fatal("source double click did not open")
	}
	clickMouse(t, m, "footer", "action/Esc")
	clickMouse(t, m, "footer", "action/Esc")
	if m.chatSources != nil || m.draft().Text != "preserved draft" {
		t.Fatal("Sources lost return state")
	}
}

func TestMouseCancelSaveKeepsUnknownOutcomeAndDraft(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.Document.Paste("# keep me\n")
	before := a.Document.Text()
	cancelled := false
	a.Busy, a.RunningJob = true, "apply"
	a.Cancel = func() { cancelled = true }
	clickMouse(t, m, "footer", "action/Esc")
	if !cancelled || a.Busy || a.Document.Text() != before || a.DocumentErrorTitle != "Save result unknown" {
		t.Fatal("Cancel skipped the save outcome guard or lost the draft")
	}
}

func TestMouseSearchTargetsVisibleDiagnostic(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.ShowDiagnostic("validation failed\nfull diagnostic")
	a.Diagnostic.Query = "diagnostic"
	clickMouse(t, m, "main", "readersearch")
	if !a.Diagnostic.Searching || a.Document.Searching {
		t.Fatal("diagnostic search click reached the hidden document")
	}
}
