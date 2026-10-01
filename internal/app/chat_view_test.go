package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai"
	"github.com/simjay/plous/internal/workspace"
)

func chatReadingModel(t *testing.T) *model {
	t.Helper()
	m, _ := testModel(t)
	m.focus = 2
	var body strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&body, "Reply line %02d\n", i)
	}
	m.chat().turns = []workspace.Turn{{Request: workspace.Request{Prompt: "First question", Agent: ai.DefaultConfig()}, Response: workspace.Response{Text: body.String() + "Latest answer", Status: "completed"}}}
	m.chatTranscript(40, 15)
	return m
}

func TestChatReadingNavigationAndStreaming(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	if c.reading || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "Latest answer") {
		t.Fatal("new conversation did not follow latest output")
	}
	last := c.scroll
	press(t, m, "up")
	if !c.reading || c.scroll != last-1 {
		t.Fatal("Up did not start reading one row earlier")
	}
	m.chatTranscript(40, 15)
	typeText(m, "j")
	if c.reading || c.scroll != last {
		t.Fatal("j did not return to following latest output")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.chatTranscript(40, 15)
	if c.scroll != last-(c.viewHeight-1) {
		t.Fatal("Page Up did not move by the visible page")
	}
	before := ansi.Strip(m.chatTranscript(40, 15))
	run := &chatRun{chat: c, events: make(chan chatEvent)}
	m.run = run
	m.Update(chatEvent{run: run, text: c.turns[0].Response.Text + "\nA streamed continuation\nFinal streamed line"})
	after := ansi.Strip(m.chatTranscript(40, 15))
	// The position indicator changes its total; the visible conversation stays put.
	if strings.Join(strings.Split(before, "\n")[:14], "\n") != strings.Join(strings.Split(after, "\n")[:14], "\n") {
		t.Fatal("streaming moved the visible conversation while reading")
	}
	press(t, m, "home")
	if c.scroll != 0 || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "First question") {
		t.Fatal("Home did not reveal the oldest message")
	}
	press(t, m, "up")
	if c.scroll != 0 {
		t.Fatal("scroll moved before first row")
	}
	press(t, m, "end")
	if c.reading || !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "Final streamed line") {
		t.Fatal("End did not resume latest output")
	}
	m.Update(chatEvent{run: run, text: c.turns[0].Response.Text + "\nNew tail"})
	if !strings.Contains(ansi.Strip(m.chatTranscript(40, 15)), "New tail") {
		t.Fatal("following did not track the next streamed chunk")
	}
}

func TestChatReadingResizeAndTyping(t *testing.T) {
	m := chatReadingModel(t)
	c := m.chat()
	c.turns[0].Response.Text = strings.Repeat("A long line with many words and 界 cells. ", 35) + "\nLatest answer"
	m.chatTranscript(40, 15)
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.chatTranscript(40, 15)
	anchor := c.anchor
	m.chatTranscript(28, 18)
	if c.anchor.line != anchor.line || c.anchor.column > anchor.column || anchor.column-c.anchor.column >= 28 {
		t.Fatalf("resize lost the logical text anchor: before=%+v after=%+v", anchor, c.anchor)
	}
	m.chatEditing = true
	typeText(m, "jk / draft")
	position := c.scroll
	press(t, m, "home")
	typeText(m, "x")
	press(t, m, "end")
	typeText(m, "y")
	if m.draft().text != "xjk / drafty" || c.scroll != position {
		t.Fatal("navigation keys captured literal typing or caret movement")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if c.scroll >= position || m.draft().text != "xjk / drafty" {
		t.Fatal("Page Up changed the draft or failed to scroll while typing")
	}
}

func TestChatStyledSearchPreservesTextAndPosition(t *testing.T) {
	m := chatReadingModel(t)
	m.chat().turns[0].Request.Prompt = "界 café needle question"
	m.chat().turns[0].Response.Text += "\n## Heading\n**needle** and `needle`\n```yaml\nname: needle\n```"
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
	position := m.chat().scroll
	press(t, m, "esc")
	m.chatTranscript(40, 15)
	if m.searches[2] != nil || m.chat().scroll > position || !m.chat().reading {
		t.Fatal("closing search lost the found position")
	}
}

func TestChatWordWrapFindsMatchesAcrossVisualRows(t *testing.T) {
	m := chatReadingModel(t)
	m.chat().turns[0].Response.Text = "## Heading\n" + strings.Repeat("界 text ", 80) + "**distinct phrase** and `code value`"
	m.chatTranscript(19, 14)
	typeText(m, "/distinct phrase")
	view := ansi.Strip(m.chatTranscript(19, 14))
	if m.searches[2].total != 1 || !strings.Contains(view, "distinct") || !strings.Contains(view, "phrase") {
		t.Fatalf("wrapped search did not reveal the full match: %s", view)
	}
	rows := chatRows("A sentence with words", 12)
	if len(rows) != 2 || rows[0].text != "A sentence" || rows[1].text != "with words" || rows[1].start != 11 {
		t.Fatalf("word wrapping split a word or lost its source position: %+v", rows)
	}
}

func TestChatPresentationFitsAndShowsScope(t *testing.T) {
	m := chatReadingModel(t)
	createWorkspace(t, m, "Commerce")
	m.focus = 2
	m.status = ""
	m.settings.Agent = ai.Config{Name: "OpenAI", Provider: "openai", Model: "gpt-5.4-mini"}
	m.chat().turns = []workspace.Turn{{Request: workspace.Request{Prompt: "How can I inspect this deployment?", Agent: ai.DefaultConfig()}, Response: workspace.Response{Text: "## Check the rollout\nRun `kubectl rollout status deployment/frontend`.\n\n**Inspect the image** in your manifest:\n```yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n```\n\n> Review changes before applying them.\n\nUse the Events tab to investigate startup failures.", Status: "completed"}}}
	m.chat().turns[0].Request.Agent = m.settings.Agent
	m.current().Connections = []workspace.Connection{{ID: "local", Tool: "kubernetes", Name: "minikube"}}
	m.tool, m.connection = "kubernetes", "local"
	for _, width := range []int{24, 40, 80} {
		view := ansi.Strip(m.chatTranscript(width, 20))
		if !strings.Contains(view, "Commerce") || !strings.Contains(view, "Kubernetes · minikube") || strings.Contains(view, "shared") {
			t.Fatal("chat header did not show its selected scope", view)
		}
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {160, 40}, {200, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
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
	for _, style := range []string{chatYouStyle.Render("You"), chatAssistantStyle.Render("Assistant"), chatAssistantStyle.Render("Check the rollout")} {
		if !strings.Contains(content, style) {
			t.Fatal("message roles and heading lost their distinct styling")
		}
	}
}
