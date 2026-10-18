package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"k8s.io/client-go/rest"
)

func overviewMetricModel(t *testing.T) (*model, *overviewView) {
	t.Helper()
	m, v := overviewModel(t)
	m.width, m.height = 180, 45
	now := time.Now()
	k := kubernetes.Overview{ReadAt: now, ActivePods: 321, Reads: map[string]kubernetes.OverviewRead{"pods": {ReadAt: now, Count: 345}, "nodes": {ReadAt: now, Count: 13}}}
	k.CPU = kubernetes.OverviewMetric{Requests: new(70.12), Limits: new(79.68), RequestsMissing: 33, LimitsMissing: 108}
	k.Memory = kubernetes.OverviewMetric{Requests: new(275.4 * (1 << 30)), Limits: new(298.7 * (1 << 30)), RequestsMissing: 59, LimitsMissing: 73}
	v.State.Apply(monitor.OverviewData{Kubernetes: k})
	for i := range 60 {
		at := now.Add(-time.Duration(59-i) * 5 * time.Second)
		v.State.ApplyUsage(kubernetes.OverviewUsage{ReadAt: at, SourceStart: at, SourceEnd: at, Observed: 13, Expected: 13,
			CPU: new(6.91 + float64(i%7)*0.2), Memory: new((183.4 + float64(i%5)*2) * (1 << 30)), CPUAllocatable: new(118.79), MemoryAllocatable: new(569.5 * (1 << 30))})
	}
	return m, v
}

func TestOverviewMetricClickSearchSourcesAndBack(t *testing.T) {
	m, v := overviewMetricModel(t)
	v.State.Cursor[0] = 0
	v.State.Cursor[2], v.State.Query = 1, "keep filter"
	clickMouse(t, m, "main", "overview-tile/1")
	d := v.State.MetricDetail
	if d == nil || d.Index != 1 || m.detail != "" {
		t.Fatal("Memory click did not open its own chart")
	}
	screen := ansi.Strip(m.render())
	for _, want := range []string{"Memory · All namespaces", "GiB", "Coverage: 13/13 Nodes", "73 observed active Pods have missing memory limits."} {
		if !strings.Contains(screen, want) {
			t.Fatalf("metric view missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "Overview evidence") || strings.Contains(screen, "CPU requests") {
		t.Fatal("chart still includes unrelated evidence")
	}
	m.Update(localMouseKey("/"))
	m.pasteText("ff:cc")
	if d.Reader().Query != "ff:cc" || m.viewExpanded() || m.chatEditing {
		t.Fatal("metric search lost literal input")
	}
	m.Update(localMouseKey("esc"))
	m.Update(localMouseKey("end"))
	m.render()
	position := d.Reader().Cursor
	clickMouse(t, m, "main", "action/enter")
	if !d.Sources {
		t.Fatal("source details action did not open")
	}
	if text := ansi.Strip(m.render()); !strings.Contains(text, "metrics.k8s.io Node observations") || strings.Contains(text, "HPA current replica") {
		t.Fatal("sources are not scoped to the metric", text)
	}
	m.Update(localMouseKey("esc"))
	if d.Sources || d.Reader().Cursor != position {
		t.Fatal("returning from sources lost chart position")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != nil || v.State.Cursor[0] != 1 || v.State.Cursor[2] != 1 || v.State.Query != "keep filter" {
		t.Fatal("returning to Overview lost selection or filter")
	}
}

func TestOverviewMetricLayersAndPolling(t *testing.T) {
	m, v := overviewMetricModel(t)
	m.Update(localMouseKey("enter"))
	d := v.State.MetricDetail
	if d == nil {
		t.Fatal("Enter did not open CPU")
	}
	cancelled := false
	v.UsageCancel = func() { cancelled = true }
	m.syncOverview()
	if cancelled || !m.overviewVisible() {
		t.Fatal("metric detail stopped Overview polling")
	}
	usage := *v.State.Usage
	usage.ReadAt = usage.ReadAt.Add(time.Second)
	usage.SourceStart, usage.SourceEnd, usage.CPU = usage.ReadAt, usage.ReadAt, new(59.395)
	v.State.ApplyUsage(usage)
	if !strings.Contains(ansi.Strip(m.render()), "50% used") || v.State.MetricDetail != d {
		t.Fatal("new sample did not update the same detail view")
	}
	m.execute("fullscreen")
	m.execute("help")
	if !strings.Contains(ansi.Strip(m.render()), "CPU") {
		t.Fatal("Help lost metric context")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != d || !m.viewExpanded() {
		t.Fatal("Help did not preserve detail and fullscreen")
	}
	m.Update(localMouseKey("enter"))
	m.Update(localMouseKey("esc"))
	if d.Sources || !m.viewExpanded() {
		t.Fatal("source Escape restored layout too early")
	}
	m.Update(localMouseKey("esc"))
	if v.State.MetricDetail != nil || !m.viewExpanded() {
		t.Fatal("detail Escape restored layout too early")
	}
	m.Update(localMouseKey("esc"))
	if m.viewExpanded() {
		t.Fatal("Overview Escape did not restore layout")
	}
}

func TestOverviewPulseResourceListsPreserveSelectionAndPreferences(t *testing.T) {
	for index, resource := range map[int]string{2: "nodes", 3: "pods", 4: "horizontalpodautoscalers.autoscaling"} {
		for _, width := range []int{60, 180} {
			m, v := overviewMetricModel(t)
			m.width = width
			client, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1:1"})
			if err != nil {
				t.Fatal(err)
			}
			v.Cluster.Client = client
			old := monitor.ExplorePreference{Resource: "deployments.apps", Namespace: "saved", Filter: "keep", Selector: "team=one"}
			key := m.active + "/" + m.connection
			m.explorePreferences = map[string]monitor.ExplorePreference{key: old}
			v.State.Cursor[0] = index
			m.Update(localMouseKey("enter"))
			if m.explore == nil || m.mode != 1 || m.explore.Resource.Key() != resource || !m.explore.Temporary || m.explore.Preference.Namespace != "" {
				t.Fatalf("tile %d at width %d did not open its cluster-wide list", index, width)
			}
			m.rememberExplore()
			m.Update(localMouseKey("esc"))
			if m.mode != 0 || v.State.Cursor[0] != index || m.explorePreferences[key] != old {
				t.Fatalf("tile %d at width %d did not return with saved preferences", index, width)
			}
		}
	}
}

func TestOverviewMetricScreenMatrix(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {150, 40}, {180, 45}, {240, 60}} {
		for _, index := range []int{0, 1} {
			m, v := overviewMetricModel(t)
			m.width, m.height = size[0], size[1]
			v.State.Cursor[0] = index
			m.Update(localMouseKey("enter"))
			for _, full := range []bool{false, true} {
				if full {
					m.execute("fullscreen")
				}
				name := fmt.Sprintf("overview-metric-%d-%dx%d-full-%t", index, size[0], size[1], full)
				captureOverviewScreen(t, m, name)
				m.Update(localMouseKey("end"))
				if text := ansi.Strip(m.render()); !strings.Contains(text, "unbounded") {
					t.Fatalf("%s cannot reach the last warning:\n%s", name, text)
				}
				m.Update(localMouseKey("home"))
			}
		}
	}
	m, v := overviewMetricModel(t)
	v.State.OpenMetric()
	v.State.Data.Kubernetes.MetricsAt = time.Now().Add(-2 * time.Minute)
	v.State.Data.Kubernetes.Reads["pods"] = kubernetes.OverviewRead{ReadAt: time.Now(), Partial: true}
	text := ansi.Strip(m.render())
	for _, want := range []string{"STALE", "Partial Pod inventory", "budget totals unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing limitation %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "70.12") || strings.Contains(text, "79.68") {
		t.Fatal("partial budget was presented as a complete total")
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	captureOverviewScreen(t, m, "overview-metric-stale-partial")
}
