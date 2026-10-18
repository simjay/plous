package activity

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
)

func Timeline(entries []ActivityEntry, selected, width, height int) string {
	if height <= 0 || len(entries) == 0 {
		return ""
	}
	selected = max(0, min(selected, len(entries)-1))
	start, used := selected, 3
	for start > 0 {
		extra := 2
		if day(entries[start-1]) != day(entries[start]) {
			extra++
		}
		if used+extra > height {
			break
		}
		start--
		used += extra
	}
	lines := []string{}
	previous := ""
	for i := start; i < len(entries) && len(lines) < height; i++ {
		e := entries[i]
		date := day(e)
		if date != previous {
			if height-len(lines) >= 2 {
				lines = append(lines, ui.Muted.Bold(true).Render(ui.Clipped(date, width)))
			} else if i > start {
				break
			}
		}
		previous = date
		target := "activity/" + e.Identity()
		lines = append(lines, ui.MouseTarget(timelineRow(e, i == selected, width), target))
		if len(lines) < height {
			metadata := "        " + e.Metadata()
			if width < 46 {
				clock := "--:--"
				if !e.At.IsZero() {
					clock = e.At.Local().Format("15:04")
				}
				metadata = "  " + clock + " · " + e.Outcome() + " · " + e.Metadata()
			}
			lines = append(lines, ui.MouseTarget(ui.Muted.Render(ui.Clipped(metadata, width)), target))
		}
	}
	return strings.Join(lines, "\n")
}

func day(e ActivityEntry) string {
	if e.At.IsZero() {
		return "Date unavailable"
	}
	return e.At.Local().Format("Mon, Jan 2 · 2006")
}

func timelineRow(e ActivityEntry, selected bool, width int) string {
	prefix, style := "  ", lipgloss.NewStyle()
	if selected {
		prefix, style = "› ", ui.Selected.Background(ui.Highlight)
	}
	label := e.Label()
	if e.Change != nil {
		mark, tone := ChangeAppearance(*e.Change)
		label = tone.Render(mark) + " " + label
	}
	if width < 46 {
		return style.Width(width).Render(ui.Clipped(prefix+label, width))
	}
	clock := "--:--"
	if !e.At.IsZero() {
		clock = e.At.Local().Format("15:04")
	}
	titleWidth := max(1, width-8)
	outcome := ""
	if width >= 46 {
		label := e.Outcome()
		titleWidth -= ansi.StringWidth(label) + 2
		tone := ui.Muted
		switch label {
		case "failed", "error":
			tone = ui.RemovedStyle
		case "unknown", "pending-install", "pending-upgrade", "pending-rollback":
			tone = ui.ChangedStyle
		case "saved", "completed", "deployed":
			tone = ui.AddedStyle
		}
		outcome = "  " + tone.Render(label)
	}
	title := ui.Clipped(label, titleWidth)
	title += strings.Repeat(" ", max(0, titleWidth-ansi.StringWidth(title)))
	return style.Width(width).Render(prefix + clock + " " + title + outcome)
}
