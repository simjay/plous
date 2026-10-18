package code

import "fmt"

type CodeDetailReturn struct {
	View, ReturnView, RenderRoot, NodeGraph, BrowserItem, Group string
	BrowserDetail                                               bool
	Section, Area, Cursor, Offset, FieldCursor, Stage           int
	Selected, Resource, Field, Filter                           string
	ValidationView, ValidationCategory, ValidationSource        string
	TraceReference                                              *CodeTraceReference
	TraceUnavailable                                            string
	ComparisonOffset                                            int
}

func (s *CodeState) ContextID() string {
	if s.View == "manifests" {
		return fmt.Sprintf("render/%d", s.Preference.Section)
	}
	return fmt.Sprintf("%s/%s/%t/%s/%s", s.ViewID(), s.Group, s.BrowserDetail, s.NodeGraph, s.Preference.BrowserItem)
}

func (s *CodeState) OpenDetail(section int) {
	if section == s.Preference.Section {
		return
	}
	s.PushLocation()
	s.ChooseSection(section)
}

func (s *CodeState) PushLocation() {
	s.RememberSelection()
	s.Details = append(s.Details, CodeDetailReturn{
		View: s.View, ReturnView: s.ReturnView, RenderRoot: s.RenderRoot, NodeGraph: s.NodeGraph, Group: s.Group,
		BrowserDetail: s.BrowserDetail, BrowserItem: s.Preference.BrowserItem,
		Section: s.Preference.Section, Area: s.InspectorArea, Cursor: s.Cursor,
		Offset: s.DetailOffset, FieldCursor: s.FieldCursor, Stage: s.Stage,
		Selected: s.Preference.Selected, Resource: s.Preference.Resource,
		Field: s.Preference.Field, Filter: s.Filter,
		ValidationView: s.ValidationView, ValidationCategory: s.ValidationCategory, ValidationSource: s.ValidationSource,
		TraceReference: s.TraceReference, TraceUnavailable: s.TraceUnavailable, ComparisonOffset: s.ComparisonOffset,
	})
}

func (s *CodeState) CloseDetail() bool {
	if len(s.Details) == 0 && (s.View != "manifests" || s.Preference.Section == 0) {
		return false
	}
	if len(s.Details) == 0 {
		s.ChooseSection(0)
		return true
	}
	previous := s.Details[len(s.Details)-1]
	s.Details = s.Details[:len(s.Details)-1]
	if previous.View != "" {
		s.View, s.ReturnView, s.RenderRoot = previous.View, previous.ReturnView, previous.RenderRoot
		s.NodeGraph, s.BrowserDetail, s.Group = previous.NodeGraph, previous.BrowserDetail, previous.Group
		if previous.View != "code-kustomize" || previous.Group != "" {
			s.Preference.BrowserItem = previous.BrowserItem
		}
	}
	s.Err, s.Notice = "", ""
	s.Preference.Selected = previous.Selected
	s.ChooseSection(previous.Section)
	s.Preference.Resource, s.Preference.Field = previous.Resource, previous.Field
	s.InspectorArea, s.DetailOffset = previous.Area, previous.Offset
	s.Cursor, s.Filter, s.Stage = previous.Cursor, previous.Filter, previous.Stage
	s.FieldResource, s.FieldCursor = "", previous.FieldCursor
	s.ValidationView, s.ValidationCategory, s.ValidationSource = previous.ValidationView, previous.ValidationCategory, previous.ValidationSource
	s.TraceReference, s.TraceUnavailable, s.ComparisonOffset = previous.TraceReference, previous.TraceUnavailable, previous.ComparisonOffset
	s.RestoreCursor()
	if s.View == "manifests" && previous.Section == 3 {
		s.Cursor = min(s.Cursor, max(0, len(s.Findings())-1))
	}
	return true
}

func (s *CodeState) DetailBackLabel() string {
	if s.Filter != "" {
		return "Clear Filter"
	}
	if len(s.Details) > 0 {
		previous := s.Details[len(s.Details)-1]
		if previous.View == "code-kustomize" {
			if previous.BrowserDetail {
				return "Overview"
			}
			if previous.Group == "" {
				return "Groups"
			}
			if previous.NodeGraph == "" {
				return "Group graph"
			}
			return "Graph"
		}
		return CodeSections[previous.Section]
	}
	if s.ReturnView != "" {
		return BrowserLabel(s.ReturnView)
	}
	return "Configuration Map"
}

func (s *CodeState) OpenConfigurationMap() {
	s.RememberSelection()
	s.Menu, s.Details = "", nil
	s.ChooseSection(0)
	s.InspectorArea, s.DetailOffset = 0, 0
}

func (s *CodeState) OpenFindings(all bool) {
	s.OpenDetail(3)
	s.ValidationView, s.ValidationCategory = "Findings", ""
	s.ValidationSource = s.Preference.Selected
	if all {
		s.ValidationSource = ""
	}
	s.Filter, s.Cursor, s.DetailOffset, s.InspectorArea = "", 0, 0, 0
}

// The source snapshot must match before offering an original file as evidence.
func (s *CodeState) ResourceOrigin() string {
	r, ok := s.CurrentResource()
	b := s.Builds[s.Preference.Selected]
	if !ok || b == nil || s.Inventory == nil || b.SourceIdentity != s.Inventory.Source.Identity || s.Stage != 0 {
		return ""
	}
	name := b.Origins[r.Key()]
	if !s.Inventory.Source.Has(name) {
		return ""
	}
	return name
}

func (s *CodeState) FieldSourceAvailable() bool {
	f, ok := s.CurrentField()
	if !ok || s.Inventory == nil {
		return false
	}
	name := f.File
	if len(f.Writes) > 0 {
		name = f.Writes[len(f.Writes)-1].File
	}
	return name != "" && s.Inventory.Source.Has(name)
}
