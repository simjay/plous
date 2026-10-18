package app

import (
	"fmt"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"

	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) beginHelmDraft(h *monitor.HelmState) {
	if h.Workflow != nil {
		m.resumeHelmWorkflow(h)
		h.Notice = "Resumed the unfinished Helm action. Discard it before starting another."
		return
	}
	values := "{}\n"
	name, ns := "", h.Preference.Namespace
	if ns == "*" || ns == "" {
		ns = h.Client.Config.Namespace
	}
	a := h.Artifact
	ref, repo, version := "", "", ""
	if a != nil {
		ref, repo, version = a.Reference, a.Repository, a.Version
		name = fmt.Sprint(a.Metadata["name"])
	}
	d := &monitor.HelmDraft{Artifact: a, Values: values}
	if h.Detail != nil {
		d.Baseline = h.Detail
		name, ns = h.Detail.Name, h.Detail.Namespace
		d.Values = manifestutil.YAML(h.Detail.Config, false)
		d.Artifact = nil
		ref, repo, version = "", "", fmt.Sprint(h.Detail.Chart.Metadata["version"])
	}
	d.Form = &appsettings.Form{Kind: "helmchart", Title: "Chart inputs", Fields: []appsettings.Field{{Key: "chart", Label: "Chart reference or local path", Required: true, Value: textview.NewField(ref)}, {Key: "repository", Label: "Repository URL (Optional)", Value: textview.NewField(repo)}, {Key: "version", Label: "Version (required for remote charts)", Value: textview.NewField(version)}, {Key: "release", Label: "Release name", Required: true, Value: textview.NewField(name)}, {Key: "namespace", Label: "Namespace", Required: true, Value: textview.NewField(ns)}}}
	h.BeginWorkflow()
	h.Draft = d
	h.Plan = nil
	h.Menu = nil
	m.rememberHelm(h)
}

func (m *model) restoreHelmWorkflowOrigin(h *monitor.HelmState) {
	w := h.Workflow
	if h.Preference.Namespace != w.Preference.Namespace {
		h.Releases, h.Positions = nil, nil
		h.Loaded = false
	}
	h.Preference, h.Detail, h.Artifact = w.Preference, w.Detail, w.Artifact
	h.Timeline = w.Timeline
	h.Cursor, h.Tab, h.Subcursor = w.Cursor, w.Tab, w.Subcursor
	h.ValuesVariant, h.ChartDocument, h.InspectorPositions = w.ValuesVariant, w.ChartDocument, w.InspectorPositions
	h.Menu, h.Input, h.Comparison = nil, nil, nil
	h.Confirm = false
	h.StopNamespacePicker()
	h.Err, h.Notice = "", ""
	m.rememberHelm(h)
}

func (m *model) resumeHelmWorkflow(h *monitor.HelmState) {
	if h.Workflow == nil {
		return
	}
	m.restoreHelmWorkflowOrigin(h)
	h.Workflow.Active = true
	h.StopTimelineLoad()
	h.Detail, h.Artifact = nil, nil
	h.Timeline = nil
	h.Find = nil
	h.Subcursor = 0
}

func (m *model) leaveHelmWorkflow(h *monitor.HelmState, discard bool) {
	if h.Workflow == nil {
		return
	}
	m.restoreHelmWorkflowOrigin(h)
	h.Workflow.Active = false
	if discard {
		h.Workflow, h.Draft, h.Plan = nil, nil, nil
	} else {
		h.Notice = "Unfinished Helm action kept. Use Actions to resume or discard it."
	}
}
