package app

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

var exploreTabs = []string{"Describe", "YAML", "Events", "Related", "Logs"}

var logRanges = []struct {
	label   string
	seconds int64
}{
	{"Latest 200 lines", 0}, {"Last minute", 60}, {"Last 5 minutes", 300}, {"Last 15 minutes", 900}, {"Last hour", 3600}, {"Last 6 hours", 21600}, {"Last 24 hours", 86400},
}

func resourceTitle(resource kubernetes.Resource) string {
	return kubernetes.ResourceTitle(resource)
}

func (m *model) exploreHeader(width int) string {
	e := m.explorer()
	c := m.currentConnection()
	if c == nil {
		return ""
	}
	ns := e.preference.Namespace
	if ns == "" {
		ns = "all namespaces"
	}
	if e.resource.Name != "" && !e.resource.Namespaced {
		ns = "cluster-scoped"
	}
	contextLabel := muted.Render("ctx ") + titleStyle.Render(oneLine(c.Settings["context"]))
	namespace := selected.Render("[ " + oneLine(ns) + " ▾ ]")
	namespaceHint := muted.Render(" n")
	if e.resource.Name != "" && !e.resource.Namespaced {
		namespace = muted.Render("cluster-scoped")
		namespaceHint = ""
	}
	header := clipped(contextLabel+"  "+namespace+namespaceHint, width)
	if ansi.StringWidth(contextLabel+"  "+namespace+namespaceHint) > width {
		header = clipped(contextLabel, width) + "\n" + clipped(namespace, max(1, width-ansi.StringWidth(namespaceHint))) + namespaceHint
	}
	title := kubernetes.Category(e.resource) + " / " + resourceTitle(e.resource)
	if e.resource.Name == "" {
		title = "Kubernetes Explore"
	}
	state := e.status
	if e.connecting || (!e.listed && e.err == "") {
		state = ""
	}
	if e.err != "" {
		state = "Unavailable"
		if len(e.collection.Objects) > 0 {
			state = "Stale"
		}
	}
	state = clipped(oneLine(state), max(1, min(24, width/3)))
	if ansi.StringWidth(title)+ansi.StringWidth(state)+4 > width {
		title = resourceTitle(e.resource)
	}
	heading := gradientHeading(clipped(title, max(1, width-ansi.StringWidth(state)-3)))
	heading += strings.Repeat(" ", max(1, width-ansi.StringWidth(heading)-ansi.StringWidth(state))) + resourceStatusStyle(state).Render(oneLine(state))
	header += "\n" + clipped(heading, width)
	return header
}

func (m *model) exploreBody(width, height int) string {
	e := m.explorer()
	if m.valueDocumentActive() {
		return m.valueDocumentBody(width, height)
	}
	if m.documentActive() {
		return m.documentBody(width, height)
	}
	if e.terminal != nil {
		return m.terminalBody(width, height)
	}
	if e.picker == "connections" {
		return m.explorePickerBody(width, height)
	}
	c := m.currentConnection()
	if c == nil {
		return "Choose a Kubernetes connection with :K."
	}
	header := m.exploreHeader(width)
	available := max(0, height-lipgloss.Height(header)-2)
	var body string
	if e.input != nil && e.picker == "" {
		label := map[string]string{"namespace": "Namespace (* = all)", "filter": "Filter", "selector": "Label selector"}[e.inputKind]
		body = titleStyle.Render(label) + "\n" + selected.Render(editorLine(*e.input, width, m.focus == 1)) + "\n" + muted.Render("Enter apply · Esc cancel")
		if e.inputKind == "filter" {
			body = searchField(*e.input, width, m.focus == 1, "Filter resources", "Type to filter…", "") + "\n" + muted.Render("Enter apply · Esc cancel")
		}
		inputHeight := lipgloss.Height(body)
		if available > inputHeight+2 {
			body += "\n\n" + m.exploreTableBody(width, available-inputHeight-2)
		}
	} else if e.detail != nil {
		body = m.exploreDetailBody(width, available)
	} else if e.connecting {
		body = m.loading("Discovering Kubernetes APIs…")
	} else if e.err != "" && len(e.collection.Objects) == 0 {
		body = textView(e.err+"\n\ng Retry · n Namespace · :kinds Resource types", width, available, 0)
	} else {
		body = m.exploreTableBody(width, available)
	}
	base := styledView(header+"\n\n"+body, width, height)
	if e.picker != "" {
		base = lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(base)
		pickerWidth := max(12, min(width, 70))
		pickerHeight := max(3, min(height, 18))
		window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Background(surface).Padding(0, 1).Width(pickerWidth).Height(pickerHeight).MaxWidth(pickerWidth).MaxHeight(pickerHeight).Render(m.explorePickerBody(pickerWidth-4, pickerHeight-2))
		return centeredOverlay(base, window)
	}
	return base
}

func (m *model) exploreTableBody(width, height int) string {
	if height <= 0 {
		return ""
	}
	e := m.explorer()
	if !e.listed && e.err == "" {
		return styledTextView(m.loading("Loading "+resourceTitle(e.resource)+"…"), width, height, 0)
	}
	columns, rows := m.exploreRows()
	var before []string
	if e.input == nil || e.inputKind != "filter" {
		if e.preference.Filter == "" {
			before = append(before, muted.Render(clipped("/ Filter resources", width)))
		} else {
			filter := searchField(newEditor(e.preference.Filter), width, false, "Filter resources", "", "Backspace clear")
			before = append(before, strings.Split(filter, "\n")...)
		}
	}
	if e.preference.Selector != "" {
		before = append(before, muted.Render(clipped("Labels: "+oneLine(e.preference.Selector), width)))
	}
	if e.warning != "" {
		before = append(before, changedStyle.Render(clipped(e.warning, width)))
	}
	if e.err != "" {
		before = append(before, changedStyle.Render(clipped(e.err, width)))
	}
	if e.collection.Truncated {
		before = append(before, changedStyle.Render("First 10,000 objects. Narrow the namespace or labels."))
	}
	if len(rows) == 0 {
		message := "No " + resourceTitle(e.resource) + " found."
		if e.resource.Namespaced {
			message = "No " + resourceTitle(e.resource) + " across all namespaces."
			if e.preference.Namespace != "" {
				message = "No " + resourceTitle(e.resource) + " in namespace " + e.preference.Namespace + "."
			}
		}
		if e.preference.Filter != "" || e.preference.Selector != "" {
			message = "No resources match this filter."
		}
		before = append(before, oneLine(message), "")
		if e.resource.Namespaced && e.preference.Namespace != "" {
			before = append(before, selected.Render(":N *  Show all namespaces"))
		}
		before = append(before, muted.Render(":k Resource types · n Namespace · / Filter"))
		return styledTextView(strings.Join(before, "\n"), width, height, 0)
	}
	indices := []int{}
	for i, column := range columns {
		if column == "Namespace" && e.preference.Namespace != "" {
			continue
		}
		indices = append(indices, i)
	}
	indices, widths, compact := resourceTableColumns(columns, rows, indices, width, e.resource.Key() == "pods")
	if compact {
		return m.compactResourceList(columns, rows, before, width, height)
	}
	line := func(cells []string, heading, active bool) string {
		parts := make([]string, len(indices))
		for i, index := range indices {
			text := clipped(oneLine(cells[index]), widths[i])
			style := lipgloss.NewStyle()
			if heading {
				style = muted
			} else if columns[index] == "Status" || (e.resource.Name == "events" && columns[index] == "Type") {
				style = resourceStatusStyle(cells[index])
			} else if columns[index] == "Age" {
				style = muted
			} else if active && index == 0 {
				style = titleStyle
			}
			if active {
				style = style.Background(highlight)
			}
			parts[i] = style.Render(text + strings.Repeat(" ", max(0, widths[i]-ansi.StringWidth(text))))
		}
		separator := "  "
		if active {
			separator = lipgloss.NewStyle().Background(highlight).Render(separator)
		}
		text := clipped(strings.Join(parts, separator), width)
		if active {
			text = lipgloss.NewStyle().Foreground(foreground).Background(highlight).Width(width).Render(text)
		}
		return text
	}
	headings := append([]string(nil), columns...)
	if e.preference.Sort < len(headings) && e.preference.Sort >= 0 {
		mark := " ↑"
		if e.preference.Descending {
			mark = " ↓"
		}
		headings[e.preference.Sort] += mark
	}
	before = append(before, line(headings, true, false), muted.Render(strings.Repeat("─", width)))
	items := make([]string, 0, len(rows))
	cursor := 0
	for i, row := range rows {
		active := kubernetes.Identity(row.Object) == e.selected
		if active {
			cursor = i
		}
		items = append(items, line(row.Cells, false, active))
	}
	available := max(0, height-len(before)-1)
	return strings.Join(before, "\n") + "\n" + scrollSelection(items, cursor, available) + "\n" + muted.Render(clipped(m.resourceCountSummary(rows), width))
}

func (m *model) resourceBackLabel() string {
	e := m.explorer()
	if e.tab == 1 {
		if e.yamlOrigin != nil {
			return "Esc " + exploreTabs[e.yamlOrigin.tab]
		}
		if len(e.trail) == 0 {
			return "Esc list"
		}
	}
	return "Esc back"
}

func (m *model) resourceSectionTabs(width int) string {
	e := m.explorer()
	tabs := exploreTabs
	if e.tab != 4 {
		tabs = tabs[:4]
	}
	if e.tab < 0 || e.tab >= len(tabs) {
		return ""
	}
	labels := make([]string, len(tabs))
	keys := []string{"d", "y", "e", "r", "l"}
	if e.tab == 1 {
		keys[1] = ":y" // The YAML reader owns y for copying.
	}
	for i, tab := range tabs {
		label := keys[i] + " " + tab
		labels[i] = muted.Render(label)
		if e.tab == i {
			labels[i] = selected.Render("[" + label + "]")
		}
	}
	tabLine := strings.Join(labels, "   ")
	if ansi.StringWidth(tabLine) > width {
		tabLine = selected.Render("["+keys[e.tab]+" "+tabs[e.tab]+"]") + muted.Render(" · [ ] sections")
	}
	return clipped(tabLine, width)
}

func (m *model) exploreDetailBody(width, height int) string {
	e := m.explorer()
	if e.detail == nil || height <= 0 {
		return ""
	}
	if e.tab == 4 {
		return m.exploreLogBody(width, height)
	}
	object := e.detail
	tabLine := m.resourceSectionTabs(width)
	back := m.resourceBackLabel()
	identity := titleStyle.Render(clipped(oneLine(object.GetName()), max(1, width-ansi.StringWidth(back)-5))) + muted.Render("  ‹ "+back)
	if e.detailRun != nil {
		identity = clipped(identity+muted.Render(" · Refreshing…"), width)
	}
	heading := identity + "\n" + clipped(tabLine, width)
	if e.tab == 4 {
		heading = identity + "\n" + selected.Render("Logs")
	} else {
		heading += "\n\n" + m.resourceActionBar(width)
		if e.tab == 0 && e.resource.Key() == "pods" {
			heading += "\n" + clipped(selected.Render("o Choose container")+muted.Render(" · "+oneLine(e.container)), width)
		}
	}
	heading += "\n"
	available := max(0, height-lipgloss.Height(heading))
	if !e.detailValid && e.detailRun == nil && (e.detailText != "" || e.detailError != "") {
		if m.searches[1] != nil {
			return heading + "\n" + m.findText(1, plain(strings.TrimSpace(e.detailError+"\n"+e.detailText)), width, available, e.scroll)
		}
		return heading + "\n" + textView(strings.TrimSpace(e.detailError+"\n"+e.detailText), width, available, e.scroll)
	}
	var text string
	switch e.tab {
	case 0:
		text = m.resourceSummary(width)
	case 1:
		return heading + "\n" + m.yamlDocumentBody(width, available)
	case 2:
		text = e.detailText
		if e.detailRun != nil && !e.detailLoaded {
			text = m.loading("Loading events…")
		}
	case 3:
		text = m.relatedBody(width, available)
	}
	if e.detailError != "" {
		text = e.detailError + "\n\n" + text
	}
	if m.searches[1] != nil {
		if e.tab != 0 {
			text = plain(text)
		}
		return heading + "\n" + m.findText(1, text, width, available, e.scroll)
	}
	if e.tab == 3 {
		return heading + "\n" + styledView(text, width, available)
	}
	if e.tab == 0 {
		return heading + "\n" + styledTextView(text, width, available, e.scroll)
	}
	return heading + "\n" + textView(text, width, available, e.scroll)
}

func (m *model) exploreDetailScrollLimit() int {
	width := max(1, m.panelWidths()[1]-4)
	height := max(0, m.height-6-lipgloss.Height(m.exploreHeader(width))-2)
	if m.command != nil {
		height = max(0, height-3)
	}
	// Measure the same wrapped content without clipping it to the visible viewport.
	content := m.exploreDetailBody(width, 1<<30)
	return max(0, strings.Count(content, "\n")+1-height)
}

func (m *model) explorePickerBody(width, height int) string {
	e := m.explorer()
	if e.picker == "namespace" {
		return m.namespacePickerBody(width, height)
	}
	var items []string
	heading := ""
	hint := "Enter choose · Esc close"
	switch e.picker {
	case "kinds":
		heading = "Resource kinds\n" + muted.Render("← "+m.exploreCategory()+" →")
		for _, resource := range m.exploreKinds() {
			label := resourceTitle(resource)
			if resource.Group != "" {
				label += " · " + resource.Group
			}
			items = append(items, label)
		}
	case "connections":
		heading = "Choose Kubernetes connection"
		for _, choice := range m.exploreConnections() {
			items = append(items, choice.label)
		}
	case "containers":
		heading = "Choose container"
		if e.detail != nil {
			items = m.logContainers()
		}
	case "sort":
		heading = "Sort column"
		items, _ = m.exploreRows()
		hint = "Enter sort / reverse · Esc close"
	case "logrange":
		heading = "Log time range"
		for _, entry := range logRanges {
			items = append(items, entry.label)
		}
	}
	if e.input != nil {
		heading += "\n" + searchField(*e.input, width, m.focus == 1, "Search", "Type to search…", "")
	}
	headingLines := len(strings.Split(heading, "\n"))
	if len(items) == 0 {
		message := "No matching resources."
		if e.connecting {
			message = m.loading("Discovering APIs…")
		}
		if e.picker == "connections" {
			message = "No Kubernetes connections. Add one in Settings."
		}
		return styledView(heading+"\n\n"+message+"\n"+hint, width, height)
	}
	return styledView(heading+"\n\n"+listView(items, e.pickerCursor, width, max(1, height-headingLines-3))+"\n\n"+muted.Render(hint), width, height)
}

func (m *model) exploreHint() string {
	e := m.explorer()
	if e.picker != "" || e.input != nil {
		return "↑↓ choose · Enter apply · Esc cancel"
	}
	if e.detail != nil {
		if e.tab == 0 && e.resource.Key() == "pods" {
			return "o Choose container · / Find · Esc Back · hh Help"
		}
		if e.tab == 4 {
			viewer := m.podLogViewer()
			if viewer.Reader.Searching {
				return "Enter Finish search · Esc Cancel search"
			}
			follow := "Space Pause"
			if !viewer.Following {
				follow = "Space Jump live"
			}
			return follow + " · / Find · Esc " + viewer.Reader.EscapeLabel("Back") + " · hh Help"
		}
		if e.tab == 1 && e.resource.Key() == "secrets" {
			return "/ find · v reveal/hide values · [ ] sections · Esc back"
		}
		return "a More actions · / Find · Esc Back · hh Help"
	}
	back := m.sidebarHint()
	if len(e.trail) > 0 {
		back = "Esc Back"
	}
	if e.err != "" {
		return "g Retry · n Namespace · " + back + " · hh Help"
	}
	return "Enter Open · / Filter · n Namespace · " + back + " · hh Help"
}

func displayLogText(text string, timestamps bool) string {
	if timestamps {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		stamp, rest, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			lines[i] = rest
		}
	}
	return strings.Join(lines, "\n")
}
