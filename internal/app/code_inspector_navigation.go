package app

import (
	"fmt"
	appcode "github.com/simjay/plous/internal/app/code"
	"github.com/simjay/plous/internal/ui"

	"github.com/simjay/plous/internal/source"

	"slices"
	"strings"
	"unicode/utf8"
)

func (m *model) renderSelectedCode() {
	s := m.coder()
	if s.Inventory == nil {
		return
	}
	if s.Preference.Section == 0 && (s.Cursor < 0 || s.Cursor >= len(s.Entries())) {
		return
	}
	s.RememberSelection()
	entry := s.Inventory.Entry(s.Preference.Selected)
	if entry == nil || !entry.Renderable() {
		return
	}
	if s.Preference.Section == 0 {
		s.OpenDetail(1)
	}
	s.Pending = "render"
	m.rememberCode(s)
}

func (m *model) selectCodeSource(index int) {
	s := m.coder()
	entries := s.NavigationEntries()
	if index < 0 || index >= len(entries) {
		return
	}
	s.CaptureTraceReference()
	s.RememberSelection()
	s.Preference.Selected = entries[index].ID
	if s.RenderRoot != "" && !s.RenderChain()[s.Preference.Selected] {
		s.RenderRoot = s.Preference.Selected
	}
	s.Err = ""
	if s.Preference.Section == 0 && s.SourceIndex(s.Preference.Selected) < 0 {
		s.Filter = ""
	}
	s.Stage = 0
	s.DetailOffset = 0
	s.SyncListScope()
	s.RestoreCursor()
	s.RestoreTraceReference()
	m.rememberCode(s)
}

func (m *model) codeInspectorKey(key string) bool {
	s := m.coder()
	if s.View != "manifests" || s.Inventory == nil {
		return false
	}
	section := s.Preference.Section
	if key == "enter" && section == 1 {
		if entry := s.Inventory.Entry(s.Preference.Selected); entry == nil {
			m.openCodeGraph()
			return true
		} else if entry.Component {
			if !m.openCodeNodeOverview(entry.ID) {
				s.OpenDetail(0)
			}
			return true
		}
	}
	if key == "j" {
		key = "down"
	} else if key == "k" {
		key = "up"
	}
	if section == 0 && s.InspectorArea == 0 && (key == "up" || key == "down" || key == "home" || key == "end" || key == "pgup" || key == "pgdown") {
		order := s.GraphOrder()
		if len(order) > 0 {
			current := max(0, slices.Index(order, s.Preference.Selected))
			selected := order[ui.MoveCursor(current, len(order), key)]
			s.Cursor, s.DetailOffset = s.SourceIndex(selected), 0
			s.RememberSelection()
			m.rememberCode(s)
		}
		return true
	}
	if key == "left" || key == "right" {
		areas := []int{0, 1}
		if section == 1 || section == 2 {
			areas = []int{1, 2}
			if section == 1 && s.Builds[s.Preference.Selected] == nil {
				areas = []int{2}
			}
			if section == 2 {
				areas = []int{2, 3}
				if s.TraceReference != nil {
					areas = []int{0, 2, 3}
				}
			}
		}
		delta := 1
		if key == "left" {
			delta = -1
		}
		index := max(0, slices.Index(areas, s.InspectorArea))
		s.InspectorArea = areas[(index+len(areas)+delta)%len(areas)]
		return true
	}
	if s.SelectedUnsupported() && key == "enter" {
		return false
	}
	if section == 0 || section == 3 {
		if s.InspectorArea == 1 {
			switch key {
			case "home":
				s.DetailOffset = 0
				return true
			case "end":
				s.DetailOffset = 10000
				return true
			case "up", "k":
				s.DetailOffset = max(0, s.DetailOffset-1)
				return true
			case "down", "j":
				s.DetailOffset++
				return true
			case "pgup":
				s.DetailOffset = max(0, s.DetailOffset-10)
				return true
			case "pgdown":
				s.DetailOffset += 10
				return true
			}
		}
		return false
	}
	if key == "up" || key == "down" || key == "home" || key == "end" || key == "pgup" || key == "pgdown" {
		if s.InspectorArea == 2 {
			if _, ok := s.CurrentResource(); !ok {
				s.DetailOffset = ui.MoveCursor(s.DetailOffset, 10000, key)
				return true
			}
		}
		if s.InspectorArea == 0 {
			entries := s.NavigationEntries()
			index := slices.IndexFunc(entries, func(e source.Entry) bool { return e.ID == s.Preference.Selected })
			m.selectCodeSource(ui.MoveCursor(max(0, index), len(entries), key))
			return true
		}
		if s.InspectorArea == 2 && s.ComparisonActive() {
			s.ComparisonOffset = ui.MoveCursor(s.ComparisonOffset, s.ComparisonRowCount(), key)
			return true
		}
		if s.InspectorArea == 2 {
			fields := s.CurrentFields()
			s.DetailOffset = 0
			s.FieldCursor = ui.MoveCursor(s.FieldCursor, len(fields), key)
			if f, ok := s.CurrentField(); ok {
				s.Preference.Field = f.Pointer
			}
			s.RememberSelection()
			m.rememberCode(s)
			return true
		}
		if s.InspectorArea == 3 {
			s.DetailOffset = ui.MoveCursor(s.DetailOffset, 10000, key)
			return true
		}
	}
	if key == "enter" && s.InspectorArea == 0 {
		s.InspectorArea = 2
		return true
	}
	if key == "enter" && s.InspectorArea == 1 {
		s.InspectorArea = 2
		return true
	}
	if key == "enter" && section == 2 && s.InspectorArea == 3 {
		m.openCodeFieldSource()
		return true
	}
	if key == "enter" && s.InspectorArea == 2 {
		if section == 1 {
			if _, ok := s.CurrentResource(); !ok {
				return true
			}
			if s.ComparisonActive() {
				m.openCodeResource()
				return true
			}
			s.OpenDetail(2)
			s.InspectorArea = 3
			m.rememberCode(s)
		} else {
			m.openCodeFieldSource()
		}
		return true
	}
	return false
}

func (m *model) openCodeFieldSource() {
	s := m.coder()
	f, ok := s.CurrentField()
	if ok && len(f.Writes) > 0 {
		write := f.Writes[len(f.Writes)-1]
		f.File, f.Line = write.File, write.Line
		f.Evidence = "Observed writer configuration"
	}
	if !ok || f.File == "" || !s.Inventory.Source.Has(f.File) {
		m.status = "No captured source location is available for this field."
		return
	}
	preview := *s
	if result := s.Builds[s.Preference.Selected]; result != nil && result.CapturedSource() != nil {
		preview.Inventory = result.CapturedSource()
	}
	s.ShowDocument(f.File, appcode.CodeFilePreview(&preview, f.File))
	s.DocumentSource = preview.Inventory
	s.Document.Syntax = "yaml"
	s.DocumentKind, s.DocumentPath = "file", f.File
	s.Document.Target = "Captured source · " + f.Evidence
	if f.Line > 0 {
		data, _ := preview.Inventory.Source.ReadFile(f.File)
		line := source.PreviewLine(string(data), s.Document.Text(), f.Line)
		if line > 0 {
			lines := strings.Split(s.Document.Text(), "\n")
			for _, text := range lines[:min(line-1, len(lines))] {
				s.Document.Cursor += utf8.RuneCountInString(text) + 1
			}
			s.Document.Anchor = s.Document.Cursor
		}
		s.Document.Notice = fmt.Sprintf("Original source location: %s:%d", f.File, f.Line)
	}
}

func (m *model) codeFindingDestination(destination string) {
	s := m.coder()
	findings := s.Findings()
	if s.Cursor < 0 || s.Cursor >= len(findings) {
		return
	}
	f := findings[s.Cursor]
	if s.Inventory.Entry(f.Target) == nil {
		return
	}
	if destination == "sources" && m.openCodeNodeOverview(f.Target) {
		return
	}
	s.TraceReference, s.TraceUnavailable = nil, ""
	section := 0
	if destination == "render" {
		section = 1
	} else if destination == "trace" {
		section = 2
	}
	s.OpenDetail(section)
	s.Preference.Selected = f.Target
	s.Stage = 0
	s.SyncListScope()
	if section > 0 {
		s.Filter = ""
		s.Preference.Resource, s.Preference.Field = f.Resource, f.Field
		s.FieldResource = ""
		s.InspectorArea = 2
		if section == 2 {
			s.InspectorArea = 3
		}
	}
	s.RestoreCursor()
	m.rememberCode(s)
}
