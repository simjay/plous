package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

const operationObservationLimit = 5 * time.Minute

type resourceProgress struct {
	target         kubernetes.OperationTarget
	status         kubernetes.OperationProgress
	deadline, next time.Time
	checking       bool
	serial         uint64
}

type resourceProgressResult struct {
	action   *resourceActionState
	progress *resourceProgress
	serial   uint64
	status   kubernetes.OperationProgress
}

func (m *model) syncResourceProgress(a *resourceActionState) tea.Cmd {
	p := a.progress
	if p == nil || p.checking || !p.status.Watching() {
		return nil
	}
	if !time.Now().Before(p.deadline) {
		p.status = kubernetes.OperationProgress{State: "attention", Summary: "Still waiting after 5 minutes", Details: p.status.Details + "\nAutomatic checks stopped. This does not mean Kubernetes stopped the operation."}
		return nil
	}
	p.checking = true
	p.serial++
	serial, target, client, delay := p.serial, p.target, a.client, max(time.Duration(0), time.Until(p.next))
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		request, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		object, err := client.Get(request, target.Resource, target.Object.GetNamespace(), target.Object.GetName())
		return resourceProgressResult{a, p, serial, target.Progress(object, err)}
	}
}

func (m *model) updateResourceProgress(msg resourceProgressResult) {
	if m.explore == nil || m.explore.action != msg.action || msg.action.progress != msg.progress || msg.serial != msg.progress.serial {
		return
	}
	p := msg.progress
	p.checking, p.status, p.next = false, msg.status, time.Now().Add(2*time.Second)
	msg.action.cancel = nil
	if !p.status.Watching() {
		m.refreshExplore()
	}
}

func renderOperationProgress(a *resourceActionState, width, height int) string {
	p := a.progress
	style := titleStyle
	switch p.status.State {
	case "complete":
		style = addedStyle
	case "attention", "unavailable":
		style = changedStyle
	}
	body := muted.Render(a.note) + "\n\n" + style.Render(plain(p.status.Summary))
	if p.status.Details != "" {
		body += "\n" + plain(p.status.Details)
	}
	// Wrap before scrolling so long finalizers and controller reasons remain readable.
	body = ansi.Wrap(body, max(1, width), "")
	a.scroll = min(max(0, a.scroll), max(0, lipgloss.Height(body)-height))
	return styledTextView(body, width, height, a.scroll)
}

func operationProgressFooter(p *resourceProgress, width int) string {
	parts := []string{"Enter/Esc details", "↑↓ scroll"}
	if p.status.Watching() {
		parts = append(parts, "checking every 2s")
	} else if p.status.State != "complete" {
		parts = append(parts, "g check again")
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(parts, " · "))
}
