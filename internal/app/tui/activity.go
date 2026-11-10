package tui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	"github.com/simjay/plous/internal/workspace"
)

func (m *model) activityBody(width, height int) string {
	entries := m.activityEntries()
	header := ui.Muted.Render(ui.Clipped(ui.OneLine(m.current().Name), width)) + "\n"
	header += m.activityTypeControls(width) + "\n"
	scope := "All connections"
	if c := m.currentConnection(); c != nil {
		scope = "Connection: " + c.Name + " · Workspace: all"
	} else if m.tool != "" {
		scope = "Tool: " + m.tool + " · Workspace: all"
	}
	noun := "entries"
	if len(entries) == 1 {
		noun = "entry"
	}
	header += ui.Muted.Render(ui.Clipped(ui.OneLine(scope)+fmt.Sprintf(" · %d %s", len(entries), noun), width)) + "\n"
	if m.searches[1] != nil {
		header += m.searchBar(1, width, "Search activity") + "\n"
	}
	if m.activityUI.Pending || m.activityUI.Load != nil {
		header += ui.Muted.Render(m.loading("Loading activity…")) + "\n"
	}
	if m.activityUI.Error != "" {
		header += ui.Clipped(ui.MouseCommand(ui.ShortcutStyle.Render("g Retry"), "refresh")+"  "+ui.MouseCommand(ui.ShortcutStyle.Render("d Error details"), "describe"), width) + "\n"
		header += ui.RemovedStyle.Render(ui.Clipped(ui.OneLine(m.activityUI.Error), width)) + "\n"
	}
	available := max(0, height-strings.Count(header, "\n"))
	if len(entries) == 0 {
		message := "No activity yet."
		if m.activityUI.Filter != activity.All || m.searches[1] != nil {
			message = "No matching activity.\n←→ Change type · / Search"
		}
		if m.searches[1] != nil {
			message = "No matching activity.\n" + ui.MouseAction(ui.ShortcutStyle.Render("Esc Clear search"), "esc")
		}
		if m.activityUI.Pending || m.activityUI.Load != nil || m.activityUI.Error != "" {
			message = ""
		}
		return ui.StyledView(header+message, width, height)
	}
	selected := min(max(0, m.activityPosition()), len(entries)-1)
	listWidth := width
	previewWidth := 0
	if width >= 100 && available >= 8 {
		listWidth = width * 55 / 100
		previewWidth = width - listWidth - 3
	}
	list := activity.Timeline(entries, selected, listWidth, available)
	if previewWidth > 0 {
		preview := ui.TitleStyle.Render("Preview") + "\n\n" + ui.StyledTextView(entries[selected].Preview(), previewWidth, available-4, 0)
		preview = lipgloss.NewStyle().Width(previewWidth).Height(available - 1).Render(preview)
		preview += "\n" + ui.MouseAction(ui.ShortcutStyle.Render("Enter Details"), "enter")
		divider := strings.TrimSuffix(strings.Repeat(" │ \n", available), "\n")
		list = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Height(available).Render(list), ui.Muted.Render(divider), preview)
	}
	return ui.StyledView(header+list, width, height)
}

func (m *model) activityTypeControls(width int) string {
	controls := []string{ui.Muted.Render("Type")}
	for filter := activity.All; filter <= activity.HelmHistory; filter++ {
		label := filter.String()
		style := ui.Muted
		if filter == m.activityUI.Filter {
			label, style = "["+label+"]", ui.Selected
		}
		controls = append(controls, ui.MouseTarget(style.Render(label), fmt.Sprintf("activity-type/%d", filter)))
	}
	if row := strings.Join(controls, "  "); ansi.StringWidth(row) <= width {
		return row
	}
	previous, next := (int(m.activityUI.Filter)+3)%4, (int(m.activityUI.Filter)+1)%4
	return ui.MouseTarget(ui.ShortcutStyle.Render("‹"), fmt.Sprintf("activity-type/%d", previous)) + " " +
		ui.Selected.Render(ui.Clipped(m.activityUI.Filter.String(), max(1, width-4))) + " " +
		ui.MouseTarget(ui.ShortcutStyle.Render("›"), fmt.Sprintf("activity-type/%d", next))
}

func (m *model) cycleActivityType(step int) {
	entries := m.activityEntries()
	identity := ""
	if pos := m.activityPosition(); pos >= 0 && pos < len(entries) {
		identity = entries[pos].Identity()
	}
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		m.restoreActivityPosition(search)
	}
	m.activityUI.Filter = activity.Filter((int(m.activityUI.Filter) + step + 4) % 4)
	m.rebaseActivitySearch(m.searches[1])
	m.setActivityPosition(0)
	for i, entry := range m.activityEntries() {
		if entry.Identity() == identity {
			m.setActivityPosition(i)
			break
		}
	}
}

func (m *model) activityCommand(command string) bool {
	if command != "observations" && command != "refresh" && command != "describe" {
		return false
	}
	if m.mode != 2 || m.current() == nil || m.page != "" || m.form != nil || m.settingsOverlay != nil || m.helpVisible() {
		return false
	}
	if command == "observations" && m.detail != "" && m.activityRecord != nil && m.activityRecord.Operation == m.operationDetail && len(m.activityRecord.Observations) > 0 {
		m.activityObservations = !m.activityObservations
		m.updateActivityRecordDetail()
		m.cursor = 0
	} else if command == "describe" && m.detail == "" && m.activityUI.Error != "" {
		if search := m.searches[1]; search != nil && search.kind == "activity" {
			m.activityListSearch, m.searches[1] = search, nil
		}
		m.operationDetail, m.activityRecord, m.activityReview = nil, nil, nil
		m.detail, m.cursor = ui.Plain(m.activityUI.Error), 0
	} else if command == "refresh" && m.detail == "" {
		m.loadHistory()
	} else {
		return false
	}
	m.focus, m.status = 1, ""
	return true
}

func (m *model) activityHasObservations() bool {
	return m.activityRecord != nil && m.activityRecord.Operation == m.operationDetail && len(m.activityRecord.Observations) > 0
}

func (m *model) activityOperationControls(width int) string {
	controls := []string{ui.Muted.Render("Release unavailable")}
	if m.activityReleaseUnavailable(m.operationDetail) == "" {
		controls[0] = ui.MouseAction(ui.ShortcutStyle.Render("Enter Open release"), "enter")
	}
	if m.activityHasObservations() {
		label := "O Observations"
		if m.activityObservations {
			label = "O Hide observations"
		}
		controls = append(controls, ui.MouseCommand(ui.ShortcutStyle.Render(label), "observations"))
	}
	separator := "  "
	if ansi.StringWidth(strings.Join(controls, separator)) > width {
		separator = "\n"
	}
	return strings.Join(controls, separator)
}

func (m *model) scrollActivityDetail(key string) bool {
	if m.mode != 2 || m.page != "" || m.detail == "" || m.form != nil {
		return false
	}
	if key != "up" && key != "down" && key != "pgup" && key != "pgdown" && key != "home" && key != "end" {
		return false
	}
	width, height := max(1, m.panelWidths()[1]-4), max(1, m.height-8)
	if m.command != nil {
		height -= 3
	}
	if m.activityDetailActive() {
		height -= 2
	} else if m.operationDetail != nil {
		height -= strings.Count(m.activityOperationControls(width), "\n") + 2
	}
	if m.searches[1] != nil {
		height -= ui.SearchBarHeight
	}
	height = max(1, height)
	maximum := max(0, strings.Count(ansi.Wrap(m.detail, width, ""), "\n")+1-height)
	m.cursor = min(max(0, m.cursor), maximum)
	switch key {
	case "up":
		m.cursor--
	case "down":
		m.cursor++
	case "pgup":
		m.cursor -= height
	case "pgdown":
		m.cursor += height
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = maximum
	}
	m.cursor = min(max(0, m.cursor), maximum)
	return true
}

func (m *model) restoreActivitySearch() {
	if s := m.activityListSearch; s != nil {
		if s.scope == m.viewKey()+"/" {
			m.searches[1] = s
		}
		m.activityListSearch = nil
	}
}

func (m *model) setActivityDetail(change workspace.Change) {
	m.operationDetail = nil
	workspaceID := change.WorkspaceID
	if workspaceID == "" {
		workspaceID = change.After.ID
	}
	if workspaceID == "" {
		workspaceID = m.active
	}
	data, _ := json.Marshal(change)
	summary := activity.ChangeDetail(change)
	r := &activity.ActivityReview{
		WorkspaceID: workspaceID,
		Summary:     summary,
		Before:      activity.WorkspaceSnapshot(change.Before),
		After:       activity.WorkspaceSnapshot(&change.After),
		Target:      ui.OneLine(change.After.Name) + " · " + activity.ChangeSummary(change) + " · Read-only",
		Identity:    fmt.Sprintf("%x", sha256.Sum256(append([]byte(workspaceID+"\x00"), data...))),
		BeforeLabel: "Empty workspace",
		AfterLabel:  fmt.Sprintf("Revision %d", change.After.Revision),
		Revision:    change.Revision,
	}
	if change.Before != nil {
		r.BeforeLabel = fmt.Sprintf("Revision %d", change.Before.Revision)
	}
	m.activityReview = r
	m.detail, m.cursor = summary, 0
}

func (m *model) activitySummaryVisible() bool {
	r := m.activityReview
	return r != nil && m.current() != nil && m.active == r.WorkspaceID && m.mode == 2 && m.page == "" && m.detail == r.Summary
}

func (m *model) activityDetailActive() bool {
	return m.activitySummaryVisible() && !m.activityReview.Open
}

func (m *model) activityReviewVisible() bool {
	return m.activitySummaryVisible() && m.activityReview.Open
}

func (m *model) openActivityReview() bool {
	if !m.activitySummaryVisible() {
		return false
	}
	r := m.activityReview
	if r.Comparison == nil {
		r.Comparison = diffview.New(r.Before, r.After, r.Target, r.Revision, false)
		r.Comparison.ID = r.Identity
		r.Comparison.BeforeLabel, r.Comparison.AfterLabel = r.BeforeLabel, r.AfterLabel
		r.Comparison.BackLabel = "Back to summary"
	}
	r.Open = true
	return true
}

func (m *model) activityReviewSearching() bool {
	return m.activityReviewVisible() && m.activityReview.Comparison.Reader.Searching
}

func (m *model) activityReviewKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.activityReviewVisible() {
		return false, nil
	}
	r := m.activityReview
	decision, copied := r.Comparison.Key(msg.String(), msg.Text)
	if decision.ID == r.Identity && (decision.Action == "back" || decision.Action == "close") {
		r.Open = false
	}
	return true, m.copyWorkspaceText(copied)
}

func (m *model) activityReviewPaste(text string) bool {
	if !m.activityReviewVisible() {
		return false
	}
	m.activityReview.Comparison.Reader.Paste(text)
	return true
}

func (m *model) activityReviewBody(width, height int) string {
	if !m.activityReviewVisible() {
		return ""
	}
	m.activityReview.Comparison.ViewActions = []textview.ActionGroup{{Label: "View", Actions: []textview.Action{m.expandAction()}}}
	return m.activityReview.Comparison.View(width, height, ui.WorkspaceTheme())
}

func (m *model) stopActivity() { m.activityUI.Stop() }
func (m *model) loadHistory()  { m.activityUI.Reload(m.active, m.current() != nil && m.store != nil) }
func (m *model) syncActivity() tea.Cmd {
	if m.mode != 2 || m.page != "" || m.form != nil || m.store == nil || m.exploreClosed {
		return nil
	}
	return m.activityUI.Start(m.store)
}

func (m *model) applyActivity(result activity.ActivityResult) {
	if !m.activityUI.Matches(result, m.active) {
		return
	}
	selected := ""
	entries := m.activityEntries()
	if pos := m.activityPosition(); pos >= 0 && pos < len(entries) {
		selected = entries[pos].Identity()
	}
	if !m.activityUI.Apply(result) {
		return
	}
	m.rebaseActivitySearch(m.searches[1])
	m.rebaseActivitySearch(m.activityListSearch)
	m.setActivityPosition(min(max(0, m.activityPosition()), max(0, len(m.activityEntries())-1)))
	for i, entry := range m.activityEntries() {
		if entry.Identity() == selected {
			m.setActivityPosition(i)
			break
		}
	}
}

func (m *model) activityKey() string {
	return m.active + "/" + m.tool + "/" + m.connection + "/" + modes[2]
}

func (m *model) activityPosition() int {
	if m.activityUI.Filter == activity.All {
		return m.positions[m.activityKey()]
	}
	return m.activityUI.Positions[fmt.Sprintf("%s/%d", m.activityKey(), m.activityUI.Filter)]
}

func (m *model) setActivityPosition(position int) {
	if m.activityUI.Filter == activity.All {
		m.positions[m.activityKey()] = position
		return
	}
	if m.activityUI.Positions == nil {
		m.activityUI.Positions = map[string]int{}
	}
	m.activityUI.Positions[fmt.Sprintf("%s/%d", m.activityKey(), m.activityUI.Filter)] = position
}

func (m *model) rebaseActivitySearch(s *panelSearch) {
	if s == nil || s.kind != "activity" || s.scope != m.activityKey()+"/" {
		return
	}
	entries := m.activityUI.Entries(m.active, m.tool, m.connection, "")
	s.cursor = min(max(0, s.cursor), max(0, len(entries)-1))
	for i, entry := range entries {
		if entry.Identity() == s.activityIdentity {
			s.cursor = i
			break
		}
	}
	s.activityFilter = m.activityUI.Filter
}

func (m *model) restoreActivityPosition(s *panelSearch) {
	if s.scope == m.activityKey()+"/" {
		m.rebaseActivitySearch(s)
		m.setActivityPosition(s.cursor)
	} else if s.activityFilter == activity.All {
		m.positions[strings.TrimSuffix(s.scope, "/")] = s.cursor
	}
}

func (m *model) activityEntries() []activity.ActivityEntry {
	query := ""
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		query = search.input.Text
	} else if search := m.activityListSearch; search != nil && search.scope == m.viewKey()+"/" {
		query = search.input.Text
	}
	return m.activityUI.Entries(m.active, m.tool, m.connection, query)
}

func (m *model) openActivityEntry(e activity.ActivityEntry) {
	if search := m.searches[1]; search != nil && search.kind == "activity" {
		m.activityListSearch, m.searches[1] = search, nil
	}
	m.activityRecord, m.activityObservations = &e, false
	if e.Change != nil {
		m.setActivityDetail(*e.Change)
		return
	}
	op := e.Operation
	m.operationDetail = op
	m.activityReview = nil
	m.cursor = 0
	m.updateActivityRecordDetail()
}

func (m *model) activityReleaseUnavailable(op *workspace.Operation) string {
	if op == nil {
		return "No release is recorded for this entry."
	}
	if op.WorkspaceID != m.active {
		return "The recorded release belongs to another workspace."
	}
	found := false
	if w := m.current(); w != nil {
		for _, c := range w.Connections {
			if c.ID == op.ConnectionID && monitorCluster(c.Tool) {
				if c.Settings["context"] != op.Context || op.ConnectionFingerprint != "" && monitor.HelmConfig(&c).Fingerprint() != op.ConnectionFingerprint {
					return "The saved Helm connection has changed. Restore its recorded settings before opening this release."
				}
				found = true
			}
		}
	}
	if !found {
		return "The recorded Helm connection is no longer saved in this workspace."
	}
	return ""
}

func (m *model) updateActivityRecordDetail() {
	m.detail = m.activityRecord.Detail(m.activityObservations)
	if reason := m.activityReleaseUnavailable(m.operationDetail); reason != "" {
		m.detail += "\n\n" + reason
	}
}

func (m *model) openOperationRelease() {
	op := m.operationDetail
	if op == nil {
		return
	}
	if reason := m.activityReleaseUnavailable(op); reason != "" {
		m.status = reason
		return
	}
	m.selectScope(op.WorkspaceID, "helm", op.ConnectionID)
	h := m.helmer()
	if !m.chooseHelmSection("Releases") {
		return
	}
	m.mode, m.focus = 1, 1
	revision := op.Revision
	if op.Kind != "revision" {
		revision = 0
	}
	h.RequestedRelease = &helmtool.Release{Name: op.Release, Namespace: op.Namespace, Revision: revision}
	if h.Client != nil {
		h.Pending = "detail"
	}
	m.rememberHelm(h)
}
