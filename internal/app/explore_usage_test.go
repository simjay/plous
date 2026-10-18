package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func enableTestUsage(m *model) *monitor.ExploreUsage {
	m.explore.Resources = append(m.explore.Resources, kubecommon.Resource{Group: "metrics.k8s.io", Version: "v1beta1", Name: "pods", Namespaced: true})
	m.syncExploreUsage()
	return m.explore.Usage
}

func TestUsageLifecycleRejectsLateResponsesAndPreservesNavigation(t *testing.T) {
	m := exploreModel(t)
	e := m.explore
	first := enableTestUsage(m)
	if first == nil || first.Ctx.Err() != nil {
		t.Fatal("missing metrics run")
	}
	object := e.Collection.Objects[0].DeepCopy()
	e.Detail = object
	m.syncExploreUsage()
	second := e.Usage
	if first.Ctx.Err() == nil || second == first {
		t.Fatal("table metrics survived detail selection")
	}
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: first, Err: errors.New("late")})
	if second.Loaded || second.Err != "" {
		t.Fatal("old metrics crossed target identity")
	}
	m.focus = 2
	m.help = &helpState{}
	m.setTreeExpanded(m.connection, false)
	m.syncExploreUsage()
	if e.Usage != second || second.Ctx.Err() != nil {
		t.Fatal("overlay, chat focus, or folding stopped metrics")
	}
	e.Detail = object.DeepCopy()
	e.Detail.SetUID("replacement")
	m.syncExploreUsage()
	if e.Usage == second || second.Ctx.Err() == nil || len(e.Usage.Points) != 0 {
		t.Fatal("replacement inherited metrics history")
	}
	last := e.Usage
	m.mode = 0
	m.syncExplore()
	if last.Ctx.Err() == nil || e.Usage != nil {
		t.Fatal("leaving Explore kept metrics running")
	}
}

func TestUsageScopeChangesClearSamplesAndHistory(t *testing.T) {
	for _, change := range []string{"namespace", "selector", "resource", "close", "stop"} {
		t.Run(change, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			e.Detail = e.Collection.Objects[0].DeepCopy()
			run := enableTestUsage(m)
			run.Points = []kubernetes.Usage{{CPU: 1, Timestamp: time.Now()}}
			switch change {
			case "namespace":
				e.Preference.Namespace = "elsewhere"
			case "selector":
				e.Preference.Selector = "app=other"
			case "resource":
				e.Resource = kubecommon.Resource{Name: "services"}
			case "close":
				e.Detail = nil
			case "stop":
				m.stopExplore()
			}
			if change != "stop" {
				m.syncExploreUsage()
			}
			if run.Ctx.Err() == nil || e.Usage != nil && len(e.Usage.Points) != 0 {
				t.Fatal("scope change retained request or history")
			}
		})
	}
}

func TestUsageHistoryDeduplicatesAndShowsUnavailableWithoutZeros(t *testing.T) {
	m := exploreModel(t)
	m.explore.Detail = m.explore.Collection.Objects[0].DeepCopy()
	run := enableTestUsage(m)
	now := time.Now()
	point := kubernetes.Usage{CPU: .25, Memory: 32 * 1024 * 1024, Timestamp: now.Add(-time.Second), Window: monitor.UsageInterval}
	run.Points = []kubernetes.Usage{{Timestamp: now.Add(-monitor.UsageHistory - time.Second)}}
	for range 2 {
		m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Usage: point})
	}
	if len(run.Points) != 1 {
		t.Fatal("history retained expired or repeated samples")
	}
	for _, width := range []int{26, 40, 64, 100} {
		view := ansi.Strip(m.usageSummary(width))
		for _, want := range []string{"CPU", "Memory", "250.0m", "32.0Mi"} {
			if !strings.Contains(view, want) {
				t.Errorf("width %d missing %s: %s", width, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Errorf("width %d overflow: %s", width, line)
			}
		}
		plotWidth := 0
		for _, line := range strings.Split(view, "\n") {
			start, end := strings.IndexAny(line, "│┤"), strings.LastIndex(line, "│")
			if start >= 0 && end > start {
				w := ansi.StringWidth(line[start+len("│") : end])
				if plotWidth != 0 && w != plotWidth {
					t.Fatal("CPU and memory use different time scales")
				}
				plotWidth = w
			}
		}
		if plotWidth == 0 {
			t.Fatal("missing CPU and memory plots")
		}
	}
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Err: context.DeadlineExceeded})
	view := ansi.Strip(m.usageSummary(72))
	if strings.Count(view, "N/A") != 2 || !strings.Contains(view, "timed out") || len(run.Points) != 1 {
		t.Fatalf("failed sample became current usage: %s", view)
	}
}

func TestUsagePlotsAdaptHeightAndMoveSourceDetailsIntoHelp(t *testing.T) {
	m := exploreModel(t)
	m.explore.Detail = m.explore.Collection.Objects[0].DeepCopy()
	run := enableTestUsage(m)
	point := kubernetes.Usage{CPU: .25, Memory: 32 * 1024 * 1024, Timestamp: time.Now().Add(-time.Second), Window: monitor.UsageInterval}
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Usage: point})
	m.height = 48
	tall := ansi.Strip(m.usageSummary(80))
	m.height = 24
	compact := ansi.Strip(m.usageSummary(80))
	if lipgloss.Height(tall)-lipgloss.Height(compact) < 8 {
		t.Fatal("compact terminal did not reduce both plots to two rows")
	}
	for _, want := range []string{"15m ago", "10m", "5m", "Now", "Refresh 15s", "Sampled "} {
		if !strings.Contains(tall, want) {
			t.Fatalf("missing %q: %s", want, tall)
		}
	}
	if strings.Contains(tall, "Metrics API") || strings.Contains(tall, "window") {
		t.Fatal("technical source details still crowd the chart")
	}
	if help := m.viewHelp(); !strings.Contains(help, "Metrics API") || !strings.Contains(help, "measurement window: 15s") || !strings.Contains(help, "independent scales") {
		t.Fatal("resource Help omits metrics source or scale details", help)
	}
}

func TestUsageColumnsKeepHealthVisibleAndSortQuantities(t *testing.T) {
	m := exploreModel(t)
	run := enableTestUsage(m)
	now := time.Now()
	run.Samples = map[string]kubernetes.Usage{
		"shop/checkout": {CPU: 1, Memory: 1024 * 1024 * 1024, Timestamp: now},
		"shop/catalog":  {CPU: .02, Memory: 128 * 1024 * 1024, Timestamp: now},
	}
	columns, _ := m.exploreRows()
	for _, name := range []string{"CPU", "Memory"} {
		m.explore.Preference.Sort = slices.Index(columns, name)
		_, rows := m.exploreRows()
		if rows[0].Object.GetName() != "catalog" {
			t.Fatalf("%s did not sort numerically", name)
		}
	}
	m.explore.Preference.Filter = "20.0m"
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Samples: run.Samples})
	if m.explore.Selected != "two" {
		t.Fatal("metrics filter left selection on a hidden row")
	}
	m.explore.Preference.Filter = ""
	for _, width := range []int{42, 76, 104, 150} {
		view := ansi.Strip(m.exploreTableBody(width, 20))
		if !strings.Contains(view, "Running") || !strings.Contains(view, "checkout") {
			t.Fatalf("width %d hid health or identity: %s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("table overflow at %d: %s", width, line)
			}
		}
	}
}

func TestFirstConnectionEnterConnectsAndExpandsPreopenedBranch(t *testing.T) {
	for _, open := range []bool{false, true} {
		m := exploreModel(t)
		connection, client, resources := m.connection, m.explore.Client, m.explore.Resources
		m.clusterFor(m.active, *m.savedConnection()).HelmDriver = "secret"
		m.selectScope(m.active, "kubernetes", "")
		m.exploreCatalogs = nil
		m.expanded[m.active], m.expanded[m.active+"/monitor"] = true, true
		m.setTreeExpanded(connection, open)
		m.treeCursor, m.focus = treeIndexFor(t, m, connection), 0
		press(t, m, "enter")
		if m.connection != connection || !m.expanded[connection] || !m.explore.Connecting {
			t.Fatal("first Enter did not select, connect, and expand together")
		}
		m.Update(monitor.ExploreConnected{Generation: m.explore.Generation, Client: client, Resources: resources})
		for _, tool := range []string{"kubernetes", "helm"} {
			if m.treeRows()[treeIndexFor(t, m, connection+"/view/"+tool)].open {
				t.Fatal("discovery expanded a tool branch", tool)
			}
		}
		if m.focus != 0 || m.treeRows()[m.treeCursor].key != connection {
			t.Fatal("discovery moved focus or tree cursor")
		}
		press(t, m, "enter")
		if m.expanded[connection] {
			t.Fatal("second Enter no longer folds an active connection")
		}
	}
}
