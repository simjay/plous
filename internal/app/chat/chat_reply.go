package chat

import (
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/ui"
	markdownview "github.com/simjay/plous/internal/ui/markdown"
	textview "github.com/simjay/plous/internal/ui/text"
)

func NewReplyViewer(turn history.Turn) *markdownview.Model {
	reply := markdownview.New(turn.Response.Text)
	reply.Name = "Reply to: " + ui.OneLine(turn.Request.Prompt)
	reply.Actions = []textview.Action{{Key: "Enter", Label: "Sources"}}
	return reply
}
