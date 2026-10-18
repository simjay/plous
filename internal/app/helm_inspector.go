package app

import (
	"github.com/simjay/plous/internal/app/monitor"
)

func (m *model) rollbackHelmSelection(h *monitor.HelmState) {
	if h.Busy {
		return
	}
	revision := 0
	if h.Detail != nil {
		revision = h.Detail.Version
	} else if h.Timeline != nil {
		revision = h.Timeline.Selected()
	}
	if revision == 0 {
		return
	}
	if revision == h.LatestRevision() {
		if h.Detail != nil {
			h.OpenInspectorPicker("revision")
		}
		h.Notice = "Choose an earlier revision to preview rollback."
		return
	}
	h.StopTimelineLoad()
	h.JobRevision = revision
	h.Pending = "rollback"
	h.Notice = ""
}
