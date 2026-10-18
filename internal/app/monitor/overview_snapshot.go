package monitor

import (
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
)

func overviewCounterLabel(label string) string {
	switch label {
	case "HelmReleases":
		return "Helm releases"
	case "OutOfSync":
		return "Out of sync"
	case "NotReady":
		return "Not ready"
	}
	return label
}

func overviewCounterText(c OverviewCounter) string {
	label := overviewCounterLabel(c.Label)
	if c.Label != "HelmReleases" {
		label = strings.ToLower(label)
	}
	if c.Value == "1" && strings.HasSuffix(label, "s") {
		label = strings.TrimSuffix(label, "s")
	}
	return strings.TrimSuffix(c.Value, " (partial)") + " " + label
}

func overviewConditionCounter(c OverviewCounter) bool {
	switch c.Label {
	case "Failed", "Pending", "Degraded", "OutOfSync", "NotReady":
		return true
	}
	return false
}

func overviewSnapshotCounter(c OverviewCounter, primary, selected bool) string {
	style := ui.SectionHeadingStyle.Bold(false)
	value := overviewCounterText(c)
	switch {
	case c.Value == "?" || c.Value == "Unavailable":
		style = ui.ChangedStyle
	case c.Value == "0" || c.Value == "0+":
		style = ui.Muted
	case overviewConditionCounter(c):
		style = ui.ChangedStyle.Bold(true)
		marker := "! "
		if c.Label == "Failed" || c.Label == "Degraded" || c.Label == "NotReady" {
			style = ui.RemovedStyle.Bold(true)
			marker = "× "
		}
		value = marker + value
	case primary:
		style = ui.TitleStyle.Bold(true)
	}
	if selected {
		value = "› " + value
		style = style.Background(ui.Highlight).Underline(true)
	}
	return style.Render(value)
}

// Keep count and noun together whenever the available width can hold them.
func overviewSnapshotLines(parts []string, width int) []string {
	lines := []string{}
	line := ""
	for _, part := range parts {
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(part) <= width {
			line += ui.Muted.Render(" · ") + part
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		wrapped := strings.Split(ansi.Wrap(part, max(1, width), ""), "\n")
		lines = append(lines, wrapped[:len(wrapped)-1]...)
		line = wrapped[len(wrapped)-1]
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func overviewSnapshotAvailability(row OverviewRow, now time.Time) []string {
	var badges []string
	if row.Status != "" {
		badges = append(badges, row.Status)
	}
	if row.At.IsZero() {
		badges = append(badges, "time unknown")
	} else if now.Sub(row.At) > 45*time.Second {
		badges = append(badges, "stale")
	}
	return badges
}

func overviewCompactIntegration(row OverviewRow, width int, selected bool, counter int, now time.Time) []string {
	brand := overviewBrand(row.Target.Tool)
	if brand == "" {
		return []string{ui.Muted.Render(ui.Clipped(row.Title+" · Enter details", width))}
	}
	marker, style := " ", ui.SectionHeadingStyle.Bold(true)
	if selected {
		marker, style = "▸", ui.OverlayTitleStyle
	}
	heading := style.Render(marker + brand)
	var parts []string
	for i, c := range row.Counters {
		active := selected && counter == i+1
		if i == 0 || active || overviewConditionCounter(c) && c.Value != "0" && c.Value != "0+" {
			parts = append(parts, overviewSnapshotCounter(c, i == 0, active))
		}
	}
	for _, badge := range overviewSnapshotAvailability(row, now) {
		parts = append(parts, ui.ChangedStyle.Render(badge))
	}
	// Wrap the facts as a group before shortening resource names or losing conditions.
	lines := overviewSnapshotLines(parts, max(1, width-2))
	if len(lines) == 0 {
		lines = []string{heading}
	} else if lipgloss.Width(heading)+2+lipgloss.Width(lines[0]) <= width {
		lines[0] = heading + "  " + lines[0]
		for i := 1; i < len(lines); i++ {
			lines[i] = "  " + lines[i]
		}
	} else {
		for i := range lines {
			lines[i] = "  " + lines[i]
		}
		lines = append([]string{heading}, lines...)
	}
	if selected {
		for i, line := range lines {
			lines[i] = lipgloss.NewStyle().Background(ui.Highlight).Width(width).Render(line)
		}
	}
	return lines
}

func overviewIntegration(row OverviewRow, width int, selected bool, counter int, now time.Time) []string {
	brand := overviewBrand(row.Target.Tool)
	if brand == "" {
		return []string{ui.TitleStyle.Render(overviewIdentity(row.Title, width)), ui.Muted.Render(ui.Clipped(row.Detail, width))}
	}
	art, color := overviewSnapshotArt(row.Target.Tool)
	inner := max(1, width)
	offset := lipgloss.Width(strings.Join(art, "\n")) + 1
	if inner-offset < 25 {
		offset = 0
	}
	textWidth := inner - offset
	var primary, inventory, conditions []string
	for i, c := range row.Counters {
		part := overviewSnapshotCounter(c, i == 0, selected && counter == i+1)
		switch {
		case i == 0:
			primary = append(primary, part)
		case overviewConditionCounter(c):
			conditions = append(conditions, part)
		default:
			inventory = append(inventory, part)
		}
	}
	text := overviewSnapshotLines(primary, textWidth)
	text = append(text, overviewSnapshotLines(inventory, textWidth)...)
	conditionLines := overviewSnapshotLines(conditions, textWidth)
	// Reserve a consistent bottom row for conditions beside the four-row artwork.
	for len(text)+len(conditionLines) < 4 {
		text = append(text, "")
	}
	text = append(text, conditionLines...)
	if len(row.Counters) == 0 {
		text = strings.Split(ansi.Wrap(row.Detail, textWidth, ""), "\n")
	}
	body := make([]string, max(4, len(text)))
	for i := range body {
		if offset > 0 {
			piece := ""
			if i < len(art) {
				piece = art[i]
			}
			body[i] = lipgloss.NewStyle().Width(offset).Render(color.Render(piece))
		}
		if i < len(text) {
			body[i] += text[i]
		}
	}
	marker, headingStyle := " ", ui.TitleStyle.Bold(true)
	if selected {
		marker, headingStyle = "▸", ui.OverlayTitleStyle
	}
	heading := headingStyle.Render(marker + " " + brand)
	for _, badge := range overviewSnapshotAvailability(row, now) {
		heading += ui.Muted.Render(" · ") + ui.ChangedStyle.Render(badge)
	}
	headings := strings.Split(ansi.Wrap(heading, width, ""), "\n")
	last := len(headings) - 1
	if room := width - lipgloss.Width(headings[last]) - 1; room > 0 {
		headings[last] += " " + lipgloss.NewStyle().Foreground(ui.BorderColor).Render(strings.Repeat("─", room))
	}
	return append(append(headings, body...), "")
}

func overviewSnapshotArt(tool string) ([]string, lipgloss.Style) {
	art := map[string][]string{
		"kubernetes": {
			"  ⢠⠔⠚⣩⣙⠒⢤  ",
			"  ⡎⢨⣻⣼⣼⢻⠈⡆ ",
			"  ⢧⡉⢯⣏⣯⠟⣡⠇ ",
			"   ⠑⠬⠤⠬⠔⠁  ",
		},
		"helm": {
			"   ⣤⠔⠺⠒⢦⡄  ",
			" ⢀⣰⡁HELM⣹⣀ ",
			"  ⠸⣄   ⢀⡼  ",
			"   ⠉⠓⢺⠒⠋⠁  ",
		},
		"argocd": {
			"   ⡰⠊⠉⠉⠲⡄  ",
			"   ⡇◉  ◉⢸  ",
			" ⣀⢀⡷⢤⢤⢤⢴⠻⣄⡄",
			" ⠉⠉⠤⠞⠜⠸⠘⢦⠎ ",
		},
		"flux": {
			"  ⣀⣤⣶⣿⣷⣦⣄⡀ ",
			"  ⠉⠛⠿⣿⡿⠟⠋⠉ ",
			" ⢐⣯⣀ ⢸ ⢀⣨⣗ ",
			" ⠈⠑⠲⠭⣺⡫⠵⠒⠉ ",
		},
	}[tool]
	color := lipgloss.NewStyle().Foreground(lipgloss.Color("#67B7F7"))
	if tool == "helm" {
		color = lipgloss.NewStyle().Foreground(lipgloss.Color("#B99AFF"))
	}
	if tool == "argocd" {
		color = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF956B"))
	}
	if tool == "flux" {
		color = lipgloss.NewStyle().Foreground(ui.DashboardAccent).Bold(true)
	}
	return art, color
}
