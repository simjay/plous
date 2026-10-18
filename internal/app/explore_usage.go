package app

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func (m *model) stopExploreUsage() { m.explore.StopUsage() }

func (m *model) syncExploreUsage() tea.Cmd {
	e := m.explorer()
	if !m.exploreActive() || e.Client == nil || !kubernetes.HasUsage(e.Resource) || e.Detail == nil && e.Resource.Key() != "pods" && e.Resource.Key() != "nodes" {
		m.stopExploreUsage()
		return nil
	}
	key := e.Key + "/" + e.Resource.Key() + "/" + e.Resource.Version + "/" + e.Preference.Namespace + "/" + e.Preference.Selector
	if e.Detail != nil {
		key += "/" + kubernetes.UsageKey(*e.Detail) + "/" + string(e.Detail.GetUID())
	}
	if e.Usage != nil && e.Usage.Key == key {
		return nil
	}
	m.stopExploreUsage()
	ctx, cancel := context.WithCancel(context.Background())
	run := &monitor.ExploreUsage{Key: key, Ctx: ctx, Cancel: cancel, Events: make(chan monitor.ExploreUsageResult, 1)}
	e.Usage = run
	metrics, ok := kubernetes.UsageResource(e.Resource, e.Resources)
	if !ok {
		run.Loaded = true
		run.Err = "Metrics API unavailable. A resource metrics provider such as Metrics Server is required."
		return nil
	}
	client, resource, namespace, resources := e.Client, e.Resource, e.Preference.Namespace, slices.Clone(e.Resources)
	object := e.Detail.DeepCopy()
	if !resource.Namespaced {
		namespace = ""
	}
	return func() tea.Msg {
		go func() {
			defer close(run.Events)
			for ctx.Err() == nil {
				result := monitor.ExploreUsageResult{Run: run}
				if object == nil {
					result.Samples, result.Err = client.UsageList(ctx, metrics, namespace)
				} else {
					result.Usage, result.Err = client.ObjectUsage(ctx, resource, object, resources, time.Now())
				}
				select {
				case run.Events <- result:
				case <-ctx.Done():
					return
				}
				timer := time.NewTimer(monitor.UsageInterval)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return
				}
			}
		}()
		return monitor.WaitExploreUsage(run)()
	}
}

func (m *model) updateExploreUsage(result monitor.ExploreUsageResult) tea.Cmd {
	e := m.explorer()
	if result.Run != e.Usage || result.Done || result.Run.Ctx.Err() != nil {
		return nil
	}
	run := e.Usage
	run.Apply(result, time.Now())
	e.Inventory = nil
	if e.Detail == nil {
		m.reconcileExploreSelection()
	}
	return monitor.WaitExploreUsage(run)
}

func (m *model) usageSummary(width int) string {
	height := m.exploreContentHeight(width, m.mainBodyHeight())
	height -= lipgloss.Height(m.exploreDetailHeading(width, height))
	return m.explorer().UsageSummaryFit(width, max(0, height), m.viewExpanded(), time.Now())
}

func (m *model) usageHelp() string { return m.explore.UsageHelp() }
