package diff

import (
	"fmt"
	"strings"
	"testing"

	textview "github.com/simjay/plous/internal/ui/text"
)

func BenchmarkDiff(b *testing.B) {
	var source strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&source, "field-%04d: original\n", i)
	}
	before := source.String()
	after := strings.ReplaceAll(before, "field-1000: original", "field-1000: changed")
	for _, operation := range []string{"open", "redraw", "scroll", "search"} {
		b.Run(operation, func(b *testing.B) {
			m := New(before, after, "fixture", 1, false)
			theme := textview.DefaultTheme()
			m.View(100, 30, theme)
			b.ReportAllocs()
			for b.Loop() {
				switch operation {
				case "open":
					New(before, after, "fixture", 1, false).View(100, 30, theme)
				case "redraw":
					m.View(100, 30, theme)
				case "scroll":
					m.Key("pgdown", "")
					m.View(100, 30, theme)
					if m.Reader.Cursor.Row == len(m.Reader.Lines)-1 {
						m.Key("home", "")
					}
				case "search":
					m.Key("/", "")
					m.Reader.Paste("field-1000")
					m.Key("enter", "")
				}
			}
		})
	}
}
