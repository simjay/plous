package app

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/simjay/plous/internal/app/monitor"
	"github.com/simjay/plous/internal/ui"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func reviewTerminal(t *testing.T, width, height int) *monitor.ResourceTerminal {
	t.Helper()
	_, cancel := context.WithCancel(context.Background())
	tm := &monitor.ResourceTerminal{Emulator: vt.NewSafeEmulator(width, height), Cancel: cancel, Captured: true}
	tm.Emulator.SetScrollbackSize(monitor.TerminalScrollbackLimit)
	t.Cleanup(tm.Close)
	return tm
}

func writeTerminalLines(t *testing.T, tm *monitor.ResourceTerminal, start, count int) {
	t.Helper()
	for i := start; i < start+count; i++ {
		if _, err := fmt.Fprintf(tm, "line-%04d\r\n", i); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTerminalReviewRetainsOlderOutputAndFreezesReading(t *testing.T) {
	tm := reviewTerminal(t, 30, 5)
	writeTerminalLines(t, tm, 0, 100)
	tm.BeginReview()
	if tm.Captured || len(tm.Review.Viewer.Reader.Lines) < 100 {
		t.Fatal("review did not retain the scrolled output")
	}
	tm.ReviewKey("home", "")
	before := tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme())
	if !strings.Contains(ansi.Strip(before), "line-0000") || strings.Contains(before, "line-0099") {
		t.Fatalf("earliest output is unavailable: %q", before)
	}
	writeTerminalLines(t, tm, 100, 10)
	if got := tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme()); got != before {
		t.Fatal("new output moved or changed the review screen")
	}
	_, state := tm.TerminalState()
	if !strings.Contains(state, "New output available") {
		t.Fatalf("new output is not discoverable: %s", state)
	}
	tm.ReviewKey("end", "")
	if !strings.Contains(tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme()), "line-0109") {
		t.Fatal("End did not update review with the latest output")
	}
}

func TestTerminalReviewHistoryIsBoundedAndAltScreenStaysSeparate(t *testing.T) {
	tm := reviewTerminal(t, 30, 5)
	writeTerminalLines(t, tm, 0, monitor.TerminalScrollbackLimit+20)
	tm.BeginReview()
	if len(tm.Review.Viewer.Reader.Lines) != monitor.TerminalScrollbackLimit+5 || !tm.Review.Limited {
		t.Fatalf("unbounded terminal history: %d", len(tm.Review.Viewer.Reader.Lines))
	}
	tm.ReviewKey("home", "")
	if strings.Contains(tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme()), "line-0000") {
		t.Fatal("history did not discard its oldest line")
	}
	_, _ = tm.Write([]byte("\x1b[?1049h\x1b[HALTERNATE"))
	tm.BeginReview()
	if !tm.Review.Alt || len(tm.Review.Viewer.Reader.Lines) != 5 || !strings.Contains(tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme()), "ALTERNATE") {
		t.Fatal("alternate screen mixed current content with shell scrollback")
	}
	_, _ = tm.Write([]byte("\x1b[?1049l"))
	tm.BeginReview()
	if tm.Review.Alt || len(tm.Review.Viewer.Reader.Lines) <= 5 {
		t.Fatal("returning from alternate screen lost main screen history")
	}
}

func TestTerminalReviewPreservesANSIAndWideCharacters(t *testing.T) {
	tm := reviewTerminal(t, 30, 3)
	_, _ = tm.Write([]byte("\x1b[31mError 界\x1b[0m\r\nnext\r\nlast\r\n"))
	tm.BeginReview()
	tm.ReviewKey("home", "")
	view := tm.Review.Viewer.Reader.View(30, 3, ui.WorkspaceTheme())
	if !strings.Contains(view, "\x1b[") || !strings.Contains(ansi.Strip(view), "Error 界") {
		t.Fatalf("review lost styled terminal cells: %q", view)
	}
	tm.Resize(8, 2)
	for _, row := range strings.Split(tm.Review.Viewer.Reader.View(8, 2, ui.WorkspaceTheme()), "\n") {
		if lipgloss.Width(row) > 8 {
			t.Fatalf("resized review overflows: %q", row)
		}
	}
}

func TestTerminalReviewNavigationAndResumeAreLocal(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 30, 5)
	m.explore.Terminal = tm
	writeTerminalLines(t, tm, 0, 100)
	tm.BeginReview()
	m.updateResourceTerminalKey(tea.KeyPressMsg{Code: tea.KeyHome})
	m.updateResourceTerminalKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if tm.Review.Viewer.Reader.Cursor.Row != 1 || tm.Captured {
		t.Fatal("review keys were not handled locally")
	}
	m.updateResourceTerminalKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !tm.Captured || tm.Review != nil {
		t.Fatal("Resume did not return to live input")
	}
	tm.BeginReview()
	m.updateResourceTerminalKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.explore.Terminal != nil {
		t.Fatal("Close did not restore the resource detail")
	}
}

func TestTerminalInputPreservesCaseUnicodeAndSpecialKeys(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 80, 24)
	m.explore.Terminal = tm
	var input strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&input, tm.Emulator)
		close(done)
	}()
	keys := []tea.KeyPressMsg{
		{Code: 'a', Text: "A", Mod: tea.ModShift},
		{Code: 'b', Text: "B", Mod: tea.ModCapsLock},
		{Code: 'c', Text: "c"},
		{Code: '1', Text: "!", Mod: tea.ModShift},
		{Code: 'é', Text: "é"},
		{Code: '界', Text: "界"},
		{Code: tea.KeyExtended, Text: "e\u0301"},
		{Code: tea.KeyTab},
		{Code: tea.KeyUp},
		{Code: tea.KeyEnter},
		{Code: 'd', Mod: tea.ModCtrl},
	}
	for _, key := range keys {
		m.updateResourceTerminalKey(key)
	}
	tm.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("terminal input did not close")
	}
	if got, want := input.String(), "ABc!é界e\u0301\t\x1b[A\r\x04"; got != want {
		t.Fatalf("container received %q, want %q", got, want)
	}
}

func TestTerminalLaunchErrorShowsCauseWithoutBlankScreenPadding(t *testing.T) {
	tm := reviewTerminal(t, 84, 28)
	tm.Done, tm.Err = true, "Internal error occurred: error executing command in container: failed to exec in container: failed to start exec: OCI runtime exec failed: exec: \"/bin/bash\": stat /bin/bash: no such file or directory"
	tm.BeginReview()
	view := ansi.Strip(tm.Review.Viewer.Reader.View(84, 20, ui.WorkspaceTheme()))
	for _, text := range []string{"Session error", "Internal error", "no such file or directory"} {
		if !strings.Contains(view, text) {
			t.Fatalf("launch error hides %q: %s", text, view)
		}
	}
	if len(tm.Review.Viewer.Reader.Lines) != 2 {
		t.Fatal("empty terminal rows push the launch error below the viewport")
	}
}

func TestTerminalReviewSurvivesConcurrentOutputAndResize(t *testing.T) {
	tm := reviewTerminal(t, 40, 8)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 300 {
			_, _ = io.WriteString(tm, "\x1b[32moutput\x1b[0m\r\n")
		}
	}()
	for i := range 20 {
		tm.BeginReview()
		tm.ReviewKey("home", "")
		tm.Resize(30+i, 5+i%3)
		_ = tm.Review.Viewer.Reader.View(30+i, 5+i%3, ui.WorkspaceTheme())
	}
	wg.Wait()
}

func TestTerminalEndedOutputAndWrappedErrorRemainReviewable(t *testing.T) {
	m := exploreModel(t)
	tm := reviewTerminal(t, 30, 5)
	m.explore.Terminal = tm
	writeTerminalLines(t, tm, 0, 20)
	m.Update(monitor.ResourceTerminalResult{Terminal: tm, Err: fmt.Errorf("The shell could not start because this image does not include the requested executable. Choose another command.")})
	if !tm.Done || tm.Captured || tm.Review == nil {
		t.Fatal("ended session did not retain output for review")
	}
	parts := []string{}
	for _, line := range tm.Review.Viewer.Reader.Lines {
		parts = append(parts, line.Text)
	}
	all := strings.Join(parts, "\n")
	if !strings.Contains(all, "line-0000") || !strings.Contains(strings.Join(strings.Fields(all), " "), tm.Err) {
		t.Fatalf("output or full error lost: %s", all)
	}
	for _, row := range strings.Split(tm.Review.Viewer.Reader.View(30, 5, ui.WorkspaceTheme()), "\n") {
		if lipgloss.Width(row) > 30 {
			t.Fatalf("error line exceeds terminal: %q", row)
		}
	}
}

func TestTerminalOverlayFitsAndShowsTargetModesAndControls(t *testing.T) {
	for _, size := range [][2]int{{80, 16}, {120, 30}, {240, 60}} {
		m := exploreModel(t)
		m.width, m.height = size[0], size[1]
		width, height := m.terminalDimensions()
		tm := reviewTerminal(t, width, height)
		tm.Context, tm.Namespace, tm.Pod, tm.Container, tm.Command = "fixture", "default", "checkout-pod", "app", "sh"
		m.explore.Terminal = tm
		for _, captured := range []bool{true, false} {
			if !captured {
				m.status = ""
				tm.BeginReview()
			}
			view := m.render()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("%dx%d overlay overflow: %dx%d", m.width, m.height, lipgloss.Width(view), lipgloss.Height(view))
			}
			text := ansi.Strip(view)
			for _, expected := range []string{"checkout-pod", "Command sh"} {
				if !strings.Contains(text, expected) {
					t.Fatalf("%dx%d missing %q", m.width, m.height, expected)
				}
			}
			if captured && !strings.Contains(text, "Terminal input active") {
				t.Fatal("input capture not explained")
			}
			if captured && (!strings.Contains(text, "Ctrl+C Exit Plous") || !strings.Contains(text, "Ctrl+] Review")) {
				t.Fatal("captured terminal should explain how to leave input")
			}
			if !captured && (!strings.Contains(text, "Exec review") || !strings.Contains(text, "End Latest")) {
				t.Fatalf("%dx%d review controls not explained: %s", m.width, m.height, ansi.Strip(m.footer()))
			}
		}
	}
}

func TestTerminalCursorMatchesOutputAndIsHiddenDuringReview(t *testing.T) {
	m := exploreModel(t)
	m.width, m.height = 120, 30
	width, height := m.terminalDimensions()
	tm := reviewTerminal(t, width, height)
	tm.CursorVisible.Store(true)
	m.explore.Terminal = tm
	_, _ = tm.Write([]byte("CURSOR"))
	view := m.View()
	if view.Cursor == nil {
		t.Fatal("live terminal cursor is missing")
	}
	for y, row := range strings.Split(ansi.Strip(m.render()), "\n") {
		if x := strings.Index(row, "CURSOR"); x >= 0 {
			column := lipgloss.Width(row[:x]) + len("CURSOR")
			if view.Cursor.X != column || view.Cursor.Y != y {
				t.Fatalf("cursor (%d,%d) does not match output (%d,%d)", view.Cursor.X, view.Cursor.Y, column, y)
			}
		}
	}
	tm.BeginReview()
	if m.View().Cursor != nil {
		t.Fatal("review mode retained a live input cursor")
	}
}
