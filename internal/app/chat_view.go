package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/ui"
	"strings"
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
