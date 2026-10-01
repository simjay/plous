package app

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	docedit "github.com/simjay/plous/internal/editor"
)

type chatRow struct {
	text         string
	line, column int
	start        int
}

var chatYouStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EBDD8B")).Bold(true)
var chatAssistantStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
var chatInline = regexp.MustCompile("`[^`\n]+`|\\*\\*[^*\n]+\\*\\*")

func chatInlineText(text string) string {
	return chatInline.ReplaceAllStringFunc(text, func(match string) string {
		if strings.HasPrefix(match, "`") {
			return addedStyle.Background(highlight).Render(match[1 : len(match)-1])
		}
		return titleStyle.Foreground(accent).Render(match[2 : len(match)-2])
	})
}

func chatMessage(text string, user bool) string {
	lines := strings.Split(plain(strings.ReplaceAll(text, "\t", "    ")), "\n")
	fence, language := "", ""
	for i, line := range lines {
		if user {
			lines[i] = chatYouStyle.Render("│ ") + canvasStyle.Background(highlight).Render(line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		marker := ""
		if strings.HasPrefix(trimmed, "```") {
			marker = "```"
		} else if strings.HasPrefix(trimmed, "~~~") {
			marker = "~~~"
		}
		if marker != "" && (fence == "" || fence == marker) {
			if fence == "" {
				fence, language = marker, strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
				label := "Code"
				if language != "" {
					label += " · " + language
				}
				lines[i] = chatAssistantStyle.Render("╭ " + label)
			} else {
				fence = ""
				lines[i] = muted.Render("╰─")
			}
			continue
		}
		if fence != "" {
			code := addedStyle.Render(line)
			if language == "yaml" || language == "yml" {
				code = docedit.YAMLLine(line, workspaceTheme())
			}
			lines[i] = muted.Render("│ ") + code
			continue
		}
		heading := strings.TrimLeft(trimmed, "#")
		if heading != trimmed && strings.HasPrefix(heading, " ") {
			lines[i] = chatAssistantStyle.Render(strings.TrimSpace(heading))
		} else if strings.HasPrefix(trimmed, "> ") {
			lines[i] = muted.Render("│ " + strings.TrimPrefix(trimmed, "> "))
		} else {
			lines[i] = chatInlineText(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) chatContent() string {
	c := m.chat()
	var b strings.Builder
	for i, t := range c.turns {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s\n%s\n\n%s\n", chatYouStyle.Render("You"), chatMessage(t.Request.Prompt, true), chatAssistantStyle.Render("Assistant"))
		identity := chatModelName(t.Request.Agent)
		if t.Request.Agent.Name != "" {
			if identity != "" {
				identity += " · "
			}
			identity += t.Request.Agent.Name
		}
		if identity != "" {
			fmt.Fprintln(&b, muted.Render(oneLine(identity)))
		}
		b.WriteString(chatMessage(t.Response.Text, false))
		if t.Response.Status != "completed" {
			status := changedStyle.Render("[" + oneLine(t.Response.Status) + "]")
			if t.Response.Status == "error" {
				status = removedStyle.Render("[error]")
			}
			if t.Response.Status == "streaming" && m.run != nil && m.run.chat == c {
				label := c.progress
				if label == "" {
					label = "Responding…"
				}
				status = m.loading(label)
			}
			fmt.Fprintf(&b, "\n%s", status)
		}
		if t.Response.Error != "" {
			fmt.Fprintf(&b, "\n%s", removedStyle.Render(plain(t.Response.Error)))
		}
		if t.Response.Usage.Total > 0 {
			fmt.Fprintf(&b, "\n%s", muted.Render(fmt.Sprintf("%d tokens", t.Response.Usage.Total)))
		}
	}
	return b.String()
}

func chatRows(body string, width int) []chatRow {
	var rows []chatRow
	for i, line := range strings.Split(body, "\n") {
		clean := ansi.Strip(line)
		position, column := 0, 0
		for _, row := range strings.Split(ansi.Wrap(line, max(1, width), ""), "\n") {
			visible := ansi.Strip(row)
			// Word wrapping can omit the spaces between visual rows.
			skipped := max(0, strings.Index(clean[position:], visible))
			column += ansi.StringWidth(clean[position : position+skipped])
			position += skipped
			rows = append(rows, chatRow{text: row, line: i, column: column, start: position})
			position += len(visible)
			column += ansi.StringWidth(visible)
		}
	}
	return rows
}

func (m *model) scrollChat(key string) {
	c := m.chat()
	last := max(0, c.viewRows-c.viewHeight)
	top := c.scroll
	if !c.reading {
		top = last
	}
	switch key {
	case "up", "k":
		top--
	case "down", "j":
		top++
	case "pgup":
		top -= max(1, c.viewHeight-1)
	case "pgdown":
		top += max(1, c.viewHeight-1)
	case "home":
		top = 0
	case "end":
		top = last
	default:
		return
	}
	c.scroll = max(0, min(last, top))
	c.reading = c.scroll < last
	// A keyboard move supplies the next anchor when the view is rendered.
	c.viewWidth = 0
	m.status = ""
}

func (m *model) chatTranscript(width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	c := m.chat()
	if c.history {
		heading := "Conversations · Esc back\n"
		if m.searches[2] != nil {
			heading = m.searchBar(2, width, "Search conversations") + "\n"
		}
		available := max(0, height-strings.Count(heading, "\n"))
		items := []string{}
		for _, v := range m.filteredConversations() {
			items = append(items, v.Created.Local().Format("Jan 02 15:04")+" · "+v.Title)
		}
		if len(items) == 0 {
			message := "No saved conversations."
			if m.searches[2] != nil {
				message = "No matching conversations."
			}
			return styledView(heading+textView(message, width, available, 0), width, height)
		}
		return styledView(heading+listView(items, c.cursor, width, available), width, height)
	}
	heading := clipped("General chat", width)
	if w := m.current(); w != nil {
		scope := []string{oneLine(w.Name)}
		tool := m.tool
		connection := m.currentConnection()
		if connection != nil {
			tool = connection.Tool
		}
		if tool != "" {
			name := toolNames[tool]
			if name == "" {
				name = tool
			}
			scope = append(scope, oneLine(name))
		}
		if connection != nil {
			scope = append(scope, oneLine(connection.Name))
		}
		heading = strings.Join(scope, " · ")
		if ansi.StringWidth(heading) > width && len(scope) > 1 && height >= 6 {
			heading = clipped(scope[0], width) + "\n" + clipped(strings.Join(scope[1:], " · "), width)
		} else {
			heading = clipped(heading, width)
		}
	}
	heading = chatAssistantStyle.Render(heading)
	available := max(0, height-lipgloss.Height(heading))
	if c.err != "" || len(c.turns) == 0 {
		body := emptyState("No conversation yet.", "Ask about your infrastructure or plan your next change.", ":c / cc  Write a message", available)
		if m.settings.Agent.Model == "" {
			body = emptyState("Connect a vendor", "Add an AI connection to start chatting. You can write a draft anytime.", ":s  Open Settings", available)
		}
		if c.err != "" {
			body = removedStyle.Render("Chat storage error: " + plain(c.err))
		}
		return styledView(heading+"\n"+styledTextView(body, width, available, 0), width, height)
	}
	statusHeight := 0
	if available >= 3 {
		statusHeight = 1
	}
	available -= statusHeight
	search := m.searches[2]
	contentHeight := available
	if search != nil {
		contentHeight = max(0, available-searchBarHeight)
	}
	body := m.chatContent()
	rows := chatRows(body, width)
	if c.reading && c.viewWidth != 0 && c.viewWidth != width {
		for i, row := range rows {
			if row.line > c.anchor.line || row.line == c.anchor.line && row.column > c.anchor.column {
				break
			}
			c.scroll = i
		}
	}
	last := max(0, len(rows)-contentHeight)
	if !c.reading {
		c.scroll = last
	}
	c.scroll = max(0, min(last, c.scroll))
	c.viewRows, c.viewHeight, c.viewWidth = len(rows), contentHeight, width
	if search != nil {
		body = m.findText(2, body, width, available, c.scroll)
	} else {
		lines := make([]string, 0, contentHeight)
		source := strings.Split(body, "\n")
		for _, row := range rows[c.scroll:min(len(rows), c.scroll+contentHeight)] {
			// Retain the original style when the viewport starts inside a wrapped line.
			lines = append(lines, ansi.Cut(source[row.line], row.column, row.column+ansi.StringWidth(row.text)))
		}
		body = strings.Join(lines, "\n")
	}
	c.anchor = rows[min(c.scroll, len(rows)-1)]
	view := heading + "\n" + body
	if statusHeight > 0 {
		status := "Latest · ↑↓ scroll"
		if m.chatEditing {
			status = "Latest · PgUp scroll"
		}
		if c.reading {
			status = "Earlier · End latest"
			if m.chatEditing {
				status = "Earlier · PgDn newer"
			} else if width >= 32 {
				status = fmt.Sprintf("%d-%d/%d · End latest", c.scroll+1, min(len(rows), c.scroll+contentHeight), len(rows))
			}
		}
		view += "\n" + changedStyle.Render(clipped(status, width))
	}
	return styledView(view, width, height)
}
