package app

import (
	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
	"strings"
)

func (m *model) helmKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.helmVisible() || m.focus != 1 || m.helpVisible() || m.settingsOverlay != nil || m.commandFocused() || m.tooSmall() {
		return false, nil
	}
	h := m.helmer()
	key := msg.String()
	if h.Find != nil && h.Menu == nil && h.Input == nil && h.Document == nil && h.Comparison == nil && !h.Busy {
		if m.helmFindKey(h, msg) {
			return true, nil
		}
	}
	if h.Input != nil {
		if h.InputKind == "picker" {
			m.shortcutPrefix = ""
			switch key {
			case "esc":
				h.Input, h.Menu = nil, nil
			case "enter", "up", "down", "home", "end", "pgup", "pgdown":
				m.updateHelm(key)
			default:
				h.Input.Key(msg, false)
				h.Choice = 0
			}
			return true, nil
		}
		m.shortcutPrefix = ""
		if h.Namespaces != nil {
			m.updateHelmNamespaceInput(h, msg)
			return true, nil
		}
		switch key {
		case "esc":
			h.Input = nil
		case "enter":
			value := strings.TrimSpace(h.Input.Text)
			switch h.InputKind {
			case "filter":
				h.Preference.Filter = value
				m.rememberHelm(h)
			}
			h.Input = nil
			h.Cursor = 0
		default:
			h.Input.Key(msg, false)
		}
		return true, nil
	}
	if h.Draft != nil && h.Plan == nil && h.Document == nil && h.Comparison == nil && h.Menu == nil && h.WorkflowVisible() && !h.Busy && h.Draft.Form.Index < len(h.Draft.Form.Fields) {
		f := h.Draft.Form
		m.shortcutPrefix = ""
		switch key {
		case "esc":
			f.Index = len(f.Fields)
		case "up":
			f.Index = max(0, f.Index-1)
		case "down", "enter":
			if key == "enter" && f.Fields[f.Index].Required && strings.TrimSpace(f.Fields[f.Index].Value.Text) == "" {
				h.Err = f.Fields[f.Index].Label + " is required"
				return true, nil
			}
			f.Index = min(len(f.Fields), f.Index+1)
		default:
			field := &f.Fields[f.Index]
			if len(field.Options) > 0 {
				if key == "left" || key == "right" || key == "space" {
					if field.Value.Text == "No" {
						field.Value = textview.NewField("Yes")
					} else {
						field.Value = textview.NewField("No")
					}
				}
			} else {
				field.Value.Key(msg, false)
			}
		}
		return true, nil
	}
	if h.Document == nil && h.Comparison == nil {
		return false, nil
	}
	if h.Discard {
		switch key {
		case "left", "right":
			h.Choice = 1 - h.Choice
		case "esc":
			h.Discard = false
		case "enter":
			if h.Choice == 1 {
				h.Document = nil
				h.Comparison = nil
				h.Discard = false
			} else {
				h.Discard = false
			}
		}
		return true, nil
	}
	searching := h.Document != nil && h.Document.Searching || h.Comparison != nil && (h.Comparison.Reader.Searching || h.Comparison.ErrorActive && h.Comparison.ErrorReader.Searching)
	typing := h.Document != nil && !h.Document.ReadOnly && !h.Document.Controls && h.Comparison == nil
	insert := typing && h.Document.Mode == docedit.Insert
	if !searching {
		if key == "tab" && !typing {
			return false, nil
		}
		if key == ":" && !insert {
			if h.Document != nil {
				h.Document.ClearPrefix()
			}
			return false, nil
		}
		if !typing && m.updateShortcut(msg) {
			return true, nil
		}
	}
	m.shortcutPrefix = ""
	if h.Comparison != nil {
		decision, copy := h.Comparison.Key(key, msg.Text)
		if decision.Action == "back" || decision.Action == "close" {
			h.Comparison = nil
		}
		return true, m.copyWorkspaceText(copy)
	}
	d := h.Document
	if d.Controls {
		switch key {
		case "enter":
			d.Controls = false
		case "esc":
			m.closeHelmDocument(h)
		}
		return true, nil
	}
	wasSearching := d.Searching
	wasMode := d.Mode
	hadQuery := d.Query != ""
	prefix := d.Prefix()
	event := d.Key(key, msg.Text)
	if d.ReadOnly && key == "esc" && !wasSearching && wasMode == docedit.Normal && !hadQuery && prefix == "" {
		h.Document = nil
	}
	return true, m.copyWorkspaceText(event.Copy)
}

func (m *model) updateHelm(key string) {
	h := m.helmer()
	if h == nil {
		return
	}
	if key == "!" && h.Err != "" {
		m.openHelmDocument(h, "Helm error details", h.Err)
		return
	}
	if key == "n" && h.Running != "execute" && !h.Confirm && h.Menu == nil {
		m.executeHelm("namespace")
		return
	}
	if h.Busy {
		if key == "esc" && h.Cancel != nil {
			h.Cancel()
			h.Notice = "Cancelling Helm request."
			if h.Running == "execute" {
				h.Notice = "Cancelling Helm operation. Check the release before retrying."
			}
		}
		return
	}
	if h.Confirm {
		switch key {
		case "esc":
			h.Confirm = false
		case "left", "right":
			h.Choice = 1 - h.Choice
		case "up":
			h.ConfirmScroll = max(0, h.ConfirmScroll-1)
		case "down":
			h.ConfirmScroll++
		case "enter":
			if h.Choice == 1 {
				h.Pending = "execute"
			}
			h.Confirm = false
		}
		return
	}
	if h.Menu != nil {
		choices, _ := h.MenuChoices()
		switch key {
		case "esc":
			h.Menu = nil
			if h.InputKind == "picker" {
				h.Input = nil
			}
		case "up":
			h.Choice = max(0, h.Choice-1)
		case "down":
			h.Choice = min(max(0, len(choices)-1), h.Choice+1)
		case "home":
			h.Choice = 0
		case "end":
			h.Choice = max(0, len(choices)-1)
		case "pgup":
			h.Choice = max(0, h.Choice-10)
		case "pgdown":
			h.Choice = min(max(0, len(choices)-1), h.Choice+10)
		case "enter":
			m.activateHelmMenu(h)
		}
		return
	}
	if h.Client == nil {
		if key == "g" {
			h.Err = ""
			h.Pending = "connect"
		}
		return
	}
	if key == "a" {
		m.helmActions(h)
		return
	}
	if key == "g" {
		m.executeHelm("refresh")
		return
	}
	if key == "B" {
		m.executeHelm("rollback")
		return
	}
	if h.WorkflowVisible() && h.Draft != nil && h.Plan == nil {
		f := h.Draft.Form
		n := len(f.Fields)
		switch key {
		case "up":
			f.Index = max(0, f.Index-1)
		case "down":
			f.Index = min(n+2, f.Index+1)
		case "esc":
			m.leaveHelmWorkflow(h, false)
		case "enter":
			switch f.Index - n {
			case 0:
				for i, field := range f.Fields {
					if field.Required && strings.TrimSpace(field.Value.Text) == "" {
						f.Index = i
						h.Err = field.Label + " is required"
						return
					}
				}
				h.Pending = "prepare"
			case 1:
				h.Document = docedit.New(h.Draft.Values)
				h.Document.Name = "Draft values"
				h.Document.Target = h.Config.Context + " · Save destination: draft"
				h.Document.Syntax = "yaml"
			case 2:
				m.leaveHelmWorkflow(h, true)
			}
		}
		return
	}
	if h.Plan != nil && h.WorkflowVisible() {
		switch key {
		case "/":
			m.openHelmFind(h, "Preview summary", h.Plan.Summary())
		case "home":
			h.Subcursor = 0
		case "end":
			h.Subcursor = 1 << 30
		case "up":
			h.Subcursor = max(0, h.Subcursor-1)
		case "down":
			h.Subcursor++
		case "pgup":
			h.Subcursor = max(0, h.Subcursor-10)
		case "pgdown":
			h.Subcursor += 10
		case "enter", "v":
			m.executeHelm("review")
		case "esc":
			m.leaveHelmWorkflow(h, false)
		}
		return
	}
	if h.Timeline != nil && h.Detail == nil {
		m.helmTimelineKey(h, key)
		return
	}
	if h.Detail != nil {
		if key == "J" || key == "F" {
			kind := "release"
			if key == "F" {
				kind = "revision"
			}
			h.OpenInspectorPicker(kind)
			return
		}
		switch key {
		case "pgup", "pgdown", "home", "end":
			switch key {
			case "pgup":
				h.Subcursor = max(0, h.Subcursor-10)
			case "pgdown":
				h.Subcursor += 10
			case "home":
				h.Subcursor = 0
			case "end":
				h.Subcursor = 1 << 30
			}
			if h.Tab == 5 {
				h.Subcursor = min(h.Subcursor, max(0, h.RowsCount()-1))
			}

		case "/":
			h.BeginFind()
		case "esc":
			if m.returnToOverview() {
				return
			}
			h.Detail = nil
			h.Find = nil
			h.Notice = ""
			h.Subcursor = 0
		case "left", "[":
			h.MoveInspectorTab(-1)
		case "right", "]":
			h.MoveInspectorTab(1)
		case "up":
			h.Subcursor = max(0, h.Subcursor-1)
		case "down":
			if h.Tab == 5 {
				h.Subcursor = min(max(0, h.RowsCount()-1), h.Subcursor+1)
			} else {
				h.Subcursor++
			}
		case "enter":
			switch h.Tab {
			case 5:
				if h.RowsCount() == 0 {
					return
				}
				h.Menu = []string{"Recorded configuration"}
				h.MenuKind, h.Choice = "resource", 0
				m.activateHelmMenu(h)
			default:
				h.OpenInspectorDocuments()
			}
		}
		return
	}
	if h.Preference.Section == "Charts" && m.helmChartCardKey(h, key) {
		return
	}
	switch key {
	case "S":
		h.Menu, _, _ = h.CollectionData()
		h.MenuKind, h.Choice = "sort", h.SortColumn(h.Menu)
	case "up", "k":
		h.Cursor = max(0, h.Cursor-1)
	case "down", "j":
		h.Cursor = min(max(0, h.RowsCount()-1), h.Cursor+1)
	case "home":
		h.Cursor = 0
	case "end":
		h.Cursor = max(0, h.RowsCount()-1)
	case "pgup":
		h.Cursor = max(0, h.Cursor-10)
	case "pgdown":
		h.Cursor = min(max(0, h.RowsCount()-1), h.Cursor+10)
	case "backspace":
		if h.Preference.Section != "Releases" && h.Preference.Section != "Charts" {
			return
		}
		key := h.CollectionSelectionKey()
		h.Preference.Filter = ""
		h.RestoreCollectionSelection(key)
		m.rememberHelm(h)
	case "/":
		m.helmInput(h, "filter", h.Preference.Filter)
	case "esc":
		if !m.returnToOverview() {
			m.focusRootSidebar()
		}
	case "enter":
		switch h.Preference.Section {
		case "Charts", "Releases":
			rows := h.ReleaseRows()
			if len(rows) > 0 {
				h.Preference.Selected = rows[min(h.Cursor, len(rows)-1)].Key()
				m.rememberHelm(h)
				h.Pending = "detail"
				if h.Preference.Section == "Releases" {
					h.Pending = "timeline"
				}
			}
		}
	default:
		return
	}
	if h.Detail == nil && (h.Preference.Section == "Releases" || h.Preference.Section == "Charts") {
		rows := h.ReleaseRows()
		if h.Cursor >= 0 && h.Cursor < len(rows) {
			h.Preference.Selected = rows[h.Cursor].Key()
			m.rememberHelm(h)
		}
	}
}
