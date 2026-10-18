package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	"github.com/simjay/plous/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chatReadingModel(t *testing.T) *model {
	t.Helper()
	m, _ := testModel(t)
	m.focus = 2
	var body strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&body, "Reply line %02d\n", i)
	}
	m.chat().Turns = []history.Turn{{Request: history.Request{Prompt: "First question", Agent: ai.DefaultConfig()}, Response: history.Response{Text: body.String() + "Latest answer", Status: "completed"}}}
	m.chatTranscript(40, 15)
	return m
}

func TestChatReadingNavigationAndStreaming(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	if c.Reading || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "Latest answer") {
		t.Fatal("new conversation did not follow latest output")
	}
	last := c.Scroll
	press(t, m, "up")
	if !c.Reading || c.Scroll != last-1 {
		t.Fatal("Up did not start reading one row earlier")
	}
	m.chatTranscript(40, 15)
	typeText(m, "j")
	if c.Reading || c.Scroll != last {
		t.Fatal("j did not return to following latest output")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.chatTranscript(40, 15)
	if c.Scroll != last-(c.ViewHeight-1) {
		t.Fatal("Page Up did not move by the visible page")
	}
	before := ansi.Strip(m.chatTranscript(40, 15))
	run := &appchat.ChatRun{Chat: c, Events: make(chan appchat.ChatEvent)}
	m.run = run
	m.Update(appchat.ChatEvent{Run: run, Text: c.Turns[0].Response.Text + "\nA streamed continuation\nFinal streamed line"})
	after := ansi.Strip(m.chatTranscript(40, 15))
	// The position indicator changes its total; the visible conversation stays put.
	if strings.Join(strings.Split(before, "\n")[:14], "\n") != strings.Join(strings.Split(after, "\n")[:14], "\n") {
		t.Fatal("streaming moved the visible conversation while reading")
	}
	press(t, m, "home")
	if c.Scroll != 0 || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "First question") {
		t.Fatal("Home did not reveal the oldest message")
	}
	press(t, m, "up")
	if c.Scroll != 0 {
		t.Fatal("scroll moved before first row")
	}
	press(t, m, "end")
	if c.Reading || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "Final streamed line") {
		t.Fatal("End did not resume latest output")
	}
	m.Update(appchat.ChatEvent{Run: run, Text: c.Turns[0].Response.Text + "\nNew tail"})
	if !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "New tail") {
		t.Fatal("following did not track the next streamed chunk")
	}
}

func TestChatReadingResizeAndTyping(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	c.Turns[0].Response.Text = strings.Repeat("A long line with many words and 界 cells. ", 35) + "\nLatest answer"
	m.chatTranscript(40, 15)
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.chatTranscript(40, 15)
	anchor := c.Anchor
	m.chatTranscript(28, 18)
	if c.Anchor.Line != anchor.Line || c.Anchor.Column > anchor.Column || anchor.Column-c.Anchor.Column >= 28 {
		t.Fatalf("resize lost the logical text anchor: before=%+v after=%+v", anchor, c.Anchor)
	}
	m.chatEditing = true
	typeText(m, "jk / draft")
	position := c.Scroll
	press(t, m, "home")
	typeText(m, "x")
	press(t, m, "end")
	typeText(m, "y")
	if m.draft().Text != "xjk / drafty" || c.Scroll != position {
		t.Fatal("navigation keys captured literal typing or caret movement")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if c.Scroll >= position || m.draft().Text != "xjk / drafty" {
		t.Fatal("Page Up changed the draft or failed to scroll while typing")
	}
}

func TestChatStyledSearchPreservesTextAndPosition(t *testing.T) {
	m := chatReadingModel(t)
	m.chat().Turns[0].Request.Prompt = "界 café needle question"
	m.chat().Turns[0].Response.Text += "\n## Heading\n**needle** and `needle`\n```yaml\nname: needle\n```"
	m.chatTranscript(40, 15)
	typeText(m, "/needle")
	view := ansi.Strip(m.chatTranscript(40, 15))
	if !strings.Contains(view, "界 café needle question") || m.searches[2].total != 4 || strings.Contains(view, "[38;") {
		t.Fatalf("styled text was corrupted or omitted from search: %s", view)
	}
	press(t, m, "enter")
	typeText(m, "n")
	view = ansi.Strip(m.chatTranscript(40, 15))
	if !strings.Contains(view, "needle and needle") || strings.Contains(view, "[38;") {
		t.Fatalf("inline styles corrupted search: %s", view)
	}
	position := m.chat().Scroll
	press(t, m, "esc")
	m.chatTranscript(40, 15)
	if m.searches[2] != nil || m.chat().Scroll > position || !m.chat().Reading {
		t.Fatal("closing search lost the found position")
	}
}

func TestChatWordWrapFindsMatchesAcrossVisualRows(t *testing.T) {
	m := chatReadingModel(t)
	m.chat().Turns[0].Response.Text = "## Heading\n" + strings.Repeat("界 text ", 80) + "**distinct phrase** and `code value`"
	m.chatTranscript(19, 14)
	typeText(m, "/distinct phrase")
	view := ansi.Strip(m.chatTranscript(19, 14))
	if m.searches[2].total != 1 || !strings.Contains(view, "distinct") || !strings.Contains(view, "phrase") {
		t.Fatalf("wrapped search did not reveal the full match: %s", view)
	}
	rows := appchat.ChatRows("A sentence with words", 12)
	if len(rows) != 2 || rows[0].Text != "A sentence" || rows[1].Text != "with words" || rows[1].Start != 11 {
		t.Fatalf("word wrapping split a word or lost its source position: %+v", rows)
	}
}

func TestChatPresentationFitsAndShowsScope(t *testing.T) {
	m := chatReadingModel(t)
	createWorkspace(t, m, "Commerce")
	m.focus = 2
	m.status = ""
	m.settings.Agent = ai.Config{Name: "OpenAI", Provider: "openai", Model: "gpt-5.4-mini"}
	m.current().Connections = []workspace.Connection{{ID: "local", Tool: "kubernetes", Name: "minikube"}}
	m.selectScope(m.active, "kubernetes", "local")
	m.focus = 2
	m.chat().Turns = []history.Turn{{Request: history.Request{Prompt: "How can I inspect this deployment?", Agent: ai.DefaultConfig()}, Response: history.Response{Text: "## Check the rollout\nRun `kubectl rollout status deployment/frontend`.\n\n**Inspect the image** in your manifest:\n```yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n```\n\n> Review changes before applying them.\n\nUse the Events tab to investigate startup failures.", Status: "completed"}}}
	m.chat().Turns[0].Request.Agent = m.settings.Agent
	m.current().Connections = []workspace.Connection{{ID: "local", Tool: "kubernetes", Name: "minikube"}}

	for _, width := range []int{24, 40, 80} {
		view := ansi.Strip(m.chatPanelTitle(width) + "\n" + m.chatBody(width, 20))
		if !strings.Contains(view, "minikube") || strings.Contains(view, "Commerce") {
			t.Fatal("chat header did not show the selected connection", view)
		}
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.scrollChat("home")
		view := m.render()
		if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] {
			t.Fatalf("chat overflow at %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("chat-%d.ansi", size[0])), []byte(view), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	content := m.chatContent()
	for _, style := range []string{appchat.ChatYouStyle.Render("You"), ui.Muted.Render("Assistant")} {
		if !strings.Contains(content, style) {
			t.Fatal("message roles and heading lost their distinct styling")
		}
	}
}

func TestQuestionBubblesWrapAndPreserveLiteralText(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	c.Turns[0].Request.Prompt = "Short question"
	rows := c.ChatRows(m.chatContent(), 40)
	if !strings.HasSuffix(ansi.Strip(rows[1].Text), "╮") || !strings.Contains(ansi.Strip(rows[2].Text), "│ Short question │") || !strings.HasSuffix(ansi.Strip(rows[3].Text), "╯") {
		t.Fatalf("question is not enclosed: %+v", rows[:4])
	}
	if ansi.StringWidth(rows[1].Text) != 40 {
		t.Fatalf("short question bubble was not aligned to the right: width %d, %q", ansi.StringWidth(rows[1].Text), rows[1].Text)
	}
	for _, prompt := range []string{"", "One\n\nTwo", "**literal** `text`\n    indented", "界 café 👩‍💻 " + strings.Repeat("long-unbroken-text", 12)} {
		c.Turns[0].Request.Prompt = prompt
		for _, width := range []int{3, 6, 19, 24, 40, 80} {
			body := m.chatContent()
			rows := c.ChatRows(body, width)
			for _, row := range rows {
				if ansi.StringWidth(row.Text) > width {
					t.Fatalf("bubble overflow at width %d: %q", width, ansi.Strip(row.Text))
				}
			}
			if !strings.Contains(ansi.Strip(body), prompt) || c.Turns[0].Request.Prompt != prompt {
				t.Fatal("presentation changed the original question")
			}
		}
	}
}

func TestQuestionBubbleSearchResizeAndReplySelection(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	c.Turns[0].Request.Prompt = strings.Repeat("Earlier words ", 10) + "distinct phrase" + strings.Repeat(" later words", 20)
	c.Turns = append(c.Turns, history.Turn{Request: history.Request{Prompt: "Second question"}, Response: history.Response{Text: "Second answer", Status: "completed"}})
	m.chatTranscript(24, 16)
	typeText(m, "/distinct phrase")
	view := ansi.Strip(m.chatTranscript(24, 16))
	if m.searches[2].total != 1 || !strings.Contains(view, "distinct") || !strings.Contains(view, "phrase") || c.SelectedReply() != 0 {
		t.Fatalf("bubble search lost text or selected the wrong reply: %s", view)
	}
	press(t, m, "esc")
	m.chatTranscript(24, 16)
	anchor := c.Anchor
	m.chatTranscript(40, 16)
	if c.Anchor.Line != anchor.Line || c.Anchor.Column > anchor.Column || anchor.Column-c.Anchor.Column >= 30 {
		t.Fatalf("resizing a question lost its text anchor: %+v -> %+v", anchor, c.Anchor)
	}
	m.selectChatReply(1)
	view = ansi.Strip(m.chatTranscript(40, 16))
	if c.SelectedReply() != 1 || !strings.Contains(view, "Second answer") {
		t.Fatal("bubble height disrupted reply navigation", view)
	}
}
