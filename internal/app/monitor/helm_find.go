package monitor

import (
	"fmt"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

type HelmFind struct {
	Reader  textview.Viewer
	Targets []int
}

func (h *HelmState) BeginFind() {
	h.Find = &HelmFind{Reader: textview.Viewer{Wrap: true, Searching: true}}
	h.RefreshFind()
	if h.Detail != nil && h.Tab != 5 {
		h.Find.Reader.Top = h.Subcursor
		h.Find.Reader.Cursor.Row = min(h.Subcursor, max(0, len(h.Find.Reader.Lines)-1))
	}
}

func (h *HelmState) RefreshFind() {
	f := h.Find
	var lines []textview.Line
	previousLine, previousTarget := "", -1
	if len(f.Targets) > f.Reader.Cursor.Row {
		previousLine, previousTarget = h.FindLine(), f.Targets[f.Reader.Cursor.Row]
	}
	f.Targets = nil
	add := func(text string, target int, yaml bool) {
		for _, line := range strings.Split(ui.Plain(text), "\n") {
			item := textview.Line{Text: line}
			if yaml {
				item.Styled = docedit.YAMLLine(line, ui.WorkspaceTheme())
			}
			lines = append(lines, item)
			f.Targets = append(f.Targets, target)
		}
	}
	switch {
	case h.Detail != nil && h.Tab == 5:
		resources, _ := h.InspectorResources()
		for i, r := range resources {
			add(r.Group+" / "+r.Key()+"\nTemplate: "+r.Source, i, false)
		}
	case h.Detail != nil:
		add(HelmDetailText(h), 0, true)
	case h.Timeline != nil:
		for i, r := range h.Timeline.Revisions {
			add(fmt.Sprintf("Revision %d · %s · %s\n%s\n%s", r.Revision, r.Status, r.Updated, r.Chart, r.Description), i, false)
			entry := h.Timeline.Entries[r.Revision]
			if entry.Changes != nil {
				add(HelmRevisionChangeDetails(entry.Changes), i, false)
				add(entry.Changes.Diff, i, false)
			}
			if entry.Err != "" {
				add(entry.Err, i, false)
			}
		}
	}
	for i, line := range lines {
		if line.Text == previousLine && f.Targets[i] == previousTarget {
			f.Reader.Cursor.Row = i
			break
		}
	}
	f.Reader.SetLines(lines)
}

func (h *HelmState) FindTarget() (int, bool) {
	f := h.Find
	if f == nil || f.Reader.Query == "" || len(f.Targets) == 0 || !ui.SearchContains(h.FindLine(), f.Reader.Query) {
		return 0, false
	}
	return f.Targets[min(f.Reader.Cursor.Row, len(f.Targets)-1)], true
}

func (h *HelmState) PositionFind() {
	target, matched := h.FindTarget()
	if !matched {
		return
	}

	if h.Detail != nil && h.Tab == 5 {
		h.Subcursor = target
	}
	if h.Detail == nil && h.Timeline != nil && h.Timeline.Cursor != target {
		h.StopTimelineLoad()
		h.Timeline.Cursor, h.Timeline.Scroll = target, 0
	}
}

func (h *HelmState) FindLine() string {
	f := h.Find
	if f == nil || len(f.Reader.Lines) == 0 {
		return ""
	}
	return f.Reader.Lines[min(f.Reader.Cursor.Row, len(f.Reader.Lines)-1)].Text
}

func (h *HelmState) FindBar(width int) string {
	f := h.Find
	label := "Find"
	if h.Detail == nil {
		label = "Find loaded history"
	}
	label += " · " + f.Reader.Status()
	hint := "Enter finish · ↑↓ matches"
	if !f.Reader.Searching {
		hint = "n/N matches · Esc clear"
	}
	bar := ui.SearchField(textview.QueryField(&f.Reader), width, f.Reader.Searching, label, "Type to search…", hint)
	return bar
}
