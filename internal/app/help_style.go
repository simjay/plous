package app

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	helpview "github.com/simjay/plous/internal/ui/help"
	"strings"
)

// Style before wrapping so continuation lines preserve their semantic role.
func helpLines(s helpview.Section, width int) []string {
	width = max(1, width)
	var lines []string
	wrap := func(text string) { lines = append(lines, strings.Split(ansi.Wrap(text, width, ""), "\n")...) }
	entry := func(key, label, reason string) {
		if reason != "" {
			label += " (" + reason + ")"
		}
		keyWidth := ansi.StringWidth(key)
		column := 8
		if width < 28 {
			column = 6
		}
		if keyWidth+2 >= width/2 {
			wrap(ui.ShortcutStyle.Render(key))
			for _, line := range strings.Split(ansi.Wrap(label, max(1, width-2), ""), "\n") {
				lines = append(lines, "  "+line)
			}
		} else {
			column = max(column, keyWidth+2)
			for i, line := range strings.Split(ansi.Wrap(label, max(1, width-column), ""), "\n") {
				prefix := strings.Repeat(" ", column)
				if i == 0 {
					prefix = ui.ShortcutStyle.Render(key) + strings.Repeat(" ", column-keyWidth)
				}
				lines = append(lines, prefix+line)
			}
		}
	}
	for _, g := range s.Groups {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		if g.Title != "" {
			wrap(ui.SectionHeadingStyle.Render(g.Title))
		}
		for _, e := range g.Entries {
			entry(e.Key, e.Label, e.Reason)
		}
		if g.Note != "" {
			wrap(ui.Muted.Render(g.Note))
		}
	}
	var alternatives []helpview.Entry
	for _, g := range s.Groups {
		for _, e := range g.Entries {
			if len(e.Alternatives) > 0 {
				alternatives = append(alternatives, e)
			}
		}
	}
	if len(alternatives) > 0 {
		lines = append(lines, "")
		wrap(ui.SectionHeadingStyle.Render("Alternative keys"))
		for _, e := range alternatives {
			entry(strings.Join(e.Alternatives, " / "), e.Label, "")
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}
