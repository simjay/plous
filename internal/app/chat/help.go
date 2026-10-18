package chat

import (
	"github.com/simjay/plous/internal/ai/agent"
	helpview "github.com/simjay/plous/internal/ui/help"
)

func (c *ChatState) Help(editing, running bool, kind agent.Kind) helpview.Section {
	h := helpview.New("AI chat", helpview.Command("chat", "cc", "Write a message"), helpview.Command("newchat", ":C", "New chat"), helpview.Command("openchat", ":H", "History"))
	if running {
		h.Add("Response", helpview.Command("stopchat", ":X", "Stop response"))
	}
	if c.Models {
		s := helpview.Picker("AI chat · Models", "Choose model")
		s.Add("Navigation", helpview.Key("ends", "Home / End", "First / last"))
		return s
	}
	if c.History {
		s := helpview.Picker("AI chat · History", "Open conversation")
		s.Add("Search", helpview.Key("filter", "/", "Search history"))
		for _, group := range h.Groups {
			s.Add("Chat", group.Entries...)
		}
		return s
	}
	if editing {
		h.Title = "AI chat · Compose"
		h.Add("Typing", helpview.Key("send", "Enter", "Send"), helpview.Key("newline", "Shift+Enter", "New line", "Ctrl+J"), helpview.Key("back", "Esc", "Keep draft; leave input"))
		if kind.Effective() == agent.Plous {
			h.Add("Typing", helpview.Key("model", "/model", "Choose model", "/m"))
			h.Note("Submit /model as a freshly typed draft. //model sends it literally.")
		}
	} else {
		h.Add("Read replies", helpview.Key("reply", "[ / ]", "Previous / next reply"), helpview.Key("sources", "Enter", "Sources / reply details"), helpview.Command("readreply", ":A", "Read selected reply"), helpview.Key("find", "/", "Find replies"))
		h.Add("Navigation", helpview.Scroll()...)
		h.Add("Draft", helpview.Command("restoreprompt", ":p", "Restore last message into empty draft"))
	}
	return h
}

func (s *ChatSources) Help() helpview.Section {
	if s.ShowReply && s.Reply != nil {
		h := s.Reply.Reader.Help("AI chat · Read reply", "Clear selection / search, then return")
		if !s.Reply.Reader.Searching {
			h.Add("Reply", helpview.Key("sources", "Enter", "Sources for this reply"), helpview.Key("source", "R", "Toggle rendered / source"), helpview.Key("copy-source", "Y", "Copy source"))
		}
		return h
	}
	if s.Document != nil {
		return s.Document.Help("Sources · Captured result", "", "back")
	}
	if s.List {
		return helpview.Picker("AI chat · Sources", "Open selected source")
	}
	h := helpview.New("Sources · Details", helpview.Key("open", "Enter", "Read captured result"), helpview.Key("back", "Esc", "Return one layer; keep draft"))
	h.Add("Navigation", helpview.Scroll()...)
	h.Note("Sources reads saved records only.")
	return h
}
