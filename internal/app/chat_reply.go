package app

import (
	"errors"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) openChatReply() {
	if s := m.chatSources; m.chatSourcesVisible() {
		if s.Loading {
			m.status = "Wait for the saved source to finish loading."
			return
		}
		if s.Reply == nil {
			s.Reply = appchat.NewReplyViewer(s.Evidence.Turn)
		}
		s.Document, s.ShowReply = nil, true
		return
	}
	c := m.chat()
	if c.Restore != nil || len(c.Turns) == 0 || c.History || c.Models {
		m.status = "Select a reply to read."
		return
	}
	turn := c.Turns[c.SelectedReply()]
	unsaved := turn.Response.Status == "streaming" && m.run != nil && m.run.Chat == c && m.run.Finished && c.ActiveTurn != nil && c.ActiveTurn.Request.ID == turn.Request.ID
	if turn.Response.Status == "streaming" && !unsaved {
		m.status = "Read reply is available when this response finishes."
		return
	}
	m.closeHelp()
	m.restoreView()
	m.focusChat(false)
	// Sources loading is scheduled by Update; the reader can open immediately.
	e := c.Evidence
	if unsaved {
		e = &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn, Loaded: true, Err: errors.New("this response has not been saved; its displayed text can be copied")}
	} else if e == nil || e.Turn.Request.ID != turn.Request.ID {
		e = &appchat.ChatEvidence{Conversation: c.Conversation, Turn: turn}
	}
	m.sourcesTravel = overlayTravel{}
	m.chatSources = &appchat.ChatSources{Chat: c, Evidence: e, Reply: appchat.NewReplyViewer(turn), ShowReply: true, List: len(e.Records) > 1, Follow: !c.Reading, Tail: c.Turns[len(c.Turns)-1].Response.Text}
	if unsaved {
		m.chatSources.Reply.Name = "Unsaved response to: " + ui.OneLine(turn.Request.Prompt)
	}
	c.Reading = true
	m.status = ""
}
