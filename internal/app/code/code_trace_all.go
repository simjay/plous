package code

import (
	"github.com/simjay/plous/internal/source"

	manifest "github.com/simjay/plous/internal/integrations/manifest"
)

type CodeTraceReference struct {
	Result   *source.Result
	Resource manifest.Resource
	Field    string
}

func (s *CodeState) CaptureTraceReference() {
	if s.Preference.Section != 2 || s.Status(s.Preference.Selected) == "stale" || s.PreviewFailure(s.Preference.Selected) != "" {
		return
	}
	r, ok := s.CurrentResource()
	f, hasField := s.CurrentField()
	if ok && hasField {
		s.TraceReference = &CodeTraceReference{Result: s.Builds[s.Preference.Selected], Resource: r, Field: f.Pointer}
	}
}

func (s *CodeState) RestoreTraceReference() {
	ref := s.TraceReference
	if s.Preference.Section != 2 || ref == nil {
		return
	}
	s.TraceUnavailable = ""
	s.FieldResource = ""
	b := s.Builds[s.Preference.Selected]
	switch {
	case s.PreviewFailure(s.Preference.Selected) != "":
		s.TraceUnavailable = "This source failed to render. Field values are unavailable."
	case b == nil:
		s.TraceUnavailable = "This source has no captured render yet."
	case s.Status(s.Preference.Selected) == "stale" || ref.Result.SourceIdentity != s.Inventory.Source.Identity:
		s.TraceUnavailable = "The captured trace is stale. Render and select the field again."
	default:
		for _, change := range source.Compare(b, ref.Result) {
			if change.After.Key() != ref.Resource.Key() || change.Before.Name == "" {
				continue
			}
			pointer, safe := source.CorrespondingField(change.Before, ref.Resource, ref.Field)
			if !safe {
				s.TraceUnavailable = "The traced field is absent or cannot be matched safely in this source."
				return
			}
			visible := false
			for _, r := range s.Resources() {
				if r.Key() == change.Before.Key() {
					visible = true
					break
				}
			}
			if !visible {
				s.TraceUnavailable = "The matched resource is hidden by the current filter. Clear the filter to inspect it."
				return
			}
			s.Preference.Resource, s.Preference.Field = change.Before.Key(), pointer
			s.RestoreCursor()
			return
		}
		s.TraceUnavailable = "The traced resource has no unambiguous match in this source."
	}
}
