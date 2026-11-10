package tui

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	manifestutil "github.com/simjay/plous/internal/integrations/manifest"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) openHelmDocument(h *monitor.HelmState, name, text string) {
	h.Document = docedit.New(ui.Plain(text))
	h.Document.ReadOnly = true
	h.Document.Name = name
	h.Document.Target = h.Config.Context
	h.Document.Syntax = "yaml"
}

func (m *model) openHelmFind(h *monitor.HelmState, name, text string) {
	m.openHelmDocument(h, name, text)
	h.Document.Key("/", "/")
}

func (m *model) helmPaste(text string) bool {
	if !m.helmVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false
	}
	h := m.helmer()
	if h.Find != nil && h.Find.Reader.Searching && h.Input == nil && h.Document == nil && h.Comparison == nil {
		h.Find.Reader.Paste(text)
		h.PositionFind()
		return true
	}
	if h.Input != nil {
		if h.InputKind == "picker" {
			h.Choice = 0
		}
		h.Input.Insert(text, false)
		return true
	}
	if h.Document != nil {
		if !h.Discard {
			h.Document.Paste(text)
		}
		return true
	}
	if h.Comparison != nil {
		h.Comparison.Reader.Paste(text)
		return true
	}
	if h.Draft != nil && h.Plan == nil && h.WorkflowVisible() && !h.Busy {
		h.Draft.Form.Paste(text)
		return true
	}
	return false
}

func (m *model) closeHelmDocument(h *monitor.HelmState) {
	if h.Document != nil && !h.Document.ReadOnly && h.Document.Dirty() {
		h.Discard = true
		h.Choice = 0
		return
	}
	h.Document = nil
	h.Comparison = nil
}

func (m *model) helmDocumentCommand(command string) bool {
	if !m.helmVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil {
		return false
	}
	h := m.helmer()
	d := h.Document
	if d == nil {
		return false
	}
	switch command {
	case "q", "q!":
		if command == "q!" {
			h.Document = nil
			h.Comparison = nil
		} else {
			m.closeHelmDocument(h)
		}
	case "w", "wq":
		if d.ReadOnly {
			return false
		}
		if h.Comparison != nil && !h.Comparison.Matches(h.Draft.Values, d.Text(), h.Config.Context+" / draft values", d.Revision) {
			h.Comparison.Stale = true
			h.Comparison.Error = "Draft changed after review. Return to the editor and review or save again."
			return true
		}
		if err := helmtool.ValidateValues(d.Text()); err != nil {
			d.Diagnostic = err.Error()
			return true
		}
		h.Draft.Values = d.Text()
		d.MarkSaved(d.Text())
		d.SavedStatus = "Saved to draft"
		d.Diagnostic = ""
		h.Comparison = nil
		if command == "wq" {
			h.Document = nil
		}
	case "v", "review":
		if d.ReadOnly {
			return false
		}
		if err := helmtool.ValidateValues(d.Text()); err != nil {
			d.Diagnostic = err.Error()
			return true
		}
		h.Comparison = diffview.New(h.Draft.Values, d.Text(), h.Config.Context+" / draft values", d.Revision, false)
		h.Comparison.Notice = "Draft values only. :w saves to the draft."
	case "u", "undo":
		if d.ReadOnly {
			return false
		}
		d.Undo()
		h.Comparison = nil
	case "U", "redo":
		if d.ReadOnly {
			return false
		}
		d.Redo()
		h.Comparison = nil
	default:
		return false
	}
	return true
}

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

func (m *model) valueDocumentActive() bool {
	return m.explore != nil && m.exploreVisible() && m.explore.Action != nil && m.explore.Action.Stage == "valueview" && m.explore.Action.ValueDocument != nil
}

func (m *model) valueDocumentKey(msg tea.KeyPressMsg) tea.Cmd {
	a := m.explore.Action
	d := a.ValueDocument
	key := msg.String()
	if !d.Searching && d.Mode != docedit.Select {
		if key == "a" {
			d.Controls = !d.Controls
			return nil
		}
		if d.Controls {
			switch key {
			case "left", "right":
				count, step := len(monitor.ValueControls(a.Resource)), 1
				if key == "left" {
					step = count - 1
				}
				a.Cursor = (a.Cursor + step) % count
				return nil
			case "enter":
				switch a.Cursor {
				case 0:
					a.ClearValueDocument()
					a.Stage, a.Cursor = "values", 0
				case 1:
					text := d.Text()
					a.ClearValueDocument()
					a.Stage, a.Note = "done", "Selected value copy requested."
					return m.copyWorkspaceText(text)
				case 2:
					a.ExportValue, a.ExportText = true, d.Text()
					a.Stage = "export"
					a.Input = textview.NewField(a.Object.GetName() + "-" + a.Container + ".pem")
				}
				return nil
			}
		}
	}
	if key != "esc" {
		d.Controls = false
	}
	if !d.Searching {
		switch key {
		case "home":
			d.ClearPrefix()
			d.Key("g", "g")
			key = "g"
		case "end":
			key = "G"
		}
	}
	event := d.Key(key, msg.Text)
	if event.Action == "back" {
		a.ClearValueDocument()
		a.Stage, a.Cursor = "values", 0
	}
	return m.copyWorkspaceText(event.Copy)
}

func (m *model) valueDocumentBody(width, height int) string {
	a := m.explore.Action
	d := a.ValueDocument
	buttons := []textview.Action{}
	for i, label := range monitor.ValueControls(a.Resource) {
		button := textview.Action{Mouse: "valueaction/" + strconv.Itoa(i), Label: label, Focused: d.Controls && a.Cursor == i}
		if button.Focused {
			button.Key = "Enter"
		}
		buttons = append(buttons, button)
	}
	label := "Actions"
	if d.Controls {
		label = "Read"
	}
	d.Actions = []textview.ActionGroup{
		{Label: "Value", Actions: buttons},
		{Label: "Read", Actions: []textview.Action{{Key: "a", Label: label}, {Key: "/", Label: "Find"}, {Key: "v", Label: "Select"}, {Key: "y", Label: "Copy"}}},
		m.documentDisplayActions(d),
	}
	return d.View(width, height, ui.WorkspaceTheme())
}
