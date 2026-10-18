package code

import (
	lipgloss "charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/source"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"path"
	"strings"
)

func CodePathTail(value string, width int) string {
	if ansi.StringWidth(value) <= width {
		return value
	}
	return "…" + ansi.TruncateLeft(value, ansi.StringWidth(value)-max(0, width-1), "")
}

func (s *CodeState) CurrentResource() (manifest.Resource, bool) {
	if s.Preference.Section == 2 && s.TraceUnavailable != "" {
		return manifest.Resource{}, false
	}
	resources := s.Resources()
	if len(resources) == 0 {
		return manifest.Resource{}, false
	}
	return resources[min(max(0, s.Cursor), len(resources)-1)], true
}

func (s *CodeState) CurrentFields() []source.Field {
	r, ok := s.CurrentResource()
	if !ok || s.Inventory == nil {
		return nil
	}
	b := s.Builds[s.Preference.Selected]
	if b == nil {
		return nil
	}
	identity := fmt.Sprint(s.Stage) + "/" + b.OperationID + "/" + s.Inventory.Source.Identity + "/" + r.Key()
	if identity != s.FieldResource {
		s.FieldResource = identity
		observed := b
		if s.CapturedStage() != nil {
			copy := *b
			build := *b.BuildResult
			build.Observation = nil
			copy.BuildResult = &build
			copy.Origins = nil
			observed = &copy
		}
		s.TraceFields = source.Trace(s.Inventory, observed, r)
		s.FieldCursor = 0
		for n, f := range s.TraceFields {
			if f.Pointer == s.Preference.Field {
				s.FieldCursor = n
				break
			}
		}
	}
	return s.TraceFields
}

func (s *CodeState) CurrentField() (source.Field, bool) {
	fields := s.CurrentFields()
	if len(fields) == 0 {
		return source.Field{}, false
	}
	return fields[min(max(0, s.FieldCursor), len(fields)-1)], true
}

func (s *CodeState) SourceNavigation(width, height int, trace bool) string {
	rows := []string{}
	cursor := 0
	current, has := s.CurrentResource()
	field, hasField := s.CurrentField()
	reference := s.Builds[s.Preference.Selected]
	if trace && s.TraceReference != nil && (!has || !hasField) {
		current, field.Pointer, reference = s.TraceReference.Resource, s.TraceReference.Field, s.TraceReference.Result
		has, hasField = true, true
	}
	entries := s.NavigationEntries()
	nodes, folders := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		nodes[e.Path] = true
	}
	for n, e := range entries {
		parts := strings.Split(e.Path, "/")
		for depth := 1; depth < len(parts); depth++ {
			folder := strings.Join(parts[:depth], "/")
			if !nodes[folder] && !folders[folder] {
				indent := strings.Repeat("  ", min(depth-1, 4))
				rows = append(rows, ui.Muted.Render(ui.Clipped(indent+"▾ "+parts[depth-1]+"/", width)))
				folders[folder] = true
			}
		}
		label := strings.Repeat("  ", min(len(parts)-1, 4)) + path.Base(e.Path)
		if trace && has && hasField {
			value := "not rendered"
			if b := s.Builds[e.ID]; b != nil {
				value = "not matched"
				if s.Status(e.ID) == "stale" {
					value = "stale"
				} else {
					if reference != nil {
						for _, change := range source.Compare(b, reference) {
							if change.After.Key() == current.Key() && change.Before.Name != "" {
								if v, safe := source.FieldValue(change.Before, current, field.Pointer); safe {
									value = v
								}
								break
							}
						}
					}
				}
			}
			if s.PreviewFailure(e.ID) != "" {
				value = "failed"
			}
			value = ui.Clipped(ui.OneLine(value), max(6, width/3))
			left := max(1, width-ansi.StringWidth(value)-4)
			label = ui.Clipped(label, left)
			label += strings.Repeat(" ", max(1, width-2-ansi.StringWidth(label)-ansi.StringWidth(value))) + value
		}
		if e.ID == s.Preference.Selected {
			cursor = len(rows)
		}
		rows = append(rows, ui.MouseTarget(ui.SettingsChoiceLine(label, e.ID == s.Preference.Selected, width), fmt.Sprintf("code-source/%d", n)))
	}
	return ui.ScrollSelection(rows, cursor, height)
}

func (s *CodeState) ResourceNavigation(width, height int) string {
	resources := s.Resources()
	if len(resources) == 0 {
		if s.Filter != "" {
			return "No resources match this filter."
		}
		if s.ComparisonActive() {
			return "No changed resources."
		}
		return "No generated resources."
	}
	rows := []string{}
	cursor := 0
	columns := []string{"Kind", "Name", "Namespace"}
	kindWidth := 10
	for _, r := range resources {
		kindWidth = max(kindWidth, min(18, ansi.StringWidth(r.Kind)))
	}
	widths := []int{kindWidth, max(9, (width-kindWidth-4)/2), max(9, width-kindWidth-4-max(9, (width-kindWidth-4)/2))}
	table := width >= 42
	style := func(column, value string) lipgloss.Style {
		if column == "Name" {
			return ui.TitleStyle
		}
		return ui.CanvasStyle
	}
	heading := "Resource (Kind / Name)"
	if table {
		heading, _ = textview.MouseLayout(ui.CollectionTableRow(columns, columns, []int{0, 1, 2}, widths, width, true, false, style))
	}
	changes := s.ComparisonChanges()
	for n, r := range resources {
		if n == s.Cursor {
			cursor = len(rows)
		}
		name := r.Name
		if n < len(changes) {
			state := changes[n].Status
			if s.ComparisonSwapped {
				if state == "Added" {
					state = "Removed"
				} else if state == "Removed" {
					state = "Added"
				}
			}
			name = map[string]string{"Added": "+ ", "Removed": "- ", "Changed": "~ ", "Unchanged": "= "}[state] + name
		}
		namespace := resourceNamespaceLabel(r)
		row := ""
		if table {
			row = ui.CollectionTableRow(columns, []string{r.Kind, name, namespace}, []int{0, 1, 2}, widths, width, false, n == s.Cursor, style)
		} else {
			row = ui.SettingsChoiceLine(r.Kind+"/"+name, n == s.Cursor, width) + "\n" + ui.Muted.Render(ui.Clipped("  Namespace: "+namespace, width))
		}
		for _, line := range strings.Split(row, "\n") {
			rows = append(rows, ui.MouseTarget(line, fmt.Sprintf("code-row/%d", n)))
		}
	}
	return ui.Muted.Render(heading) + "\n" + ui.ScrollSelection(rows, cursor, max(0, height-1))
}

func resourceNamespaceLabel(r manifest.Resource) string {
	if r.Namespace != "" {
		return r.Namespace
	}
	if r.ClusterScoped() {
		return "Cluster-scoped"
	}
	return "Not specified"
}

func (s *CodeState) YamlPreview(resource manifest.Resource, width, height int) string {
	fields := s.CurrentFields()
	selectedLine := 0
	if f, ok := s.CurrentField(); ok {
		selectedLine = f.RenderedLine
	}
	byLine := map[int]int{}
	for n, f := range fields {
		byLine[f.RenderedLine] = n
	}
	rows := []string{}
	for n, line := range strings.Split(strings.TrimSuffix(manifest.YAML(resource.Object, true), "\n"), "\n") {
		label := fmt.Sprintf("%3d │ %s", n+1, line)
		styled := docedit.YAMLLine(ui.Clipped(label, width), ui.WorkspaceTheme())
		if n+1 == selectedLine {
			styled = ui.Selected.Render(ui.Clipped(label, width))
		}
		if index, ok := byLine[n+1]; ok {
			styled = ui.MouseTarget(styled, fmt.Sprintf("code-field/%d", index))
		}
		rows = append(rows, styled)
	}
	return ui.ScrollSelection(rows, max(0, selectedLine-1), height)
}

func (s *CodeState) InspectorTabs(width int) string {
	areas, labels := []int{1, 2}, []string{"Resources", "YAML"}
	if s.Preference.Section == 2 {
		areas, labels = []int{2, 3}, []string{"YAML", "Field History"}
		if s.TraceReference != nil {
			areas, labels = []int{0, 2, 3}, []string{"Entries", "YAML", "Field History"}
		}
	}
	var items []string
	for n, label := range labels {
		style := ui.Muted
		if areas[n] == s.InspectorArea {
			style, label = ui.TitleStyle, "["+label+"]"
		}
		items = append(items, ui.MouseTarget(style.Render(label), fmt.Sprintf("code-area/%d", areas[n])))
	}
	return ui.Clipped(strings.Join(items, "  "), width)
}

func CodeScroll(body string, offset, height int) string {
	lines := strings.Split(body, "\n")
	height = max(1, height)
	start := min(max(0, offset), max(0, len(lines)-height))
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}

func (s *CodeState) FieldEvidence(width, height int) string {
	f, ok := s.CurrentField()
	if !ok {
		return "Select a field."
	}
	lines := []string{ui.TitleStyle.Render(ansi.Wrap(f.Pointer, width, "")), ansi.Wrap("Final value: "+f.Value, width, "")}
	if b := s.Builds[s.Preference.Selected]; b != nil && b.Observation != nil && b.Observation.Truncated {
		lines = append(lines, ui.ChangedStyle.Render(ansi.Wrap("Partial capture: some transformations were not recorded.", width, "")))
	}
	if len(f.Writes) > 0 {
		lines = append(lines, "", ui.TitleStyle.Render("OBSERVED CHANGES"))
		for n, write := range f.Writes {
			lines = append(lines, ansi.Wrap(fmt.Sprintf("%d · %s · %s", n+1, source.TransformationLabel(write.Kind), write.Scope), width, ""), ansi.Wrap(write.Before+" → "+write.After, width, ""))
			if write.File != "" {
				location := write.File
				if write.Line > 0 {
					location += fmt.Sprintf(":%d", write.Line)
				}
				lines = append(lines, ui.Muted.Render(ansi.Wrap(location, width, "")))
			}
			lines = append(lines, "")
		}
		if !f.ObservedComplete {
			lines = append(lines, ansi.Wrap("Partial history: some changes could not be attributed.", width, ""))
		}
		lines = append(lines, ansi.Wrap("Captured builtin changes in execution order. Same-value writes are not recorded.", width, ""))
		if f.Writes[len(f.Writes)-1].File != "" {
			lines = append(lines, ui.MouseTarget(ui.TitleStyle.Render("d Open last writer configuration"), "action/d"))
		}
	} else if f.Line > 0 {
		lines = append(lines, "", ui.TitleStyle.Render("Exact source field"), ansi.Wrap(fmt.Sprintf("%s:%d", f.File, f.Line), width, ""), ui.MouseTarget(ui.TitleStyle.Render("d Open source"), "action/d"))
	} else {
		lines = append(lines, "", ui.TitleStyle.Render("No writer history captured"), ansi.Wrap("This capture cannot identify what set this field. Select another YAML field to inspect its evidence.", width, ""))
		if f.File != "" {
			lines = append(lines, "", ui.Muted.Render("RESOURCE ORIGIN"), ansi.Wrap(f.File, width, ""), ansi.Wrap("This file created the resource. It does not identify who set this field.", width, ""), ui.MouseTarget(ui.TitleStyle.Render("d Open resource origin"), "action/d"))
		} else if f.Evidence != "" && f.Evidence != "Field attribution unavailable" {
			lines = append(lines, "", ui.Muted.Render(ansi.Wrap(f.Evidence, width, "")))
		}
	}
	return CodeScroll(strings.Join(lines, "\n"), s.DetailOffset, height)
}

func (s *CodeState) InspectorPane(title, body string, width, height, area int) string {
	return ui.MouseTarget(CodePane(title, body, width, height, s.InspectorArea == area), fmt.Sprintf("code-area/%d", area))
}

func (s *CodeState) Breadcrumb(width int) string {
	revision := "Working tree"
	if s.Inventory.Source.Revision != "" {
		revision = "Git " + s.Inventory.Source.Revision[:min(12, len(s.Inventory.Source.Revision))]
	}
	entry := "No Node Selected"
	if e := s.Inventory.Entry(s.Preference.Selected); e != nil {
		entry = e.Path
	}
	if s.Preference.Section == 3 {
		entry = "All sources"
		if e := s.Inventory.Entry(s.ValidationSource); e != nil {
			entry = e.Path
		}
		return ui.Muted.Render(ui.Clipped("Findings · "+entry+" · "+revision, width))
	}
	suffix := ""
	if width >= 76 {
		suffix += " · " + revision
	}
	label := "Entry: " + CodePathTail(entry, max(1, width-7-ansi.StringWidth(suffix))) + suffix
	if s.ReturnView == "code-kustomize" && s.Preference.Section == 1 {
		prefix := "Render target: "
		if s.Builds[s.Preference.Selected] != nil && s.Status(s.Preference.Selected) != "stale" && s.Status(s.Preference.Selected) != "failed" {
			prefix = "Rendered: "
		}
		label = prefix + CodePathTail(entry, max(1, width-len(prefix)-ansi.StringWidth(suffix))) + suffix
	}
	if s.Preference.Section == 0 {
		label = "Configuration Map · " + CodePathTail(entry, max(1, width-20))
	}
	if s.Preference.Section == 2 {
		label = "Field History · " + entry
	}
	return ui.TitleStyle.Render(ui.Clipped(label, width))
}

func (s *CodeState) RenderInspector(width, height int) string {
	trace := s.Preference.Section == 2
	b := s.Builds[s.Preference.Selected]
	resource, hasResource := s.CurrentResource()
	failure := s.PreviewFailure(s.Preference.Selected)
	status := "Preparing preview"
	if b == nil && !s.Busy && s.ReturnView == "code-kustomize" {
		status = "Not rendered yet"
	}
	if b != nil {
		status = "Final output · " + s.Status(b.ID)
		if s.Status(b.ID) == "failed" || s.Status(b.ID) == "stale" {
			status = "Previous output · " + s.Status(b.ID)
		}
	}
	if failure != "" && b == nil {
		status = "Render failed · r Retry"
	}
	if e := s.Inventory.Entry(s.Preference.Selected); e != nil && !e.Renderable() {
		status = "Files only · rendering unavailable"
		if e.Component {
			status = "Component · rendered by a consuming entry"
		}
	}
	if trace {
		status = "Field History · " + s.Status(s.Preference.Selected)
		if hasResource {
			status = resource.Kind + "/" + resource.Name
			if state := s.Status(s.Preference.Selected); state == "failed" || state == "stale" {
				status = "Previous output · " + state + " · " + status
			}
		}
	}
	if stage := s.CapturedStage(); stage != nil {
		status = fmt.Sprintf("Stage %d · %s · %s", s.Stage, source.TransformationLabel(stage.Kind), stage.Scope)
		if s.Status(b.ID) == "stale" || s.Status(b.ID) == "failed" {
			status = "Previous capture · " + status
		}
	}
	header := ui.Muted.Render(ui.Clipped(status, width)) + "\n"
	if width < 76 && b != nil {
		header += s.InspectorTabs(width) + "\n"
	}
	if !trace {
		errors, warnings := 0, 0
		for _, f := range s.AllFindings() {
			if f.Target == s.Preference.Selected {
				if f.Severity == "error" {
					errors++
				}
				if f.Severity == "warning" {
					warnings++
				}
			}
		}
		if errors+warnings > 0 {
			var counts []string
			if errors > 0 {
				counts = append(counts, CodeCountLabel(errors, "error", "errors"))
			}
			if warnings > 0 {
				counts = append(counts, CodeCountLabel(warnings, "warning", "warnings"))
			}
			label := strings.Join(counts, " · ")
			if width >= 52 {
				label += " · a View Findings"
			}
			header += ui.MouseTarget(ui.ChangedStyle.Render(ui.Clipped(label, width)), "code-findings/open") + "\n"
		}
	}
	if b != nil && b.Observation != nil && b.Observation.Truncated {
		header += ui.ChangedStyle.Render(ansi.Wrap("Partial capture: limit reached or snapshot unavailable. Final output is complete.", width, "")) + "\n"
	}
	if failure != "" && b != nil {
		header += s.FailureView(s.Preference.Selected, width) + "\n"
	}
	if s.ComparisonActive() {
		header += ui.Muted.Render(ui.Clipped(s.ComparisonLabel(), width)) + "\n"
		if width >= 52 {
			header += ui.MouseTarget(ui.TitleStyle.Render("a Show Final YAML"), "code-comparison/toggle") + "\n"
		}
	}
	height = max(1, height-strings.Count(header, "\n"))
	output := func(w, h int) string {
		if b == nil || !hasResource {
			message := "Preparing preview."
			switch {
			case failure != "":
				message = s.FailureView(s.Preference.Selected, w) + "\n\nr Retry · Esc " + s.DetailBackLabel()
			case trace && s.TraceUnavailable != "":
				message = s.TraceUnavailable + "\na Configuration Map selects another input.\na Trace Across All Overlays renders remaining sources."
			case s.SelectedUnsupported():
				message = "Rendering Unavailable for this source. Open its inputs in Files."
			case s.ComparisonActive() && s.ComparisonChanged:
				message = "No changed resources match this filter.\nBackspace Clear filter"
				if !s.ComparisonCurrent() {
					message = "Comparison uses previous output.\na Show Final YAML opens the latest output."
				} else if s.Filter == "" {
					message = "No manifest changes.\nThe rendered outputs are identical.\na Show Final YAML opens the complete result."
				}
			case b != nil && s.Filter != "":
				message = "No resources match.\nBackspace Clear filter"
			case b != nil:
				message = "This source renders no resources."
			default:
				if e := s.Inventory.Entry(s.Preference.Selected); e != nil && e.Component {
					message = "Component: select a consuming entry in the Configuration Map."
				} else if !s.Busy && s.ReturnView == "code-kustomize" {
					message = "No output yet. Use r to render this configuration."
				}
			}
			return s.InspectorPane("OUTPUT", CodeScroll(ansi.Wrap(message, w, ""), s.DetailOffset, h-2), w, h, 2)
		}
		if trace && s.InspectorArea == 3 && w < 74 {
			return s.InspectorPane("FIELD HISTORY", s.FieldEvidence(w, h-2), w, h, 3)
		}
		if trace && w >= 74 {
			ew := max(30, w/3)
			yw := w - ew - 3
			return CodeColumns(s.InspectorPane(resource.Kind+"/"+resource.Name, s.YamlPreview(resource, yw, h-2), yw, h, 2), s.InspectorPane("FIELD HISTORY", s.FieldEvidence(ew, h-2), ew, h, 3), yw, ew, h)
		}
		detail := ""
		if trace {
			if f, ok := s.CurrentField(); ok {
				detail = "\n" + ui.Muted.Render(ui.Clipped(f.Evidence+" · → Evidence", w))
			}
		}
		identity := ui.Muted.Render(ui.Clipped("Namespace: "+resourceNamespaceLabel(resource), w)) + "\n"
		content := s.YamlPreview(resource, w, max(1, h-3-strings.Count(detail, "\n")))
		if s.ComparisonActive() {
			content = s.ComparisonPreview(resource, w, max(1, h-3))
		}
		return s.InspectorPane(resource.Kind+" / "+resource.Name, identity+content+detail, w, h, 2)
	}
	if trace {
		if s.TraceReference != nil && s.InspectorArea == 0 {
			return header + s.InspectorPane("OVERLAY VALUES", s.SourceNavigation(width, height-2, true), width, height, 0)
		}
		return header + output(width, height)
	}
	if b == nil {
		return header + output(width, height)
	}
	if width < 76 {
		if s.InspectorArea == 1 {
			return header + s.InspectorPane("RESOURCES", s.ResourceNavigation(width, height-2), width, height, 1)
		}
		return header + output(width, height)
	}
	navWidth := min(62, max(32, width*2/5))
	nav := s.InspectorPane("RESOURCES", s.ResourceNavigation(navWidth, height-2), navWidth, height, 1)
	return header + CodeColumns(nav, output(width-navWidth-3, height), navWidth, width-navWidth-3, height)
}
