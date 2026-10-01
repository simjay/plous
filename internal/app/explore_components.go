package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

func resourceTableColumns(columns []string, rows []kubernetes.Row, indices []int, width int, pod bool) ([]int, []int, bool) {
	if pod && width < 110 {
		keep := indices[:0]
		for _, index := range indices {
			if columns[index] != "Node" && columns[index] != "Pod IP" {
				keep = append(keep, index)
			}
		}
		indices = keep
	}
	widths, minimum := make([]int, len(indices)), make([]int, len(indices))
	total, least := max(0, len(indices)-1)*2, max(0, len(indices)-1)*2
	for i, index := range indices {
		minimum[i] = max(3, ansi.StringWidth(columns[index]))
		if columns[index] == "Name" {
			minimum[i] = 24
		}
		if pod && columns[index] == "Status" {
			minimum[i] = 18
		}
		widths[i] = minimum[i]
		for _, row := range rows {
			if index < len(row.Cells) {
				widths[i] = max(widths[i], min(64, ansi.StringWidth(oneLine(row.Cells[index]))))
			}
		}
		if pod && columns[index] == "Status" {
			minimum[i] = widths[i]
		}
		total += widths[i]
		least += minimum[i]
	}
	// Preserve useful identity and health columns before secondary wide-view
	// fields. Every field remains available in the structured details.
	for least > width && len(indices) > 3 {
		drop := -1
		for i := len(indices) - 1; i >= 0; i-- {
			column := columns[indices[i]]
			if i == 0 || i == 1 || column == "Namespace" || column == "Status" || column == "Age" || pod && (column == "Ready" || column == "Restarts") {
				continue
			}
			drop = i
			break
		}
		if drop < 0 {
			break
		}
		least -= minimum[drop] + 2
		total -= widths[drop] + 2
		indices = append(indices[:drop], indices[drop+1:]...)
		minimum = append(minimum[:drop], minimum[drop+1:]...)
		widths = append(widths[:drop], widths[drop+1:]...)
	}
	if least > width || width < 42 {
		return indices, widths, true
	}
	for total > width {
		largest := -1
		for i := range widths {
			if widths[i] > minimum[i] && (largest < 0 || widths[i]-minimum[i] > widths[largest]-minimum[largest]) {
				largest = i
			}
		}
		if largest < 0 {
			break
		}
		widths[largest]--
		total--
	}
	if len(widths) > 0 && total < width {
		widths[0] += width - total
	}
	return indices, widths, false
}

func (m *model) resourceCountSummary(rows []kubernetes.Row) string {
	e := m.explorer()
	text := fmt.Sprintf("%d %s", len(rows), resourceTitle(e.resource))
	if e.resource.Namespaced {
		ns := e.preference.Namespace
		if ns == "" {
			ns = "all namespaces"
		}
		text += " · " + oneLine(ns)
	}
	if e.resource.Key() == "pods" {
		unready, pending := 0, 0
		for _, row := range rows {
			ready, total, _, status := kubernetes.PodState(row.Object.Object)
			if status == "Pending" || status == "ContainerCreating" || status == "PodInitializing" {
				pending++
			} else if ready < total && status != "Succeeded" && status != "Completed" {
				unready++
			}
		}
		if unready > 0 {
			text += fmt.Sprintf(" · %d not ready", unready)
		}
		if pending > 0 {
			text += fmt.Sprintf(" · %d pending", pending)
		}
	}
	return text
}

func (m *model) compactResourceList(columns []string, rows []kubernetes.Row, before []string, width, height int) string {
	e := m.explorer()
	var lines []string
	selectedLine := 0
	for _, row := range rows {
		active := kubernetes.Identity(row.Object) == e.selected
		name := row.Object.GetName()
		if e.resource.Namespaced && e.preference.Namespace == "" {
			name = row.Object.GetNamespace() + "/" + name
		}
		prefix := "  "
		if active {
			prefix = "› "
		}
		item := []string{titleStyle.Render(clipped(prefix+oneLine(name), width))}
		if e.resource.Key() == "pods" {
			ready, total, restarts, state := kubernetes.PodState(row.Object.Object)
			item = append(item, strings.Split(ansi.Wrap(fmt.Sprintf("  %d/%d ", ready, total)+statusValue(state)+fmt.Sprintf("  ↻%d", restarts), width, ""), "\n")...)
		} else {
			var values []string
			for i, column := range columns {
				if column != "Name" && column != "Namespace" && i < len(row.Cells) {
					values = append(values, muted.Render(column+" ")+summaryValue(column, row.Cells[i]))
				}
			}
			wrapped := strings.Split(ansi.Wrap(strings.Join(values, " · "), max(1, width-2), ""), "\n")
			for _, line := range wrapped {
				item = append(item, "  "+line)
			}
		}
		if active {
			selectedLine = len(lines)
		}
		for _, line := range item {
			if active {
				line = lipgloss.NewStyle().Background(highlight).Width(width).Render(line)
			}
			lines = append(lines, line)
		}
	}
	available := max(0, height-len(before)-1)
	return strings.Join(before, "\n") + "\n" + scrollSelection(lines, selectedLine, available) + "\n" + muted.Render(clipped(m.resourceCountSummary(rows), width))
}

func statusValue(value string) string {
	value = oneLine(value)
	return statusStyle(value).Render(value)
}

func statusStyle(value string) lipgloss.Style {
	for _, state := range []string{"CrashLoopBackOff", "ImagePullBackOff", "OOMKilled", "ErrImagePull", "Failed", "NotReady", "Error", "Pending", "Terminating", "Running", "Ready", "Bound", "Succeeded"} {
		if strings.Contains(value, state) {
			return resourceStatusStyle(state)
		}
	}
	return lipgloss.NewStyle()
}

func summaryValue(label, value string) string {
	return summaryValueStyle(label, value).Render(oneLine(value))
}

func summaryValueStyle(label, value string) lipgloss.Style {
	switch label {
	case "Status", "State", "Phase", "Previous exit", "Previous termination", "Reason":
		return statusStyle(value)
	default:
		return lipgloss.NewStyle()
	}
}

func (m *model) resourceSummary(width int) string {
	e := m.explorer()
	if e.detail == nil || width <= 0 {
		return ""
	}
	var sections []string
	content := kubernetes.Summary(e.resource, *e.detail)
	if len(e.supplement) > 0 {
		var combined []kubernetes.DetailSection
		if len(content) > 0 {
			combined = append(combined, content[0])
		}
		for _, section := range e.supplement {
			if section.Title != "Assigned Pods" {
				combined = append(combined, section)
			}
		}
		if len(content) > 1 {
			combined = append(combined, content[1:]...)
		}
		for _, section := range e.supplement {
			if section.Title == "Assigned Pods" {
				combined = append(combined, section)
			}
		}
		content = combined
	}
	for _, section := range content {
		if len(section.Fields) == 0 && len(section.Rows) == 0 {
			continue
		}
		lines := []string{summaryHeading(section.Title, width)}
		lines = append(lines, renderSummaryFields(section.Fields, width)...)
		if len(section.Rows) > 0 {
			if len(section.Fields) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, renderDetailTable(section, width, e.container)...)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if e.supplementNote != "" {
		sections = append([]string{changedStyle.Render(ansi.Wrap(plain(e.supplementNote), width, ""))}, sections...)
	}
	return strings.Join(sections, "\n\n")
}

func summaryHeading(title string, width int) string {
	if width <= 0 {
		return ""
	}
	heading := lipgloss.NewStyle().Foreground(accent).Bold(true)
	text := ansi.Wrap(oneLine(title), max(1, width-2), "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		prefix := "  "
		if i == 0 {
			prefix = "▎ "
		}
		if width < 3 {
			prefix = ""
		}
		remaining := width - ansi.StringWidth(prefix+line)
		rule := ""
		if i == len(lines)-1 && remaining >= 3 {
			rule = " " + lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("─", remaining-1))
		}
		lines[i] = heading.Render(prefix+line) + rule
	}
	return strings.Join(lines, "\n")
}

func renderSummaryFields(fields []kubernetes.DetailField, width int) []string {
	if len(fields) == 0 || width <= 0 {
		return nil
	}
	labelWidth := summaryLabelWidth(fields, width)
	var lines []string
	for i := 0; i < len(fields); {
		leftWidth := (width - 3) / 2
		if width >= 76 && i+1 < len(fields) && summaryFieldFits(fields[i], leftWidth, labelWidth) && summaryFieldFits(fields[i+1], width-leftWidth-3, labelWidth) {
			left := alignedSummaryField(fields[i], leftWidth, labelWidth)
			right := alignedSummaryField(fields[i+1], width-leftWidth-3, labelWidth)
			lines = append(lines, left+strings.Repeat(" ", leftWidth-ansi.StringWidth(left)+3)+right)
			i += 2
		} else {
			lines = append(lines, alignedSummaryField(fields[i], width, labelWidth))
			i++
		}
	}
	return lines
}

func summaryLabelWidth(fields []kubernetes.DetailField, width int) int {
	longest := 0
	for _, field := range fields {
		longest = max(longest, ansi.StringWidth(oneLine(field.Label))+1)
	}
	return min(longest, 26, max(1, width/3))
}

func summaryFieldFits(field kubernetes.DetailField, width, labelWidth int) bool {
	return !strings.Contains(field.Value, "\n") && ansi.StringWidth(oneLine(field.Label))+1 <= labelWidth && labelWidth+2+ansi.StringWidth(summaryFieldValue(field.Value)) <= width
}

func summaryField(field kubernetes.DetailField, width int) string {
	return alignedSummaryField(field, width, summaryLabelWidth([]kubernetes.DetailField{field}, width))
}

func alignedSummaryField(field kubernetes.DetailField, width, labelWidth int) string {
	if width <= 0 {
		return ""
	}
	label := oneLine(field.Label) + ":"
	value := summaryFieldValue(field.Value)
	style := detailValueStyle(field.Label, value)
	if ansi.StringWidth(label) > labelWidth || width-labelWidth-2 < 10 || ansi.StringWidth(oneLine(value)) > (width-labelWidth-2)*3 {
		if !strings.Contains(value, "\n") && ansi.StringWidth(label)+1+ansi.StringWidth(value) <= width {
			return muted.Render(label) + " " + style.Render(value)
		}
		indent := min(2, width-1)
		lines := strings.Split(ansi.Wrap(value, max(1, width-indent), ""), "\n")
		for i, line := range lines {
			lines[i] = strings.Repeat(" ", indent) + style.Render(line)
		}
		return muted.Render(ansi.Wrap(label, width, "")) + "\n" + strings.Join(lines, "\n")
	}
	lines := strings.Split(ansi.Wrap(value, max(1, width-labelWidth-2), ""), "\n")
	for i, line := range lines {
		prefix := strings.Repeat(" ", labelWidth+2)
		if i == 0 {
			prefix = muted.Render(label) + strings.Repeat(" ", labelWidth-ansi.StringWidth(label)+2)
		}
		lines[i] = prefix + style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func summaryFieldValue(value string) string {
	value = plain(value)
	if value == "" {
		return "Not specified"
	}
	return value
}

func detailCellValue(value string, present bool) string {
	if !present {
		return "Not reported"
	}
	value = plain(value)
	if value == "" {
		return "(empty)"
	}
	return value
}

func detailValueStyle(label, value string) lipgloss.Style {
	switch value {
	case "(empty)", "Not reported", "Not specified", "Not applicable", "none", "N/A", "-":
		return muted
	default:
		return summaryValueStyle(label, value)
	}
}

func detailTableWidths(section kubernetes.DetailSection, width int) ([]int, bool) {
	columns := len(section.Columns)
	if columns == 0 {
		return nil, false
	}
	widths, minimum, desired := make([]int, columns), make([]int, columns), make([]int, columns)
	total := (columns - 1) * 3
	for i, column := range section.Columns {
		natural := ansi.StringWidth(oneLine(column))
		for _, row := range section.Rows {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			for _, line := range strings.Split(detailCellValue(value, i < len(row)), "\n") {
				natural = max(natural, ansi.StringWidth(line))
			}
		}
		minimum[i] = min(natural, max(8, min(18, ansi.StringWidth(column))))
		if i == 0 {
			minimum[i] = min(natural, max(12, minimum[i]))
		}
		if section.Title == "Containers" && column == "State" {
			minimum[i] = min(natural, 24)
		}
		widths[i] = max(minimum[i], min(64, natural))
		desired[i] = natural
		total += minimum[i]
	}
	if total > width {
		return nil, false
	}
	available := width - (columns-1)*3
	for sumDetailWidths(widths) > available {
		widest := -1
		for i := range widths {
			if widths[i] > minimum[i] && (widest < 0 || widths[i]-minimum[i] > widths[widest]-minimum[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		widths[widest]--
	}
	for sumDetailWidths(widths) < available {
		grow := -1
		for i := range widths {
			if widths[i] < desired[i] && (grow < 0 || desired[i]-widths[i] > desired[grow]-widths[grow]) {
				grow = i
			}
		}
		if grow < 0 {
			break
		}
		widths[grow]++
	}
	return widths, true
}

func sumDetailWidths(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func renderDetailTable(section kubernetes.DetailSection, width int, container string) []string {
	if len(section.Columns) == 0 || width <= 0 {
		return nil
	}
	widths, tabular := detailTableWidths(section, width)
	if !tabular {
		return renderDetailRecords(section, width, container)
	}
	var expanded []int
	for i, column := range section.Columns {
		if i == 0 {
			continue
		}
		switch column {
		case "Image", "Message", "Description", "Expression", "Value", "Source", "Reference", "Previous exit":
			for _, row := range section.Rows {
				if i < len(row) && widths[i] < 36 && (ansi.StringWidth(oneLine(row[i])) > widths[i]*2 || strings.Contains(row[i], "\n")) {
					expanded = append(expanded, i)
					break
				}
			}
		}
	}
	if len(expanded) > 0 {
		return renderExpandedDetailTable(section, width, container, expanded)
	}
	lines := renderDetailTableHeader(section.Columns, widths)
	for i, row := range section.Rows {
		active := section.Title == "Containers" && len(row) > 0 && row[0] == container
		rendered := renderDetailTableRow(section.Columns, widths, row, false, active)
		lines = append(lines, rendered...)
		if len(rendered) > 1 && i+1 < len(section.Rows) {
			lines = append(lines, "")
		}
	}
	return lines
}

func renderDetailTableRow(columns []string, widths []int, row []string, header, active bool) []string {
	ruleStyle := lipgloss.NewStyle().Foreground(borderColor)
	separator := ruleStyle.Render(" │ ")
	cells := make([][]string, len(widths))
	height := 1
	for i, w := range widths {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		if header {
			value = oneLine(value)
		} else {
			value = detailCellValue(value, i < len(row))
		}
		cells[i] = strings.Split(ansi.Wrap(value, w, ""), "\n")
		height = max(height, len(cells[i]))
	}
	lines := make([]string, height)
	for line := 0; line < height; line++ {
		parts := make([]string, len(widths))
		for i, w := range widths {
			value := ""
			if line < len(cells[i]) {
				value = cells[i][line]
			}
			original := ""
			if i < len(row) {
				original = row[i]
			}
			style := detailValueStyle(columns[i], detailCellValue(original, i < len(row)))
			if header {
				style = muted.Bold(true).Background(highlight)
			} else if i == 0 {
				style = style.Bold(true)
			}
			if active {
				style = style.Background(highlight)
			}
			parts[i] = style.Render(value + strings.Repeat(" ", max(0, w-ansi.StringWidth(value))))
		}
		gap := separator
		if header || active {
			gap = ruleStyle.Background(highlight).Render(" │ ")
		}
		lines[line] = strings.Join(parts, gap)
	}
	return lines
}

func renderDetailTableHeader(columns []string, widths []int) []string {
	lines := renderDetailTableRow(columns, widths, columns, true, false)
	var rule []string
	for _, w := range widths {
		rule = append(rule, strings.Repeat("─", w))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(borderColor).Render(strings.Join(rule, "─┼─")))
	return lines
}

func renderExpandedDetailTable(section kubernetes.DetailSection, width int, container string, expanded []int) []string {
	expands := map[int]bool{}
	for _, index := range expanded {
		expands[index] = true
	}
	compact := kubernetes.DetailSection{Title: section.Title}
	for i, column := range section.Columns {
		if !expands[i] {
			compact.Columns = append(compact.Columns, column)
		}
	}
	for _, row := range section.Rows {
		var cells []string
		for i := range section.Columns {
			if !expands[i] {
				value := ""
				if i < len(row) {
					value = row[i]
				}
				cells = append(cells, detailCellValue(value, i < len(row)))
			}
		}
		compact.Rows = append(compact.Rows, cells)
	}
	widths, tabular := detailTableWidths(compact, width)
	if !tabular {
		return renderDetailRecords(section, width, container)
	}
	lines := renderDetailTableHeader(compact.Columns, widths)
	for index, row := range compact.Rows {
		if index > 0 {
			lines = append(lines, "")
		}
		active := section.Title == "Containers" && len(row) > 0 && row[0] == container
		lines = append(lines, renderDetailTableRow(compact.Columns, widths, row, false, active)...)
		for _, column := range expanded {
			value := ""
			if column < len(section.Rows[index]) {
				value = section.Rows[index][column]
			}
			lines = append(lines, summaryField(kubernetes.DetailField{Label: section.Columns[column], Value: detailCellValue(value, column < len(section.Rows[index]))}, width))
		}
	}
	return lines
}

func renderDetailRecords(section kubernetes.DetailSection, width int, container string) []string {
	var lines []string
	for rowIndex, row := range section.Rows {
		if rowIndex > 0 {
			lines = append(lines, "")
		}
		if section.Title == "Containers" && len(row) >= 6 {
			prefix := "  "
			if row[0] == container {
				prefix = "› "
			}
			name := titleStyle.Render(ansi.Wrap(prefix+oneLine(row[0]), width, ""))
			if row[0] == container {
				name = lipgloss.NewStyle().Background(highlight).Width(width).Render(name)
			}
			lines = append(lines, name)
			info := statusValue(row[3]) + muted.Render(" · "+row[1]+" · ready "+row[2]+" · ↻"+row[4])
			lines = append(lines, ansi.Wrap(info, width, ""))
			if row[5] != "" && row[5] != "none" {
				lines = append(lines, summaryField(kubernetes.DetailField{Label: "Previous exit", Value: row[5]}, width))
			}
			continue
		}
		fields := make([]kubernetes.DetailField, len(section.Columns))
		for i, label := range section.Columns {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			fields[i] = kubernetes.DetailField{Label: label, Value: detailCellValue(value, i < len(row))}
		}
		lines = append(lines, renderSummaryFields(fields, width)...)
	}
	return lines
}
