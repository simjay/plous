package app

import (
	tea "charm.land/bubbletea/v2"
	"context"

	"fmt"
	appchat "github.com/simjay/plous/internal/app/chat"

	"github.com/simjay/plous/internal/ui"

	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"time"
)

func (m *model) selectChatReply(step int) {
	c := m.chat()
	if len(c.Turns) == 0 {
		return
	}
	if step < 0 && c.SelectedReply() == 0 && c.Older || step > 0 && c.SelectedReply() == len(c.Turns)-1 && c.Newer {
		m.chatPage(step)
		return
	}
	c.Reply = max(0, min(c.SelectedReply()+step, len(c.Turns)-1))
	c.ReplySelected, c.ReplyJump, c.Reading = true, true, true
	c.ViewWidth = 0
	m.status = ""
}

func (m *model) syncChatEvidence() tea.Cmd {
	c := m.chat()
	if m.focus != 2 || c.History || c.Models || len(c.Turns) == 0 || m.history == nil {
		return nil
	}
	turn := c.Turns[c.SelectedReply()]
	if turn.Request.ID == "" || turn.Response.Status == "streaming" {
		return nil
	}
	if e := c.Evidence; e != nil && e.Conversation.ID == c.Conversation.ID && e.Turn.Request.ID == turn.Request.ID && e.Turn.Response.Status == turn.Response.Status {
		return nil
	}
	e := &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn}
	c.Evidence = e
	if s := m.chatSources; s != nil && s.Chat == c && s.Evidence.Turn.Request.ID == turn.Request.ID {
		s.Evidence = e
	}
	store := m.history
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		records, err := store.TurnEvidence(ctx, e.Conversation.WorkspaceID, e.Conversation.ID, e.Turn.Request.ID)
		return appchat.ChatEvidenceResult{Evidence: e, Records: records, Err: err}
	}
}

func (m *model) chatReplyHint(index int) string {
	c := m.chat()
	if index != c.SelectedReply() || c.Turns[index].Response.Status == "streaming" {
		return "\n"
	}
	label, action, value := "Sources", "View sources", ""
	if e := c.Evidence; e != nil && e.Turn.Request.ID == c.Turns[index].Request.ID && e.Loaded {
		if e.Err != nil {
			label, action = "Sources unavailable", "View source details"
		} else if len(e.Records) == 0 {
			label, action = "Reply details", "View reply details"
		} else {
			value = fmt.Sprintf("· %d", len(e.Records))
		}
	}
	if m.focus == 2 && !m.chatEditing && m.searches[2] == nil {
		return textview.ActionChip(textview.Action{Mouse: "sources/" + c.Turns[index].Request.ID, Key: "Enter", Label: action, Value: value}, max(1, c.ContentWidth), ui.WorkspaceTheme()) + "\n" +
			textview.ActionChip(textview.Action{Mouse: "readreply/" + c.Turns[index].Request.ID, Key: ":A", Label: "Read reply"}, max(1, c.ContentWidth), ui.WorkspaceTheme())
	}
	return ui.MouseTarget(ui.Muted.Render(strings.TrimSpace(label+" "+value)), "sources/"+c.Turns[index].Request.ID) + "\n" +
		ui.MouseTarget(ui.Muted.Render("Read reply"), "readreply/"+c.Turns[index].Request.ID)
}

func (m *model) openChatSources() tea.Cmd {
	c := m.chat()
	if len(c.Turns) == 0 {
		return nil
	}
	if c.Turns[c.SelectedReply()].Response.Status == "streaming" {
		m.status = "Sources will be available when this reply finishes."
		return nil
	}
	cmd := m.syncChatEvidence()
	if c.Evidence == nil || c.Evidence.Turn.Request.ID != c.Turns[c.SelectedReply()].Request.ID {
		m.status = "No saved source records are available for this reply."
		return cmd
	}
	m.sourcesTravel = overlayTravel{}
	m.chatSources = &appchat.ChatSources{Chat: c, Evidence: c.Evidence, List: len(c.Evidence.Records) > 1, Follow: !c.Reading, Tail: c.Turns[len(c.Turns)-1].Response.Text}
	c.Reading = true
	m.status = ""
	return cmd
}

func (m *model) closeChatSources() {
	if s := m.chatSources; s != nil {
		c := s.Chat
		if s.Follow && len(c.Turns) > 0 && c.Turns[len(c.Turns)-1].Response.Text == s.Tail {
			c.Reading = false
		}
		if m.chatSourcesVisible() {
			m.focus = 2
		}
		m.chatSources = nil
		m.ensureVisibleFocus()
	}
}

func (m *model) chatSourcesKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.chatSources
	key := msg.String()
	searching := s.Document != nil && s.Document.Searching || s.ShowReply && s.Reply.Reader.Searching
	if !searching {
		if key == ":" {
			m.beginCommand()
			return nil
		}
		if m.updateShortcut(msg) {
			return nil
		}
	}
	m.shortcutPrefix, m.status = "", ""
	if s.ShowReply {
		if !searching && key == "enter" {
			s.ShowReply = false
			return nil
		}
		used, copied := s.Reply.Key(key, msg.Text)
		if !used && key == "esc" {
			m.closeChatSources()
		}
		return m.copyWorkspaceText(copied)
	}
	if s.Document != nil {
		event := s.Document.Key(key, msg.Text)
		if event.Action == "back" {
			s.Document = nil
		}
		return m.copyWorkspaceText(event.Copy)
	}
	if key == "esc" {
		if !s.List && len(s.Evidence.Records) > 1 && !s.Loading {
			s.List = true
		} else if s.Reply != nil {
			s.ShowReply = true
		} else {
			m.closeChatSources()
		}
		return nil
	}
	if !s.Evidence.Loaded || s.Loading {
		return nil
	}
	if s.List {
		switch key {
		case "up", "k":
			s.Index = max(0, s.Index-1)
		case "down", "j":
			s.Index = min(len(s.Evidence.Records)-1, s.Index+1)
		case "enter":
			s.List = false
			s.Scroll = 0
		}
		return nil
	}
	if key == "enter" && len(s.Evidence.Records) > 0 {
		return m.openChatSourceDocument()
	}
	switch key {
	case "up", "k":
		s.Scroll--
	case "down", "j":
		s.Scroll++
	case "pgup":
		s.Scroll -= max(1, s.Page-1)
	case "pgdown":
		s.Scroll += max(1, s.Page-1)
	case "home":
		s.Scroll = 0
	case "end":
		s.Scroll = s.Last
	}
	s.Scroll = max(0, min(s.Last, s.Scroll))
	return nil
}

func (m *model) openChatSourceDocument() tea.Cmd {
	return m.chatSources.OpenDocument()
}

func (m *model) updateChatSourceDocument(result appchat.ChatSourceDocumentResult) tea.Cmd {
	if m.chatSources == result.Sources {
		m.chatSources.ApplyDocument(result)
	}
	return nil
}

func (m *model) chatSourcesBody(width, height int) string { return m.chatSources.Body(width, height) }

func (m *model) renderChatSourcesOverlay() string {
	base := *m
	base.chatSources = nil
	width, height := min(96, m.width-4), min(18, m.height-4)
	if m.chatSources.Document != nil || m.chatSources.ShowReply {
		width, height = min(110, m.width-4), min(38, m.height-4)
	} else if m.chatSources.List {
		height = min(m.height-4, min(24, max(10, len(m.chatSources.Evidence.Records)+6)))
	}
	available := height - 3
	if m.command != nil {
		available -= 3
	}
	body := textview.Fit(m.chatSourcesBody(width-4, available), width-4, available)
	if m.command != nil {
		body += "\n" + m.commandBox(width-4)
	}
	title := "Sources"
	if e := m.chatSources.Evidence; e.Loaded && len(e.Records) == 0 && e.Err == nil {
		title = "Reply details"
	}
	if m.chatSources.ShowReply {
		title = "Read reply"
	}
	screen := strings.Split(ui.CenteredOverlay(base.render(), textview.MouseScope(helpBox(title, body, width, height, true), "sources")), "\n")
	screen[len(screen)-1] = textview.MouseScope(m.footer(), "sources")
	return strings.Join(screen, "\n")
}

func (m *model) chatSourcesHint() string {
	s := m.chatSources
	if s.ShowReply {
		if s.Reply.Reader.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · v Select · y Copy · R Source · Enter Sources · Esc Back · hh Help"
	}
	if s.Document != nil {
		if s.Document.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		return "↑↓ Scroll · / Find · y Copy · Esc " + documentBackHint(s.Document, "Source details") + " · hh Help"
	}
	if s.List {
		return "↑↓ Choose source · Enter Open · Esc Reply · hh Help"
	}
	if len(s.Evidence.Records) == 0 || !s.Evidence.Loaded || s.Evidence.Err != nil {
		return "↑↓ Scroll · Esc Reply · hh Help"
	}
	return "Enter View captured result · ↑↓ Scroll · Esc Back · hh Help"
}
