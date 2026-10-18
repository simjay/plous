package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
)

func progressModel(t *testing.T) (*model, *monitor.ResourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.DetailValid = true
	m.openResourceActions("")
	a := m.explore.Action
	a.Pending = ""
	a.Stage = "done"
	a.Progress = &monitor.ResourceProgress{Status: kubernetes.OperationProgress{State: "waiting", Summary: "Deletion in progress", Details: "Waiting on finalizers: test.example/hold"}, Deadline: time.Now().Add(time.Minute)}
	a.Note = "Deletion requested."
	return m, a
}

func TestObservationEscapeReturnsToDetailsAndIgnoresLateResults(t *testing.T) {
	m, a := progressModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.Cancel = cancel
	a.Progress.Checking = true
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ctx.Err() == nil || m.explore.Action != nil || m.explore.Detail == nil {
		t.Fatal("Esc did not stop observation and retain details")
	}
	m.updateResourceProgress(monitor.ResourceProgressResult{Action: a, Progress: a.Progress, Status: kubernetes.OperationProgress{State: "complete"}})
	if m.explore.Action != nil {
		t.Fatal("late progress reopened action")
	}
}

func TestObservationTimeoutAndRecheckNeverResubmit(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Deadline = time.Now().Add(-time.Second)
	if cmd := m.syncResourceProgress(a); cmd != nil || a.Progress.Status.Watching() || !strings.Contains(a.Progress.Status.Details, "does not mean") {
		t.Fatal("timeout misreported operation failure")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !a.Progress.Status.Watching() || a.Pending != "" || a.Progress.Deadline.Before(time.Now()) {
		t.Fatal("recheck did not restart only observation")
	}
	old := a.Progress
	a.Progress = &monitor.ResourceProgress{Status: kubernetes.OperationProgress{State: "waiting"}}
	m.updateResourceProgress(monitor.ResourceProgressResult{Action: a, Progress: old, Status: kubernetes.OperationProgress{State: "complete"}})
	if !a.Progress.Status.Watching() {
		t.Fatal("stale observer changed new result")
	}
}

func TestProgressOverlayRetainsTargetStatusAndControlsAcrossSizes(t *testing.T) {
	m, a := progressModel(t)
	for _, size := range [][2]int{{80, 24}, {120, 32}, {220, 50}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.render())
		for _, want := range []string{"UID one", "Deletion requested.", "Deletion in progress", "test.example/hold", "Enter/Esc details"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%v missing %q: %s", size, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatal("width overflow")
			}
		}
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatal("height overflow")
		}
	}
	a.Progress.Status = kubernetes.OperationProgress{State: "unavailable", Summary: "Status unavailable"}
	if !strings.Contains(ansi.Strip(m.render()), "g check again") {
		t.Fatal("missing recovery control")
	}
}

func TestObservationCancelsPendingReadAndDoesNotScheduleDuplicates(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Next = time.Now().Add(time.Hour)
	cmd := m.syncResourceProgress(a)
	if cmd == nil || m.syncResourceProgress(a) != nil {
		t.Fatal("observer was missing or scheduled twice")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if msg := cmd(); msg != nil {
		t.Fatal("closed observer performed a read")
	}
}

func TestProgressScrollClampsAndStripsServerControlSequences(t *testing.T) {
	m, a := progressModel(t)
	a.Progress.Status.Details = strings.Repeat("long controller message\n", 50) + "last line\x1b[2J"
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	view := monitor.RenderOperationProgress(a, 60, 8)
	if !strings.Contains(ansi.Strip(view), "last line") || strings.Contains(view, "\x1b[2J") {
		t.Fatal("end or terminal sanitization failed")
	}
	end := a.Scroll
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = monitor.RenderOperationProgress(a, 60, 8)
	if a.Scroll != end-1 {
		t.Fatal("scrolling up remained stuck at the end")
	}
}
