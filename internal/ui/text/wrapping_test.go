package text

import "testing"

func TestCachedWrappingTracksTextWidthPrefixAndIndent(t *testing.T) {
	v := Viewer{Wrap: true}
	v.SetLines([]Line{{Text: "界 a very long line with repeated words and tabs\tend"}, {Text: "last"}})
	for _, width := range []int{18, 40, 10, 18} {
		for _, prefix := range []string{"", "source: "} {
			for _, indent := range []int{0, 5, 20} {
				v.Lines[0].Prefix = prefix
				v.Lines[0].ContinuationIndent = indent
				v.Cursor = Point{Row: 1}
				fresh := Viewer{Wrap: true, Cursor: v.Cursor}
				fresh.SetLines(v.Lines)
				want := fresh.View(width, 5, DefaultTheme())
				got := v.View(width, 5, DefaultTheme())
				if got != want {
					t.Fatalf("cached wrap differs at width %d prefix %q indent %d", width, prefix, indent)
				}
			}
		}
	}
	v.Lines[0].Text = "short"
	fresh := Viewer{Wrap: true, Lines: v.Lines, Cursor: v.Cursor}
	if v.View(18, 5, DefaultTheme()) != fresh.View(18, 5, DefaultTheme()) {
		t.Fatal("in-place text change reused geometry")
	}
}

func TestSearchCacheInvalidatesMutatedInputSlice(t *testing.T) {
	rows := []Line{{Text: "needle"}, {Text: "another"}}
	v := Viewer{}
	v.SetLines(rows)
	v.Key("/", "")
	v.Paste("needle")
	v.Key("enter", "")
	rows[0].Text = "changed"
	rows[1].Text = "needle"
	v.SetLines(rows)
	v.Key("n", "")
	if v.Cursor.Row != 1 || v.Status() != "1/1 matches" {
		t.Fatalf("stale search after a reused input slice: %+v %s", v.Cursor, v.Status())
	}
}
