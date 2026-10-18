package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

const overviewUsageInterval = 5 * time.Second

type overviewUsageResult struct {
	View *overviewView
	Run  uint64
	Data kubernetes.OverviewUsage
}

type overviewUsagePoll struct {
	View *overviewView
	Run  uint64
}

func (m *model) syncOverviewUsage(v *overviewView) tea.Cmd {
	if !v.Cluster.Loaded || v.Cluster.Client == nil || v.Cluster.Err != nil || v.UsageCancel != nil || v.UsagePoll {
		return nil
	}
	if delay := time.Until(v.UsageNext); delay > 0 {
		v.UsagePoll = true
		run := v.UsageRun
		return tea.Tick(delay, func(time.Time) tea.Msg { return overviewUsagePoll{v, run} })
	}
	return m.readOverviewUsage(v)
}

func (m *model) readOverviewUsage(v *overviewView) tea.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	v.UsageCancel = cancel
	v.UsageRun++
	run, client, catalog := v.UsageRun, v.Cluster.Client, v.Cluster.Catalog
	return func() tea.Msg {
		defer cancel()
		return overviewUsageResult{v, run, client.OverviewUsage(ctx, catalog)}
	}
}

func (m *model) applyOverviewUsage(r overviewUsageResult) {
	v := r.View
	if v == nil || v.UsageRun != r.Run || v.UsageCancel == nil {
		return
	}
	v.UsageCancel = nil
	v.UsageNext = time.Now().Add(overviewUsageInterval)
	v.State.ApplyUsage(r.Data)
}
