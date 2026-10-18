package monitor

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/ui"
)

func HelmTimelineBody(h *HelmState, width, height int) string {
	if height <= 0 {
		return ""
	}
	t := h.Timeline
	head := ui.TitleStyle.Render(ui.Clipped(ui.OneLine(t.Release.Name), width)) + "\n"
	if height >= 12 {
		head += ui.Muted.Render(ui.Clipped("Namespace: "+ui.OneLine(t.Release.Namespace), width)) + "\n\n"
	}
	if height >= 8 {
		changeLabel := "Full changes"
		if c := t.Entries[t.Selected()].Changes; c != nil && c.Unavailable != "" {
			changeLabel = "Change summary"
		}
		buttons := []string{ui.ActionButton("Enter", "Inspect chart", false), ui.ActionButton("v", changeLabel, false)}
		if t.Selected() < t.Release.Revision && t.Selected() > 0 {
			buttons = append(buttons, ui.ActionButton("B", "Preview rollback", false))
		}
		head += strings.Join(ui.ActionButtonRows(buttons, width), "\n") + "\n\n"
	}
	if h.Find != nil {
		head += h.FindBar(width) + "\n"
	}
	headLines := strings.Split(strings.TrimSuffix(head, "\n"), "\n")
	head = strings.Join(headLines[:min(len(headLines), height-1)], "\n")
	if head != "" {
		head += "\n"
	}
	available := max(1, height-strings.Count(head, "\n"))
	if len(t.Revisions) == 0 {
		return head + ui.Clipped(ui.Muted.Render("No retained revisions."), width)
	}
	var lines []string
	selectedLine, selectedEnd := 0, 0
	for i, r := range t.Revisions {
		marker := "  "
		if i == t.Cursor {
			selectedLine = len(lines)
			marker = "› "
		}
		label := fmt.Sprintf("%sRevision %d", marker, r.Revision)
		if i == 0 {
			label += " · latest"
		}
		row := ui.TitleStyle.Render(label) + " · " + HelmCellStyle("Status", r.Status).Render(ui.OneLine(r.Status))
		row = ui.Clipped(row, width)
		if i == t.Cursor {
			row = ui.Selected.Background(ui.Highlight).Width(width).Render(row)
		}
		block := []string{row}
		appendText := func(text string) {
			for _, line := range strings.Split(ansi.Wrap(text, max(1, width-2), ""), "\n") {
				block = append(block, ui.Clipped("  "+line, width))
			}
		}
		metadata := HelmRevisionTime(r.Updated)
		if r.Chart != "" {
			if metadata != "" {
				metadata += " · "
			}
			metadata += ui.OneLine(r.Chart)
		}
		if metadata != "" {
			appendText(ui.Muted.Render(metadata))
		}
		if r.Description != "" {
			appendText(ui.OneLine(r.Description))
		}
		if target, matched := h.FindTarget(); matched && target == i && i == t.Cursor {
			appendText(ui.ChangedStyle.Render("Match: " + h.FindLine()))
		}
		block = append(block, "")
		baseline := fmt.Sprintf("Changes from revision %d", r.Revision-1)
		if r.Revision == 1 {
			baseline = "Initial installation"
		}
		appendText(ui.Muted.Render(baseline))
		entry, ok := t.Entries[r.Revision]
		switch {
		case !ok:
			label := "Select to load recorded changes"
			if i >= t.Cursor && i < t.Cursor+3 {
				label = "Loading recorded changes…"
			}
			appendText(ui.Muted.Render(label))
		case entry.Err != "":
			appendText(ui.ChangedStyle.Render(ui.OneLine(entry.Err)))
		case entry.Changes != nil:
			for _, line := range HelmRevisionSummary(entry.Changes, i == t.Cursor) {
				appendText(line)
			}
		}
		for j := range block {
			block[j] = ui.MouseTarget(block[j], fmt.Sprintf("helmrevision/%d", r.Revision))
		}
		lines = append(lines, block...)
		if i == t.Cursor {
			selectedEnd = len(lines)
		}
		lines = append(lines, "", "")
	}
	// Paging stays inside the selected revision. Its identity remains visible above the body.
	scroll := min(max(0, t.Scroll), max(0, selectedEnd-selectedLine-available))
	if _, loaded := t.Entries[t.Selected()]; loaded {
		t.Scroll = scroll
	}
	if available == 1 {
		return head + lines[selectedLine]
	}
	start := selectedLine + 1 + scroll
	return head + lines[selectedLine] + "\n" + strings.Join(lines[start:min(len(lines), start+available-1)], "\n")
}

func HelmRevisionTime(value string) string {
	if stamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return stamp.Format("2006-01-02 15:04 -07:00")
	}
	return ui.OneLine(value)
}

func HelmRevisionSummary(c *helmtool.RevisionChanges, expanded bool) []string {
	var lines []string
	for _, group := range []string{"resources", "hooks"} {
		counts := map[string]int{}
		for _, change := range c.Changes {
			if change.Group == group {
				counts[change.Action]++
			}
		}
		var parts []string
		for _, action := range []string{"added", "changed", "removed"} {
			if counts[action] > 0 {
				parts = append(parts, HelmRevisionChangeStyle(action).Render(fmt.Sprintf("%d %s", counts[action], action)))
			}
		}
		if len(parts) > 0 {
			label := "Resources"
			if group == "hooks" {
				label = "Hooks"
			}
			lines = append(lines, label+"  "+strings.Join(parts, ui.Muted.Render(" · ")))
		}
	}
	var inputs []string
	objects := 0
	for _, change := range c.Changes {
		if change.Group != "resources" && change.Group != "hooks" {
			inputs = append(inputs, ui.OneLine(change.Label))
		} else {
			objects++
		}
	}
	if len(inputs) > 0 {
		lines = append(lines, "Inputs     "+strings.Join(inputs, " · "))
	}
	if len(c.Changes) == 0 {
		lines = append(lines, ui.Muted.Render("No recorded configuration changes."))
	}
	if expanded && objects > 0 {
		lines = append(lines, "")
		shown := 0
		for _, change := range c.Changes {
			if change.Group != "resources" && change.Group != "hooks" {
				continue
			}
			if shown == 3 {
				break
			}
			mark := map[string]string{"added": "+", "changed": "~", "removed": "−"}[change.Action]
			lines = append(lines, HelmRevisionChangeStyle(change.Action).Render(mark)+" "+ui.OneLine(change.Label))
			shown++
		}
		if objects > shown {
			lines = append(lines, ui.Muted.Render(fmt.Sprintf("… %d more", objects-shown)))
		}
	}
	if c.Unavailable != "" {
		lines = append(lines, ui.Muted.Render("Full diff too large; object summary available."))
	}
	return lines
}

func HelmRevisionChangeStyle(action string) lipgloss.Style {
	switch action {
	case "added":
		return ui.AddedStyle
	case "removed":
		return ui.RemovedStyle
	default:
		return ui.ChangedStyle
	}
}

func HelmRevisionChangeDetails(c *helmtool.RevisionChanges) string {
	var lines []string
	for _, change := range c.Changes {
		lines = append(lines, change.Action+" · "+change.Label)
	}
	if c.Unavailable != "" {
		lines = append(lines, "", c.Unavailable)
	}
	return strings.Join(lines, "\n")
}
