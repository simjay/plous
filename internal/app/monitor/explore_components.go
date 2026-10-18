package monitor

import (
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func ResourceTableColumns(columns []string, rows []kubernetes.Row, indices []int, width int, pod bool) ([]int, []int, bool) {
	cells := make([][]string, len(rows))
	for i, row := range rows {
		cells[i] = row.Cells
	}
	return CollectionTableColumns(columns, cells, indices, width, pod)
}

func CollectionTableColumns(columns []string, rows [][]string, indices []int, width int, pod bool) ([]int, []int, bool) {
	if pod {
		order := []string{"Name", "Namespace", "Ready", "Status", "Restarts", "CPU", "Memory", "Node", "Owner", "QoS", "Pod IP", "Age"}
		slices.SortStableFunc(indices, func(a, b int) int { return slices.Index(order, columns[a]) - slices.Index(order, columns[b]) })
	} else if slices.Contains(columns, "CPU") && slices.Contains(columns, "Scheduling") {
		order := []string{"Name", "Ready", "CPU", "Memory", "Scheduling", "Roles", "Version", "Internal IP", "Age"}
		slices.SortStableFunc(indices, func(a, b int) int { return slices.Index(order, columns[a]) - slices.Index(order, columns[b]) })
	}
	if pod && width < 110 {
		keep := indices[:0]
		for _, index := range indices {
			if columns[index] != "Node" && columns[index] != "Pod IP" && columns[index] != "Owner" && columns[index] != "QoS" {
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
		if columns[index] == "CPU" || columns[index] == "Memory" {
			minimum[i] = 8
		}
		if pod && columns[index] == "Status" {
			minimum[i] = 18
		}
		widths[i] = minimum[i]
		for _, row := range rows {
			if index < len(row) {
				widths[i] = max(widths[i], min(64, ansi.StringWidth(ui.OneLine(row[index]))))
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
		stretch := 0
		if columns[indices[0]] != "Name" {
			stretch = len(widths) - 1
		}
		widths[stretch] += width - total
	}
	return indices, widths, false
}

func StatusValue(value string) string {
	value = ui.OneLine(value)
	return StatusStyle(value).Render(value)
}

func StatusStyle(value string) lipgloss.Style {
	for _, state := range []string{"CrashLoopBackOff", "ImagePullBackOff", "OOMKilled", "ErrImagePull", "Failed", "NotReady", "Error", "Pending", "Terminating", "Running", "Ready", "Bound", "Succeeded"} {
		if strings.Contains(value, state) {
			return ui.ResourceStatusStyle(state)
		}
	}
	return lipgloss.NewStyle()
}

func SummaryValue(label, value string) string {
	return SummaryValueStyle(label, value).Render(ui.OneLine(value))
}

func SummaryValueStyle(label, value string) lipgloss.Style {
	switch label {
	case "Recorded status":
		return HelmCellStyle("Status", value)
	case "Status", "State", "Phase", "Previous exit", "Previous termination", "Reason":
		return StatusStyle(value)
	default:
		return lipgloss.NewStyle()
	}
}

func SummaryHeading(title string, width int) string {
	return ui.DetailHeading(title, width)
}

func RenderSummaryFields(fields []kubernetes.DetailField, width int) []string {
	if len(fields) == 0 || width <= 0 {
		return nil
	}
	labelWidth := SummaryLabelWidth(fields, width)
	var lines []string
	for i := 0; i < len(fields); {
		leftWidth := (width - 3) / 2
		if width >= 76 && i+1 < len(fields) && SummaryFieldFits(fields[i], leftWidth, labelWidth) && SummaryFieldFits(fields[i+1], width-leftWidth-3, labelWidth) {
			left := AlignedSummaryField(fields[i], leftWidth, labelWidth)
			right := AlignedSummaryField(fields[i+1], width-leftWidth-3, labelWidth)
			lines = append(lines, left+strings.Repeat(" ", leftWidth-ansi.StringWidth(left)+3)+right)
			i += 2
		} else {
			lines = append(lines, AlignedSummaryField(fields[i], width, labelWidth))
			i++
		}
	}
	return lines
}

func SummaryLabelWidth(fields []kubernetes.DetailField, width int) int {
	longest := 0
	for _, field := range fields {
		longest = max(longest, ansi.StringWidth(ui.OneLine(field.Label))+1)
	}
	return min(longest, 26, max(1, width/3))
}

func SummaryFieldFits(field kubernetes.DetailField, width, labelWidth int) bool {
	return !strings.Contains(field.Value, "\n") && ansi.StringWidth(ui.OneLine(field.Label))+1 <= labelWidth && labelWidth+2+ansi.StringWidth(SummaryFieldValue(field.Value)) <= width
}

func SummaryField(field kubernetes.DetailField, width int) string {
	return AlignedSummaryField(field, width, SummaryLabelWidth([]kubernetes.DetailField{field}, width))
}

func AlignedSummaryField(field kubernetes.DetailField, width, labelWidth int) string {
	if width <= 0 {
		return ""
	}
	label := ui.OneLine(field.Label) + ":"
	value := SummaryFieldValue(field.Value)
	style := DetailValueStyle(field.Label, value)
	if ansi.StringWidth(label) > labelWidth || width-labelWidth-2 < 10 || ansi.StringWidth(ui.OneLine(value)) > (width-labelWidth-2)*3 {
		if !strings.Contains(value, "\n") && ansi.StringWidth(label)+1+ansi.StringWidth(value) <= width {
			return ui.Muted.Render(label) + " " + style.Render(value)
		}
		indent := min(2, width-1)
		lines := strings.Split(ansi.Wrap(value, max(1, width-indent), ""), "\n")
		for i, line := range lines {
			lines[i] = strings.Repeat(" ", indent) + style.Render(line)
		}
		return ui.Muted.Render(ansi.Wrap(label, width, "")) + "\n" + strings.Join(lines, "\n")
	}
	lines := strings.Split(ansi.Wrap(value, max(1, width-labelWidth-2), ""), "\n")
	for i, line := range lines {
		prefix := strings.Repeat(" ", labelWidth+2)
		if i == 0 {
			prefix = ui.Muted.Render(label) + strings.Repeat(" ", labelWidth-ansi.StringWidth(label)+2)
		}
		lines[i] = prefix + style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func SummaryFieldValue(value string) string {
	value = ui.Plain(value)
	if value == "" {
		return "Not specified"
	}
	return value
}

func DetailCellValue(value string, present bool) string {
	if !present {
		return "Not reported"
	}
	value = ui.Plain(value)
	if value == "" {
		return "(empty)"
	}
	return value
}

func DetailValueStyle(label, value string) lipgloss.Style {
	switch value {
	case "(empty)", "Not reported", "Not specified", "Not applicable", "none", "N/A", "-":
		return ui.Muted
	default:
		return SummaryValueStyle(label, value)
	}
}

func DetailTableWidths(section kubernetes.DetailSection, width int) ([]int, bool) {
	columns := len(section.Columns)
	if columns == 0 {
		return nil, false
	}
	widths, minimum, desired := make([]int, columns), make([]int, columns), make([]int, columns)
	total := (columns - 1) * 3
	for i, column := range section.Columns {
		natural := ansi.StringWidth(ui.OneLine(column))
		for _, row := range section.Rows {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			for _, line := range strings.Split(DetailCellValue(value, i < len(row)), "\n") {
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
	for SumDetailWidths(widths) > available {
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
	for SumDetailWidths(widths) < available {
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

func SumDetailWidths(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func RenderDetailTable(section kubernetes.DetailSection, width int, container string) []string {
	if len(section.Columns) == 0 || width <= 0 {
		return nil
	}
	widths, tabular := DetailTableWidths(section, width)
	if !tabular {
		return RenderDetailRecords(section, width, container)
	}
	var expanded []int
	for i, column := range section.Columns {
		if i == 0 {
			continue
		}
		switch column {
		case "Image", "Message", "Description", "Expression", "Value", "Source", "Reference", "Previous exit":
			for _, row := range section.Rows {
				if i < len(row) && widths[i] < 36 && (ansi.StringWidth(ui.OneLine(row[i])) > widths[i]*2 || strings.Contains(row[i], "\n")) {
					expanded = append(expanded, i)
					break
				}
			}
		}
	}
	if len(expanded) > 0 {
		return RenderExpandedDetailTable(section, width, container, expanded)
	}
	lines := RenderDetailTableHeader(section.Columns, widths)
	for i, row := range section.Rows {
		active := section.Title == "Containers" && len(row) > 0 && row[0] == container
		rendered := RenderDetailTableRow(section.Columns, widths, row, false, active)
		lines = append(lines, rendered...)
		if len(rendered) > 1 && i+1 < len(section.Rows) {
			lines = append(lines, "")
		}
	}
	return lines
}

func RenderDetailTableRow(columns []string, widths []int, row []string, header, active bool) []string {
	ruleStyle := lipgloss.NewStyle().Foreground(ui.BorderColor)
	separator := ruleStyle.Render(" │ ")
	cells := make([][]string, len(widths))
	height := 1
	for i, w := range widths {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		if header {
			value = ui.OneLine(value)
		} else {
			value = DetailCellValue(value, i < len(row))
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
			style := DetailValueStyle(columns[i], DetailCellValue(original, i < len(row)))
			if header {
				style = ui.Muted.Bold(true).Background(ui.Highlight)
			} else if i == 0 {
				style = style.Bold(true)
			}
			if active {
				style = style.Background(ui.Highlight)
			}
			parts[i] = style.Render(value + strings.Repeat(" ", max(0, w-ansi.StringWidth(value))))
		}
		gap := separator
		if header || active {
			gap = ruleStyle.Background(ui.Highlight).Render(" │ ")
		}
		lines[line] = strings.Join(parts, gap)
	}
	return lines
}

func RenderDetailTableHeader(columns []string, widths []int) []string {
	lines := RenderDetailTableRow(columns, widths, columns, true, false)
	var rule []string
	for _, w := range widths {
		rule = append(rule, strings.Repeat("─", w))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(ui.BorderColor).Render(strings.Join(rule, "─┼─")))
	return lines
}

func RenderExpandedDetailTable(section kubernetes.DetailSection, width int, container string, expanded []int) []string {
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
				cells = append(cells, DetailCellValue(value, i < len(row)))
			}
		}
		compact.Rows = append(compact.Rows, cells)
	}
	widths, tabular := DetailTableWidths(compact, width)
	if !tabular {
		return RenderDetailRecords(section, width, container)
	}
	lines := RenderDetailTableHeader(compact.Columns, widths)
	for index, row := range compact.Rows {
		if index > 0 {
			lines = append(lines, "")
		}
		active := section.Title == "Containers" && len(row) > 0 && row[0] == container
		lines = append(lines, RenderDetailTableRow(compact.Columns, widths, row, false, active)...)
		for _, column := range expanded {
			value := ""
			if column < len(section.Rows[index]) {
				value = section.Rows[index][column]
			}
			lines = append(lines, SummaryField(kubernetes.DetailField{Label: section.Columns[column], Value: DetailCellValue(value, column < len(section.Rows[index]))}, width))
		}
	}
	return lines
}

func RenderDetailRecords(section kubernetes.DetailSection, width int, container string) []string {
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
			name := ui.TitleStyle.Render(ansi.Wrap(prefix+ui.OneLine(row[0]), width, ""))
			if row[0] == container {
				name = lipgloss.NewStyle().Background(ui.Highlight).Width(width).Render(name)
			}
			lines = append(lines, name)
			info := StatusValue(row[3]) + ui.Muted.Render(" · "+row[1]+" · ready "+row[2]+" · ↻"+row[4])
			lines = append(lines, ansi.Wrap(info, width, ""))
			if row[5] != "" && row[5] != "none" {
				lines = append(lines, SummaryField(kubernetes.DetailField{Label: "Previous exit", Value: row[5]}, width))
			}
			continue
		}
		fields := make([]kubernetes.DetailField, len(section.Columns))
		for i, label := range section.Columns {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			fields[i] = kubernetes.DetailField{Label: label, Value: DetailCellValue(value, i < len(row))}
		}
		lines = append(lines, RenderSummaryFields(fields, width)...)
	}
	return lines
}
