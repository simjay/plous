package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/agent"
	"github.com/simjay/plous/internal/ai/history"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) chatContent() string {
	c := m.chat()
	var b strings.Builder
	c.ReplyLines = nil
	c.Questions = nil
	line := 0
	for i, t := range c.Turns {
		if i > 0 {
			b.WriteString("\n")
			line++
		}
		var turn strings.Builder
		prompt := appchat.ChatMessage(t.Request.Prompt, true)
		c.Questions = append(c.Questions, appchat.ChatQuestion{First: line + 1, Last: line + strings.Count(prompt, "\n") + 1, Width: lipgloss.Width(prompt)})
		c.ReplyLines = append(c.ReplyLines, line+strings.Count(prompt, "\n")+3)
		marker := "  "
		if i == c.SelectedReply() && m.focus == 2 && !m.chatEditing {
			marker = appchat.ChatAssistantStyle.Render("› ")
		}
		fmt.Fprintf(&turn, "%s\n%s\n\n%s%s\n", appchat.ChatYouStyle.Render("You"), prompt, marker, ui.Muted.Render("Assistant"))
		turn.WriteString(c.RenderedReply(i))
		if t.Response.Status != "completed" && t.Response.Status != "streaming" {
			status := ui.ChangedStyle.Render("[" + ui.OneLine(t.Response.Status) + "]")
			if t.Response.Status == "error" {
				status = ui.RemovedStyle.Render("[error]")
			}

			fmt.Fprintf(&turn, "\n%s", status)
		}
		if t.Response.Error != "" {
			fmt.Fprintf(&turn, "\n%s", ui.RemovedStyle.Render(ui.Plain(t.Response.Error)))
		}
		fmt.Fprintf(&turn, "\n%s", m.chatReplyHint(i))
		text := turn.String()
		parts := strings.Split(text, "\n")
		for j := range parts {
			parts[j] = ui.MouseTarget(parts[j], "reply/"+t.Request.ID)
		}
		text = strings.Join(parts, "\n")
		b.WriteString(text)
		line += strings.Count(text, "\n")
	}
	return b.String()
}

func (m *model) scrollChat(key string) {
	c := m.chat()
	if c.Restore != nil {
		return
	}
	if key == "home" && c.Older {
		m.chatEndpoint(true)
		return
	}
	if key == "end" && c.Newer {
		m.chatEndpoint(false)
		return
	}
	if c.Reading && c.Scroll == 0 && c.Older && (key == "up" || key == "k" || key == "pgup") {
		if m.chatPage(-1) {
			c.ReplyJump = false
			c.Scroll = int(^uint(0) >> 1)
		}
		return
	}
	if c.Scroll >= max(0, c.ViewRows-c.ViewHeight) && c.Newer && (key == "down" || key == "j" || key == "pgdown") {
		if m.chatPage(1) {
			c.ReplyJump, c.Scroll = false, 0
		}
		return
	}
	last := max(0, c.ViewRows-c.ViewHeight)
	top := c.Scroll
	if !c.Reading {
		top = last
	}
	switch key {
	case "up", "k":
		top--
	case "down", "j":
		top++
	case "pgup":
		top -= max(1, c.ViewHeight-1)
	case "pgdown":
		top += max(1, c.ViewHeight-1)
	case "home":
		top = 0
	case "end":
		top = last
		c.ReplySelected = false
	default:
		return
	}
	c.Scroll = max(0, min(last, top))
	c.Reading = c.Scroll < last || c.Newer
	if c.Reading && c.ContentWidth > 0 {
		index := 0
		c.WalkChatRows(m.chatContent(), c.ContentWidth, func(row appchat.ChatRow) bool {
			if index == c.Scroll {
				c.SelectReplyForLine(row.Line)
				return false
			}
			index++
			return true
		})
	} else {
		c.ReplySelected = false
	}
	// A keyboard move supplies the next anchor when the view is rendered.
	c.ViewWidth = 0
	m.status = ""
}

func (m *model) chatTranscript(width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	c := m.chat()
	if c.History {
		heading := "Conversations · Esc back\n"
		if m.searches[2] != nil {
			heading = m.searchBar(2, width, "Search conversations") + "\n"
		}
		available := max(0, height-strings.Count(heading, "\n"))
		items := m.filteredConversations()
		if len(items) == 0 {
			message := "No saved conversations."
			if m.searches[2] != nil {
				message = "No matching conversations."
			}
			return ui.StyledView(heading+ui.TextView(message, width, available, 0), width, height)
		}
		body := ui.CollectionWindow(len(items), c.Cursor, available, func(i int) []string {
			v := items[i]
			if v.Created.IsZero() {
				return []string{ui.MouseChoice(ui.ChoiceLine(ui.OneLine(v.Title), i == c.Cursor, width), i, v.ID)}
			}
			return []string{
				ui.MouseChoice(ui.ChoiceLine(ui.OneLine(v.Title), i == c.Cursor, width), i, v.ID),
				ui.MouseChoice(ui.Muted.Render(ui.Clipped("  "+v.Created.Local().Format("Jan 02 15:04"), width)), i, v.ID),
			}
		})
		return ui.StyledView(heading+body, width, height)
	}
	available := height
	if c.Restore != nil {
		return ui.StyledView(m.loading("Restoring saved conversation…"), width, height)
	}
	if len(c.Turns) == 0 {
		description := "Ask about your infrastructure or plan your next change."
		if m.current() != nil {
			description = "Ask about the selected connection. Select another connection to change scope."
		}
		body := ui.EmptyState("No conversation yet.", description, ":c / cc  Write a message", available)
		if m.settings.Agent.Model == "" {
			body = ui.EmptyState("Connect a vendor", "Add a Plous provider connection in Settings to start chatting. You can write a draft anytime.", ":s  Settings → Agent → Plous", available)
		}
		if c.Err != "" {
			body = ui.RemovedStyle.Render("Chat storage error: " + ui.Plain(c.Err))
		}
		return ui.StyledView(ui.StyledTextView(body, width, available, 0), width, height)
	}
	statusHeight := 0
	if available >= 5 {
		statusHeight = 1
	}
	available -= statusHeight
	search := m.searches[2]
	contentHeight := available
	if search != nil {
		contentHeight = max(0, available-ui.SearchBarHeight)
	}
	c.ContentWidth = width
	body := m.chatContent()
	rowCount := 0
	jumped := false
	c.WalkChatRows(body, width, func(row appchat.ChatRow) bool {
		if c.ReplyJump && !jumped && row.Line == c.ReplyLines[c.SelectedReply()] {
			c.Scroll, jumped = rowCount, true
		}
		if !c.ReplyJump && c.Reading && c.ViewWidth != 0 && c.ViewWidth != width && (row.Line < c.Anchor.Line || row.Line == c.Anchor.Line && row.Column <= c.Anchor.Column) {
			c.Scroll = rowCount
		}
		rowCount++
		return true
	})
	c.ReplyJump = false
	last := max(0, rowCount-contentHeight)
	if !c.Reading {
		c.Scroll = last
	}
	c.Scroll = max(0, min(last, c.Scroll))
	c.ViewRows, c.ViewHeight, c.ViewWidth = rowCount, contentHeight, width
	if search != nil {
		body = m.findText(2, body, width, available, c.Scroll)
	} else {
		lines := make([]string, 0, contentHeight)
		index := 0
		c.WalkChatRows(body, width, func(row appchat.ChatRow) bool {
			if index == c.Scroll {
				c.Anchor = row
			}
			if index >= c.Scroll && len(lines) < contentHeight {
				lines = append(lines, row.Text)
			}
			index++
			return len(lines) < contentHeight
		})
		body = strings.Join(lines, "\n")
	}
	view := body
	if statusHeight > 0 {
		status := ""
		if c.Err != "" {
			status = "Save error: " + ui.Plain(c.Err)
		} else if c.Oversized {
			status = "Large saved exchange · adjacent messages load separately"
		} else if c.Reading && m.chatEditing {
			status = "Earlier · PgDn newer"
		} else if c.Reading && width >= 32 {
			status = fmt.Sprintf("%d-%d/%d · End latest", c.Scroll+1, min(rowCount, c.Scroll+contentHeight), rowCount)
		} else if c.Reading {
			status = "Earlier · End latest"
		}
		view += "\n" + ui.ChangedStyle.Render(ui.Clipped(status, width))
	}
	return ui.StyledView(view, width, height)
}

func (m *model) chatScope() string { return m.assistantScopeLabel() }

func (m *model) chatPanelTitle(width int) string {
	if m.externalAgentSelected() {
		label := m.settings.MainAgent.Label()
		if m.nativeAgent != nil {
			label += " · " + m.nativeAgent.Label
		}
		return ui.Clipped(label, width)
	}
	if width < 30 {
		return "AI assistant"
	}
	return "AI assistant · " + appchat.ChatAssistantStyle.Render(ui.Clipped(m.chatScope(), width-15))
}

func (m *model) chatBody(width, height int) string {
	if m.externalAgentSelected() {
		return m.nativeAgentBody(width, height)
	}
	if m.chat().Models {
		return m.modelPickerBody(width, height)
	}
	historyLabel := "History"
	if width < 24 {
		historyLabel = "Chats"
	}
	controls := textview.ActionBar([]textview.ActionGroup{{Actions: []textview.Action{
		{Key: ":H", Label: historyLabel}, {Key: ":C", Label: "New"},
	}}}, width, 1, ui.WorkspaceTheme())
	if width < 30 {
		controls = appchat.ChatAssistantStyle.Render(ui.Clipped(m.chatScope(), width)) + "\n" + controls
	}
	active := m.focus == 2 && m.chatEditing && !m.chat().History
	e := m.draft()
	inputWidth := max(1, width-4)
	text := e.MouseDisplay(active, "composer", ui.WorkspaceTheme())
	if e.Text == "" {
		text = "cc Write a message"
		if active {
			text = "▏Ask about " + m.chatScope() + "…"
		}
		text = ui.Muted.Render(ui.Clipped(text, inputWidth))
	}
	lines := strings.Split(ansi.Hardwrap(text, inputWidth, true), "\n")
	inputHeight := min(len(lines), min(5, max(1, height/3-3)))
	offset := max(0, len(lines)-inputHeight)
	if active {
		prefix := string([]rune(e.Text)[:min(e.Cursor, len([]rune(e.Text)))]) + "▏"
		cursorLine := len(strings.Split(ansi.Hardwrap(prefix, inputWidth, true), "\n")) - 1
		offset = max(0, cursorLine-inputHeight+1)
	}
	offset = min(offset, max(0, len(lines)-inputHeight))
	editorContent := strings.Join(lines[offset:min(len(lines), offset+inputHeight)], "\n")
	border := ui.BorderColor
	if active {
		border = ui.Accent
	}
	kind := m.settings.MainAgent.Effective()
	if m.run != nil && m.run.Chat == m.chat() {
		kind = m.run.Agent.Effective()
	}
	label := kind.Label()
	if kind == agent.Plous {
		label = appchat.ChatModelName(m.settings.Agent)
		if m.settings.Agent.Model == "" {
			label = "No model"
		}
		label = ui.Clipped(ui.OneLine(label), max(1, inputWidth-7))
		label += strings.Repeat(" ", max(1, inputWidth-ansi.StringWidth(label)-6)) + ui.MouseTarget("/model", "models")
	} else {
		label = ui.Clipped(label+" · CLI model", inputWidth)
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(width).MaxWidth(width).
		Render(editorContent + "\n" + ui.Muted.Render(label))
	progress := m.chatProgress(width)
	progressHeight := 0
	if progress != "" {
		progress += "\n"
		progressHeight = 1
	}
	transcriptHeight := max(0, height-lipgloss.Height(controls)-lipgloss.Height(box)-1-progressHeight)
	transcript := lipgloss.NewStyle().Width(width).Height(transcriptHeight).MaxWidth(width).MaxHeight(transcriptHeight).
		Render(m.chatTranscript(width, transcriptHeight))
	return textview.Fit(controls+"\n"+transcript+"\n"+progress+ui.MouseTarget(box, "composer")+"\n"+ui.Muted.Render(m.chatContextLine(width)), width, height)
}

func (m *model) chatProgress(width int) string {
	if m.run == nil {
		return ""
	}
	label := m.run.Chat.Progress
	if label == "" {
		label = "Responding…"
	}
	if m.run.Chat != m.chat() {
		label = "Reply running in another conversation"
	}
	stop := textview.ActionChip(textview.Action{Key: ":X", Label: "Stop"}, width, ui.WorkspaceTheme())
	room := width - ansi.StringWidth(stop) - 1
	if room < 4 {
		return stop
	}
	return ui.Clipped(m.loading(label), room) + " " + stop
}

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
