package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestGradientHeadingsPreserveGraphemesAndTerminalWidth(t *testing.T) {
	for _, text := range []string{"", "X", "Workloads / Pods", "e\u0301", "👩🏽‍💻", "東京 / 👩🏽‍💻 / e\u0301", "Control\x1b[2J\ntext"} {
		got := gradientHeading(text)
		want := oneLine(text)
		if ansi.Strip(got) != want || ansi.StringWidth(got) != ansi.StringWidth(want) {
			t.Fatalf("gradient changed heading content or width: %q", got)
		}
		for _, cluster := range []string{"e\u0301", "👩🏽‍💻"} {
			if strings.Contains(text, cluster) && !strings.Contains(got, cluster) {
				t.Fatalf("gradient split the grapheme %q", cluster)
			}
		}
		if strings.Contains(got, "\x1b[2J") {
			t.Fatal("gradient retained terminal controls from the title")
		}
	}
}
