package app

import (
	"testing"
	"time"

	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func TestOverviewUsageArrivesWhileInventoryAndDiscoveryWait(t *testing.T) {
	m, v := overviewModel(t)
	v.State = monitor.OverviewState{Loading: true, Pane: 2, Cursor: [4]int{0, 0, 3, 0}}
	v.Cluster.HelmChecked = false
	v.Cluster.Client = &kubernetes.Client{}
	v.Cancel = func() {}
	v.Run = 12
	if m.syncOverview() == nil || v.UsageCancel == nil {
		t.Fatal("usage waited for Helm detection or the inventory request")
	}
	defer v.stop()
	run := v.UsageRun
	m.syncOverview()
	if v.UsageRun != run {
		t.Fatal("started overlapping usage reads")
	}
	now := time.Now()
	data := kubernetes.OverviewUsage{ReadAt: now, SourceStart: now.Add(-time.Second), SourceEnd: now.Add(-time.Second), CPU: new(2.0), CPUAllocatable: new(8.0), Memory: new(1.0), MemoryAllocatable: new(4.0), Observed: 1, Expected: 1}
	m.applyOverviewUsage(overviewUsageResult{v, run, data})
	if v.State.Loaded || !v.State.Loading || *v.State.Data.Kubernetes.CPU.Used != 2 || v.State.Cursor[2] != 3 || v.Cancel == nil {
		t.Fatal("metrics refresh changed inventory state or navigation", v.State)
	}
	if delay := time.Until(v.UsageNext); delay <= 0 || delay > 5*time.Second {
		t.Fatal("usage did not schedule a fast follow-up", delay)
	}
	if m.syncOverviewUsage(v) == nil || !v.UsagePoll || m.syncOverviewUsage(v) != nil {
		t.Fatal("usage poll was missing or scheduled twice")
	}
	m.applyOverview(overviewResult{View: v, Run: 12, Data: monitor.OverviewData{Kubernetes: kubernetes.Overview{ReadAt: now, CPU: kubernetes.OverviewMetric{Requests: new(3.0), Used: new(99.0)}}}})
	if *v.State.Data.Kubernetes.CPU.Used != 2 || *v.State.Data.Kubernetes.CPU.Requests != 3 || len(v.State.Samples[0]) != 1 {
		t.Fatal("late inventory replaced usage, erased requests, or duplicated history")
	}
}

func TestOverviewUsageStopsOnExitAndRejectsLateResults(t *testing.T) {
	m, v := overviewModel(t)
	cancelled := false
	v.UsageCancel = func() { cancelled = true }
	v.UsageRun = 10
	v.UsagePoll = true
	m.mode = 1
	m.syncOverview()
	if !cancelled || v.UsageCancel != nil || v.UsagePoll {
		t.Fatal("usage continued after leaving Overview")
	}
	m.applyOverviewUsage(overviewUsageResult{v, 10, kubernetes.OverviewUsage{ReadAt: time.Now(), CPU: new(999.0)}})
	if v.State.Usage != nil {
		t.Fatal("a late usage response crossed the view lifetime")
	}
}
