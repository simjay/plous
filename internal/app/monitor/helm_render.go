package monitor

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	helmcommon "github.com/simjay/plous/internal/integrations/helm/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	textview "github.com/simjay/plous/internal/ui/text"
)

func HelmDetailText(h *HelmState) string {
	return HelmDetailContent(h, false)
}

func HelmDetailContent(h *HelmState, reveal bool) string {
	s := h.Detail
	if s == nil {
		return ""
	}
	switch h.Tab {
	case 0:
		r := s.Release()
		return fmt.Sprintf("Release: %s\nNamespace: %s\nRevision: %d\nChart: %s\nChart version: %s\nRecorded status: %s\nUpdated: %s\n\n%s\n\nRecorded Helm status is separate from current workload readiness.\nUse Resources to compare recorded and live configuration.", r.Name, r.Namespace, r.Revision, r.Chart, r.Version, r.Status, r.Updated, s.Info.Description)
	case 1:
		if h.ChartDocument == 5 {
			var files []string
			for _, file := range append(append([]helmcommon.ChartFile(nil), s.Chart.Templates...), s.Chart.Files...) {
				files = append(files, "# "+file.Name+"\n"+string(file.Data))
			}
			return strings.Join(files, "\n---\n")
		}
		if h.ChartDocument > 0 {
			shadow := *h
			shadow.Tab = []int{1, 3, 4, 7, 0}[h.ChartDocument]
			return HelmDetailContent(&shadow, reveal)
		}
		return manifestutil.YAML(s.Chart.Metadata, false) + "\n# Values schema\n" + string(s.Chart.Schema)
	case 2:
		values := s.Effective
		if h.ValuesVariant == 1 {
			values = s.Config
		}
		if h.ValuesVariant == 2 {
			values = s.Chart.Values
		}
		return "# " + HelmValueLabels[h.ValuesVariant] + " for this revision\n" + manifestutil.YAML(values, !reveal)
	case 3:
		if reveal {
			return s.Manifest
		}
		return manifestutil.RedactedManifest(s.Manifest)
	case 4:
		var lines []string
		for _, h := range s.Hooks {
			manifest := manifestutil.RedactedManifest(h.Manifest)
			if reveal {
				manifest = h.Manifest
			}
			lines = append(lines, "# "+h.Name+" · "+strings.Join(h.Events, ", ")+"\n"+manifest)
		}
		if len(lines) == 0 {
			return "No hooks recorded."
		}
		return strings.Join(lines, "\n---\n")
	case 5:
		resources, err := h.InspectorResources()
		if err != nil {
			return err.Error()
		}
		var lines []string
		for _, r := range resources {
			lines = append(lines, r.Group+" / "+r.Key()+"\nTemplate: "+r.Source)
		}
		return strings.Join(lines, "\n\n")
	case 7:
		return s.Info.Notes
	}
	return ""
}

func HelmSectionTabs(tab, width int) string {
	var labels []string
	for _, i := range HelmInspectorTabs {
		label := HelmDetailSections[i]
		if i == tab {
			labels = append(labels, ui.Selected.Render("["+label+"]"))
		} else {
			labels = append(labels, ui.Muted.Render(label))
		}
		labels[len(labels)-1] = ui.MouseTarget(labels[len(labels)-1], fmt.Sprintf("helmtab/%d", i))
	}
	line := strings.Join(labels, "   ")
	hint := ui.Muted.Render("  ←→ sections")
	if ansi.StringWidth(line+hint) > width {
		line = ui.Selected.Render("[" + HelmDetailSections[tab] + "]")
	}
	return ui.Clipped(line+hint, width)
}

func HelmReleaseSummary(h *HelmState, width int) string {
	r := h.Detail.Release()
	fields := []kubernetes.DetailField{{Label: "Release", Value: r.Name}, {Label: "Namespace", Value: r.Namespace}, {Label: "Revision", Value: fmt.Sprint(r.Revision)}, {Label: "Recorded status", Value: r.Status}, {Label: "Chart", Value: r.Chart}, {Label: "Version", Value: r.Version}, {Label: "App version", Value: r.AppVersion}, {Label: "Updated", Value: r.Updated}}
	lines := []string{SummaryHeading("Release", width)}
	lines = append(lines, RenderSummaryFields(fields, width)...)
	if h.Detail.Info.Description != "" {
		lines = append(lines, "", SummaryHeading("Description", width), ui.Plain(h.Detail.Info.Description))
	}
	lines = append(lines, "", ui.Muted.Render("Recorded Helm status is separate from workload readiness."), ui.Muted.Render("Resources compares recorded and live configuration."))
	return strings.Join(lines, "\n")
}

func HelmStyledText(text string, width, height int, offset *int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	*offset = max(0, min(*offset, max(0, len(lines)-height)))
	return ui.StyledView(strings.Join(lines[*offset:min(len(lines), *offset+max(0, height))], "\n"), width, max(0, height))
}

func HelmPreviewBody(h *HelmState, width, height int) string {
	p := h.Plan
	title := map[string]string{"rollback": "Rollback preview", "preview": "Chart change preview"}[p.Operation]
	if title == "" {
		title = "Change preview"
	}
	fields := []kubernetes.DetailField{{Label: "Release", Value: p.Name}, {Label: "Namespace", Value: p.Namespace}}
	if p.Artifact != nil {
		fields = append(fields, kubernetes.DetailField{Label: "Chart", Value: fmt.Sprint(p.Artifact.Metadata["name"])}, kubernetes.DetailField{Label: "Version", Value: p.Artifact.Version})
	}
	if p.Operation == "rollback" {
		fields = append(fields, kubernetes.DetailField{Label: "Current revision", Value: fmt.Sprint(p.CurrentRevision)}, kubernetes.DetailField{Label: "Target revision", Value: fmt.Sprint(p.TargetRevision)})
	}
	lines := []string{SummaryHeading(title, width)}
	lines = append(lines, RenderSummaryFields(fields, width)...)
	lines = append(lines, "")
	content := strings.Split(ui.Plain(p.Summary()), "\n")
	if len(content) > 2 {
		content = content[2:]
	}
	for _, line := range content {
		switch {
		case line == "Resource changes:":
			line = SummaryHeading("Resource changes", width)
		case strings.HasPrefix(line, "+ Add "):
			line = ui.AddedStyle.Render(line)
		case strings.HasPrefix(line, "~ Change "):
			line = ui.ChangedStyle.Render(line)
		case strings.HasPrefix(line, "- Remove "):
			line = ui.RemovedStyle.Render(line)
		case strings.HasPrefix(line, "= Keep "):
			line = ui.Muted.Render(line)
		}
		lines = append(lines, line)
	}
	return HelmStyledText(strings.Join(lines, "\n"), width, height, &h.Subcursor)
}

func HelmActionRow(width int, key, label, otherKey, otherLabel string) string {
	return strings.Join(ui.ActionButtonRows([]string{ui.ActionButton(key, label, false), ui.ActionButton(otherKey, otherLabel, false)}, width), "\n")
}

func HelmScrollingText(text string, width, height int, offset *int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(ansi.Wrap(ui.Plain(text), max(1, width), ""), "\n")
	*offset = max(0, min(*offset, max(0, len(lines)-height)))
	return strings.Join(lines[*offset:min(len(lines), *offset+height)], "\n")
}

func HelmConfirmationBody(h *HelmState, width, height int) string {
	p := h.Plan
	identity := fmt.Sprintf("Context: %s\nNamespace: %s\nRelease: %s\nCurrent revision %d → retained revision %d", p.Context, p.Namespace, p.Name, p.CurrentRevision, p.TargetRevision)
	if p.Artifact != nil {
		identity = fmt.Sprintf("Context: %s\nNamespace: %s\nRelease: %s\nChart: %s · %s\nSHA256: %s", p.Context, p.Namespace, p.Name, p.Artifact.Metadata["name"], p.Artifact.Version, p.Artifact.Digest)
	}
	identity += "\nUses the reviewed inputs. Chart hooks may run."
	heading := ui.Clipped(ui.Heading("Confirm "+p.Operation), width)
	var buttons []string
	for i, label := range []string{"Cancel", strings.ToUpper(p.Operation[:1]) + p.Operation[1:]} {
		button := ui.Muted.Render("  " + label + "  ")
		if i == h.Choice {
			button = ui.Selected.Background(ui.Highlight).Render("[ " + label + " ]")
		}
		buttons = append(buttons, ui.MouseTarget(button, fmt.Sprintf("confirm/%d", i)))
	}
	actions := strings.Join(ui.ActionButtonRows(buttons, width), "\n")
	available := max(1, height-3-lipgloss.Height(actions))
	details := textview.Fit(ui.TextView(identity, width, available, h.ConfirmScroll), width, available)
	return textview.Fit(heading+"\n\n"+details+"\n"+actions, width, height)
}
