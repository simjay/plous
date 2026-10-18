package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
	"time"
)

func textMousePoint(t *testing.T, m *model, scope, owner string, row, col int) tea.Mouse {
	t.Helper()
	m.View()
	for _, span := range m.mouse.spans {
		target, ok := textview.ParseTextTarget(span.Target)
		if !ok || span.Scope != scope || target.Owner != owner || target.Row != row {
			continue
		}
		lines := m.mouseTextSurface(scope, owner).lines()
		for x := span.X; x < span.X+span.Width; x++ {
			point, valid := textPoint(span, target, lines, x)
			if valid && point.Col == col {
				return tea.Mouse{X: x, Y: span.Y, Button: tea.MouseLeft}
			}
		}
	}
	t.Fatalf("missing text %s/%s at %d:%d", scope, owner, row, col)
	return tea.Mouse{}
}

func clickText(t *testing.T, m *model, scope, owner string, row, col int) {
	t.Helper()
	p := textMousePoint(t, m, scope, owner, row, col)
	m.updateMouse(tea.MouseClickMsg(p))
	m.View()
	m.updateMouse(tea.MouseReleaseMsg(p))
}

func TestMousePlacesFieldAndComposerCursorWithoutSubmitting(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.beginWorkspace()
	m.form.Fields[0].Value = textview.NewField("alpha 界 beta")
	clickText(t, m, "main", "field/name", 0, 6)
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if m.form.Fields[0].Value.Text != "alpha X界 beta" || len(m.workspaces) != 0 {
		t.Fatal("field cursor or submission changed")
	}
	m.cancelForm()
	m.focusChat(true)
	*m.draft() = textview.NewField("first line\nsecond 界 line")
	clickText(t, m, "chat", "composer", 0, 11)
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if m.draft().Text != "first line\nXsecond 界 line" || m.run != nil {
		t.Fatal("composer did not place cursor", m.draft().Text)
	}
}

func TestMouseDragAndWordSelectionInInputs(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.focusChat(true)
	*m.draft() = textview.NewField("one cafe\u0301 two")
	now := time.Unix(100, 0)
	m.mouse.now = func() time.Time { return now }
	clickText(t, m, "chat", "composer", 0, 5)
	now = now.Add(100 * time.Millisecond)
	clickText(t, m, "chat", "composer", 0, 5)
	if !m.draft().Selecting {
		t.Fatal("double click did not select word")
	}
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if m.draft().Text != "one X two" {
		t.Fatal("word replacement", m.draft().Text)
	}
	*m.draft() = textview.NewField("a界e\u0301z")
	start := textMousePoint(t, m, "chat", "composer", 0, 1)
	end := textMousePoint(t, m, "chat", "composer", 0, 2)
	m.updateMouse(tea.MouseClickMsg(start))
	m.updateMouse(tea.MouseMotionMsg(end))
	m.View()
	m.updateMouse(tea.MouseReleaseMsg(end))
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.draft().Text != "az" {
		t.Fatal("drag split Unicode", m.draft().Text)
	}
}

func TestMouseDocumentDragPreservesWritesAndCancelsStaleGestures(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.Document = docedit.New("first word\nsecond line\nthird")
	start := textMousePoint(t, m, "main", "document", 0, 6)
	end := textMousePoint(t, m, "main", "document", 1, 5)
	m.updateMouse(tea.MouseClickMsg(start))
	m.updateMouse(tea.MouseMotionMsg(end))
	m.View()
	m.updateMouse(tea.MouseReleaseMsg(end))
	if got := a.Document.SelectedText(); got != "word\nsecond" {
		t.Fatal(got)
	}
	if a.SaveRequested || a.Document.Dirty() {
		t.Fatal("selection supplied write intent")
	}
	before := a.Document.Text()
	m.updateMouse(tea.MouseClickMsg(start))
	m.Update(tea.WindowSizeMsg{Width: 210, Height: 50})
	m.View()
	m.updateMouse(tea.MouseReleaseMsg(end))
	if a.Document.Text() != before || m.mouse.text != nil {
		t.Fatal("resize retained gesture")
	}
	p := textMousePoint(t, m, "main", "document", 0, 0)
	m.updateMouse(tea.MouseClickMsg(p))
	a.Document.Paste("changed ")
	m.View()
	cursor := a.Document.Cursor
	m.updateMouse(tea.MouseReleaseMsg(p))
	if a.Document.Cursor != cursor {
		t.Fatal("stale source moved cursor")
	}
}

func TestMouseSelectionStopsAtPanelAndEscapePreservesDraft(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.focusChat(true)
	*m.draft() = textview.NewField(strings.Repeat("draft ", 8))
	start := textMousePoint(t, m, "chat", "composer", 0, 0)
	end := textMousePoint(t, m, "chat", "composer", 0, 4)
	m.updateMouse(tea.MouseClickMsg(start))
	m.updateMouse(tea.MouseMotionMsg(end))
	m.View()
	m.updateMouse(tea.MouseReleaseMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if !m.draft().Selecting || m.focus != 2 {
		t.Fatal("drag escaped its input owner")
	}
	before := m.draft().Text
	if !strings.Contains(m.footer(), "Clear selection") {
		t.Fatal("footer hid selection escape")
	}
	m.mouseControl("Esc")
	if m.draft().Selecting || !m.chatEditing || m.draft().Text != before {
		t.Fatal("Escape discarded draft or left typing before clearing selection")
	}
}

func TestMousePlacesSearchCursorInVisibleDocument(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.Document.Key("/", "")
	a.Document.Paste("original query")
	clickText(t, m, "main", "query", 0, 9)
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if a.Document.Query != "original Xquery" {
		t.Fatal(a.Document.Query)
	}
	if a.Document.Dirty() || a.SaveRequested {
		t.Fatal("search edited or saved the document")
	}
}

func TestMouseLogSelectionKeepsCollectionAndRejectsEvictedGesture(t *testing.T) {
	m := exploreModel(t)
	m.width, m.height = 200, 50
	m.openExploreSelection(4)
	m.explore.DetailValid = true
	m.explore.Log = &monitor.ExploreLogRun{Cancel: func() {}}
	viewer := m.podLogViewer()
	viewer.Append("first evidence\nsecond evidence\n")
	clickText(t, m, "main", "reader", 0, 6)
	if viewer.Following || m.explore.Log == nil {
		t.Fatal("click did not enter reading while keeping collection")
	}
	start := textMousePoint(t, m, "main", "reader", 0, 0)
	end := textMousePoint(t, m, "main", "reader", 1, 5)
	m.updateMouse(tea.MouseClickMsg(start))
	m.updateMouse(tea.MouseMotionMsg(end))
	m.View()
	m.updateMouse(tea.MouseReleaseMsg(end))
	if viewer.Reader.SelectedText() != "first evidence\nsecond" {
		t.Fatal(viewer.Reader.SelectedText())
	}
	m.updateMouse(tea.MouseClickMsg(start))
	viewer.Reader.Shift(1)
	m.View()
	cursor := viewer.Reader.Cursor
	m.updateMouse(tea.MouseReleaseMsg(end))
	if viewer.Reader.Cursor != cursor || viewer.Reader.Selecting {
		t.Fatal("evicted gesture changed the retained selection")
	}
}

func TestMouseComparisonSelectsTextWithoutConfirming(t *testing.T) {
	m, a := documentModel(t)
	m.width, m.height = 200, 50
	a.Stage = "review"
	a.Review = diffview.New("before line\n", "after line\n", "test target", 1, true)
	a.Review.Actions = true
	m.View()
	row := -1
	for i, line := range a.Review.Reader.Lines {
		if line.Text == "after line" {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("missing diff line")
	}
	clickText(t, m, "main", "reader", row, 2)
	if a.Review.Actions || a.Review.Reader.Cursor != (textview.Point{Row: row, Col: 2}) || a.SaveRequested {
		t.Fatal("comparison click supplied Save intent")
	}
}

func TestMouseSavedSourceDocumentOwnsSelection(t *testing.T) {
	m := sourceChatModel(t, 1)
	m.width, m.height = 200, 50
	m.openChatSources()
	m.chatSources.Document = docedit.New("saved evidence\nmore evidence")
	m.chatSources.Document.ReadOnly = true
	clickText(t, m, "sources", "document", 0, 7)
	if m.chatSources.Document.Cursor != 7 || m.focus != 2 {
		t.Fatal("Sources routed to main")
	}
	m.execute("help")
	m.View()
	for _, span := range m.mouse.spans {
		if strings.HasPrefix(span.Target, "text/") {
			t.Fatal("text target leaked through Help")
		}
	}
}

func TestMouseComposerTerminalGeometry(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 120, 30
	m.focusChat(true)
	*m.draft() = textview.NewField("alpha beta")
	p := textMousePoint(t, m, "chat", "composer", 0, 2)
	if p.X != 90 || p.Y != 24 {
		t.Fatalf("terminal fixture coordinates moved: %+v", p)
	}
}

func TestMouseMaskedFieldRejectsReplacedValue(t *testing.T) {
	m, _ := testModel(t)
	m.width, m.height = 200, 50
	m.beginWorkspace()
	f := &m.form.Fields[0]
	f.Secret = true
	f.Value = textview.NewField("first secret")
	p := textMousePoint(t, m, "main", "secret/name", 0, 3)
	m.updateMouse(tea.MouseClickMsg(p))
	f.Value = textview.NewField("other secret")
	view := m.View()
	if strings.Contains(view.Content, "other secret") {
		t.Fatal("masked field exposed its value")
	}
	m.updateMouse(tea.MouseReleaseMsg(p))
	if f.Value.Cursor != len([]rune(f.Value.Text)) {
		t.Fatal("stale masked field click changed the new value's cursor")
	}
}
