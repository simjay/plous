package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"

	"regexp"
)

type chatFind struct {
	ctx                        context.Context
	cancel                     context.CancelFunc
	chat                       *appchat.ChatState
	query                      string
	direction                  int
	boundary                   uint64
	scanned                    int
	pending, done, wrap, found bool
	err                        string
	result                     chan chatFindResult
}

type chatFindResult struct {
	search *panelSearch
	find   *chatFind
	page   history.HistoryPage
	err    error
}

func (s *panelSearch) cancelChatFind() {
	if s != nil && s.archive != nil {
		s.archive.cancel()
		s.archive = nil
	}
}

func (m *model) beginChatFind(s *panelSearch, direction int, boundary uint64, wrap bool) {
	s.cancelChatFind()
	ctx, cancel := context.WithCancel(context.Background())
	s.archive = &chatFind{ctx: ctx, cancel: cancel, chat: m.chat(), query: s.input.Text, direction: direction, boundary: boundary, wrap: wrap}
}

func (m *model) syncChatFind() tea.Cmd {
	s := m.searches[2]
	if s == nil || s.kind != "find" || s.input.Text == "" || m.history == nil || m.chat().Conversation.ID == "" || m.chat().Restore != nil {
		return nil
	}
	if s.archive == nil {
		m.beginChatFind(s, 1, 0, false)
	}
	f := s.archive
	if f.pending || f.done || f.ctx.Err() != nil {
		return nil
	}
	f.pending = true
	store, c, direction, boundary := m.history, f.chat.Conversation, f.direction, f.boundary
	result := make(chan chatFindResult, 1)
	f.result = result
	go func() {
		var page history.HistoryPage
		var err error
		if direction < 0 {
			page, err = store.TurnPage(f.ctx, c.WorkspaceID, c.ID, boundary, 16, 512<<10)
		} else {
			page, err = store.TurnPageAfter(f.ctx, c.WorkspaceID, c.ID, boundary, 16, 512<<10)
		}
		result <- chatFindResult{s, f, page, err}
	}()
	return func() tea.Msg { return <-result }

}

func visibleTurnMatches(t history.Turn, query string) int {
	pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	text := ansi.Strip(appchat.ChatMessage(t.Request.Prompt, true)) + "\n" + ansi.Strip(appchat.ChatMessage(t.Response.Text, false)) + "\n" + ui.Plain(t.Response.Error)
	count := 0
	for len(text) > 0 {
		match := pattern.FindStringIndex(text)
		if match == nil {
			break
		}
		count++
		text = text[match[1]:]
	}
	return count
}

func (m *model) updateChatFind(result chatFindResult) {
	s, f := result.search, result.find
	if m.searches[2] != s || s.archive != f || m.chat() != f.chat || f.ctx.Err() != nil {
		return
	}
	f.pending = false
	if result.err != nil {
		f.err, f.done = result.err.Error(), true
		return
	}
	page := result.page
	for n := range page.Turns {
		i := n
		if f.direction < 0 {
			i = len(page.Turns) - 1 - n
		}
		t := page.Turns[i]
		f.scanned++
		matches := visibleTurnMatches(t, f.query)
		if matches == 0 {
			continue
		}
		m.replaceTranscript(history.HistoryPage{Turns: []history.Turn{t}, HasOlder: page.HasOlder || i > 0, HasNewer: page.HasNewer || i < len(page.Turns)-1})
		f.done, f.found = true, true
		s.match, s.total, s.positioned = 0, matches, false
		if f.direction < 0 {
			s.match = matches - 1
		}
		m.chat().Reading = true
		return
	}
	more := page.HasNewer
	if f.direction < 0 {
		more = page.HasOlder
	}
	if more && len(page.Turns) > 0 {
		f.boundary = page.Turns[len(page.Turns)-1].Sequence
		if f.direction < 0 {
			f.boundary = page.Turns[0].Sequence
		}
		return
	}
	if f.wrap {
		f.boundary, f.wrap = 0, false
		return
	}
	f.done = true
	s.total = 0
}

func (m *model) stepChatFind(s *panelSearch, direction int) bool {
	if s.archive == nil {
		return false
	}
	if !s.archive.done {
		return true
	}
	if s.total > 0 && s.match+direction >= 0 && s.match+direction < s.total {
		s.match += direction
		s.positioned = false
		return true
	}
	c := m.chat()
	if len(c.Turns) > 0 {
		boundary := c.Turns[0].Sequence
		if direction > 0 {
			boundary = c.Turns[len(c.Turns)-1].Sequence
		}
		m.beginChatFind(s, direction, boundary, true)
	}
	return true
}

func (s *panelSearch) chatFindLabel(label string) string {
	f := s.archive
	if f == nil {
		return label
	}
	if f.err != "" {
		return "Find unavailable: " + ui.OneLine(f.err)
	}
	if !f.done {
		return fmt.Sprintf("Find · searching saved messages (%d)", f.scanned)
	}
	if s.total == 0 {
		return "Find · no matching saved messages"
	}
	return fmt.Sprintf("Find · %d/%d in this exchange · n/N saved history", s.match+1, s.total)
}
