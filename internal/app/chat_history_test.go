package app

import (
	"context"
	"fmt"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	textview "github.com/simjay/plous/internal/ui/text"
	"path/filepath"
	"time"

	"strings"
	"testing"
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
