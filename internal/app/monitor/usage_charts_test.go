package monitor

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestUsageSnapshotsKeepHistoryAndSelectionImmutable(t *testing.T) {
	now := time.Now()
	first := kubernetes.Usage{CPU: .3, Memory: 1048576, Timestamp: now.Add(-30 * time.Second), Collected: now.Add(-time.Second), SourceEnd: now.Add(-15 * time.Second), Snapshot: "first", Aggregated: true}
	run := &ExploreUsage{}
	run.Apply(ExploreUsageResult{Usage: first}, now)
	run.SelectSample(usagePlotTime(first), now)
	second := first
	second.CPU, second.Collected, second.SourceEnd, second.Snapshot = 1.1, now, now, "second"
	run.Apply(ExploreUsageResult{Usage: second}, now)
	if len(run.Points) != 2 || run.Latest.CPU != 1.1 || run.Points[0].CPU != .3 || !run.Charts.Selected.Equal(usagePlotTime(first)) {
		t.Fatalf("changed total replaced a selected snapshot: %+v", run)
	}
	repeated := second
	repeated.Collected = now.Add(15 * time.Second)
	run.Apply(ExploreUsageResult{Usage: repeated}, now.Add(15*time.Second))
	if len(run.Points) != 2 || !usagePlotTime(run.Latest).Equal(usagePlotTime(second)) {
		t.Fatal("unchanged measurements added an observation or changed its time")
	}
	run.Charts.Selected = time.Time{}
	run.MoveSample(1, now)
	if !run.Charts.Selected.Equal(usagePlotTime(run.Latest)) || run.Points[1].CPU != run.Latest.CPU {
		t.Fatal("latest and inspection disagree")
	}
	e := &ExploreState{Resource: kubecommon.Resource{Group: "apps", Name: "deployments"}, Usage: run}
	text := ansi.Strip(e.UsageSummary(100, 4, now))
	for _, want := range []string{"1100.0m", "Collected ", "Source samples " + first.Timestamp.Local().Format("15:04:05") + "–" + now.Local().Format("15:04:05")} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing snapshot timing or value %q: %s", want, text)
		}
	}
}

func TestUsageLayoutDoesNotChangeSelectionScaleOrScroll(t *testing.T) {
	now := time.Now()
	p := kubernetes.Usage{CPU: .3, Memory: 1048576, Timestamp: now.Add(-16 * time.Minute)}
	run := &ExploreUsage{Latest: p, Points: []kubernetes.Usage{p}, Loaded: true}
	run.Charts = UsageCharts{Inspect: true, Selected: p.Timestamp, Maximum: [2]float64{5, 1048576}, Scroll: 999}
	e := &ExploreState{Resource: kubecommon.Resource{Name: "pods"}, Detail: &unstructured.Unstructured{}, Usage: run}
	before := run.Charts
	for _, size := range [][2]int{{28, 7}, {60, 20}, {236, 51}} {
		layout := e.ChartLayout(size[0], size[1], true, now)
		layout.View(run.Charts.Scroll)
	}
	if run.Charts != before {
		t.Fatal("drawing changed controller state")
	}
	run.ReconcileCharts(now)
	if !run.Charts.Selected.IsZero() || !strings.Contains(run.Charts.Notice, "left this range") {
		t.Fatal("controller update did not expire the selection")
	}
}

func TestUsageLayoutFitsPlotsAfterWrappedNotices(t *testing.T) {
	now := time.Now()
	run := &ExploreUsage{Loaded: true, Err: "Access denied to metrics. Check the selected connection and its resource metrics permissions."}
	e := &ExploreState{Resource: kubecommon.Resource{Name: "pods"}, Detail: &unstructured.Unstructured{}, Usage: run}
	run.Charts.Inspect = true
	for _, size := range [][2]int{{60, 30}, {100, 40}, {236, 51}} {
		layout := e.ChartLayout(size[0], size[1], true, now)
		view := ansi.Strip(layout.View(0))
		if layout.Limit != 0 || !strings.Contains(view, "Now") || !strings.Contains(view, "Refresh 15s") {
			t.Fatalf("plots did not account for header and notices at %v: %s", size, view)
		}
	}
}
