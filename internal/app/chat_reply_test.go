package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func TestReplyReaderPreservesDraftSelectionAndSources(t *testing.T) {
	m := sourceChatModel(t, 1, 2)
	c := m.chat()
	c.Turns[0].Response.Text = "## Recorded answer\n\nExact `value` in observed-ns."
	m.selectChatReply(-1)
	m.render()
	before, scroll := c.Turns[0].Response.Text, c.Scroll
	*m.draft() = textview.NewField("unsent /model")
	m.execute("A")
	s := m.chatSources
	if s == nil || !s.ShowReply || s.Evidence.Turn.Request.ID != c.Turns[0].Request.ID {
		t.Fatal("reader did not bind the selected reply")
	}
	runSourceCommand(t, m, m.syncChatEvidence())
	if s.Evidence != c.Evidence || !s.Evidence.Loaded || len(s.Evidence.Records) != 1 {
		t.Fatal("reader did not load the selected turn's saved sources")
	}
	m.render()
	_, copied := s.Reply.Key("Y", "")
	if copied != c.Turns[0].Response.Text {
		t.Fatal("source copy changed the stored answer", copied)
	}
	typeText(m, "/")
	m.Update(tea.PasteMsg{Content: "value"})
	press(t, m, "enter")
	if s.Reply.Reader.Query != "value" || s.Reply.Reader.Searching || !s.ShowReply {
		t.Fatal("search input opened Sources or lost its query")
	}
	press(t, m, "tab")
	if m.focus != 2 || !s.ShowReply {
		t.Fatal("Tab escaped the reader")
	}
	press(t, m, "enter")
	if s.ShowReply || !strings.Contains(ansi.Strip(m.render()), "observed-ns") {
		t.Fatal("Sources did not open from the reader")
	}
	press(t, m, "esc")
	if !s.ShowReply || s.Reply.Reader.Query != "value" {
		t.Fatal("Sources did not restore the reader")
	}
	press(t, m, "esc") // Clear the retained search first.
	if m.chatSources == nil {
		t.Fatal("Esc skipped the reader's local search")
	}
	press(t, m, "esc")
	if m.chatSources != nil || m.draft().Text != "unsent /model" || c.SelectedReply() != 0 || c.Scroll != scroll || c.Turns[0].Response.Text != before {
		t.Fatal("return lost transcript state or draft")
	}
}

func TestReplyReaderHelpChatAndLiteralSearch(t *testing.T) {
	m := sourceChatModel(t, 0)
	m.execute("A")
	s := m.chatSources
	m.render()
	typeText(m, "/")
	typeText(m, ":ccRY")
	if s.Reply.Reader.Query != ":ccRY" || m.command != nil || s.Reply.Source {
		t.Fatal("reader search dispatched application or viewer keys")
	}
	press(t, m, "esc")
	typeText(m, "hh")
	if m.help == nil || m.chatSources != s {
		t.Fatal("Help lost the reader")
	}
	press(t, m, "esc")
	if m.help != nil || m.chatSources != s {
		t.Fatal("Help did not restore the reader")
	}
	typeText(m, "cc")
	if m.chatSources != nil || !m.chatEditing {
		t.Fatal("chat shortcut did not leave the reader")
	}
}

func TestChatComposerGrowsAndKeepsProgressVisible(t *testing.T) {
	m := chatReadingModel(t)
	m.chatEditing = true
	empty := ansi.Strip(m.chatBody(36, 28))
	if strings.Contains(empty, "[38;") || !strings.Contains(empty, "Ask about General chat") {
		t.Fatal("empty composer leaked terminal styling", empty)
	}
	*m.draft() = textview.NewField("one line")
	short := ansi.Strip(m.chatBody(36, 28))
	*m.draft() = textview.NewField("first\nsecond\nthird")
	long := ansi.Strip(m.chatBody(36, 28))
	boxTop := func(view string) int {
		for i, row := range strings.Split(view, "\n") {
			if strings.HasPrefix(row, "╭") {
				return i
			}
		}
		return -1
	}
	if boxTop(short)-boxTop(long) != 2 {
		t.Fatal("composer did not grow by the draft's two extra rows", short, long)
	}
	c := m.chat()
	c.Progress = "Reading Helm / production"
	m.run = &appchat.ChatRun{Chat: c}
	m.scrollChat("home")
	view := ansi.Strip(m.chatBody(36, 28))
	if !strings.Contains(view, "[:X] Stop") || !strings.Contains(view, "Reading Helm") || c.Scroll != 0 || m.draft().Text != "first\nsecond\nthird" {
		t.Fatal("progress disappeared or moved the transcript", view)
	}
}

func TestChatControlsAndReplyReaderMouse(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 50}} {
		m, _ := testModel(t)
		m.width, m.height = size[0], size[1]
		m.focus = 2
		if !strings.Contains(ansi.Strip(m.footer()), "cc Write message") {
			t.Fatal("chat navigation hid the write action")
		}
		mousePoint(t, m, "chat", "action/:H")
		mousePoint(t, m, "chat", "action/:C")
	}
	m := sourceChatModel(t, 1)
	m.width, m.height = 160, 40
	*m.draft() = textview.NewField("preserved draft")
	turn := m.chat().Turns[0]
	clickMouse(t, m, "chat", "readreply/"+turn.Request.ID)
	if m.chatSources == nil || !m.chatSources.ShowReply {
		t.Fatal("Read reply click did not open the reader")
	}
	clickMouse(t, m, "sources", "action/Enter")
	if m.chatSources.ShowReply {
		t.Fatal("reader Sources action did not open saved evidence")
	}
	press(t, m, "esc")
	clickMouse(t, m, "sources", "action/Esc")
	if m.chatSources != nil || m.draft().Text != "preserved draft" {
		t.Fatal("reader close lost the draft")
	}
}
