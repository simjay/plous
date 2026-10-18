package app

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) exploreHeader(width int) string {
	e := m.explorer()
	c := m.currentConnection()
	if c == nil {
		return ""
	}
	ns := e.Preference.Namespace
	if ns == "" {
		ns = "all namespaces"
	}
	if e.Resource.Name != "" && !e.Resource.Namespaced {
		ns = "cluster-scoped"
	}
	header := ui.ScopeHeader(c.Settings["context"], ns, e.Resource.Name == "" || e.Resource.Namespaced, width)
	title := kubernetes.Category(e.Resource) + " / " + monitor.ResourceTitle(e.Resource)
	if e.Resource.Name == "" {
		title = "Kubernetes Explore"
	}
	state := e.Status
	if e.Connecting || (!e.Listed && e.Err == "") {
		state = ""
	}
	if e.Err != "" {
		state = "Unavailable"
		if len(e.Collection.Objects) > 0 {
			state = "Stale"
		}
	}
	state = ui.Clipped(ui.OneLine(state), max(1, min(24, width/3)))
	if ansi.StringWidth(title)+ansi.StringWidth(state)+4 > width {
		title = monitor.ResourceTitle(e.Resource)
	}
	heading := ui.Heading(ui.Clipped(title, max(1, width-ansi.StringWidth(state)-3)))
	if e.Detail == nil && e.Input == nil && e.Picker == "" {
		heading = ui.MouseTarget(heading, "resource-types/")
	}
	heading += strings.Repeat(" ", max(1, width-ansi.StringWidth(heading)-ansi.StringWidth(state))) + ui.ResourceStatusStyle(state).Render(ui.OneLine(state))
	header += "\n" + ui.Clipped(heading, width)
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
	if e.Terminal != nil {
		return m.terminalBody(width, height)
	}
	if e.Picker == "connections" {
		return m.explorePickerBody(width, height)
	}
	c := m.currentConnection()
	if c == nil {
		return "Choose a Kubernetes connection with :K."
	}
	header := m.exploreHeader(width)
	gap := "\n\n"
	if height < 14 {
		gap = "\n"
	}
	available := m.exploreContentHeight(width, height)
	var body string
	if e.Input != nil && e.Picker == "" {
		label := map[string]string{"namespace": "Namespace (* = all)", "filter": "Filter", "selector": "Label selector"}[e.InputKind]
		body = ui.TitleStyle.Render(label) + "\n" + ui.Selected.Render(ui.EditorLine(*e.Input, width, m.focus == 1)) + "\n" + ui.Muted.Render("Enter apply · Esc cancel")
		if e.InputKind == "filter" {
			body = ui.SearchField(*e.Input, width, m.focus == 1, "Filter resources", "Type to filter…", "") + "\n" + ui.Muted.Render("Enter apply · Esc cancel")
		}
		inputHeight := lipgloss.Height(body)
		if available > inputHeight+2 {
			body += "\n\n" + m.exploreTableBody(width, available-inputHeight-2)
		}
	} else if m.chartsActive() {
		body = m.chartsBody(width, available)
	} else if e.Detail != nil {
		body = m.exploreDetailBody(width, available)
	} else if e.Connecting {
		body = m.loading("Discovering Kubernetes APIs…")
	} else if e.Err != "" && len(e.Collection.Objects) == 0 {
		body = ui.TextView(e.Err+"\n\ng Retry · n Namespace · Esc Browse resources", width, available, 0)
	} else {
		body = m.exploreTableBody(width, available)
	}
	base := ui.StyledView(header+gap+body, width, height)
	if e.Picker != "" {
		pickerWidth, pickerHeight := ui.PickerSize(width, height)
		return ui.PickerOverlay(base, m.explorePickerBody(pickerWidth-4, pickerHeight-2), width, height)
	}
	return base
}

func (m *model) exploreTableBody(width, height int) string {
	if height <= 0 {
		return ""
	}
	e := m.explorer()
	if !e.Listed && e.Err == "" {
		return ui.StyledTextView(m.loading("Loading "+monitor.ResourceTitle(e.Resource)+"…"), width, height, 0)
	}
	columns, rows := m.exploreRows()
	var before []string
	if e.Input == nil || e.InputKind != "filter" {
		if e.Preference.Filter == "" && len(rows) > 0 {
			hint := ui.MouseTarget(ui.Muted.Render("/ Filter resources"), "search")
			if m.chartsAvailable() {
				hint += ui.MouseTarget(ui.Muted.Render(" · I Top consumers"), "action/I")
			}
			before = append(before, ui.Clipped(hint, width))
		} else if e.Preference.Filter != "" {
			filter := ui.SearchField(textview.NewField(e.Preference.Filter), width, false, "Filter resources", "", "Backspace clear")
			before = append(before, strings.Split(filter, "\n")...)
		}
	}
	if e.Preference.Selector != "" {
		before = append(before, ui.Muted.Render(ui.Clipped("Labels: "+ui.OneLine(e.Preference.Selector), width)))
	}
	if e.Warning != "" {
		before = append(before, ui.ChangedStyle.Render(ui.Clipped(e.Warning, width)))
	}
	if e.Err != "" {
		before = append(before, ui.ChangedStyle.Render(ui.Clipped(e.Err, width)))
	}
	if e.Collection.Truncated {
		before = append(before, ui.ChangedStyle.Render("First 10,000 objects. Narrow the namespace or labels."))
	}
	if len(rows) > 0 && e.Usage != nil && e.Usage.Err != "" {
		before = append(before, ui.Muted.Render(ui.Clipped("Usage N/A: "+e.Usage.Err, width)))
	}
	if len(rows) == 0 {
		body := m.exploreEmptyBody(width, max(0, height-len(before)))
		if len(before) > 0 {
			body = strings.Join(before, "\n") + "\n" + body
		}
		return ui.StyledView(body, width, height)
	}
	indices, widths, compact := e.TableColumns(width)
	if compact {
		return m.compactResourceList(columns, rows, before, width, height)
	}
	line := func(cells []string, heading, active bool) string {
		return ui.CollectionTableRow(columns, cells, indices, widths, width, heading, active, func(column, value string) lipgloss.Style {
			if column == "Status" || e.Resource.Name == "events" && column == "Type" {
				return ui.ResourceStatusStyle(value)
			}
			if column == "Age" {
				return ui.Muted
			}
			return lipgloss.NewStyle()
		})
	}
	headings := append([]string(nil), columns...)
	if e.Preference.Sort < len(headings) && e.Preference.Sort >= 0 {
		mark := " ↑"
		if e.Preference.Descending {
			mark = " ↓"
		}
		headings[e.Preference.Sort] += mark
	}
	before = append(before, line(headings, true, false), ui.Muted.Render(strings.Repeat("─", width)))
	cursor := 0
	for i, row := range rows {
		if kubernetes.Identity(row.Object) == e.Selected {
			cursor = i
			break
		}
	}
	available := max(0, height-len(before)-1)
	start, end := ui.SelectionWindow(len(rows), cursor, available)
	items := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, ui.MouseTarget(line(rows[i].Cells, false, kubernetes.Identity(rows[i].Object) == e.Selected), "resource/"+mouseResourceID(rows[i].Object)))
	}
	return strings.Join(before, "\n") + "\n" + strings.Join(items, "\n") + "\n" + ui.Muted.Render(ui.Clipped(m.resourceCountSummary(rows), width))
}

func (m *model) exploreEmptyBody(width, height int) string {
	title, description, actions := m.exploreEmptyState()
	return ui.EmptyCollection(title, description, actions, width, height)
}

func (m *model) exploreEmptyState() (string, string, []textview.Action) {
	e := m.explorer()
	title := "No " + monitor.ResourceTitle(e.Resource) + " found."
	description := "Choose another resource in the sidebar, or refresh to check again."
	actions := []textview.Action{{Key: "g", Label: "Refresh"}}
	if e.Resource.Namespaced {
		title = "No " + monitor.ResourceTitle(e.Resource) + " across all namespaces."
		if e.Preference.Namespace != "" {
			title = "No " + monitor.ResourceTitle(e.Resource) + " in namespace " + e.Preference.Namespace + "."
			description = "Try another namespace, or choose All namespaces in the namespace picker."
			actions = append([]textview.Action{{Key: "n", Label: "Choose namespace"}}, actions...)
		}
	}
	if e.Preference.Filter != "" || e.Preference.Selector != "" {
		title = "No matching " + monitor.ResourceTitle(e.Resource) + "."
		description = "Try clearing the text filter."
		actions = nil
		if e.Preference.Filter != "" {
			actions = append(actions, textview.Action{Key: "Backspace", Label: "Clear text filter", Mouse: "action/backspace"})
		}
		if e.Preference.Selector != "" {
			description = "Try changing or removing the label selector."
			if e.Preference.Filter != "" {
				description = "Try clearing the text filter or changing the label selector."
			}
			actions = append(actions, textview.Action{Key: "L", Label: "Edit label selector"})
		}
	} else if m.leftHidden && len(actions) == 1 {
		actions = append(actions, textview.Action{Key: "bb", Label: "Show sidebar"})
	}
	if e.Input != nil {
		actions = nil
	}
	return title, description, actions
}

func (m *model) resourceBackLabel() string {
	e := m.explorer()
	if e.Tab == 1 {
		if e.YamlOrigin != nil {
			return "Esc " + monitor.ExploreTabs[e.YamlOrigin.Tab]
		}
		if len(e.Trail) == 0 {
			return "Esc list"
		}
	}
	return "Esc back"
}

func (m *model) resourceSectionTabs(width int) string {
	e := m.explorer()
	tabs := monitor.ExploreTabs
	if e.Tab != 4 {
		tabs = tabs[:4]
	}
	if e.Tab < 0 || e.Tab >= len(tabs) {
		return ""
	}
	labels := make([]string, len(tabs))
	keys := []string{"d", "y", "e", "r", "l"}
	if e.Tab == 1 {
		keys[1] = ":y" // The YAML reader owns y for copying.
	}
	for i, tab := range tabs {
		label := keys[i] + " " + tab
		labels[i] = ui.Muted.Render(label)
		if e.Tab == i {
			labels[i] = ui.Selected.Render("[" + label + "]")
		}
		labels[i] = ui.MouseTarget(labels[i], "section/"+[]string{"d", "y", "e", "r", "l"}[i])
	}
	tabLine := strings.Join(labels, "   ")
	if ansi.StringWidth(tabLine) > width {
		tabLine = ui.Selected.Render("["+keys[e.Tab]+" "+tabs[e.Tab]+"]") + ui.Muted.Render(" · [ ] sections")
	}
	return ui.Clipped(tabLine, width)
}

func (m *model) exploreDetailBody(width, height int) string {
	e := m.explorer()
	if e.Detail == nil || height <= 0 {
		return ""
	}
	if e.Tab == 4 {
		return m.exploreLogBody(width, height)
	}
	heading, text := m.exploreDetailContent(width, height)
	available := max(0, height-lipgloss.Height(heading))
	failed := !e.DetailValid && e.DetailRun == nil && (e.DetailError != "" || e.DetailText != "")
	if e.Tab == 1 && !failed {
		return heading + "\n" + m.yamlDocumentBody(width, available)
	}
	if e.Tab == 3 && !failed {
		text = m.relatedBody(width, available)
	}
	if m.activePanelSearch(1) != nil {
		if e.Tab != 0 {
			text = ui.Plain(text)
		}
		return heading + "\n" + m.findText(1, text, width, available, e.Scroll)
	}
	layout := ui.MeasureScroll(heading, text, width, height)
	if e.Tab == 3 && !failed {
		return layout.View(0)
	}
	return layout.View(e.Scroll)
}

func (m *model) exploreDetailContent(width, height int) (string, string) {
	e := m.explore
	heading := m.exploreDetailHeading(width, height)
	var text string
	if !e.DetailValid && e.DetailRun == nil && (e.DetailText != "" || e.DetailError != "") {
		text = ui.Plain(strings.TrimSpace(e.DetailError + "\n" + e.DetailText))
	} else {
		switch e.Tab {
		case 0:
			text = m.resourceSummary(width)
		case 2:
			text = ui.Plain(e.DetailText)
			if e.DetailRun != nil && !e.DetailLoaded {
				text = m.loading("Loading events…")
			}
		}
		if e.DetailError != "" {
			text = ui.Plain(e.DetailError) + "\n\n" + text
		}
	}
	return heading, text
}

func (m *model) exploreDetailLayout(width, height int) ui.ScrollLayout {
	heading, text := m.exploreDetailContent(width, height)
	return ui.MeasureScroll(heading, text, width, height)
}

func (m *model) exploreDetailScrollLimit() int {
	width := max(1, m.panelWidths()[1]-4)
	height := m.exploreContentHeight(width, m.mainBodyHeight())
	layout := m.exploreDetailLayout(width, height)
	if m.explore.Tab == 3 {
		related, _, _ := m.relatedViewport(width, layout.Height)
		return related.Limit
	}
	if m.activePanelSearch(1) != nil {
		return max(0, len(layout.Rows)-max(0, layout.Height-ui.SearchBarHeight))
	}
	return layout.Limit
}

func (m *model) explorePickerBody(width, height int) string {
	e := m.explorer()
	if e.Picker == "namespace" {
		return m.namespacePickerBody(width, height)
	}
	var items []string
	heading := ""
	hint := "Enter choose · Esc close"
	switch e.Picker {
	case "connections":
		heading = "Choose Kubernetes connection"
		for _, choice := range m.exploreConnections() {
			items = append(items, choice.Label)
		}
	case "containers":
		heading = "Choose container"
		if e.Detail != nil {
			items = m.logContainers()
		}
	case "sort":
		heading = "Sort column"
		items, _ = m.exploreRows()
		hint = "Enter sort / reverse · Esc close"
	case "logrange":
		heading = "Log time range"
		for _, entry := range monitor.LogRanges {
			items = append(items, entry.Label)
		}
	}
	if e.Input != nil {
		heading += "\n" + ui.SearchField(*e.Input, width, m.focus == 1, "Search", "Type to search…", "")
	}
	headingLines := len(strings.Split(heading, "\n"))
	if len(items) == 0 {
		message := "No matching resources."
		if e.Connecting {
			message = m.loading("Discovering APIs…")
		}
		if e.Picker == "connections" {
			message = "No Kubernetes connections. Add one in Settings."
		}
		return ui.StyledView(heading+"\n\n"+message+"\n"+hint, width, height)
	}
	return ui.StyledView(heading+"\n\n"+ui.ListView(items, e.PickerCursor, width, max(1, height-headingLines-3))+"\n\n"+ui.Muted.Render(hint), width, height)
}

func (m *model) exploreHint() string {
	e := m.explorer()
	if e.Picker != "" || e.Input != nil {
		return "↑↓ choose · Enter apply · Esc cancel"
	}
	if m.chartsActive() {
		if e.Detail == nil {
			return "↑↓ Scroll · I List · / Filter · Esc Back · hh Help"
		}
		return "←/→ Inspect · W Range · P Latest · Esc Back · hh Help"
	}
	if e.Detail != nil {
		if e.Tab == 0 && e.Resource.Key() == "pods" {
			return "o Choose container · / Find · Esc Back · hh Help"
		}
		if e.Tab == 4 {
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
		if e.Tab == 1 && e.Resource.Key() == "secrets" {
			return "/ find · v reveal/hide values · [ ] sections · Esc back"
		}
		return "a More actions · / Find · Esc Back · hh Help"
	}
	back := m.sidebarHint()
	if m.leftHidden {
		back = "Esc Browse resources"
	}
	if len(e.Trail) > 0 {
		back = "Esc Back"
	}
	if e.Err != "" {
		return "g Retry · n Namespace · " + back + " · hh Help"
	}
	if e.Listed && !e.Connecting {
		if _, rows := m.exploreRows(); len(rows) == 0 {
			_, _, actions := m.exploreEmptyState()
			var hints []string
			for _, action := range actions {
				if action.Key != "bb" {
					hints = append(hints, action.Key+" "+action.Label)
				}
			}
			return strings.Join(append(hints, back, "hh Help"), " · ")
		}
	}
	return "Enter Open · / Filter · n Namespace · " + back + " · hh Help"
}

func (m *model) exploreDetailHeading(width, height int) string {
	e := m.explorer()
	if e.Detail == nil {
		return ""
	}
	object := e.Detail
	tabLine := m.resourceSectionTabs(width)
	back := m.resourceBackLabel()
	identity := ui.TitleStyle.Render(ui.Clipped(ui.OneLine(object.GetName()), max(1, width-ansi.StringWidth(back)-5))) + ui.Muted.Render("  ‹ "+back)
	if e.DetailRun != nil {
		identity = ui.Clipped(identity+ui.Muted.Render(" · Refreshing…"), width)
	}
	heading := identity + "\n" + ui.Clipped(tabLine, width)
	if e.Tab == 4 {
		heading = identity + "\n" + ui.Selected.Render("Logs")
	} else if height >= 10 {
		heading += "\n\n" + m.resourceActionBar(width)
		if e.Tab == 0 && e.Resource.Key() == "pods" {
			heading += "\n" + ui.Clipped(ui.Selected.Render("o Choose container")+ui.Muted.Render(" · "+ui.OneLine(e.Container)), width)
		}
	}
	if height >= 10 {
		heading += "\n"
	}
	return heading
}
