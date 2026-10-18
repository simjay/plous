package monitor

import (
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
)

const OperationObservationLimit = 5 * time.Minute

type ResourceProgress struct {
	Target         kubernetes.OperationTarget
	Status         kubernetes.OperationProgress
	Deadline, Next time.Time
	Checking       bool
	Serial         uint64
}

type ResourceProgressResult struct {
	Action   *ResourceActionState
	Progress *ResourceProgress
	Serial   uint64
	Status   kubernetes.OperationProgress
}

func RenderOperationProgress(a *ResourceActionState, width, height int) string {
	p := a.Progress
	style := ui.TitleStyle
	switch p.Status.State {
	case "complete":
		style = ui.AddedStyle
	case "attention", "unavailable":
		style = ui.ChangedStyle
	}
	body := ui.Muted.Render(a.Note) + "\n\n" + style.Render(ui.Plain(p.Status.Summary))
	if p.Status.Details != "" {
		body += "\n" + ui.Plain(p.Status.Details)
	}
	// Wrap before scrolling so long finalizers and controller reasons remain readable.
	body = ansi.Wrap(body, max(1, width), "")
	a.Scroll = min(max(0, a.Scroll), max(0, lipgloss.Height(body)-height))
	return ui.StyledTextView(body, width, height, a.Scroll)
}

func OperationProgressFooter(p *ResourceProgress, width int) string {
	parts := []string{"Enter/Esc details", "↑↓ scroll"}
	if p.Status.Watching() {
		parts = append(parts, "checking every 2s")
	} else if p.Status.State != "complete" {
		parts = append(parts, "g check again")
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(parts, " · "))
}
