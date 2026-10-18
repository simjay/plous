package app

import (
	"fmt"
	"github.com/simjay/plous/internal/app/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
)

func (m *model) helmTimelineKey(h *monitor.HelmState, key string) {
	t := h.Timeline
	prior := t.Selected()
	switch key {
	case "esc":
		h.StopTimelineLoad()
		h.Timeline = nil
		h.Find = nil
		h.Notice = ""
	case "up", "k":
		t.Cursor = max(0, t.Cursor-1)
		t.Scroll = 0
	case "down", "j":
		t.Cursor = min(max(0, len(t.Revisions)-1), t.Cursor+1)
		t.Scroll = 0
	case "home":
		t.Cursor = 0
		t.Scroll = 0
	case "end":
		t.Cursor = max(0, len(t.Revisions)-1)
		t.Scroll = 0
	case "pgup":
		t.Scroll = max(0, t.Scroll-5)
	case "pgdown":
		t.Scroll += 5
	case "enter":
		if t.Selected() > 0 {
			h.StopTimelineLoad()
			r := t.Release
			r.Revision = t.Selected()
			h.RequestedRelease = &r
			h.JobRevision = r.Revision
			h.Pending = "detail"
		}
	case "v":
		m.reviewHelmRevision(h)
	case "/":
		h.BeginFind()
	}
	if h.Timeline == t && t.Selected() != prior {
		h.StopTimelineLoad()
		h.Notice = ""
	}
}

func (m *model) reviewHelmRevision(h *monitor.HelmState) {
	t := h.Timeline
	if t == nil {
		return
	}
	entry, ok := t.Entries[t.Selected()]
	if !ok {
		h.Notice = "Loading recorded changes for this revision."
		return
	}
	if entry.Err != "" {
		m.openHelmDocument(h, "Revision changes unavailable", entry.Err)
		return
	}
	c := entry.Changes
	if c == nil {
		return
	}
	if len(c.Changes) == 0 && c.Diff == "" && c.Unavailable == "" {
		h.Notice = "No recorded configuration changes for this revision."
		return
	}
	if c.Unavailable != "" {
		m.openHelmDocument(h, fmt.Sprintf("Revision %d change summary", t.Selected()), monitor.HelmRevisionChangeDetails(c))
		h.Document.Target = h.Config.Context + " / " + t.Release.Key()
		h.Document.Syntax = "text"
		return
	}
	h.Comparison = diffview.New(c.Before, c.After, h.Config.Context+" / "+t.Release.Key(), uint64(t.Selected()), false)
	h.Comparison.BeforeLabel = fmt.Sprintf("Recorded revision %d", t.Selected()-1)
	if t.Selected() == 1 {
		h.Comparison.BeforeLabel = "Before installation"
	}
	h.Comparison.AfterLabel = fmt.Sprintf("Recorded revision %d", t.Selected())
	h.Comparison.Notice = "Chart metadata, values, resources and hooks. Sensitive fields masked. Recorded changes do not prove successful application."
}
