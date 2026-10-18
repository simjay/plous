package app

import (
	"context"
	"fmt"
	"github.com/simjay/plous/internal/app/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
)

func logBufferModel(t *testing.T) *model {
	t.Helper()
	m := exploreModel(t)
	m.width, m.height = 160, 40
	m.openExploreSelection(4)
	m.syncExploreDetail()
	run := m.explore.DetailRun
	run.Cancel()
	m.updateExploreMessage(monitor.ExploreDetailResult{Run: run, Object: m.explore.Detail.DeepCopy()})
	m.syncExploreDetail()
	return m
}

func TestLogsReadingKeepsStreamAndResumeKeepsBuffer(t *testing.T) {
	m := logBufferModel(t)
	e := m.explore
	run, key := e.Log, e.LogKey
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: strings.Repeat("record\n", 100)})
	m.render()
	press(t, m, "space")
	v := m.podLogViewer()
	offset := v.Reader.Top
	if v.Following || offset >= 100 || m.syncExploreDetail() != nil || run.Ctx.Err() != nil {
		t.Fatal("pause stopped collection or lost viewport")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: "new record 1\nnew record 2\n"})
	m.render()
	if v.Reader.Top != offset || v.Unseen != 2 || !strings.Contains(v.Text, "new record 2") {
		t.Fatal("append lost reading position or output")
	}
	text := v.Text
	press(t, m, "space")
	if !v.Following || v.Unseen != 0 || v.Text != text || e.LogKey != key || e.Log != run || v.Reader.Cursor.Row < 100 {
		t.Fatal("resume replaced stream or lost output")
	}
	if m.syncExploreDetail() != nil || run.Ctx.Err() != nil {
		t.Fatal("resume restarted collection")
	}
}

func TestLogsEndHoldsLatestPositionUntilFollowingResumes(t *testing.T) {
	m := logBufferModel(t)
	m.appendExploreLogs(strings.Repeat("earlier record\n", 100))
	press(t, m, "home")
	press(t, m, "end")
	m.render()
	v := m.podLogViewer()
	offset, cursor := v.Reader.Top, v.Reader.Cursor
	if v.Following || cursor.Row != len(v.Reader.Lines)-1 {
		t.Fatal("End failed to hold tail")
	}
	m.appendExploreLogs(strings.Repeat("new record\n", 10))
	m.render()
	if v.Reader.Top != offset || v.Reader.Cursor != cursor {
		t.Fatal("new output moved held tail")
	}
}

func TestLogsTrimPreservesRetainedReadingPosition(t *testing.T) {
	for _, wrap := range []bool{true, false} {
		t.Run(fmt.Sprintf("wrap=%t", wrap), func(t *testing.T) {
			m := logBufferModel(t)
			v := m.podLogViewer()
			v.Reader.Wrap = wrap
			var initial strings.Builder
			for i := range 600 {
				fmt.Fprintf(&initial, "record %04d %s\n", i, strings.Repeat("word ", 35))
			}
			m.appendExploreLogs(initial.String())
			v.Pause()
			v.Reader.Cursor = textview.Point{Row: 400}
			m.render()
			anchor := v.Reader.SelectedText()
			m.appendExploreLogs(strings.Repeat("new record "+strings.Repeat("word ", 35)+"\n", 250))
			m.render()
			if len(v.Text) > monitor.ExploreLogBufferLimit || v.Dropped == 0 || v.Unseen != 250 {
				t.Fatal("retention bounds lost")
			}
			if v.Reader.SelectedText() != anchor {
				t.Fatal("eviction moved retained selection")
			}
		})
	}
}

func TestLogsSearchKeepsSelectedMatchAsBufferChanges(t *testing.T) {
	m := logBufferModel(t)
	v := m.podLogViewer()
	var initial strings.Builder
	for i := range 600 {
		fmt.Fprintf(&initial, "needle %04d %s\n", i, strings.Repeat("word ", 35))
	}
	m.appendExploreLogs(initial.String())
	typeText(m, "/needle 0400")
	press(t, m, "enter")
	m.render()
	before := v.Reader.Top
	anchor := v.Reader.SelectedText()
	m.appendExploreLogs("needle appended\n")
	m.render()
	if v.Reader.Top != before || v.Following {
		t.Fatal("append moved search result")
	}
	m.appendExploreLogs(strings.Repeat("needle "+strings.Repeat("word ", 35)+"\n", 250))
	m.render()
	if v.Reader.SelectedText() != anchor || v.Reader.Status() != "1/1 matches" || v.Dropped == 0 || m.explore.Log.Ctx.Err() != nil {
		t.Fatal("eviction lost match or stopped collection")
	}
	if m.searches[1] != nil || m.beginPanelSearch() {
		t.Fatal("logs created a second search controller")
	}
}

func TestLogsSearchNearTailDoesNotRecenterForNewOutput(t *testing.T) {
	m := logBufferModel(t)
	m.appendExploreLogs(strings.Repeat("older record\n", 20) + "needle\n")
	typeText(m, "/needle")
	press(t, m, "enter")
	m.render()
	v := m.podLogViewer()
	offset := v.Reader.Top
	m.appendExploreLogs(strings.Repeat("new record\n", 20))
	m.render()
	if v.Reader.Top != offset {
		t.Fatal("new output recentered match")
	}
}

func TestLogsConnectionStateAndSourceReset(t *testing.T) {
	m := logBufferModel(t)
	e := m.explore
	run := e.Log
	if !m.loadingVisible() {
		t.Fatal("initial connection must animate")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Connected: true})
	if m.loadingVisible() || !e.LogConnected {
		t.Fatal("connected stream must stop animation")
	}
	v := m.podLogViewer()
	v.Append("retained\n")
	v.Dropped, v.Unseen = 3, 4
	v.Reader.Wrap = false
	m.updateExplore("p")
	m.syncExploreDetail()
	if run.Ctx.Err() == nil || e.Log == run || v.Text != "" || v.Unseen != 0 || v.Dropped != 0 || e.LogConnected || v.Reader.Wrap {
		t.Fatal("source reset lost wrap preference or kept old stream")
	}
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: run, Text: "stale\n", Connected: true})
	if v.Text != "" || e.LogConnected {
		t.Fatal("stale output reached replacement")
	}
	current := e.Log
	m.updateExploreMessage(monitor.ExploreLogEvent{Run: current, Text: "previous\n", Done: true})
	if e.Log != nil || m.syncExploreDetail() != nil || v.Text != "previous\n" {
		t.Fatal("ended stream lost output or restarted")
	}
	m.updateExplore("g")
	if e.DetailKey != "" || e.LogKey != "" {
		t.Fatal("reconnect did not invalidate request")
	}
}

func TestLogsEscCancelsCollectionAndRestoresDetail(t *testing.T) {
	m := logBufferModel(t)
	run := m.explore.Log
	press(t, m, "up")
	press(t, m, "esc")
	if run.Ctx.Err() != context.Canceled || m.explore.Tab != 0 || m.explore.Detail == nil {
		t.Fatal("leaving logs did not restore origin")
	}
}
