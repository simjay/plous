package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) syncResourceProgress(a *monitor.ResourceActionState) tea.Cmd {
	p := a.Progress
	if p == nil || p.Checking || !p.Status.Watching() {
		return nil
	}
	if !time.Now().Before(p.Deadline) {
		p.Status = kubernetes.OperationProgress{State: "attention", Summary: "Still waiting after 5 minutes", Details: p.Status.Details + "\nAutomatic checks stopped. This does not mean Kubernetes stopped the operation."}
		return nil
	}
	p.Checking = true
	p.Serial++
	serial, target, client, delay := p.Serial, p.Target, a.Client, max(time.Duration(0), time.Until(p.Next))
	ctx, cancel := context.WithCancel(context.Background())
	a.Cancel = cancel
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
		return monitor.ResourceProgressResult{Action: a, Progress: p, Serial: serial, Status: target.Progress(object, err)}
	}
}

func (m *model) updateResourceProgress(msg monitor.ResourceProgressResult) {
	if m.explore == nil || m.explore.Action != msg.Action || msg.Action.Progress != msg.Progress || msg.Serial != msg.Progress.Serial {
		return
	}
	p := msg.Progress
	p.Checking, p.Status, p.Next = false, msg.Status, time.Now().Add(2*time.Second)
	msg.Action.Cancel = nil
	if !p.Status.Watching() {
		m.refreshExplore()
	}
}
