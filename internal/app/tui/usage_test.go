package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func chartModel(t *testing.T) (*model, *monitor.ExploreUsage) {
	m := exploreModel(t)
	m.width, m.height = 180, 48
	m.explore.Detail = m.explore.Collection.Objects[0].DeepCopy()
	m.syncExploreDetail()
	m.explore.DetailValid = true
	run := enableTestUsage(m)
	now := time.Now().Add(-time.Second)
	for i := 0; i < 3; i++ {
		p := kubernetes.Usage{CPU: float64(i+1) / 10, Memory: float64(i+1) * 1024 * 1024, Timestamp: now.Add(time.Duration(i-2) * 15 * time.Second), Window: 15 * time.Second}
		m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Usage: p})
	}
	return m, run
}

func TestUsageChartInspectionPreservesTimestampAndEscapeOrder(t *testing.T) {
	m, run := chartModel(t)
	m.explore.Scroll = 3
	typeText(m, "I")
	if !m.chartsActive() {
		t.Fatal("I did not open charts")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	selected := run.Points[1].Timestamp
	if !run.Charts.Selected.Equal(selected) {
		t.Fatal("left did not select previous observation")
	}
	m.execute("fullscreen")
	m.execute("help")
	m.Update(tea.WindowSizeMsg{Width: 240, Height: 60})
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Usage: kubernetes.Usage{CPU: .9, Memory: 2e6, Timestamp: time.Now()}})
	m.execute("settings")
	m.closeSettings()
	m.closeHelp()
	m.View()
	if !run.Charts.Selected.Equal(selected) {
		t.Fatal("layout, overlays or arrival changed selection")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.chartsActive() || !m.viewExpanded() || m.explore.Detail == nil || m.explore.Scroll != 3 {
		t.Fatal("Escape did not leave inspection first")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.viewExpanded() || m.explore.Detail == nil {
		t.Fatal("second Escape should restore layout")
	}
}

func TestUsageChartClickAndKeyboardSelectSameSample(t *testing.T) {
	m, run := chartModel(t)
	m.execute("charts")
	target := run.Points[1].Timestamp
	clickMouse(t, m, "main", "usage-sample/"+target.Format(time.RFC3339Nano))
	if !run.Charts.Selected.Equal(target) {
		t.Fatal("plot click did not select source timestamp")
	}
	m.execute("chartlatest")
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !run.Charts.Selected.Equal(target) {
		t.Fatal("keyboard and click disagree")
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {180, 48}, {240, 60}} {
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v", size)
			}
		}
		if !run.Charts.Selected.Equal(target) {
			t.Fatal("resize changed selection")
		}
	}
}

func TestUsageChartRangeExpiryAndScaleStability(t *testing.T) {
	m, run := chartModel(t)
	old := kubernetes.Usage{CPU: 4, Memory: 2e6, Timestamp: time.Now().Add(-10 * time.Minute)}
	run.Points = append([]kubernetes.Usage{old}, run.Points...)
	run.ReconcileCharts(time.Now())
	maximum := run.Charts.Maximum
	run.SelectSample(old.Timestamp, time.Now())
	m.execute("chartrange")
	if run.Charts.Range() != 5*time.Minute || !run.Charts.Selected.IsZero() || !strings.Contains(run.Charts.Notice, "left this range") {
		t.Fatal("range silently retained an invisible sample")
	}
	if run.Charts.Maximum != maximum {
		t.Fatal("scale shrank on range change")
	}
	run.Charts.ScaledAt = time.Now().Add(-2 * time.Minute)
	run.SelectSample(run.Points[1].Timestamp, time.Now())
	run.ReconcileCharts(time.Now())
	if run.Charts.Maximum != maximum {
		t.Fatal("scale shrank while inspecting")
	}
	run.Charts.Selected = time.Time{}
	run.ReconcileCharts(time.Now())
	if run.Charts.Maximum[0] >= maximum[0] {
		t.Fatal("scale never recovered after returning live")
	}
}

func TestUsageTopConsumersRespectFiltersIdentityAndCoverage(t *testing.T) {
	m := exploreModel(t)
	run := enableTestUsage(m)
	run.Loaded = true
	run.Samples = map[string]kubernetes.Usage{
		"shop/checkout":      {CPU: .5, Memory: 1e6, Timestamp: time.Now()},
		"shop/catalog":       {CPU: 10, Memory: 2e6, Timestamp: time.Now(), UID: "replaced"},
		"elsewhere/outsider": {CPU: 100, Timestamp: time.Now()},
	}
	text := ansi.Strip(m.explore.TopConsumers(80, time.Now()))
	if !strings.Contains(text, "1/2 filtered") || strings.Contains(text, "outsider") || strings.Contains(text, "shop/catalog") {
		t.Fatal(text)
	}
	m.explore.Preference.Filter = "catalog"
	text = ansi.Strip(m.explore.TopConsumers(80, time.Now()))
	if !strings.Contains(text, "0/1 filtered") || strings.Contains(text, "shop/checkout") {
		t.Fatal(text)
	}
	if m.explore.Detail != nil || len(run.Points) != 0 {
		t.Fatal("ranking changed acquisition or selection")
	}
}

func TestUsageChartCommandsPreserveTextOwnership(t *testing.T) {
	m, run := chartModel(t)
	m.focusChat(true)
	typeText(m, "IWP")
	if run.Charts.Inspect || m.draft().Text != "IWP" {
		t.Fatal("chart keys consumed chat input")
	}
	m.focus = 1
	m.openExploreInput("filter", "unfinished")
	m.focus = 2
	m.execute("charts")
	if run.Charts.Inspect || m.explore.Input == nil || m.explore.Input.Text != "unfinished" {
		t.Fatal("chart command changed unfinished input")
	}
}

func TestUsageFirstSampleSetsReadableScalesAndCompactValues(t *testing.T) {
	m := exploreModel(t)
	m.explore.Detail = m.explore.Collection.Objects[0].DeepCopy()
	run := enableTestUsage(m)
	m.usageSummary(26)
	m.updateExploreUsage(monitor.ExploreUsageResult{Run: run, Usage: kubernetes.Usage{CPU: .05, Memory: 32 * 1048576, Timestamp: time.Now()}})
	if run.Charts.Maximum[0] > .1 || run.Charts.Maximum[1] != 50*1048576 {
		t.Fatal("waiting scales hid the first observation", run.Charts.Maximum)
	}
	run.Latest.CPU, run.Latest.Memory = 123.45, 128*1024*1024*1024
	text := ansi.Strip(m.usageSummary(26))
	if !strings.Contains(text, "123450.0m") || !strings.Contains(text, "128.00Gi") {
		t.Fatal("compact readout lost a value", text)
	}
}

func TestUsageChartSearchOwnership(t *testing.T) {
	for _, back := range []string{"button", "escape", "search"} {
		t.Run(back, func(t *testing.T) {
			m, run := chartModel(t)
			m.beginPanelSearch()
			typeText(m, "app")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			search := m.searches[1]
			scroll := m.explore.Scroll
			typeText(m, "I")
			if !m.chartsActive() || m.activePanelSearch(1) != nil {
				t.Fatal("inspection did not suspend search")
			}
			if !strings.Contains(m.hint(), "W Range") {
				t.Fatal("search footer owns chart controls", m.hint())
			}
			typeText(m, "hh")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			typeText(m, "ss")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.searches[1] != search || !m.chartsActive() {
				t.Fatal("overlay discarded the suspended search")
			}
			clickMouse(t, m, "main", "action/W")
			if run.Charts.Range() != 5*time.Minute {
				t.Fatal("Range button was consumed by search")
			}
			run.SelectSample(run.Points[0].Timestamp, time.Now())
			clickMouse(t, m, "main", "action/P")
			if !run.Charts.Selected.IsZero() {
				t.Fatal("Latest button was consumed by search")
			}
			switch back {
			case "button":
				clickMouse(t, m, "main", "action/I")
			case "escape":
				m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			case "search":
				typeText(m, "/")
			}
			if m.chartsActive() || m.searches[1] != search || search.input.Text != "app" || m.explore.Scroll != scroll {
				t.Fatal("search state was not restored")
			}
			if back == "search" && search.finished {
				t.Fatal("search did not resume typing")
			}
		})
	}
}

func TestUsageChartRankEnter(t *testing.T) {
	m := exploreModel(t)
	run := enableTestUsage(m)
	run.Loaded = true
	run.Samples = map[string]kubernetes.Usage{"shop/checkout": {CPU: .1, Memory: 1, Timestamp: time.Now()}, "shop/catalog": {CPU: 5, Memory: 10, Timestamp: time.Now()}}
	m.execute("charts")
	t.Log("\n" + ansi.Strip(m.explore.TopConsumers(80, time.Now())))
	typeText(m, "dyeSLn")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.explore.Detail != nil {
		t.Errorf("Enter opened hidden table selection: %s", m.explore.Detail.GetName())
	}
}

func TestUsageChartWrappedScroll(t *testing.T) {
	m := exploreModel(t)
	run := enableTestUsage(m)
	run.Loaded = true
	m.width, m.height = 80, 16
	m.execute("charts")
	m.View()
	m.updateChartKey("end")
	frame := ansi.Strip(m.View().Content)
	t.Log("\n" + frame)
	if !strings.Contains(frame, "available.") {
		t.Error("End cannot reach the wrapped empty-state message")
	}
}

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
