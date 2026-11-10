package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/integrations/observation"
	textview "github.com/simjay/plous/internal/ui/text"
)

func savedHistoryModel(t *testing.T, count int) (*model, history.Conversation) {
	t.Helper()
	m, _ := testModel(t)
	conversation, err := m.history.NewConversation("", "Long conversation")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: ai.DefaultConfig(), Prompt: fmt.Sprintf("Question %03d", i)})
		if err != nil {
			t.Fatal(err)
		}
		turn.Response = history.Response{Status: "completed", Text: fmt.Sprintf("Saved answer %03d", i)}
		if i == 3 || i == count-3 {
			turn.Response.Text += " distant marker"
		}
		if err := m.history.SaveResponse("", conversation.ID, turn.Request.ID, turn.Response); err != nil {
			t.Fatal(err)
		}
	}
	m.openConversation(m.chat(), conversation)
	finishChatRestore(t, m)
	if m.chat().Err != "" {
		t.Fatal(m.chat().Err)
	}
	m.focus = 2
	return m, conversation
}

func TestHistoryWindowEvictsMessagesWithoutChangingActiveContext(t *testing.T) {
	m, _ := savedHistoryModel(t, 120)
	c := m.chat()
	if len(c.Turns) > appchat.ChatCacheTurns || !c.Older || c.Newer {
		t.Fatalf("unexpected recent window: %d %v %v", len(c.Turns), c.Older, c.Newer)
	}
	messages, _ := m.chatMessages("Next question", false)
	*m.draft() = textview.NewField("Unsent draft")
	m.chatTranscript(40, 15)
	m.scrollChat("home")
	if c.Turns[0].Request.Prompt != "Question 000" || !c.Newer || c.Older {
		t.Fatal("Home did not open the oldest saved messages")
	}
	for i := 0; i < 10 && c.Newer; i++ {
		m.chatPage(1)
		if len(c.Turns) > appchat.ChatCacheTurns {
			t.Fatal("visited pages accumulated in memory")
		}
	}
	after, _ := m.chatMessages("Next question", false)
	if fmt.Sprint(messages) != fmt.Sprint(after) || m.draft().Text != "Unsent draft" {
		t.Fatal("transcript navigation changed model context or draft")
	}
	m.scrollChat("home")
	m.scrollChat("end")
	if c.Newer || c.Reading || c.Turns[len(c.Turns)-1].Request.Prompt != "Question 119" {
		t.Fatal("End did not return to latest messages")
	}
}

func finishSavedFind(t *testing.T, m *model) {
	t.Helper()
	for i := 0; i < 100; i++ {
		cmd := m.syncChatFind()
		if cmd == nil {
			if s := m.searches[2]; s != nil && s.archive != nil && s.archive.pending {
				m.updateChatFind(<-s.archive.result)
				continue
			}
			return
		}
		m.updateChatFind(cmd().(chatFindResult))
	}
	t.Fatal("saved find failed to finish within bounded batches")
}

func TestChatFindReadsUnloadedMessagesAndCancelsOldQueries(t *testing.T) {
	m, _ := savedHistoryModel(t, 120)
	*m.draft() = textview.NewField("Keep this draft")
	m.beginPanelSearch()
	s := m.searches[2]
	s.input = textview.NewField("distant marker")
	m.searchChanged()
	old := m.syncChatFind()
	s.input = textview.NewField("Question 004")
	m.searchChanged()
	m.updateChatFind(old().(chatFindResult))
	if m.chat().Turns[0].Request.Prompt == "Question 003" {
		t.Fatal("cancelled query replaced the transcript")
	}
	finishSavedFind(t, m)
	if len(m.chat().Turns) != 1 || m.chat().Turns[0].Request.Prompt != "Question 004" {
		t.Fatal("find ignored unloaded saved messages")
	}
	s.input = textview.NewField("distant marker")
	m.searchChanged()
	finishSavedFind(t, m)
	m.chatTranscript(40, 15)
	if m.chat().Turns[0].Request.Prompt != "Question 003" {
		t.Fatal("first match is not oldest matching exchange")
	}
	m.stepChatFind(s, 1)
	finishSavedFind(t, m)
	m.chatTranscript(40, 15)
	if m.chat().Turns[0].Request.Prompt != "Question 117" {
		t.Fatal("next match failed to cross unloaded history")
	}
	m.stepChatFind(s, -1)
	finishSavedFind(t, m)
	if m.chat().Turns[0].Request.Prompt != "Question 003" || m.draft().Text != "Keep this draft" {
		t.Fatal("backward find lost saved match or draft")
	}
}

func TestStopKeepsRunUntilProducerFinishedAndSavesBufferedText(t *testing.T) {
	m, _ := testModel(t)
	conversation, err := m.history.NewConversation("", "Stopping")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: ai.DefaultConfig(), Prompt: "Question"})
	if err != nil {
		t.Fatal(err)
	}
	c := m.chat()
	c.Conversation, c.Turns, c.ActiveTurn = conversation, []history.Turn{turn}, &turn
	ctx, cancel := context.WithCancel(context.Background())
	run := &appchat.ChatRun{Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent, 2)}
	m.run = run
	m.updateChatEvent(appchat.ChatEvent{Run: run, Text: "Partial"})
	m.stopChat()
	if m.run != run || !run.Stopping || ctx.Err() == nil {
		t.Fatal("Stop released an unfinished producer")
	}
	*m.draft() = textview.NewField("Next question")
	m.sendChat()
	if !strings.Contains(m.status, "running") || m.draft().Text != "Next question" {
		t.Fatal("new response started before previous producer finished")
	}
	run.Events <- appchat.ChatEvent{Run: run, Text: "Partial buffered"}
	run.Terminal = appchat.ChatEvent{Run: run, Done: true, Err: context.Canceled}
	close(run.Events)
	m.drainChat()
	if m.run != nil || c.ActiveTurn != nil {
		t.Fatal("finished producer remained active")
	}
	page, err := m.history.TurnPage(context.Background(), "", conversation.ID, 0, 1, appchat.ChatCacheBytes)
	if err != nil || len(page.Turns) != 1 || page.Turns[0].Response.Status != "cancelled" || page.Turns[0].Response.Text != "Partial buffered" {
		t.Fatalf("cancelled output was lost: %+v %v", page, err)
	}
}

func TestRunningResponseDoesNotOverwriteOlderTranscriptWindow(t *testing.T) {
	m, conversation := savedHistoryModel(t, 60)
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: ai.DefaultConfig(), Prompt: "Newest question"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	c := m.chat()
	c.ActiveTurn = &turn
	run := &appchat.ChatRun{Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent)}
	m.run = run
	m.chatEndpoint(true)
	old := c.Turns[len(c.Turns)-1]
	m.updateChatEvent(appchat.ChatEvent{Run: run, Text: "Newest output"})
	if c.Turns[len(c.Turns)-1].Response.Text != old.Response.Text || !c.Newer {
		t.Fatal("stream wrote into an unrelated older reply")
	}
	m.stopChat()
	m.updateChatEvent(appchat.ChatEvent{Run: run, Done: true, Err: context.Canceled})
	m.drainPersistence()
	m.chatEndpoint(false)
	if c.Turns[len(c.Turns)-1].Response.Text != "Newest output" {
		t.Fatal("active response was not saved outside transcript cache")
	}
}

func TestRetiredSearchMaintenanceAliasIsUnavailable(t *testing.T) {
	if canonicalCommand("J") == "rebuildsearch" {
		t.Fatal("retired search maintenance command returned")
	}
}

func TestInactiveChatWindowsEvictAndRestoreReadingPosition(t *testing.T) {
	m, _ := savedHistoryModel(t, 70)
	c := m.chat()
	*m.draft() = textview.NewField("General draft")
	m.chatEndpoint(true)
	m.chatTranscript(40, 15)
	m.scrollChat("down")
	m.chatTranscript(40, 15)
	before, sequence, reply := c.Scroll, c.Turns[0].Sequence, c.SelectedReply()
	createWorkspace(t, m, "Other workspace")
	selectChatConnection(t, m)
	if !c.Evicted || len(c.Turns) != 0 || c.Messages != nil || c.Evidence != nil {
		t.Fatal("inactive workspace kept resident chat text")
	}
	m.active = ""
	m.loadChat()
	finishChatRestore(t, m)
	m.chatTranscript(40, 15)
	if c.Evicted || c.Turns[0].Sequence != sequence || c.Scroll != before || c.SelectedReply() != reply || m.draft().Text != "General draft" {
		t.Fatal("reopening evicted chat lost reading position, reply, or draft")
	}
}

func finishChatRestore(t *testing.T, m *model) {
	t.Helper()
	if r := m.chat().Restore; r != nil {
		command := m.syncChatRestore()
		var result appchat.ChatRestoreResult
		if command != nil {
			result = command().(appchat.ChatRestoreResult)
		} else {
			<-r.Done
			result = r.Output
		}
		m.updateChatRestore(result)
		if m.chat().Err != "" {
			t.Fatal(m.chat().Err)
		}
	}
}

func TestAsyncChatRestorePreservesDraftAndRejectsStaleCompletion(t *testing.T) {
	m, conversation := savedHistoryModel(t, 2)
	*m.draft() = textview.NewField("Draft while restoring")
	m.openConversation(m.chat(), conversation)
	c := m.chat()
	if c.Restore == nil || len(c.Turns) != 0 || !strings.Contains(m.chatTranscript(60, 15), "Restoring saved conversation") {
		t.Fatal("opening history did not expose asynchronous restoration")
	}
	command := m.syncChatRestore()
	m.sendChat()
	if !strings.Contains(m.status, "restoration") || m.draft().Text != "Draft while restoring" {
		t.Fatal("restoring conversation sent or discarded the draft")
	}
	m.persist()
	if err := m.saveSession(); err != nil || !m.dirty {
		t.Fatal("autosave did not defer while history was restoring")
	}
	m.chatCommand("newchat")
	m.updateChatRestore(command().(appchat.ChatRestoreResult))
	if m.chat().Conversation.ID != "" || len(m.chat().Turns) != 0 || m.draft().Text != "Draft while restoring" {
		t.Fatal("late restoration replaced a new conversation or draft")
	}
	m.openConversation(m.chat(), conversation)
	finishChatRestore(t, m)
	if len(m.chat().Turns) != 2 || m.chat().Restore != nil {
		t.Fatal("new restoration did not complete")
	}
}

func TestStreamingEnforcesTranscriptBudgetAndKeepsReadReply(t *testing.T) {
	m, _ := testModel(t)
	c := m.chat()
	for i := 0; i < 3; i++ {
		c.Turns = append(c.Turns, history.Turn{Sequence: uint64(i + 1), Request: history.Request{ID: fmt.Sprintf("old-%d", i), Prompt: strings.Repeat("q", 100<<10)}, Response: history.Response{Status: "completed", Text: strings.Repeat("a", 1<<20)}})
	}
	active := history.Turn{Sequence: 4, Request: history.Request{ID: "active", Prompt: strings.Repeat("q", 100<<10)}, Response: history.Response{Status: "streaming"}}
	c.Turns = append(c.Turns, active)
	c.ActiveTurn = &active
	c.Reply, c.ReplySelected, c.Reading = 1, true, true
	m.chatContent()
	c.Anchor.Line = c.ReplyLines[1]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	run := &appchat.ChatRun{Chat: c, Cancel: cancel, Events: make(chan appchat.ChatEvent)}
	m.run = run
	m.updateChatEvent(appchat.ChatEvent{Run: run, Text: strings.Repeat("b", 1<<20)})
	bytes := 0
	for _, turn := range c.Turns {
		bytes += appchat.TurnTextBytes(turn)
	}
	if bytes > appchat.ChatCacheBytes || len(c.Turns) != 1 || c.Turns[0].Request.ID != "old-1" || !c.Newer || !c.Older || c.ActiveTurn.Response.Text == "" {
		t.Fatal("growing response exceeded the cache budget or displaced the reply being read")
	}
}

func TestOpeningConversationStartsLexicalCatchupBeforeAITurn(t *testing.T) {
	m, _ := testModel(t)
	conversation, err := m.history.NewConversation("", "Saved conversation")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := m.history.StartTurn("", conversation.ID, history.Request{Agent: ai.DefaultConfig(), Prompt: "opening-index-marker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.history.SaveResponse("", conversation.ID, turn.Request.ID, history.Response{Status: "completed", Text: "recorded"}); err != nil {
		t.Fatal(err)
	}
	root, err := m.history.HistoryRoot("")
	if err != nil {
		t.Fatal(err)
	}
	m.openConversation(m.chat(), conversation)
	finishChatRestore(t, m)
	files, err := filepath.Glob(filepath.Join(root, "search", "*.db"))
	if err != nil || len(files) != 1 {
		t.Fatal("opening did not initialize lexical search", files, err)
	}
	index, err := m.history.SearchIndex("", conversation.ID, history.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	access, err := history.NewAccess(m.history, "", conversation.ID, index)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		result, err := access.Call(t.Context(), history.HistoryRequest{Operation: "search", Query: "opening-index-marker"}, 65536)
		if err != nil {
			t.Fatal(err)
		}
		if result.Search.LexicalComplete && len(result.Search.Hits) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("opened conversation did not catch up", result.Search)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.run != nil {
		t.Fatal("history opening started an AI turn")
	}
}

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
