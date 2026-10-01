package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/logview"
)

func TestLogHeaderKeepsSourceReadingAndConnectionDistinct(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	e := m.explore
	e.detail.SetNamespace("payments")
	e.detail.SetName("checkout")
	e.container, e.logKey = "app", "loaded"
	e.log = &exploreLogRun{cancel: func() {}}
	e.logConnected, e.previous = true, true
	m.podLogViewer().Pause()
	m.podLogViewer().Unseen, m.podLogViewer().Dropped = 23, 12
	text := ansi.Strip(m.exploreLogBody(110, 30))
	for _, want := range []string{"payments / checkout / app", "Previous instance · Latest 200 lines", "Reading", "Loading", "23 new lines", "12 older lines removed", "Stream", "Source", "Display", "[ Space Jump live ]", "[ o Container ]", "[ u Time range ]", "[ p Previous instance On ]", "[ t Timestamps Off ]", "[ w Wrap lines On ]", "[ g Reload logs ]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in log controls: %s", want, text)
		}
	}
	e.log = nil
	text = ansi.Strip(m.exploreLogBody(110, 30))
	if !strings.Contains(text, "Ended") || !strings.Contains(text, "Previous") || !strings.Contains(text, "Reading") {
		t.Fatal("stream completion hid the source or reading position")
	}
}

func TestMergedLogSourceFormattingPreservesCopySearchAndTimestamps(t *testing.T) {
	prefix := "[frontend-podinfo-6db9bf595b-fgl9m/podinfo] "
	stamp := "2026-09-30T20:35:10.608Z "
	message := `{"msg":"Starting podinfo","literal":"[other/source]"}`
	raw := prefix + stamp + message
	v := logview.New(4096, 100)
	v.SetFormatter(podLogFormatter(false, true))
	v.Append(raw)
	row := v.Reader.Lines[0]
	if row.Text != prefix+message || ansi.Strip(row.Styled) != row.Text || row.ContinuationIndent != len(prefix) {
		t.Fatalf("source or message changed: %#v", row)
	}
	v.Key("/", "")
	v.Reader.Paste("fgl9m")
	v.Key("enter", "")
	_, copied := v.Key("y", "")
	if copied != prefix+message || v.Reader.Status() != "1/1 matches" || v.Text != raw {
		t.Fatal("source prefix is not searchable/copyable, or raw output changed")
	}
	v.SetFormatter(podLogFormatter(true, true))
	if v.Reader.SelectedText() != raw || ansi.Strip(v.Reader.Lines[0].Styled) != raw {
		t.Fatal("timestamp toggle lost source identity or timestamp")
	}
	if got := podLogFormatter(false, false)(raw); got.Text != raw || got.ContinuationIndent != 0 {
		t.Fatal("ordinary log text was mistaken for generated metadata")
	}
	first := podLogSourceStyle("frontend-podinfo-6db9bf595b-fgl9m").GetForeground()
	second := podLogSourceStyle("frontend-podinfo-6db9bf595b-znl86").GetForeground()
	if first == second {
		t.Fatal("sample Pods are not visually distinguishable")
	}
	for _, container := range []string{"app", "proxy"} {
		line := "[same-pod/" + container + "] hello"
		formatted := podLogFormatter(false, true)(line)
		if !strings.HasPrefix(formatted.Styled, podLogSourceStyle("same-pod").Render("[same-pod/"+container+"]")) {
			t.Fatal("container changed its Pod color")
		}
	}
}

func TestLogViewerFitsCompactAndWidePanelsWithoutLosingOutput(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	e := m.explore
	e.detailValid = true
	e.container, e.logKey = "app", "loaded"
	m.podLogViewer().Append(strings.Repeat("earlier line\n", 30) + "latest evidence")
	for _, size := range [][2]int{{28, 6}, {48, 12}, {84, 26}, {196, 40}} {
		text := m.exploreLogBody(size[0], size[1])
		lines := strings.Split(text, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%v log view used %d rows", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v log view overflowed: %q", size, ansi.Strip(line))
			}
		}
		if !strings.Contains(ansi.Strip(text), "latest evidence") {
			t.Fatalf("%v controls crowded all log output out of the view", size)
		}
	}
}

func TestLogHeaderDoesNotGrowWhenReadingOrReceivingMoreLines(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	m.explore.detailValid = true
	m.podLogViewer().Append(strings.Repeat("retained record\n", 100))
	outputRows := func(view string) string {
		rows := []string{}
		for i, row := range strings.Split(ansi.Strip(view), "\n") {
			if strings.Contains(row, "retained record") {
				rows = append(rows, fmt.Sprintf("%d:%s", i, row))
			}
		}
		return strings.Join(rows, "\n")
	}
	for _, width := range []int{28, 48, 84, 196} {
		m.podLogViewer().Follow()
		m.podLogViewer().Dropped = 0
		before := outputRows(m.exploreLogBody(width, 30))
		m.podLogViewer().Pause()
		m.podLogViewer().Unseen, m.podLogViewer().Dropped = 123456, 987654
		if after := outputRows(m.exploreLogBody(width, 30)); before == "" || before != after {
			t.Fatalf("new-line notice moved the retained output at width %d", width)
		}
	}
}

func TestStyledLogTimestampsPreserveOriginalText(t *testing.T) {
	text := "2026-09-30T10:00:00Z ready\nnot-a-time message\n2026-09-30T10:00:01.123Z error\n"
	if got := ansi.Strip(styledLogTimestamps(text)); got != text {
		t.Fatalf("timestamp styling changed evidence: %q", got)
	}
}
