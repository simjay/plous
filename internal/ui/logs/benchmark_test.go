package logs

import (
	"fmt"
	"strings"
	"testing"

	textview "github.com/simjay/plous/internal/ui/text"
)

func BenchmarkLogs(b *testing.B) {
	var source strings.Builder
	for i := range 4000 {
		fmt.Fprintf(&source, "record %04d needle ready\n", i)
	}
	for _, operation := range []string{"prepare", "redraw", "scroll", "search", "append"} {
		b.Run(operation, func(b *testing.B) {
			m := New(128*1024, 5000)
			m.Append(source.String())
			m.Key("/", "")
			m.Reader.Paste("needle")
			m.Key("enter", "")
			theme := textview.DefaultTheme()
			header := Header{Title: "Logs", Target: "fixture", State: "Review"}
			m.View(header, 100, 30, theme)
			b.ReportAllocs()
			for b.Loop() {
				switch operation {
				case "prepare":
					m.Refresh()
				case "redraw":
					m.Refresh()
					m.View(header, 100, 30, theme)
				case "scroll":
					m.Key("pgdown", "")
					m.View(header, 100, 30, theme)
					if m.Reader.Cursor.Row == len(m.Reader.Lines)-1 {
						m.Key("home", "")
					}
				case "search":
					m.Key("/", "")
					m.Reader.Paste("record 0399")
					m.Key("enter", "")
				case "append":
					m.Append("new needle output\n")
				}
			}
		})
	}
}
