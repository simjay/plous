package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
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

func TestMouseHelmCardIdentityAndSort(t *testing.T) {
	m, h := helmModel(t)
	m.width, m.height = 200, 50
	h.Releases = []helmtool.Release{{Name: "one", Namespace: "apps", Revision: 3, Chart: "demo", Version: "1.0.0"}, {Name: "two", Namespace: "other", Revision: 1, Chart: "demo", Version: "1.0.0"}}
	m.chooseHelmSection("Charts")
	clickMouse(t, m, "main", "helmrow/other/two")
	if h.Preference.Selected != "other/two" || h.Pending != "" {
		t.Fatal("card selection opened or lost identity")
	}
	clickMouse(t, m, "main", "helmrow/other/two")
	if h.Pending != "detail" {
		t.Fatal("card did not open selected release")
	}
	h.Pending = ""
	m.chooseHelmSection("Releases")
	clickMouse(t, m, "main", "sort/Name")
	if !h.Preference.Descending || h.Menu != nil {
		t.Fatal("sort heading did not apply sort")
	}
}

func TestMouseDiffDisabledAndStaleSaveStayGuarded(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	before := a.Document.Text()
	a.Document.Paste("# changed\n")
	a.Review = diffview.New(before, a.Document.Text(), a.Document.Target, a.Document.Revision, true)
	a.Review.ConfirmLabel, a.Review.ConfirmKey = "Save", ":w"
	a.Stage = "review"
	a.Review.CanConfirm = false
	m.activateMouse(textview.MouseSpan{Scope: "main", Target: "disabled"}, false)
	if a.Pending != "" || a.SaveRequested {
		t.Fatal("disabled save dispatched")
	}
	a.Review.CanConfirm = true
	a.Document.Paste("# changed after review\n")
	clickMouse(t, m, "main", "diffaction/1")
	if a.Pending != "" || a.Err == "" {
		t.Fatal("stale comparison saved")
	}
}

func TestMouseHelmDraftFormsPreserveValues(t *testing.T) {
	m, h := helmModel(t)
	m.width, m.height = 200, 50
	m.beginHelmDraft(h)
	f := h.Draft.Form
	var key string
	for _, field := range f.Fields {
		if len(field.Options) == 0 {
			key = field.Key
			break
		}
	}
	clickMouse(t, m, "main", "field/"+key)
	if h.Draft.Form != f || h.Pending != "" {
		t.Fatal("field click submitted workflow")
	}
	m.View()
	for _, span := range m.mouse.spans {
		if strings.HasPrefix(span.Target, "formaction/") {
			return
		}
	}
	t.Fatal("workflow actions lack mouse targets")
}

func visibleTextPoint(t *testing.T, m *model, text string) textview.Point {
	t.Helper()
	frame := ansi.Strip(m.View().Content)
	for row, line := range strings.Split(frame, "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return textview.Point{Row: row, Col: ansi.StringWidth(line[:at])}
		}
	}
	t.Fatalf("missing visible text %q:\n%s", text, frame)
	return textview.Point{}
}

func selectScreenText(t *testing.T, m *model, text string) {
	t.Helper()
	m.Update(nil)
	p := visibleTextPoint(t, m, text)
	m.Update(tea.MouseClickMsg{X: p.Col, Y: p.Row, Button: tea.MouseLeft})
	m.View()
	m.Update(tea.MouseMotionMsg{X: p.Col + ansi.StringWidth(text) - 1, Y: p.Row, Button: tea.MouseLeft})
	m.View()
	m.Update(tea.MouseReleaseMsg{X: p.Col + ansi.StringWidth(text) - 1, Y: p.Row, Button: tea.MouseLeft})
	m.View()
	if !m.screenSelectionActive() || m.mouse.screen.Text() != text {
		t.Fatalf("drag failed to select %q", text)
	}
}

func copyScreenButton(t *testing.T, m *model, want string) {
	t.Helper()
	got := ""
	m.clipboard.write = func(_ context.Context, text string) error { got = text; return nil }
	cmd := clickMouse(t, m, "footer", "clipboard/copy")
	if cmd == nil {
		t.Fatal("copy button did not request clipboard access")
	}
	m.Update(cmd())
	if got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

func TestScreenSelectionCopiesTablesWithoutActivatingRows(t *testing.T) {
	m := exploreModel(t)
	m.width, m.height = 200, 50
	selected := m.explore.Selected
	selectScreenText(t, m, "catalog")
	if m.explore.Selected != selected || m.explore.Detail != nil || m.focus != 1 {
		t.Fatal("drag activated a resource or moved focus")
	}
	copyScreenButton(t, m, "catalog")
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("selection disabled mouse controls")
	}
	press(t, m, "esc")
	if m.screenSelectionActive() || m.focus != 1 || m.explore.Detail != nil {
		t.Fatal("Escape did not clear only the screen selection")
	}
	_, rows := m.exploreRows()
	clickMouse(t, m, "main", "resource/"+mouseResourceID(rows[0].Object))
	if m.explore.Selected != string(rows[0].Object.GetUID()) {
		t.Fatal("row clicks stopped working after copying")
	}
}

func TestScreenSelectionCopiesHelpAndFormLabelsWithoutLosingDraft(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.beginWorkspace()
	m.form.Fields[0].Value = textview.NewField("unfinished workspace")
	form := m.form
	selectScreenText(t, m, "Workspace Name")
	copyScreenButton(t, m, "Workspace Name")
	if m.form != form || form.Fields[0].Value.Text != "unfinished workspace" || len(m.workspaces) != 0 {
		t.Fatal("label selection changed or submitted the form")
	}
	m.execute("help")
	selectScreenText(t, m, "Global · Navigation")
	copyScreenButton(t, m, "Global · Navigation")
	press(t, m, "esc")
	if m.help == nil || m.form != form {
		t.Fatal("clearing selection closed Help or discarded its form")
	}
	press(t, m, "esc")
	if m.help != nil || m.form != form {
		t.Fatal("Help did not restore the unfinished form")
	}
}

func TestScreenSelectionCopiesNativeOutputWithoutForwardingCopyOrEscape(t *testing.T) {
	m, n := nativeModel(t)
	_, _ = n.Screen.Write([]byte("native output 界cafe\u0301"))
	n.HasOutput.Store(true)
	selectScreenText(t, m, "native output 界cafe\u0301")
	copyScreenButton(t, m, "native output 界cafe\u0301")
	if got := copyWithShortcut(t, m, tea.ModSuper); got != "native output 界cafe\u0301" {
		t.Fatal(got)
	}
	// Exercise the application dispatcher, which normally forwards native keys.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModShift})
	if cmd == nil {
		t.Fatal("native capture intercepted the explicit copy shortcut")
	}
	m.Update(cmd())
	press(t, m, "esc")
	if !m.nativeAgentCaptured() || m.nativeAgent != n || n.Finished || m.screenSelectionActive() {
		t.Fatal("selection changed the native session or focus")
	}
	select {
	case input := <-n.Input.Chunks:
		t.Fatalf("selection leaked input into the native process: %q", input)
	default:
	}
}

func TestScreenSelectionCopiesTranscriptAndSidebarWithoutNavigation(t *testing.T) {
	m := chatReadingModel(t)
	m.width, m.height = 200, 50
	m.chatEditing = true
	*m.draft() = textview.NewField("unfinished chat draft")
	selectScreenText(t, m, "Latest answer")
	copyScreenButton(t, m, "Latest answer")
	if !m.chatEditing || m.draft().Text != "unfinished chat draft" || m.chatSources != nil || m.run != nil {
		t.Fatal("copying transcript changed chat ownership or sent a message")
	}
	selectScreenText(t, m, "Current Workspace")
	copyScreenButton(t, m, "Current Workspace")
	if m.focus != 2 || m.draft().Text != "unfinished chat draft" {
		t.Fatal("dragging sidebar text navigated away from chat")
	}
}

func TestScreenSelectionCopiesFooterErrors(t *testing.T) {
	m, _ := testModel(t)
	m.status = "A visible request error"
	selectScreenText(t, m, "A visible request error")
	copyScreenButton(t, m, "A visible request error")
	if m.screenSelectionActive() || m.status != "Copied to clipboard." {
		t.Fatal("footer copy did not release the selection and show its result")
	}
}

func TestScreenSelectionEscapeButtonPreservesUnderlyingFormAndHelp(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.beginWorkspace()
	form := m.form
	form.Fields[0].Value = textview.NewField("unfinished workspace")
	selectScreenText(t, m, "Workspace Name")
	clickMouse(t, m, "footer", "action/Esc")
	if m.screenSelectionActive() || m.form != form || form.Fields[0].Value.Text != "unfinished workspace" {
		t.Fatal("selection Escape button closed or changed the form")
	}
	m.openHelp()
	selectScreenText(t, m, "Global · Navigation")
	clickMouse(t, m, "footer", "action/Esc")
	if m.screenSelectionActive() || m.help == nil || m.form != form {
		t.Fatal("selection Escape button closed Help or its underlying form")
	}
}

func TestScreenSelectionInvalidatesOnInputLayoutAndSourceChanges(t *testing.T) {
	for _, action := range []string{"key", "paste", "resize", "blur", "focus", "source", "wheel"} {
		t.Run(action, func(t *testing.T) {
			m := exploreModel(t)
			m.width, m.height = 200, 50
			selectScreenText(t, m, "catalog")
			switch action {
			case "key":
				press(t, m, "down")
			case "paste":
				m.Update(tea.PasteMsg{Content: "literal text"})
			case "resize":
				m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			case "blur":
				m.Update(tea.BlurMsg{})
			case "focus":
				m.focus = 0
			case "source":
				for i := range m.explore.Collection.Objects {
					if m.explore.Collection.Objects[i].GetName() == "catalog" {
						m.explore.Collection.Objects[i].SetName("replacement")
					}
				}
				m.explore.Collection.Version = "replacement-version"
			case "wheel":
				p := visibleTextPoint(t, m, "catalog")
				m.Update(tea.MouseWheelMsg{X: p.Col, Y: p.Row, Button: tea.MouseWheelDown})
			}
			m.View()
			if m.screenSelectionActive() {
				t.Fatal("screen selection survived invalidating input")
			}
		})
	}
}
