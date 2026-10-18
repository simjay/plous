package monitor

import (
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func gitOpsValue(value string) string {
	if value == "" {
		return "Not reported"
	}
	return ui.Plain(value)
}

func gitOpsTime(value time.Time) string {
	if value.IsZero() {
		return "Not reported"
	}
	return value.UTC().Format("2006-01-02 15:04:05 UTC")
}

func gitOpsDestination(destination gitops.Destination) string {
	if destination.Remote {
		return "Remote cluster / " + destination.Namespace
	}
	cluster := destination.Name
	if cluster == "" {
		cluster = destination.Server
	}
	if cluster == "https://kubernetes.default.svc" {
		cluster = "Controller cluster"
	}
	if cluster == "" {
		cluster = "Not reported"
	}
	if destination.Namespace != "" {
		cluster += " / " + destination.Namespace
	}
	return cluster
}

func gitOpsStyle(column, value string) lipgloss.Style {
	if column != "Sync" && column != "Health" && column != "Status" {
		return lipgloss.NewStyle()
	}
	switch value {
	case "Synced", "Healthy", "Ready", "Up to date", "Generated":
		return ui.AddedStyle
	case "Degraded", "Missing", "Not ready", "Failed", "Stalled":
		return ui.RemovedStyle
	case "OutOfSync", "Progressing", "Reconciling", "Awaiting status":
		return ui.ChangedStyle
	default:
		return ui.Muted
	}
}

func (s *GitOpsState) Body(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if s.Detail != nil {
		return s.detailBody(width, height)
	}
	heading := ui.Heading(s.Title() + " / " + string(s.Section))
	namespace := s.Namespace
	if namespace == "" {
		namespace = "All namespaces"
	}
	scope := ui.MouseAction(ui.Selected.Render("[ "+ui.OneLine(namespace)+" ▾ ]"), "n")
	var lines []string
	if gap := width - ansi.StringWidth(heading) - ansi.StringWidth(scope); gap >= 2 {
		lines = append(lines, heading+strings.Repeat(" ", gap)+scope)
	} else {
		lines = append(lines, ui.Clipped(heading, width), ui.Clipped(scope, width))
	}
	if height >= 10 || s.Input != nil || s.Query != "" {
		field := textview.NewField(s.Query)
		if s.Input != nil {
			field = *s.Input
		}
		hint := ""
		if s.Query != "" || s.Input != nil {
			hint = "Backspace clear · Esc cancel"
		}
		lines = append(lines, ui.SearchField(field, width, s.Input != nil, "Filter", "Name, namespace or status", hint))
	}
	if status := s.readStatus(); status != "" {
		lines = append(lines, ui.Clipped(ui.ChangedStyle.Render(status), width))
	}
	available := max(0, height-lipgloss.Height(strings.Join(lines, "\n"))-1)
	rows := s.Rows()
	if len(rows) == 0 {
		title, description := s.emptyText()
		lines = append(lines, ui.StyledTextView(ui.TitleStyle.Render(title)+"\n\n"+ui.Plain(description), width, available, 0))
	} else {
		s.Cursor = max(0, min(s.Cursor, len(rows)-1))
		previewHeight := 0
		if available >= 9 {
			previewHeight = 4
		}
		tableHeight := max(1, available-previewHeight)
		table := s.table(rows, width, tableHeight)
		lines = append(lines, lipgloss.NewStyle().Height(tableHeight).MaxHeight(tableHeight).Render(table))
		if previewHeight > 0 {
			lines = append(lines, s.preview(rows[s.Cursor], width))
		}
	}
	if available > 0 {
		lines = append(lines, ui.Clipped(ui.Muted.Render(s.summary(len(rows))), width))
	}
	return ui.StyledView(strings.Join(lines, "\n"), width, height)
}

func (s *GitOpsState) summary(count int) string {
	summary := fmt.Sprintf("%d %s", count, strings.ToLower(string(s.Section)))
	if count == 0 && s.Snapshot.State != gitops.Available && s.Snapshot.State != gitops.Empty {
		summary = "List unavailable"
	}
	if count != len(s.Snapshot.Items) {
		summary = fmt.Sprintf("%d of %d %s", count, len(s.Snapshot.Items), strings.ToLower(string(s.Section)))
	}
	if !s.Snapshot.ReadAt.IsZero() {
		summary += " · Plous read: " + s.Snapshot.ReadAt.UTC().Format("15:04:05 UTC")
	}
	return summary
}

func (s *GitOpsState) preview(item gitops.Item, width int) string {
	action := ui.MouseAction(ui.Selected.Render("Enter Details"), "enter")
	nameWidth := max(1, width-ansi.StringWidth(action)-2)
	heading := SummaryHeading(ui.Clipped(ui.OneLine(item.Ref.Name), max(1, nameWidth-2)), nameWidth)
	heading += strings.Repeat(" ", max(1, width-ansi.StringWidth(heading)-ansi.StringWidth(action))) + action
	context := item.Ref.Namespace
	if item.Destination.Namespace != "" {
		context += " · Target " + item.Destination.Namespace
	}
	if len(item.Sources) > 0 && item.Sources[0].Ref.Name != "" {
		context += " · Source " + item.Sources[0].Ref.Label()
	}
	observation := gitOpsObserved(item)
	if item.Ref.Kind == "AppProject" || item.Ref.Kind == "ApplicationSet" {
		observation = item.RelatedMessage
	}
	lines := []string{heading, ui.Clipped(ui.OneLine(gitOpsValue(item.Reason)), width), ui.Muted.Render(ui.Clipped(ui.OneLine(context), width)), ui.Muted.Render(ui.Clipped(ui.OneLine(observation), width))}
	return strings.Join(lines, "\n")
}

func (s *GitOpsState) readStatus() string {
	if s.Err != "" {
		return "Previous observations · " + ui.OneLine(s.Err)
	}
	if s.Loading {
		if len(s.Snapshot.Items) > 0 || s.Detail != nil {
			return "Refreshing · previous observations remain visible"
		}
		return "Reading cluster objects…"
	}
	if s.Snapshot.Truncated && s.Snapshot.State != gitops.Partial {
		return "Partial results · List limit reached. Narrow the namespace scope."
	}
	if s.Snapshot.State == gitops.Partial || s.Snapshot.Truncated {
		var issues []string
		for _, issue := range s.Snapshot.Issues {
			label := strings.ReplaceAll(string(issue.State), "-", " ")
			if issue.Kind != "" {
				label = issue.Kind + ": " + label
			}
			issues = append(issues, label)
		}
		message := s.Snapshot.Message
		if len(issues) > 0 {
			message = strings.Join(issues, "; ")
		}
		return "Partial results · " + ui.OneLine(message)
	}
	return ""
}

func (s *GitOpsState) emptyText() (string, string) {
	if s.Loading || !s.Loaded {
		return "Reading cluster objects…", "Uses the saved Kubernetes connection."
	}
	if (s.Input != nil && s.Input.Text != "" || s.Query != "") && (s.Snapshot.State == gitops.Available || s.Snapshot.State == gitops.Empty) {
		return "No matches", "Change the filter, or press Backspace to clear it."
	}
	message := s.Snapshot.Message
	for _, issue := range s.Snapshot.Issues {
		message += "\n" + strings.TrimSpace(issue.Kind+": "+issue.Message)
	}
	switch s.Snapshot.State {
	case gitops.ReadDenied:
		return "Read denied", message
	case gitops.APIUnavailable:
		return s.Title() + " API unavailable", message
	case gitops.ConnectionUnavailable:
		return "Connection unavailable", message
	case gitops.Partial:
		return "Some reads are unavailable", message
	}
	return "No " + string(s.Section), "No objects were returned in this namespace scope."
}

func (s *GitOpsState) collectionData(rows []gitops.Item) ([]string, [][]string) {
	columns := []string{"Name", "Namespace", "Status", "Applied revision"}
	switch s.Section {
	case gitops.Applications:
		columns = []string{"Name", "Namespace", "Sync", "Health", "Target namespace"}
	case gitops.ApplicationSets:
		columns = []string{"Name", "Namespace", "Status", "Generator", "Applications"}
	case gitops.Projects:
		columns = []string{"Name", "Namespace", "Applications", "Destinations", "Repositories"}
	case gitops.HelmReleases:
		columns = []string{"Name", "Namespace", "Status", "Chart", "Version"}
	case gitops.Sources:
		columns = []string{"Name", "Namespace", "Kind", "Status", "Fetched revision"}
	}
	var data [][]string
	for _, item := range rows {
		field := func(label string) string { return gitOpsValue(gitops.FieldValue(item, label)) }
		row := []string{item.Ref.Name, item.Ref.Namespace}
		switch s.Section {
		case gitops.Applications:
			row = append(row, gitOpsValue(item.Sync), gitOpsValue(item.Health), gitOpsValue(item.Destination.Namespace))
		case gitops.ApplicationSets:
			row = append(row, gitOpsValue(item.Status), field("Generator"), field("Applications"))
		case gitops.Projects:
			row = append(row, field("Applications"), field("Destinations"), field("Repositories"))
		case gitops.HelmReleases:
			row = append(row, gitOpsValue(item.Status), field("Chart"), field("Latest release chart version"))
		case gitops.Sources:
			row = append(row, item.Ref.Kind, gitOpsValue(item.Status), shortGitOpsRevision(field("Fetched revision")))
		default:
			row = append(row, gitOpsValue(item.Status), shortGitOpsRevision(field("Last applied revision")))
		}
		data = append(data, row)
	}
	return columns, data
}

func shortGitOpsRevision(value string) string {
	for _, marker := range []string{"sha1:", "sha256:"} {
		if start := strings.Index(value, marker); start >= 0 && len(value) > start+len(marker)+12 {
			return value[:start+len(marker)+12]
		}
	}
	return value
}

func (s *GitOpsState) table(rows []gitops.Item, width, height int) string {
	if height <= 0 {
		return ""
	}
	columns, data := s.collectionData(rows)
	indices := make([]int, len(columns))
	for i := range indices {
		indices[i] = i
	}
	indices, widths, compact := CollectionTableColumns(columns, data, indices, width, false)
	if s.Section == gitops.Applications && len(indices) < 4 {
		compact = true
	}
	var before string
	if !compact && height >= 4 {
		header, _ := textview.MouseLayout(ui.CollectionTableRow(columns, columns, indices, widths, width, true, false, gitOpsStyle))
		before = header + "\n" + ui.Muted.Render(strings.Repeat("─", width)) + "\n"
	}
	body := ui.CollectionWindow(len(rows), s.Cursor, max(1, height-strings.Count(before, "\n")), func(index int) []string {
		item, row, active := rows[index], data[index], index == s.Cursor
		var lines []string
		if !compact {
			lines = []string{ui.CollectionTableRow(columns, row, indices, widths, width, false, active, gitOpsStyle)}
		} else {
			lines = []string{ui.ChoiceLine(item.Ref.Namespace+"/"+item.Ref.Name, active, width)}
			if height >= 4 {
				var values []string
				for i := 2; i < len(columns); i++ {
					values = append(values, ui.Muted.Render(columns[i]+" ")+gitOpsStyle(columns[i], row[i]).Render(ui.OneLine(row[i])))
				}
				for _, line := range strings.Split(ansi.Wrap(strings.Join(values, " · "), max(1, width-2), ""), "\n") {
					lines = append(lines, "  "+line)
				}
			}
		}
		for i, line := range lines {
			lines[i] = ui.MouseChoice(line, index, item.Ref.Key())
		}
		return lines
	})
	return ui.StyledView(before+body, width, height)
}

func gitOpsObserved(item gitops.Item) string {
	label := item.ObservedLabel
	if label == "" {
		label = "Controller observation"
	}
	return label + ": " + gitOpsTime(item.ObservedAt)
}

func (s *GitOpsState) detailBody(width, height int) string {
	lines := s.detailLines(width)
	if s.Find != nil {
		findLines := make([]textview.Line, len(lines))
		for i, line := range lines {
			findLines[i] = textview.Line{Text: ansi.Strip(line)}
		}
		s.Find.SetLines(findLines)
		return s.Find.View(width, height, ui.WorkspaceTheme())
	}
	s.detailHeight = height
	s.Scroll = max(0, min(s.Scroll, max(0, len(lines)-height)))
	if s.revealAction && s.DetailCursor < len(s.actionLines) {
		line := s.actionLines[s.DetailCursor]
		if line < s.Scroll {
			s.Scroll = line
		} else if line >= s.Scroll+height {
			s.Scroll = max(0, line-height+2)
		}
		s.revealAction = false
	}
	return ui.StyledView(strings.Join(lines[s.Scroll:min(len(lines), s.Scroll+height)], "\n"), width, height)
}

func (s *GitOpsState) detailLines(width int) []string {
	item := s.Detail
	var lines []string
	add := func(value string) {
		lines = append(lines, strings.Split(ansi.Wrap(value, max(1, width), ""), "\n")...)
	}
	fields := func(values ...kubernetes.DetailField) {
		for _, line := range RenderSummaryFields(values, width) {
			add(line)
		}
	}
	add(ui.Muted.Render(s.Title() + " · " + string(gitops.SectionFor(item.Ref))))
	add(ui.TitleStyle.Render(ui.OneLine(item.Ref.Label())))
	add(ui.Muted.Render(ui.OneLine(item.Ref.Namespace)))
	if status := s.readStatus(); status != "" {
		add(ui.ChangedStyle.Render(status))
	}
	if item.Ref.Kind == "Application" {
		fields(kubernetes.DetailField{Label: "Sync", Value: gitOpsValue(item.Sync)}, kubernetes.DetailField{Label: "Health", Value: gitOpsValue(item.Health)})
	} else if item.Ref.Kind != "AppProject" {
		fields(kubernetes.DetailField{Label: "Status", Value: gitOpsValue(item.Status)})
	}
	add("")
	add(gitOpsValue(item.Reason))
	add("")
	if item.Ref.Kind != "AppProject" {
		add(ui.Muted.Render(gitOpsObserved(*item)))
	}
	add(ui.Muted.Render("Plous read: " + gitOpsTime(item.ReadAt)))
	if item.Stale {
		add(ui.ChangedStyle.Render("Status has not caught up with the current object generation."))
	}
	if item.Suspended {
		add(ui.Muted.Render("Reconciliation is suspended. Retained conditions describe earlier observations."))
	}
	if s.Tool == gitops.Flux {
		add(ui.Muted.Render("Ready reports reconciliation success, not continuous workload health."))
	}
	actions := s.Actions()
	s.actionLines = nil
	actionIndex := 0
	link := func(action GitOpsAction) {
		s.actionLines = append(s.actionLines, len(lines))
		label := action.Label
		if action.Disabled != "" {
			label += " (unavailable)"
		}
		line := ui.ChoiceLine(label, actionIndex == s.DetailCursor, width)
		if action.Disabled != "" && actionIndex != s.DetailCursor {
			line = ui.Muted.Render(ansi.Strip(line))
		}
		add(ui.MouseChoice(line, actionIndex, action.Kind+"/"+action.Ref.Key()))
		actionIndex++
	}
	if item.Ref.Kind == "AppProject" || item.Ref.Kind == "ApplicationSet" {
		add("")
		add(SummaryHeading("Applications", width))
		add(ui.Muted.Render(ui.Plain(item.RelatedMessage)))
		if len(item.Related) == 0 {
			add(ui.Muted.Render("No related applications available."))
		}
		for _, related := range item.Related {
			link(actions[actionIndex])
			add("  " + ui.Muted.Render("Sync ") + gitOpsStyle("Sync", related.Sync).Render(gitOpsValue(related.Sync)) + " · " + ui.Muted.Render("Health ") + gitOpsStyle("Health", related.Health).Render(gitOpsValue(related.Health)))
		}
	}
	if len(item.Dependencies) > 0 {
		add("")
		add(SummaryHeading("Dependencies", width))
		for range item.Dependencies {
			link(actions[actionIndex])
		}
	}
	for _, source := range item.Sources {
		if source.Ref.Name != "" {
			if actionIndex == len(item.Related)+len(item.Dependencies) {
				add("")
				add(SummaryHeading("Sources", width))
			}
			link(actions[actionIndex])
		}
	}
	add("")
	if item.Ref.Kind == "Application" || item.Ref.Kind == "Kustomization" || item.Ref.Kind == "HelmRelease" || len(item.Resources) > 0 {
		add(SummaryHeading("Managed resources", width))
		if reason := s.DestinationErrors[item.Ref.Key()]; reason != "" {
			add(ui.ChangedStyle.Render(ui.Plain(reason)))
		} else if item.Destination.Remote {
			add(ui.Muted.Render("Remote destination access is unavailable. Plous does not read the controller's kubeconfig credentials."))
		}
		if len(item.Resources) == 0 {
			message := "No resources reported."
			if !item.InventoryReported {
				message = "Not reported by this controller object."
			}
			add(ui.Muted.Render(message))
		}
		for _, resource := range item.Resources {
			link(actions[actionIndex])
			if s.Tool == gitops.ArgoCD {
				add("  " + ui.Muted.Render("Sync "+gitOpsValue(resource.Sync)+" · Health "+gitOpsValue(resource.Health)))
			}
			if resource.Message != "" {
				add("  " + ui.Plain(resource.Message))
			}
		}
		add("")
	}
	configuration := actions[actionIndex]
	if s.ConfigurationExpanded {
		configuration.Label = "▾ " + configuration.Label
	} else {
		configuration.Label = "▸ " + configuration.Label
	}
	link(configuration)
	if s.ConfigurationExpanded {
		fields(kubernetes.DetailField{Label: "Kind", Value: item.Ref.Kind}, kubernetes.DetailField{Label: "Namespace", Value: item.Ref.Namespace})
		if item.Ref.Kind == "Application" || item.Destination.Namespace != "" || item.Destination.Remote {
			fields(kubernetes.DetailField{Label: "Destination", Value: gitOpsDestination(item.Destination)})
		}
		for _, field := range item.Fields {
			fields(kubernetes.DetailField{Label: field.Label, Value: field.Value})
		}
		for i, source := range item.Sources {
			add("")
			add(SummaryHeading(fmt.Sprintf("Source %d", i+1), width))
			for _, field := range []kubernetes.DetailField{{Label: "Repository", Value: source.URL}, {Label: "Path", Value: source.Path}, {Label: "Declared revision", Value: source.Revision}, {Label: "Chart", Value: source.Chart}} {
				if field.Value != "" {
					fields(field)
				}
			}
		}
		if len(item.Conditions) > 0 {
			add("")
			add(SummaryHeading("Conditions", width))
			for _, condition := range item.Conditions {
				label := condition.Type + " " + condition.Status
				if condition.Stale {
					label += " · earlier generation"
				}
				add(ui.TitleStyle.Render(ui.OneLine(label)))
				add(ui.Plain(condition.Reason))
				add(ui.Plain(condition.Message))
				add(ui.Muted.Render("Condition changed: " + gitOpsTime(condition.ChangedAt)))
			}
		}
	}
	if len(s.Snapshot.Issues) > 0 {
		add("")
		add(SummaryHeading("Read coverage", width))
		for _, issue := range s.Snapshot.Issues {
			add(ui.Plain(issue.Kind + ": " + issue.Message))
		}
	}
	return lines
}
