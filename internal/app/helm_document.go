package app

import (
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	"github.com/simjay/plous/internal/ui"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
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
