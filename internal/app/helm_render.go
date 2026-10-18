package app

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"

	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) helmBody(width, height int) string {
	h := m.helmer()
	if h == nil {
		return ""
	}
	if h.Discard {
		return ui.Heading("Unsaved draft values") + "\n\n" + ui.ListView([]string{"Keep Editing", "Discard Changes"}, h.Choice, width, height-3)
	}
	if h.Confirm {
		return monitor.HelmConfirmationBody(h, width, height)
	}
	if h.Comparison != nil {
		return h.Comparison.View(width, height, ui.WorkspaceTheme())
	}
	if h.Document != nil {
		d := h.Document
		d.Actions = []textview.ActionGroup{{Label: "Document", Actions: []textview.Action{{Key: "Esc", Label: "Back"}, {Key: "/", Label: "Find"}, {Key: "y", Label: "Copy"}, {Key: ":h", Label: "Help"}}}}
		if !d.ReadOnly {
			d.Actions = []textview.ActionGroup{{Label: "Draft", Actions: []textview.Action{{Key: ":w", Label: "Save values"}, {Key: ":v", Label: "Review"}, {Key: ":q", Label: "Close"}}}}
		}
		return d.View(width, height, ui.WorkspaceTheme())
	}
	header := m.helmHeader(h, width)
	gap := "\n\n"
	if height < 16 {
		gap = "\n"
	}
	space := max(1, height-lipgloss.Height(header)-strings.Count(gap, "\n")+1)
	var body string
	switch {
	case h.Client == nil:
		body = ui.EmptyState("Helm connection", "", "g  Connect / retry", space)
	case h.WorkflowVisible() && h.Plan != nil:
		actions := monitor.HelmActionRow(width, "v", "Review changes", "a", "More ▾")
		body = actions + "\n\n" + monitor.HelmPreviewBody(h, width, space-lipgloss.Height(actions)-1)
	case h.WorkflowVisible() && h.Draft != nil:
		shadow := *m
		shadow.form = h.Draft.Form
		body = shadow.formBody(width, space)
	case h.Timeline != nil && h.Detail == nil:
		body = monitor.HelmTimelineBody(h, width, space)
	case h.Detail != nil:
		body = m.helmDetailBody(h, width, space)
	default:
		body = m.helmCollectionBody(h, width, space)
	}
	base := ui.StyledView(header+gap+body, width, height)
	pickerWidth, pickerHeight := ui.PickerSize(width, height)
	if h.Namespaces != nil {
		return ui.PickerOverlay(base, m.namespacePickerView(h.Namespaces, h.Input, h.NamespaceScope(), h.PickerCursor, pickerWidth-4, pickerHeight-2), width, height)
	}
	if h.Menu != nil {
		label := "Helm actions"
		if h.MenuKind == "release" {
			label = "Release · latest revision on selection"
		}
		if h.MenuKind == "revision" {
			label = "Recorded revision"
		}
		if h.MenuKind == "sort" {
			label = "Sort by · select again to reverse"
		}
		if h.MenuKind == "resource" {
			label = "Resource actions"
		}
		if h.MenuKind == "values" {
			label = "Values"
		}
		if h.MenuKind == "chart-document" {
			label = "Chart document"
		}
		choices, _ := h.MenuChoices()
		prefix := ui.Heading(label) + "\n\n"
		if h.InputKind == "picker" && h.Input != nil {
			prefix = ui.Heading(label) + "\n" + ui.SearchField(*h.Input, pickerWidth-4, m.focus == 1, "Filter", "Type to search…", "") + "\n"
		}
		body := ui.ListView(choices, h.Choice, pickerWidth-4, max(1, pickerHeight-2-lipgloss.Height(prefix)))
		if len(choices) == 0 {
			body = ui.Muted.Render("No matches.")
		}
		menu := prefix + body
		return ui.PickerOverlay(base, menu, width, height)
	}
	return base
}

func (m *model) helmHeader(h *monitor.HelmState, width int) string {
	namespace := h.Preference.Namespace
	selectable := !h.WorkflowVisible()
	if !selectable {
		if h.Plan != nil {
			namespace = h.Plan.Namespace
		} else if h.Draft != nil {
			for _, field := range h.Draft.Form.Fields {
				if field.Key == "namespace" {
					namespace = field.Value.Text
				}
			}
		}
	}
	if !selectable {
		if namespace == "" || namespace == "*" {
			namespace = "choose in chart inputs"
		}
		namespace = "ns " + namespace
	}
	header := ui.ScopeHeader(h.Config.Context, namespace, selectable, width)
	title := "Helm / " + h.Preference.Section
	state := ""
	if h.Busy {
		state = m.loading("Loading…")
		if h.Running == "execute" {
			state = m.loading("Applying…")
		}
	} else if h.Err != "" {
		state = "Unavailable"
		if len(h.Releases) > 0 || h.Detail != nil {
			state = "Stale"
		}
	}
	state = ui.Clipped(state, max(1, width/3))
	heading := ui.Heading(ui.Clipped(title, max(1, width-ansi.StringWidth(state)-2)))
	if state != "" {
		heading += strings.Repeat(" ", max(1, width-ansi.StringWidth(heading)-ansi.StringWidth(state))) + ui.ResourceStatusStyle(state).Render(state)
	}
	header += "\n" + heading
	if h.Err != "" {
		header += "\n" + ui.RemovedStyle.Render(ui.Clipped("! "+ui.OneLine(h.Err), width))
	} else if h.Notice != "" {
		header += "\n" + ui.Muted.Render(ui.Clipped(ui.OneLine(h.Notice), width))
	}
	return header
}

func (m *model) helmCollectionBody(h *monitor.HelmState, width, height int) string {
	label := "Filter releases"
	if h.Preference.Section == "Charts" {
		label = "Filter charts / releases"
	}
	var before []string

	if h.Input != nil && h.InputKind == "filter" {
		before = append(before, ui.SearchField(*h.Input, width, m.focus == 1, label, "Type to search…", "Enter apply · Esc cancel"))
	} else {
		query := h.Preference.Filter
		if query == "" {
			before = append(before, ui.Muted.Render(ui.Clipped("/ "+label, width)))
		} else {
			hint := "Backspace clear"
			before = append(before, ui.SearchField(textview.NewField(query), width, false, label, "", hint))
		}
	}
	prefix := strings.Join(before, "\n") + "\n"
	remaining := max(1, height-strings.Count(prefix, "\n"))
	if h.Preference.Section == "Charts" && len(h.ReleaseRows()) > 0 {
		return prefix + monitor.HelmChartCards(h, width, remaining)
	}
	columns, rows, summary := h.CollectionData()
	empty := "No releases in this scope."
	if h.Preference.Filter != "" {
		empty = "No releases match this filter."
	}
	if !h.Loaded && h.Err == "" {
		empty = m.loading("Loading releases…")
	} else if h.Err != "" && len(rows) == 0 {
		empty = "g Retry · n Namespace · ! Error details"
	}
	mark := " ↑"
	if h.Preference.Descending {
		mark = " ↓"
	}
	return prefix + monitor.HelmTable(columns, rows, h.Cursor, summary, empty, width, remaining, columns[h.SortColumn(columns)], mark)
}

func (m *model) helmDetailBody(h *monitor.HelmState, width, height int) string {
	s := h.Detail
	r := s.Release()
	back := "Esc list"
	if h.Timeline != nil {
		back = "Esc history"
	}
	identity := ui.TitleStyle.Render(ui.Clipped(ui.OneLine(r.Chart)+" · "+monitor.HelmChartVersion(r.Version), width)) + ui.Muted.Render("  ‹ "+back)
	release := ui.ActionButton("J", "Release: "+ui.OneLine(r.Key())+" ▾", false)
	revisionLabel := fmt.Sprintf("Revision: %d", s.Version)
	if s.Version < h.LatestRevision() {
		revisionLabel += " · historical"
	} else {
		revisionLabel += " · latest"
	}
	revision := ui.ActionButton("F", revisionLabel+" ▾", false)
	selectors := release + "  " + revision
	if lipgloss.Width(selectors) > width {
		selectors = ui.Clipped(release, width) + "\n" + ui.Clipped(revision, width)
	}
	gap := "\n"
	if height >= 18 {
		gap = "\n\n"
	}
	head := ui.Clipped(identity, width) + gap + selectors + gap + monitor.HelmSectionTabs(h.Tab, width) + "\n"
	if height >= 11 {
		buttons := []string{ui.ActionButton("a", "More ▾", false)}
		if h.Tab == 5 {
			buttons = append([]string{ui.ActionButton("Enter", "Full configuration", false)}, buttons...)
		}
		if h.Tab == 2 {
			buttons = append([]string{ui.ActionButton("Enter", monitor.HelmValueLabels[h.ValuesVariant]+" ▾", false)}, buttons...)
		}
		if h.Tab == 1 {
			buttons = append([]string{ui.ActionButton("Enter", monitor.HelmChartDocuments[h.ChartDocument]+" ▾", false)}, buttons...)
		}
		if s.Version < h.LatestRevision() {
			buttons = append(buttons, ui.ActionButton("B", "Preview rollback", false))
		}
		head += strings.Join(ui.ActionButtonRows(buttons, width), "\n") + "\n\n"
	}
	if h.Find != nil && h.Tab == 5 {
		head += h.FindBar(width) + "\n"
	}

	remaining := max(1, height-strings.Count(head, "\n"))
	var content string
	switch h.Tab {
	case 0:
		content = monitor.HelmStyledText(monitor.HelmReleaseSummary(h, width), width, remaining, &h.Subcursor)
	case 5:
		content = monitor.HelmResourceInventory(h, width, remaining)
	default:
		text := monitor.HelmDetailText(h)
		if h.Find != nil {
			return head + h.Find.Reader.View(width, remaining, ui.WorkspaceTheme())
		}
		if h.Tab != 7 {
			lines := strings.Split(ui.Plain(text), "\n")
			for i, line := range lines {
				lines[i] = docedit.YAMLLine(line, ui.WorkspaceTheme())
			}
			content = monitor.HelmStyledText(strings.Join(lines, "\n"), width, remaining, &h.Subcursor)
		} else {
			content = monitor.HelmScrollingText(text, width, remaining, &h.Subcursor)
		}
	}
	return head + content
}

func (m *model) helmHint() string {
	h := m.helmer()
	if h == nil {
		return "hh Help"
	}
	if h.Discard {
		return "←→ Choose · Enter Confirm · Esc Keep Editing"
	}
	if h.Namespaces != nil {
		if h.Namespaces.Manual {
			return "Enter Apply · Esc Back · hh Help"
		}
		return "↑↓ Choose · Enter Select · Esc Back · hh Help"
	}
	if h.Find != nil && h.Find.Reader.Searching {
		return "Enter Finish search · ↑↓ Matches · Esc Clear search"
	}
	if h.Input != nil {
		if h.InputKind == "picker" {
			return "↑↓ Choose · Enter Select · Esc Back"
		}
		return "Enter Apply · Esc Cancel"
	}
	if h.Busy {
		return "Esc Cancel request · hh Help"
	}
	if h.Confirm {
		return "↑↓ Inspect target · ←→ Choose · Enter Confirm · Esc Cancel"
	}
	if h.Document != nil {
		if h.Document.Searching {
			return "Enter Finish search · Esc Cancel search"
		}
		if h.Document.ReadOnly {
			return "/ Find · y Copy · Esc Back · hh Help"
		}
		if h.Document.Mode == docedit.Insert {
			return "Tab Indent · Esc Normal mode"
		}
		return "i Insert · :w Save draft · :q Close · hh Help"
	}
	if h.Comparison != nil {
		return "[ ] Change · / Find · Esc Back · hh Help"
	}
	if h.Menu != nil {
		return "↑↓ Choose · Enter Select · Esc Back · hh Help"
	}
	if h.Err != "" {
		return "! Error details · g Refresh · Esc Back · hh Help"
	}
	if h.Draft != nil && h.Plan == nil && h.WorkflowVisible() {
		if h.Draft.Form.Index >= len(h.Draft.Form.Fields) {
			return "↑↓ Actions · Enter Activate · Esc Back · hh Help"
		}
		return "↑↓ Fields · Enter Next/activate · Esc Actions · hh Help"
	}
	if h.Plan != nil && h.WorkflowVisible() {
		return "v Review · a Actions · Esc Back · hh Help"
	}
	if h.Timeline != nil && h.Detail == nil {
		return "↑↓ Revision · Enter Chart · v Changes · B Rollback · Esc List"
	}
	if h.Detail != nil {
		if h.Tab == 5 {
			return "↑↓ Resource · ←→ Section · r Release · F Revision · Enter Inspect · Esc Back"
		}
		return "↑↓ Scroll · ←→ Section · r Release · F Revision · Enter Choose document · Esc Back"
	}
	if h.Preference.Section == "Releases" || h.Preference.Section == "Charts" {
		return "Enter Open · / Filter · n Namespace · Esc Workspaces · hh Help"
	}
	return "Enter Open · / Filter · n Namespace · Esc Workspaces · hh Help"
}
