package code

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ui"
	"strings"
)

func CodeValidationRow(severity, message, where string, width int) string {
	locationWidth := max(16, width/4)
	findingWidth := max(1, width-locationWidth-12)
	pad := func(s string, n int) string {
		v := ui.Clipped(s, n)
		return v + strings.Repeat(" ", max(0, n-ansi.StringWidth(v)))
	}
	return pad(severity, 9) + "  " + pad(message, findingWidth) + " " + pad(where, locationWidth)
}

func (s *CodeState) SelectValidationView(view string) {
	if view != "Findings" && view != "Images" && view != "Generators" {
		return
	}
	if s.Filters == nil {
		s.Filters, s.Cursors = map[string]string{}, map[string]int{}
	}
	previous := s.ValidationView
	if previous == "" {
		previous = "Findings"
	}
	s.Filters["validate/"+previous], s.Cursors["validate/"+previous] = s.Filter, s.Cursor
	s.ValidationView = view
	s.Filter, s.Cursor = s.Filters["validate/"+view], s.Cursors["validate/"+view]
	s.DetailOffset, s.InspectorArea = 0, 0
}

func (s *CodeState) ValidationSourceItems() []string {
	items := []string{"All Sources"}
	entries := s.NavigationEntries()
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Path]++
	}
	for _, e := range entries {
		label := e.Path
		if counts[e.Path] > 1 {
			label += " · " + e.Kind
		}
		items = append(items, label)
	}
	return items
}

func (s *CodeState) ValidationBody(width, height int) string {
	counts := map[string]int{}
	for _, e := range s.Inventory.Entries {
		if e.Component {
			continue
		}
		status := s.Status(e.ID)
		switch status {
		case "failed", "stale", "not rendered":
			counts[status]++
		default:
			counts["rendered"]++
		}
	}
	for _, f := range s.AllFindings() {
		if s.ValidationSource == "" || f.Target == s.ValidationSource {
			counts[f.Severity]++
		}
	}
	summary := fmt.Sprintf("Repository: %d rendered · %d failed · %d stale · %d not rendered\n%d errors · %d warnings · %d info · %d omitted paths", counts["rendered"], counts["failed"], counts["stale"], counts["not rendered"], counts["error"], counts["warning"], counts["info"], len(s.Inventory.Source.Gaps))
	if len(s.Inventory.Terraform) > 0 {
		summary += fmt.Sprintf("\n%d unsupported Terraform directories", len(s.Inventory.Terraform))
	}
	if counts["failed"]+counts["not rendered"]+counts["stale"]+len(s.Inventory.Source.Gaps)+len(s.Inventory.Terraform) > 0 {
		summary += "\nCoverage is incomplete."
	}
	if len(s.Inventory.Entries) == 0 {
		summary += "\nNo supported configuration detected. Files remains available."
	}
	view := s.ValidationView
	if view == "" {
		view = "Findings"
	}
	header := ui.Muted.Render(ansi.Wrap(summary, width, "")) + "\n"
	if view != "Findings" {
		header = ui.TitleStyle.Render(view) + "\n" + header
	}
	if s.ValidationView == "" || s.ValidationView == "Findings" {
		category := s.ValidationCategory
		if category == "" {
			category = "All"
		}
		header += ui.MouseTarget(ui.Muted.Render("Category: "+category+" · a Filter Category"), "code-validation/category") + "\n"
	} else if s.ValidationView == "Images" {
		header += ui.Muted.Render(ansi.Wrap("Images from current successful renders. a More Tools: Validate All Entry Points", width, "")) + "\n"
	} else {
		header += ui.Muted.Render(ansi.Wrap("Declared generators · keys only · values hidden", width, "")) + "\n"
	}

	sourceLabel := "All Sources"
	if s.ValidationSource != "" {
		if e := s.Inventory.Entry(s.ValidationSource); e != nil {
			sourceLabel = e.Path
		} else {
			sourceLabel = "Unavailable source"
		}
	}
	header += ui.MouseTarget(ui.Muted.Render(ui.Clipped("Source: "+sourceLabel+" · a Filter Source", width)), "code-validation/source") + "\n"
	if width < 66 || strings.Count(header, "\n") > max(3, height/2) {
		view := s.ValidationView
		if view == "" {
			view = "Findings"
		}
		label := view + " · a Options"
		if s.ValidationSource != "" {
			label = view + " · " + sourceLabel
		}
		header = ""
		if view != "Findings" {
			header = ui.TitleStyle.Render(ui.Clipped(label, width)) + "\n"
		}
		for _, row := range []string{
			fmt.Sprintf("Repo: %d rendered · %d failed", counts["rendered"], counts["failed"]),
			fmt.Sprintf("%d stale · %d not rendered", counts["stale"], counts["not rendered"]),
			fmt.Sprintf("%d omitted · %d unsupported", len(s.Inventory.Source.Gaps), len(s.Inventory.Terraform)),
		} {
			header += ui.Muted.Render(ui.Clipped(row, width)) + "\n"
		}
		if s.ValidationCategory != "" && s.ValidationCategory != "All" && view == "Findings" {
			header += ui.MouseTarget(ui.Muted.Render(ui.Clipped("Category: "+s.ValidationCategory, width)), "code-validation/category") + "\n"
		}
		if len(s.Inventory.Entries) == 0 {
			header = ui.Muted.Render(ui.Clipped("No configurations · Files available", width)) + "\n"
		}
	}
	findings := s.Findings()
	if len(findings) == 0 {
		message := "No findings in completed analysis."
		if s.ValidationView == "Images" {
			message = "No images in current successful renders."
		}
		if s.ValidationView == "Generators" {
			message = "No supported generator declarations in the captured source."
		}
		if counts["not rendered"] > 0 {
			message = "Analysis is incomplete. Use Actions → More Tools → Validate All Entry Points."
		}
		if s.ValidationSource != "" && s.Status(s.ValidationSource) != "not rendered" && s.Status(s.ValidationSource) != "stale" {
			message = "No findings for this entry in completed analysis."
			if s.ValidationView == "Images" {
				message = "No images for this entry in current output."
			} else if s.ValidationView == "Generators" {
				message = "No supported generators declared by this entry."
			}
		}
		if s.Filter != "" || s.ValidationCategory != "" && s.ValidationCategory != "All" && (s.ValidationView == "" || s.ValidationView == "Findings") {
			message = "No findings match this filter."
		}
		return header + ansi.Wrap(message, width, "")
	}
	available := max(1, height-strings.Count(ansi.Strip(header), "\n"))
	detailHeight := max(3, min(8, available/3))
	tableHeight := max(1, available-detailHeight-2)
	rows := []string{}
	locationWidth := max(16, width/4)
	findingWidth := width - locationWidth - 12
	wide := width >= 66
	if wide {
		first, second := "SEVERITY", "FINDING"
		if s.ValidationView == "Images" {
			first, second = "STATUS", "IMAGE"
		} else if s.ValidationView == "Generators" {
			first, second = "TYPE", "GENERATOR"
		}
		header += ui.Muted.Render(CodeValidationRow(first, second, "WHERE", width)) + "\n"
		tableHeight = max(1, tableHeight-1)
	}
	if len(findings) < tableHeight {
		detailHeight += tableHeight - len(findings)
		tableHeight = max(1, len(findings))
	}
	for n, f := range findings {
		message := f.Summary
		if message == "" {
			message = f.Message
		}
		var label string
		location := f.File
		if e := s.Inventory.Entry(f.Target); e != nil {
			location = e.Path
		}
		style := ui.Muted
		if f.Severity == "error" {
			style = ui.RemovedStyle
		} else if f.Severity == "warning" || f.Severity == "mutable" {
			style = ui.ChangedStyle
		}
		marker := "  "
		if n == s.Cursor {
			marker = ui.Selected.Render("▸ ")
		}
		if wide && findingWidth > 0 {
			label = CodeValidationRow(style.Render(f.Severity), message, ui.Muted.Render(location), width-2)
		} else {
			label = style.Render(f.Severity) + " · " + message
		}
		label = marker + ui.Clipped(label, max(1, width-2))

		rows = append(rows, ui.MouseTarget(label, fmt.Sprintf("code-row/%d", n)))
	}
	if available < 6 {
		return header + ui.ScrollSelection(rows, s.Cursor, available)
	}
	f := findings[min(max(0, s.Cursor), len(findings)-1)]
	detail := ui.TitleStyle.Render(f.Severity+" · "+f.Rule) + "\n" + ansi.Wrap(f.Message, width, "") + "\n" + ui.Muted.Render(ansi.Wrap("Where: "+f.File, width, "")) + "\n" + ui.MouseTarget(ui.TitleStyle.Render("Enter Details"), "action/enter")
	if f.Target != "" {
		detail += "  " + ui.MouseTarget(ui.TitleStyle.Render("Entry Details"), "code-finding/sources") + "  " + ui.MouseTarget(ui.TitleStyle.Render("Open Render"), "code-finding/render")
		if f.Field != "" {
			detail += "  " + ui.MouseTarget(ui.TitleStyle.Render("Trace Field"), "code-finding/trace")
		}
	}
	detail = ansi.Wrap(detail, width, "")
	lines := strings.Split(detail, "\n")
	start := min(s.DetailOffset, max(0, len(lines)-detailHeight))
	s.DetailOffset = start
	detail = strings.Join(lines[start:min(len(lines), start+detailHeight)], "\n")
	return header + ui.ScrollSelection(rows, s.Cursor, tableHeight) + "\n" + ui.Muted.Render(strings.Repeat("─", width)) + "\n" + detail
}
