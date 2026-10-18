package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"

	"github.com/simjay/plous/internal/integrations/observation"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sourceChatModel(t *testing.T, counts ...int) *model {
	t.Helper()
	m, _ := testModel(t)
	m.focus = 2
	c := m.chat()
	var err error
	c.Conversation, err = m.history.NewConversation("", "Source review")
	if err != nil {
		t.Fatal(err)
	}
	for i, count := range counts {
		turn, err := m.history.StartTurn("", c.Conversation.ID, history.Request{Prompt: fmt.Sprintf("Question %d", i), Agent: ai.Config{Provider: "openai", Model: "gpt-5.4-mini", Name: "Captured vendor"}})
		if err != nil {
			t.Fatal(err)
		}
		turn.Response = history.Response{Text: fmt.Sprintf("Answer %d\nChecked observed-ns only.", i), Status: "completed", Usage: ai.Usage{Total: 8121}}
		for j := 0; j < count; j++ {
			result := observation.New("kubernetes/deployments.apps", "get", time.Now().UTC())
			result.Connection, result.Namespace = "recorded-cluster", "observed-ns"
			result.Facts["count"] = observation.Number(4)
			result.Complete()
			_, err := m.history.SaveObservation("", c.Conversation.ID, turn.Request.ID, "kubernetes_resources_read", json.RawMessage(`{"operation":"get","resource":"deployments","namespace":"observed-ns"}`), result)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := m.history.SaveResponse("", c.Conversation.ID, turn.Request.ID, turn.Response); err != nil {
			t.Fatal(err)
		}
		c.Turns = append(c.Turns, turn)
	}
	m.chatTranscript(44, 18)
	runSourceCommand(t, m, m.syncChatEvidence())
	return m
}

func runSourceCommand(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, child := range msg {
			runSourceCommand(t, m, child)
		}
	case appchat.ChatEvidenceResult, appchat.ChatSourceDocumentResult:
		m.Update(msg)
	}
}

func TestChatSourcesSelectionDetailsAndReturn(t *testing.T) {
	m := sourceChatModel(t, 1, 2)
	c := m.chat()
	*m.draft() = textview.NewField("Unsent draft")
	runSourceCommand(t, m, press(t, m, "enter"))
	if m.chatSources == nil || !m.chatSources.List || m.chatSources.Evidence.Turn.Request.ID != c.Turns[1].Request.ID {
		t.Fatal("Enter did not open the latest reply's source list")
	}
	press(t, m, "down")
	press(t, m, "enter")
	if m.chatSources.Index != 1 || m.chatSources.List {
		t.Fatal("source selection did not open its details")
	}
	view := ansi.Strip(m.chatSourcesBody(85, 22))
	for _, want := range []string{"recorded-cluster", "observed-ns", "Captured vendor", "8121 tokens", "View captured result"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	if strings.Contains(view, "other-default") {
		t.Fatal("details used the default namespace instead of the observed scope")
	}
	runSourceCommand(t, m, press(t, m, "enter"))
	if d := m.chatSources.Document; d == nil || !d.ReadOnly || !strings.Contains(d.Text(), "record:") || !strings.Contains(d.Text(), `"number": 4`) {
		t.Fatal("captured source did not open in read-only editor")
	}
	before := m.chatSources.Document.Text()
	typeText(m, "/")
	m.Update(tea.PasteMsg{Content: "count"})
	press(t, m, "enter")
	press(t, m, "esc")
	if m.chatSources.Document == nil || m.chatSources.Document.Text() != before {
		t.Fatal("search changed or closed the captured document")
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if m.chatSources == nil || !m.chatSources.List {
		t.Fatal("Escape did not return through source details to the list")
	}
	press(t, m, "esc")
	if m.chatSources != nil || m.draft().Text != "Unsent draft" || m.focus != 2 {
		t.Fatal("return lost the draft or chat focus")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	runSourceCommand(t, m, cmd)
	m.chatTranscript(44, 18)
	position := c.Scroll
	runSourceCommand(t, m, press(t, m, "enter"))
	if m.chatSources.List || m.chatSources.Evidence.Turn.Request.ID != c.Turns[0].Request.ID {
		t.Fatal("single source did not open directly for the older reply")
	}
	press(t, m, "esc")
	m.chatTranscript(44, 18)
	if c.Scroll != position || c.SelectedReply() != 0 {
		t.Fatal("return moved the selected reply or reading position")
	}
}

func TestChatSourcesTypingHelpAndSmallLayouts(t *testing.T) {
	m := sourceChatModel(t, 1)
	m.chatEditing = true
	typeText(m, "[] literal draft")
	if m.draft().Text != "[] literal draft" || m.chat().ReplySelected {
		t.Fatal("reply navigation captured typing")
	}
	press(t, m, "esc")
	press(t, m, "enter")
	s := m.chatSources
	for _, size := range [][2]int{{80, 16}, {120, 30}, {180, 48}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.render()
		if lipgloss.Width(view) != size[0] || lipgloss.Height(view) != size[1] || !strings.Contains(ansi.Strip(m.footer()), "Enter") {
			t.Fatalf("source overlay did not fit %v", size)
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("sources-%d.ansi", size[0])), []byte(view), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m.render()
	press(t, m, "end")
	if !strings.Contains(ansi.Strip(m.chatSourcesBody(72, 9)), "8121 tokens") {
		t.Fatal("small source details could not scroll to response metadata")
	}
	typeText(m, "hh")
	if m.help == nil {
		t.Fatal("Help did not open over Sources")
	}
	press(t, m, "tab")
	press(t, m, "esc")
	if m.chatSources != s || m.help != nil {
		t.Fatal("Help did not restore Sources")
	}
	typeText(m, ":")
	press(t, m, "tab")
	press(t, m, "esc")
	if m.chatSources != s || m.focus != 2 || m.command != nil {
		t.Fatal("command cancel did not return to Sources")
	}
	m.Update(tea.PasteMsg{Content: "cc:quit"})
	if m.draft().Text != "[] literal draft" {
		t.Fatal("source navigation paste changed the draft")
	}
	typeText(m, "cc")
	if m.chatSources != nil || !m.chatEditing || m.draft().Text != "[] literal draft" {
		t.Fatal("chat shortcut did not restore typing and its draft")
	}
}

func TestChatSourcesNoEvidenceAndStaleCompletion(t *testing.T) {
	m := sourceChatModel(t, 0)
	view := ansi.Strip(m.chatContent())
	if strings.Contains(view, "8121") || strings.Contains(view, "Captured vendor") || !strings.Contains(view, "[Enter] View reply details") || !strings.Contains(view, "Checked observed-ns only") {
		t.Fatal("chat retained routine metadata or hid scope limitations", view)
	}
	press(t, m, "enter")
	if m.chatSources == nil || strings.Contains(m.chatSourcesHint(), "View captured") {
		t.Fatal("reply without reads advertised a nonexistent source")
	}
	old := m.chatSources
	press(t, m, "esc")
	m.Update(appchat.ChatSourceDocumentResult{Sources: old, Text: "stale"})
	if m.chatSources != nil {
		t.Fatal("late load reopened a dismissed popup")
	}
	m.chat().Turns[0].Response.Status = "streaming"
	press(t, m, "enter")
	if m.chatSources != nil || !strings.Contains(m.status, "finishes") {
		t.Fatal("streaming reply opened incomplete sources")
	}
}

func TestChatSourcesResolveSavedMemoryWithoutRewritingHistory(t *testing.T) {
	m := sourceChatModel(t, 1)
	c := m.chat()
	ref := c.Evidence.Records[0].Ref
	turn, err := m.history.StartTurn("", c.Conversation.ID, history.Request{Prompt: "Read earlier evidence", Agent: ai.DefaultConfig()})
	if err != nil {
		t.Fatal(err)
	}
	receipt := observation.New("history.read.v1", "read", time.Now().UTC())
	receipt.Facts["source_1"] = observation.Text(ref)
	receipt.Complete()
	_, err = m.history.SaveObservation("", c.Conversation.ID, turn.Request.ID, "history", nil, receipt)
	if err != nil {
		t.Fatal(err)
	}
	turn.Response = history.Response{Status: "completed", Text: "Earlier observation"}
	if err := m.history.SaveResponse("", c.Conversation.ID, turn.Request.ID, turn.Response); err != nil {
		t.Fatal(err)
	}
	c.Turns = append(c.Turns, turn)
	c.Evidence = nil
	runSourceCommand(t, m, m.syncChatEvidence())
	press(t, m, "enter")
	press(t, m, "down")
	press(t, m, "enter")
	runSourceCommand(t, m, press(t, m, "enter"))
	d := m.chatSources.Document
	if d == nil || !strings.Contains(d.Text(), "Saved reference: "+ref) || !strings.Contains(d.Text(), "recorded-cluster") {
		t.Fatal("saved memory result did not resolve its original observation")
	}
	turns, err := m.history.Turns("", c.Conversation.ID)
	if err != nil || turns[0].Response.Text != c.Turns[0].Response.Text {
		t.Fatal("reading Sources changed saved Q&A", err)
	}
}

func TestChatSourcesScrollSelectsVisibleReplyAndPreservesStreamingAnchor(t *testing.T) {
	m := sourceChatModel(t, 1, 1)
	c := m.chat()
	for i := range c.Turns {
		c.Turns[i].Response.Text = strings.Repeat(fmt.Sprintf("Reply %d text\n", i), 40)
	}
	m.chatTranscript(40, 15)
	runSourceCommand(t, m, press(t, m, "home"))
	m.chatTranscript(40, 15)
	if c.SelectedReply() != 0 {
		t.Fatal("scrolling to an older reply left the newest reply selected")
	}
	before := c.Scroll
	press(t, m, "enter")
	c.Turns[1].Response.Text += "New streamed text\n"
	m.render()
	press(t, m, "esc")
	m.chatTranscript(40, 15)
	if c.Scroll != before || !c.Reading || c.SelectedReply() != 0 {
		t.Fatal("Sources lost the reading anchor during a new reply")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	runSourceCommand(t, m, cmd)
	m.chatTranscript(40, 15)
	if c.SelectedReply() != 1 || c.Scroll == before {
		t.Fatal("next reply did not bring its header into view")
	}
}

func TestChatSourcesFollowsAFoundReply(t *testing.T) {
	m := sourceChatModel(t, 1, 0)
	m.chat().Turns[1].Response.Text = strings.Repeat("Recent reply\n", 40)
	m.chatTranscript(40, 15)
	typeText(m, "/Answer 0")
	finishSavedFind(t, m)
	m.chatTranscript(40, 15)
	runSourceCommand(t, m, press(t, m, "enter"))
	press(t, m, "esc")
	runSourceCommand(t, m, press(t, m, "enter"))
	if m.chatSources == nil || m.chatSources.Evidence.Turn.Request.ID != m.chat().Turns[0].Request.ID || len(m.chatSources.Evidence.Records) != 1 {
		t.Fatal("Sources opened for a different reply than the search result")
	}
}
