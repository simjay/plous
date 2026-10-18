package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/simjay/plous/internal/ui"
	"strings"
)

func HelmChartColumnsForWidth(width int) int { return max(1, min(3, (width+1)/34)) }

// Each card has four content rows and two border rows. Measure the grid before
// styling it so navigation cost depends on the viewport, not release count.
const HelmChartCardHeight = 6

func HelmChartCards(h *HelmState, width, height int) string {
	if height <= 0 {
		return ""
	}
	groups := h.ChartGroups()
	if len(groups) == 0 {
		return ui.Muted.Render("No releases match this scope or filter.")
	}
	if h.Inventory.Headings == nil || h.Inventory.HeadingWidth != width {
		h.Inventory.Headings = make([][]string, len(groups))
		for i, group := range groups {
			h.Inventory.Headings[i] = strings.Split(SummaryHeading(ui.OneLine(group.Key)+fmt.Sprintf(" · %d %s", len(group.Releases), PluralReleases(len(group.Releases))), width), "\n")
		}
		h.Inventory.HeadingWidth = width
	}
	headings := h.Inventory.Headings
	columns := HelmChartColumnsForWidth(width)
	cardWidth := max(1, (width-columns+1)/columns)
	selectedLine, selectedEnd, selectedGroupLine := 0, 0, 0
	selectedGroup := 0
	total, count := 0, 0
	for i, group := range groups {
		if h.Cursor >= count && h.Cursor < count+len(group.Releases) {
			selectedGroup, selectedGroupLine = i, total
			selectedLine = total + len(headings[i]) + (h.Cursor-count)/columns*HelmChartCardHeight
			selectedEnd = selectedLine + HelmChartCardHeight
		}
		total += len(headings[i]) + 1 + (len(group.Releases)+columns-1)/columns*HelmChartCardHeight
		count += len(group.Releases)
	}
	available := max(1, height-1)
	start := min(max(0, selectedEnd-available), selectedLine)
	prefix := ""
	if start > selectedGroupLine && available > 2 {
		pinned := min(len(headings[selectedGroup]), max(1, available-2))
		prefix = strings.Join(headings[selectedGroup][:pinned], "\n") + "\n"
		available -= pinned
		start = min(max(start, selectedEnd-available), selectedLine)
	}
	end := min(total, start+available)
	var lines []string
	offset, index := 0, 0
	for groupIndex, group := range groups {
		if offset >= end {
			break
		}
		for _, line := range headings[groupIndex] {
			if offset >= start && offset < end {
				lines = append(lines, line)
			}
			offset++
		}
		for row := 0; row < len(group.Releases); row += columns {
			limit := min(row+columns, len(group.Releases))
			if offset < end && offset+HelmChartCardHeight > start {
				var cards []string
				for j := row; j < limit; j++ {
					r := group.Releases[j]
					color, style, marker := ui.BorderColor, ui.TitleStyle, ""
					if index+j-row == h.Cursor {
						color, style, marker = ui.FocusedBorder, ui.Selected, "› "
					}
					inner := max(1, cardWidth-4)
					content := style.Render(ui.Clipped(marker+ui.OneLine(r.Name), inner)) + "\n" + ui.Muted.Render(ui.Clipped(ui.OneLine(r.Namespace), inner)) + "\n" + ui.Clipped(fmt.Sprintf("%s · Revision %d", HelmChartVersion(r.Version), r.Revision), inner) + "\n" + HelmCellStyle("Status", r.Status).Render(ui.Clipped(ui.OneLine(r.Status), inner))
					cards = append(cards, ui.MouseTarget(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(cardWidth).Render(content), "helmrow/"+r.Key()))
				}
				rendered := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, IntersperseCards(cards)...), "\n")
				lines = append(lines, rendered[max(0, start-offset):min(len(rendered), end-offset)]...)
			}
			index += limit - row
			offset += HelmChartCardHeight
		}
		if offset >= start && offset < end {
			lines = append(lines, "")
		}
		offset++
	}
	chartLabel := "charts"
	if len(groups) == 1 {
		chartLabel = "chart"
	}
	summary := fmt.Sprintf("%d %s · %d %s", len(groups), chartLabel, count, PluralReleases(count))
	return ui.StyledView(prefix+strings.Join(lines, "\n")+"\n"+ui.Muted.Render(ui.Clipped(summary, width)), width, height)
}

func PluralReleases(count int) string {
	if count == 1 {
		return "release"
	}
	return "releases"
}

func HelmChartVersion(version string) string {
	if version == "" || version == "<nil>" {
		return "Version unavailable"
	}
	return "v" + strings.TrimPrefix(ui.OneLine(version), "v")
}

func IntersperseCards(cards []string) []string {
	var result []string
	for i, card := range cards {
		if i > 0 {
			result = append(result, " ")
		}
		result = append(result, card)
	}
	return result
}
