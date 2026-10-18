package monitor

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/simjay/plous/internal/ui"
	"strings"
	"time"
)

func overviewSource(source string) string {
	if source == "Kubernetes" {
		return "K8s"
	}
	if source == "Argo CD" {
		return "Argo"
	}
	return source
}

func (s *OverviewState) rowLines(row OverviewRow, pane, width int, selected bool, now time.Time) []string {
	marker := " "
	titleStyle := ui.TitleStyle
	if selected {
		marker = "▸"
		titleStyle = ui.OverlayTitleStyle
	}
	identity := titleStyle.Render(overviewIdentity(row.Title, max(1, width-2)))
	source := overviewSource(row.Source)
	var lines []string
	switch pane {
	case 1:
		status, style := overviewStatus(row)
		age := overviewAge(row.At, now)
		if age == "time unknown" {
			age = "?"
		}
		prefix := marker + fmt.Sprintf("%-4s ", age)
		revision := row.Revision
		if revision != "" && (row.Source == "Kubernetes" || row.Source == "Helm") {
			revision = "r" + revision
		}
		meta := source + " · " + row.Target.Namespace + " · " + row.Action
		if revision != "" {
			meta += " · " + revision
		}
		if row.Historical {
			meta = "History · " + meta
		}
		if width < 28 {
			lines = []string{marker + titleStyle.Render(ui.OneLine(row.Title)), style.Render(status), ui.Muted.Render(ui.OneLine(meta)), ui.Muted.Render(age)}
		} else if width >= 34 {
			nameWidth := max(7, width-lipgloss.Width(prefix)-lipgloss.Width(status)-2)
			lines = []string{overviewPair(ui.Muted.Render(prefix)+titleStyle.Render(overviewIdentity(row.Title, nameWidth)), style.Render(status), width), s.deploymentMeta(row, width, revision)}
		} else {
			lines = []string{ui.Muted.Render(prefix) + titleStyle.Render(overviewIdentity(row.Title, max(1, width-6))), overviewPair("      "+style.Render(status), ui.Muted.Render(overviewIdentity(revision, 10)), width), ui.Muted.Render("      " + overviewIdentity(meta, max(1, width-6)))}
		}
	case 2:
		_, style := overviewStatus(row)
		reason := row.Reason
		if reason == "" {
			reason = row.Detail
		}
		reason = strings.TrimPrefix(reason, "Error · ")
		age := overviewAge(row.At, now)
		if age == "time unknown" {
			age = "? age"
		}
		symbol := "! "
		if row.Severity == 2 {
			symbol = "× "
		}
		if row.NewFailures > 0 && !s.Retained && s.Error == "" && overviewDeltaCurrent(s.ComparedAt, now) {
			age = "NEW " + age
		}
		heading := overviewPair(marker+style.Bold(true).Render(symbol+ui.Clipped(ui.OneLine(reason), max(1, width-lipgloss.Width(age)-4))), ui.Muted.Render(age), width)
		scope := source + " · " + row.Target.Namespace
		if row.Target.Namespace == "" {
			scope = source + " · cluster"
		}
		if len(row.Members) > 1 {
			scope = fmt.Sprintf("%d affected · %s", len(row.Members), scope)
		}
		lines = []string{heading, "  " + identity, ui.Muted.Render("  " + overviewIdentity(scope, width-2))}
		if width < 28 {
			lines = []string{marker + style.Bold(true).Render(symbol+ui.OneLine(reason)), titleStyle.Render(ui.OneLine(row.Title)), ui.Muted.Render(scope), ui.Muted.Render(age)}
		}
	case 4:
		status, style := overviewStatus(row)
		lines = []string{overviewPair(marker+identity, style.Render(status), width), ui.Muted.Render("  " + overviewSource(row.Source) + " · " + row.Target.Namespace)}
	case 3:
		if row.ID == "coverage" {
			lines = []string{titleStyle.Render(marker + " Availability"), ui.Muted.Render(fmt.Sprintf("  %d notices · Enter details", len(s.notices())))}
			for _, note := range s.notices() {
				short, _, _ := strings.Cut(note, ";")
				short = strings.ReplaceAll(short, " APIs not detected in this cluster", " · not detected")
				short = strings.ReplaceAll(short, " API not detected here", " · not detected")
				short = strings.ReplaceAll(short, "Network: no telemetry provider configured", "Network · unavailable")
				lines = append(lines, ui.Muted.Render("  "+ui.Clipped(short, width-2)))
				if len(lines) == 5 {
					break
				}
			}
		} else {
			return overviewIntegration(row, width, selected, s.Counter, now)
		}
	}
	if width < 28 {
		lines = ui.MeasureViewport(strings.Join(lines, "\n"), width, 1).Rows
	}
	if selected {
		for i, line := range lines {
			lines[i] = lipgloss.NewStyle().Background(ui.Highlight).Width(width).Render(ui.Clipped(line, width))
		}
	}
	return lines
}

func (s *OverviewState) deploymentMeta(row OverviewRow, width int, revision string) string {
	// Reserve revision and source independently from the workload identity.
	source := overviewSource(row.Source)
	if row.Historical {
		source = "History/" + source
	}
	prefix := "      "
	revisionWidth := min(12, max(5, (width-6)/4))
	actionWidth, sourceWidth := 9, 4
	for _, entry := range s.Filtered(1) {
		if entry.Historical {
			sourceWidth = 11
			break
		}
	}
	available := width - 6 - revisionWidth - actionWidth - sourceWidth - 3
	if available < 4 {
		return ui.Muted.Render(ui.Clipped(prefix+source+" · "+overviewIdentity(row.Target.Namespace, max(4, width-6-sourceWidth-revisionWidth-6))+" · "+revision, width))
	}
	field := func(v string, w int) string {
		text := overviewIdentity(v, w)
		return text + strings.Repeat(" ", max(0, w-lipgloss.Width(text)))
	}
	return ui.Muted.Render(prefix + field(source, sourceWidth) + " " + field(row.Target.Namespace, available) + " " + field(row.Action, actionWidth) + " " + field(revision, revisionWidth))
}

func overviewBrand(tool string) string {
	return map[string]string{"kubernetes": "Kubernetes", "helm": "Helm", "argocd": "Argo CD", "flux": "Flux"}[tool]
}
