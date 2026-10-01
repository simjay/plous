package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/simjay/plous/internal/textview"
)

func BenchmarkYAMLDocument(b *testing.B) {
	var source strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&source, "field-%04d: value\n", i)
	}
	text := source.String()
	for _, operation := range []string{"open", "redraw", "scroll", "search"} {
		b.Run(operation, func(b *testing.B) {
			m := New(text)
			m.Syntax, m.ReadOnly = "yaml", true
			theme := textview.DefaultTheme()
			m.View(100, 30, theme)
			b.ReportAllocs()
			for b.Loop() {
				switch operation {
				case "open":
					d := New(text)
					d.Syntax, d.ReadOnly = "yaml", true
					d.View(100, 30, theme)
				case "redraw":
					m.View(100, 30, theme)
				case "scroll":
					m.Key("pgdown", "")
					m.View(100, 30, theme)
					if m.Cursor == len(text) {
						m.Key("g", "g")
						m.Key("g", "g")
					}
				case "search":
					m.Key("/", "")
					m.Paste("field-1000")
					m.Key("enter", "")
				}
			}
		})
	}
}
