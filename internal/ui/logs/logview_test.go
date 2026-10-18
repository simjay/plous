package logs

import (
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReviewSelectionAndEviction(t *testing.T) {
	m := New(100, 4)
	m.Append("one\ntwo\nthree\n")
	m.Key("home", "")
	m.Key("down", "")
	m.Key("v", "")
	m.Key("right", "")
	m.Append("four\n")
	if m.Reader.SelectedText() != "tw" {
		t.Fatal("append changed selection")
	}
	m.Append("five\nsix\nseven\n")
	if m.Reader.Selecting || m.Reader.Notice == "" {
		t.Fatal("eviction silently selected different text")
	}
	if m.Unseen != 4 || m.Dropped == 0 {
		t.Fatal("retention counters")
	}
	m.Follow()
	if m.Unseen != 0 || !m.Following {
		t.Fatal("follow")
	}
}
func TestSearchCopyWrapAndBounds(t *testing.T) {
	m := New(1024, 100)
	m.Append("界é needle\nneedle two\n")
	m.Key("/", "")
	m.Reader.Paste("needle")
	m.Key("enter", "")
	if m.Following || !strings.Contains(m.Reader.Status(), "1/2") {
		t.Fatal("search did not pause")
	}
	m.Key("n", "")
	_, copied := m.Key("y", "")
	if copied != "needle two" {
		t.Fatalf("copy:%q", copied)
	}
	for _, size := range [][2]int{{28, 6}, {48, 12}, {90, 30}} {
		view := m.View(Header{Title: "Logs", Target: "example", State: "Review", Actions: []textview.ActionGroup{{Actions: []textview.Action{{Key: "/", Label: "Search"}}}}}, size[0], size[1], textview.DefaultTheme())
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatalf("height overflow %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
	}
	m.Append(strings.Repeat("界", 1000))
	if len(m.Text) > 1024 || !utf8.ValidString(m.Text) {
		t.Fatal("oversized Unicode record")
	}
}

func TestProjectionKeepsRawRetentionAndDisplayedSearchCopy(t *testing.T) {
	m := New(30, 100)
	m.SetFormatter(func(line string) textview.Line {
		return textview.Line{Text: strings.TrimPrefix(line, "time ")}
	})
	m.Append("time one\ntime two\n")
	m.Key("/", "")
	m.Reader.Paste("two")
	m.Key("enter", "")
	_, copied := m.Key("y", "")
	if copied != "two" || m.Text != "time one\ntime two\n" {
		t.Fatalf("projection changed source or copy: source=%q copy=%q", m.Text, copied)
	}
	m.Append("time three\ntime four\n")
	if len(m.Text) > 30 || m.Text != "time two\ntime three\ntime four\n" || m.Dropped != 1 {
		t.Fatal("retention counted displayed text instead of source bytes")
	}
	if m.Reader.SelectedText() != "two" || m.Reader.Status() != "1/1 matches" {
		t.Fatal("projection lost the retained search result")
	}
}

func TestStreamMessageRemainsReadableAcrossRefresh(t *testing.T) {
	m := New(1024, 100)
	m.Append(strings.Repeat("output\n", 20))
	m.Message = "Stream ended with an error."
	m.Refresh()
	m.Key("end", "")
	for range 3 {
		m.Refresh()
		if m.Reader.SelectedText() != m.Message {
			t.Fatal("refresh moved the reader away from the stream error")
		}
	}
	if strings.Contains(m.Text, m.Message) {
		t.Fatal("stream status entered the retained source")
	}
	m.Reset()
	if m.Message != "" || len(m.Reader.Lines) != 0 {
		t.Fatal("reset retained the previous source's status")
	}
}

func TestPreparationInvalidatesOnlyWhenContentOrFormatChanges(t *testing.T) {
	m := New(1024, 100)
	formatted := 0
	m.SetFormatter(func(line string) textview.Line {
		formatted++
		return textview.Line{Text: line}
	})
	m.Append("one\ntwo")
	m.Key("/", "")
	m.Reader.Paste("two")
	m.Key("enter", "")
	count, cursor := formatted, m.Reader.Cursor
	for range 3 {
		m.Refresh()
		m.View(Header{}, 40, 12, textview.DefaultTheme())
	}
	if formatted != count || m.Reader.Cursor != cursor || m.Reader.Status() != "1/1 matches" {
		t.Fatal("unchanged redraw rebuilt content or moved search")
	}
	m.Append("\ntwo more")
	if formatted <= count || m.Reader.Status() != "1/2 matches" {
		t.Fatal("new output did not update displayed search results")
	}
	m.Message = "stream ended"
	m.Refresh()
	m.Key("end", "")
	if m.Reader.SelectedText() != m.Message {
		t.Fatal("new stream status remained cached")
	}
	m.SetFormatter(func(line string) textview.Line { return textview.Line{Text: strings.ToUpper(line)} })
	if m.Reader.Lines[0].Text != "ONE" || m.Text != "one\ntwo\ntwo more" {
		t.Fatal("formatter update failed or changed the source")
	}
	m.Reset()
	m.Append("next")
	if len(m.Reader.Lines) != 1 || m.Reader.Lines[0].Text != "NEXT" {
		t.Fatal("reset retained stale content or lost the formatter")
	}
}

func TestAppendFormatsOnlyChangedLinesAndKeepsSearch(t *testing.T) {
	m := New(4096, 4)
	formatted := 0
	m.SetFormatter(func(line string) textview.Line {
		formatted++
		return textview.Line{Text: strings.TrimPrefix(line, "time ")}
	})
	m.Append("time one\ntime needle\ntime par")
	m.Key("/", "")
	m.Reader.Paste("needle")
	m.Key("enter", "")
	formatted = 0
	m.Append("tial\ntime needle two\n")
	if formatted != 3 {
		t.Fatalf("formatted %d lines, want changed tail and two new rows", formatted)
	}
	if m.Reader.Status() != "1/2 matches" {
		t.Fatal(m.Reader.Status())
	}
	m.Append("time final\n")
	if m.Reader.Status() != "1/2 matches" || m.Reader.SelectedText() != "needle" {
		t.Fatal("eviction lost retained search")
	}
	m.SetFormatter(func(line string) textview.Line { return textview.Line{Text: line} })
	if !strings.HasPrefix(m.Reader.SelectedText(), "time ") {
		t.Fatal("formatter change reused old rows")
	}
}

func TestMouseScrollPausesViewAndFollowReturnsToTail(t *testing.T) {
	m := New(100000, 1000)
	m.Append(strings.Repeat("line\n", 80))
	m.Reader.View(40, 10, textview.DefaultTheme())
	m.Pause()
	m.Reader.Scroll(-20)
	top := m.Reader.Top
	m.Append("new line\n")
	m.Reader.View(40, 10, textview.DefaultTheme())
	if m.Reader.Top != top || m.Unseen == 0 || m.Following {
		t.Fatal("scroll did not preserve reading while collecting")
	}
	m.Follow()
	m.Reader.View(40, 10, textview.DefaultTheme())
	if m.Reader.Top <= top || !m.Following || m.Unseen != 0 {
		t.Fatal("Follow did not return to latest")
	}
}
