package diff

import (
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func TestThemeChangesPreserveComparisonAndReaderState(t *testing.T) {
	m := New("old\nunchanged\n", "new\nunchanged\n", "fixture", 9, true)
	m.View(90, 20, textview.DefaultTheme())
	m.Key("/", "")
	m.Reader.Paste("new")
	m.Key("enter", "")
	m.Key("v", "")
	id, cursor, anchor, count := m.ID, m.Reader.Cursor, m.Reader.Anchor, len(m.hunks)
	theme := textview.DefaultTheme()
	theme.Added = lipgloss.NewStyle().Foreground(lipgloss.Color("#123456"))
	theme.Removed = lipgloss.NewStyle().Foreground(lipgloss.Color("#654321"))
	theme.Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#ABCDEF"))
	for range 3 {
		m.View(90, 20, theme)
	}
	for i, kind := range m.kinds {
		line := m.Reader.Lines[i]
		if kind == '+' && line.Styled != theme.Added.Render(line.Text) || kind == '-' && line.Styled != theme.Removed.Render(line.Text) {
			t.Fatal("comparison text retained a different theme")
		}
	}
	if m.ID != id || m.Added != 1 || m.Removed != 1 || len(m.hunks) != count || m.Reader.Cursor != cursor || m.Reader.Anchor != anchor || !m.Reader.Selecting || m.Reader.Query != "new" {
		t.Fatal("theme change rebuilt the comparison or lost reading state")
	}
	if m.Reader.SelectedText() != "n" || m.Confirm().ID != id {
		t.Fatal("presentation changed copy text or confirmation identity")
	}
}

func TestComparisonIsIndependentAndConfirmationExplicit(t *testing.T) {
	m := New("before\n", "after\n", "file.txt", 7, true)
	if m.Added != 1 || m.Removed != 1 {
		t.Fatal("change counts")
	}
	decision, _ := m.Key("enter", "")
	if decision.Action != "" {
		t.Fatal("navigation enter confirmed")
	}
	m.Key("a", "")
	m.Key("right", "")
	decision, _ = m.Key("enter", "")
	if decision.Action != "confirm" || decision.ID != m.ID {
		t.Fatal("explicit decision lost identity")
	}
	m.Invalidate()
	if m.Confirm().Action != "" {
		t.Fatal("stale confirmed")
	}
	if New("before\n", "after\n", "file.txt", 8, true).ID == m.ID {
		t.Fatal("revision not captured")
	}
}

func TestBackKeyVisibleInReadingAndActionsWithoutConfirming(t *testing.T) {
	for _, focused := range []bool{false, true} {
		m := New("before\n", "after\n", "fixture", 1, true)
		m.BackLabel = "Back to editor"
		m.Actions = focused
		view := ansi.Strip(m.View(80, 24, textview.DefaultTheme()))
		if !strings.Contains(view, "[Esc] Back to editor") {
			t.Fatalf("missing return shortcut with actions=%v: %s", focused, view)
		}
		m.Key("/", "/")
		// Search stays local when the comparison has reading focus.
		if !focused {
			m.Key("a", "a")
			m.Key("enter", "")
			decision, _ := m.Key("esc", "")
			if decision.Action != "" || m.Reader.Query != "" {
				t.Fatal("Esc left comparison before clearing search")
			}
		}
		decision, _ := m.Key("esc", "")
		if decision.Action != "back" || decision.ID != m.ID {
			t.Fatal("Esc did not return without applying")
		}
	}
}
func TestWhitespaceFinalNewlineAndReadonly(t *testing.T) {
	m := New("same\n", "same", "test", 1, false)
	m.ConfirmLabel = ""
	if m.Added != 1 || m.Removed != 1 {
		t.Fatal("final newline difference lost")
	}
	view := m.View(90, 20, textview.DefaultTheme())
	if !strings.Contains(view, "No newline at end of file") || strings.Contains(view, "unavailable") {
		t.Fatal("readonly or newline marker wrong")
	}
	if m.Confirm().Action != "" {
		t.Fatal("readonly confirmed")
	}
}
func TestCopyExcludesGuttersAndMetadataAndErrorsRemainReadable(t *testing.T) {
	m := New("old\n", "new\n", "test", 1, true)
	m.Reader.Cursor = textview.Point{Row: 0}
	m.Reader.Selecting = true
	m.Reader.Anchor = textview.Point{Row: 0}
	m.Reader.Cursor = textview.Point{Row: 2, Col: 2}
	if got := m.Reader.SelectedText(); got != "old\nnew" {
		t.Fatalf("copy: %q", got)
	}
	message := strings.Repeat("cause\n", 100) + "last cause"
	m.SetError(message)
	m.Key("end", "")
	if !strings.Contains(ansi.Strip(m.View(90, 20, textview.DefaultTheme())), "last cause") {
		t.Fatal("full error inaccessible")
	}
	m.Key("esc", "")
	if m.ErrorActive {
		t.Fatal("error did not return to comparison")
	}
}

func TestSearchExclamationDoesNotReopenError(t *testing.T) {
	m := New("old!\n", "new!\n", "test", 1, true)
	m.SetError("operation failed")
	m.Key("esc", "")
	m.Key("/", "/")
	m.Key("!", "!")
	if m.ErrorActive || m.Reader.Query != "!" {
		t.Fatal("literal search text reopened operation error")
	}
	m.Key("enter", "")
	m.Key("!", "!")
	if !m.ErrorActive {
		t.Fatal("navigation ! no longer opens error")
	}
}
