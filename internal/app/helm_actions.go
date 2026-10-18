package app

import (
	"github.com/simjay/plous/internal/app/monitor"

	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	diffview "github.com/simjay/plous/internal/ui/diff"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) helmInput(h *monitor.HelmState, kind, text string) {
	e := textview.NewField(text)
	h.Input = &e
	h.InputKind = kind
}

func (m *model) confirmHelm(h *monitor.HelmState) {
	if h.Busy || h.Plan == nil || !h.WorkflowVisible() {
		return
	}
	if h.Plan.Operation != "rollback" {
		h.Notice = "Chart changes are preview only."
		return
	}
	h.Confirm = true
	h.Choice = 0
	h.ConfirmScroll = 0
}

func (m *model) helmActions(h *monitor.HelmState) {
	if h.Busy || h.Document != nil || h.Comparison != nil {
		return
	}
	h.Choice = 0
	h.MenuKind = "actions"
	h.Menu = h.ActionItems()
}

func (m *model) activateHelmMenu(h *monitor.HelmState) {
	if len(h.Menu) == 0 {
		return
	}
	_, indices := h.MenuChoices()
	if len(indices) == 0 {
		return
	}
	h.Choice = indices[min(max(0, h.Choice), len(indices)-1)]
	label := strings.TrimSuffix(h.Menu[h.Choice], "  ✓")
	kind := h.MenuKind
	h.Menu = nil
	if h.InputKind == "picker" {
		h.Input = nil
	}
	if kind == "values" || kind == "chart-document" {
		h.ChooseInspectorDocument(kind, h.Choice)
		return
	}
	if kind == "release" {
		rows := h.InspectorReleases()
		if h.Choice < len(rows) {
			r := rows[h.Choice]
			r.Revision = 0
			h.RequestedRelease = &r
			h.Pending = "detail"
		}
		return
	}
	if kind == "revision" {
		rows := h.InspectorRevisions()
		if h.Choice < len(rows) {
			h.JobRevision = rows[h.Choice].Revision
			h.Pending = "detail"
		}
		return
	}
	if kind == "sort" {
		key := h.CollectionSelectionKey()
		if h.Preference.Sort == label || h.Preference.Sort == "" && label == "Name" {
			h.Preference.Descending = !h.Preference.Descending
		} else {
			h.Preference.Sort, h.Preference.Descending = label, false
		}
		h.RestoreCollectionSelection(key)
		m.rememberHelm(h)
		return
	}
	if kind == "resource" || label == "Compare recorded and live" || label == "Reveal recorded configuration" || label == "Reveal recorded/live comparison" {
		switch label {
		case "Recorded configuration", "Reveal recorded configuration":
			resources, _ := h.InspectorResources()
			if h.Subcursor < len(resources) {
				r := resources[h.Subcursor]
				name := r.Key()
				if label == "Reveal recorded configuration" {
					name += " · Revealed"
				}
				m.openHelmDocument(h, name, "# Template: "+r.Source+"\n"+manifestutil.YAML(r.Object, label == "Recorded configuration"))
			}
		case "Compare recorded and live", "Reveal recorded/live comparison":
			h.LiveReveal = label == "Reveal recorded/live comparison"
			h.Pending = "live"
		}
		return
	}
	switch label {
	case "Open document":
		m.openHelmDocument(h, monitor.HelmDetailSections[h.Tab], monitor.HelmDetailText(h))
	case "Preview chart changes":
		m.beginHelmDraft(h)
	case "Preview rollback":
		m.executeHelm("rollback")
	case "Inspect revision chart":
		m.helmTimelineKey(h, "enter")
	case "Review recorded changes":
		m.reviewHelmRevision(h)
	case "Refresh history":
		m.executeHelm("refresh")
	case "Reveal this section":
		m.openHelmDocument(h, monitor.HelmDetailSections[h.Tab]+" · Revealed", monitor.HelmDetailContent(h, true))
	case "Refresh release":
		h.Pending = "detail"
	case "Review resource differences":
		m.executeHelm("review")
	case "Reveal resource differences":
		h.Comparison = diffview.New(h.Plan.Before, h.Plan.Manifest, h.Plan.Context+" / "+h.Plan.Namespace+" / "+h.Plan.Name, uint64(h.Plan.CurrentRevision), false)
		h.Comparison.Notice = "Revealed values. Esc returns to the preview."
	case "Review values":
		m.openHelmDocument(h, "Proposed values", manifestutil.YAMLString(h.Plan.Values, true))
	case "Inspect preview summary":
		m.openHelmDocument(h, "Preview summary", h.Plan.Summary())
	case "Edit inputs":
		if h.Draft != nil {
			h.Plan = nil
		} else {
			h.Notice = "Select a release revision to prepare another rollback."
		}
	case "Execute rollback":
		m.confirmHelm(h)
	case "Discard unfinished action":
		m.leaveHelmWorkflow(h, true)
	case "Resume unfinished action":
		m.resumeHelmWorkflow(h)
	case "Refresh releases":
		h.Pending = "list"
	case "Choose namespace":
		m.openHelmNamespacePicker(h)
	}
}
