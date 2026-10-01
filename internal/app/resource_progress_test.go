package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/kubernetes"
)

func progressModel(t *testing.T) (*model, *resourceActionState) {
	t.Helper()
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.explore.detailValid = true
	m.openResourceActions("")
	a := m.explore.action
	a.pending = ""
	a.stage = "done"
	a.progress = &resourceProgress{status: kubernetes.OperationProgress{State: "waiting", Summary: "Deletion in progress", Details: "Waiting on finalizers: test.example/hold"}, deadline: time.Now().Add(time.Minute)}
	a.note = "Deletion requested."
	return m, a
}

func TestObservationEscapeReturnsToDetailsAndIgnoresLateResults(t *testing.T) {
	m, a := progressModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.progress.checking = true
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ctx.Err() == nil || m.explore.action != nil || m.explore.detail == nil {
		t.Fatal("Esc did not stop observation and retain details")
	}
	m.updateResourceProgress(resourceProgressResult{action: a, progress: a.progress, status: kubernetes.OperationProgress{State: "complete"}})
	if m.explore.action != nil {
		t.Fatal("late progress reopened action")
	}
}

func TestObservationTimeoutAndRecheckNeverResubmit(t *testing.T) {
	m, a := progressModel(t)
	a.progress.deadline = time.Now().Add(-time.Second)
	if cmd := m.syncResourceProgress(a); cmd != nil || a.progress.status.Watching() || !strings.Contains(a.progress.status.Details, "does not mean") {
		t.Fatal("timeout misreported operation failure")
	}
	m.updateResourceActionKey(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !a.progress.status.Watching() || a.pending != "" || a.progress.deadline.Before(time.Now()) {
		t.Fatal("recheck did not restart only observation")
	}
	old := a.progress
	a.progress = &resourceProgress{status: kubernetes.OperationProgress{State: "waiting"}}
	m.updateResourceProgress(resourceProgressResult{action: a, progress: old, status: kubernetes.OperationProgress{State: "complete"}})
	if !a.progress.status.Watching() {
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
	a.progress.status = kubernetes.OperationProgress{State: "unavailable", Summary: "Status unavailable"}
	if !strings.Contains(ansi.Strip(m.render()), "g check again") {
		t.Fatal("missing recovery control")
	}
}

func TestObservationCancelsPendingReadAndDoesNotScheduleDuplicates(t *testing.T) {
	m, a := progressModel(t)
	a.progress.next = time.Now().Add(time.Hour)
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
	a.progress.status.Details = strings.Repeat("long controller message\n", 50) + "last line\x1b[2J"
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	view := renderOperationProgress(a, 60, 8)
	if !strings.Contains(ansi.Strip(view), "last line") || strings.Contains(view, "\x1b[2J") {
		t.Fatal("end or terminal sanitization failed")
	}
	end := a.scroll
	m.updateResourceActionKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = renderOperationProgress(a, 60, 8)
	if a.scroll != end-1 {
		t.Fatal("scrolling up remained stuck at the end")
	}
}
