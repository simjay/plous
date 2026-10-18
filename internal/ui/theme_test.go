package ui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestHeadingsPreserveGraphemesAndTerminalWidth(t *testing.T) {
	for _, text := range []string{"", "X", "Workloads / Pods", "e\u0301", "👩🏽‍💻", "東京 / 👩🏽‍💻 / e\u0301", "Control\x1b[2J\ntext"} {
		got := Heading(text)
		want := OneLine(text)
		if ansi.Strip(got) != want || ansi.StringWidth(got) != ansi.StringWidth(want) {
			t.Fatalf("heading changed content or width: %q", got)
		}
		for _, cluster := range []string{"e\u0301", "👩🏽‍💻"} {
			if strings.Contains(text, cluster) && !strings.Contains(got, cluster) {
				t.Fatalf("heading split the grapheme %q", cluster)
			}
		}
		if strings.Contains(got, "\x1b[2J") {
			t.Fatal("heading retained terminal controls from the title")
		}
	}
}
