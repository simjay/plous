package app

import (
	"fmt"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

func (m *model) renderResourceActionOverlay(base string) string {
	a := m.explore.Action
	if a.Stage == "execcommand" {
		return m.renderExecLaunchOverlay(base)
	}
	width, height := min(110, m.width-6), min(36, m.height-6)
	switch a.Stage {
	case "menu":
		height = min(height, 10+len(a.Items))
	case "scale", "export":
		height = min(height, 16)
	case "confirm":
		lines := lipgloss.Height(ansi.Wrap(monitor.ActionWarning(a)+"\n"+a.Note, max(1, width-4), ""))
		height = min(height, max(13, lines+10))
	case "done":
		height = min(height, 12)
		if a.Progress != nil {
			height = min(m.height-6, 16)
		}
	case "pods":
		count := len(a.Pods)
		if a.AllPodsChoice() {
			count++
		}
		height = min(height, 10+min(count, 10))
	case "containers":
		height = min(height, 10+min(len(a.Containers), 10))
	case "values":
		height = min(height, 10+min(len(a.Values), 10))
	}
	errorText := ansi.Wrap(a.Err, max(1, width-4), "")
	if errorText != "" {
		height = min(36, m.height-6, height+lipgloss.Height(errorText)+1)
	}
	innerWidth, innerHeight := width-4, height-2
	scope := a.Object.GetNamespace()
	if scope == "" {
		scope = "cluster"
	}
	heading := ui.TitleStyle.Render(a.Resource.Kind+" / "+ui.OneLine(a.Object.GetName())) + "\n" + ui.Muted.Render(ui.Clipped(a.Context+" · "+scope+" · UID "+string(a.Object.GetUID()), innerWidth))
	footer := "Esc back"
	var body string
	available := max(1, innerHeight-4)
	var errorNotice string
	if errorText != "" {
		errorHeight := min(lipgloss.Height(errorText), max(1, available-4))
		errorNotice = ui.ChangedStyle.Render(ui.TextView(errorText, innerWidth, errorHeight, 0))
		available = max(1, available-errorHeight-1)
	}
	switch a.Stage {
	case "menu":
		var items []string
		for _, item := range a.Items {
			label := item.Alias + "  " + item.Label
			if a.Busy {
				label += " · checking…"
			} else if reason := a.Reasons[item.Id]; reason != "" {
				label += " · unavailable"
			}
			items = append(items, label)
		}
		body = ui.ListView(items, a.Cursor, innerWidth, max(1, available-2))
		if a.Cursor < len(a.Items) {
			reason := a.Reasons[a.Items[a.Cursor].Id]
			if reason != "" {
				body += "\n" + ui.ChangedStyle.Render(ui.Clipped(reason, innerWidth))
			} else if a.Resource.Key() != "pods" && (a.Items[a.Cursor].Id == "logs" || a.Items[a.Cursor].Id == "exec") {
				body += "\n" + ui.Muted.Render("Pod permissions are checked after selection.")
			}
		}
		if a.PermissionUnknown {
			body += "\n" + ui.Muted.Render("Permission check unavailable. The API will authorize each request.")
		}
		footer = "↑↓ choose · Enter open · Esc close"
	case "pods", "containers":
		label := "Choose Pod"
		items := a.Containers
		if a.Stage == "pods" {
			items = nil
			if a.AllPodsChoice() {
				items = append(items, "All Pods · merged application containers")
			}
			for _, pod := range a.Pods {
				items = append(items, pod.GetName()+" · "+kubernetes.Value(pod.Object, "status", "phase"))
			}
		} else {
			label = "Choose container · " + a.Pod.GetName()
		}
		body = ui.TitleStyle.Render(label) + "\n\n" + ui.ListView(items, a.Cursor, innerWidth, max(1, available-2))
		if len(items) == 0 {
			body += "\nNo eligible " + a.Stage + " found.\nExec requires a running container."
		}
	case "values":
		items := []string{}
		for _, value := range a.Values {
			items = append(items, value.Label)
		}
		body = ui.TitleStyle.Render("Choose a value") + "\n\n" + ui.ListView(items, a.Cursor, innerWidth, available-2)
		if len(items) == 0 {
			body += "\nNo values found."
		}
		footer = "Enter reveal selected value · Esc back"
	case "scale":
		body = "Scale replicas\nCurrent desired: " + kubernetes.Value(a.Scale.Object, "spec", "replicas") + "\n\nProposed: " + ui.Selected.Render(ui.EditorLine(a.Input, innerWidth-10, true)) + "\n\n" + a.Note
		footer = "Enter review · Esc cancel"
	case "export":
		body = "Export YAML to a new file\n\n" + ui.Selected.Render(ui.EditorLine(a.Input, innerWidth, true)) + "\n\nExisting files are never overwritten. Secret values are redacted."
		if a.ExportValue {
			body = "Export " + a.Container + " as PEM to a new file\n\n" + ui.Selected.Render(ui.EditorLine(a.Input, innerWidth, true)) + "\n\nExisting files are never overwritten."
		}
		footer = "Enter export · Esc cancel"
	case "confirm":
		text := monitor.ActionWarning(a)
		if a.Operation == "delete" && a.Note != "" {
			text = a.Note + "\n\n" + text
		}
		footer = "←→ choose · Enter confirm · Esc back"
		if a.Operation == "scale" {
			text = fmt.Sprintf("Replicas: %s → %d\n\n%s\n\n%s", kubernetes.Value(a.Scale.Object, "spec", "replicas"), a.Replicas, a.Note, text)
		}
		gap := "\n\n"
		if available < 4 {
			gap = "\n"
		}
		body = ui.StyledTextView(text, innerWidth, max(0, available-len(gap)), a.Scroll) + gap
		labels := []string{"Back", "Confirm " + a.Operation}

		for i, label := range labels {
			var button string
			if i == a.Cursor {
				button = ui.Selected.Background(ui.Highlight).Render("[ " + label + " ]")
			} else {
				button = ui.Muted.Render("  " + label + "  ")
			}
			body += ui.MouseTarget(button, fmt.Sprintf("confirm/%d", i)) + "  "
		}
	case "done":
		body = ui.AddedStyle.Render(a.Note)
		footer = "Enter or Esc return to details"
		if a.Progress != nil {
			body = monitor.RenderOperationProgress(a, innerWidth, available)
			footer = monitor.OperationProgressFooter(a.Progress, innerWidth)
		}
	}
	if a.Busy {
		footer = m.loading("Working…") + " · Esc cancel request"
	}
	if a.Err == "" && a.Stage == "menu" && a.Note != "" {
		body += "\n\n" + ui.Muted.Render(a.Note)
	}
	content := heading + "\n\n" + lipgloss.NewStyle().Width(innerWidth).Height(available).MaxHeight(available).Render(body)
	if errorNotice != "" {
		content += "\n" + errorNotice
	}
	content += "\n" + ui.Muted.Render(ui.Clipped(footer, innerWidth))
	window := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Accent).Background(ui.Surface).Foreground(ui.Foreground).Padding(0, 1).Width(width).Height(height).MaxHeight(height).Render(content)
	return ui.CenteredOverlay(base, window)
}
