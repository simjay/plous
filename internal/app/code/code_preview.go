package code

func (s *CodeState) PreviewIdentity(id string) string {
	return s.Inventory.Source.Identity + "/" + s.Inputs[id].Identity()
}

func (s *CodeState) MarkPreviewAttempt(id string) {
	s.RecordPreviewAttempt(id, s.PreviewIdentity(id))
}

func (s *CodeState) RecordPreviewAttempt(id, identity string) {
	if s.PreviewAttempts == nil {
		s.PreviewAttempts = map[string]string{}
	}
	s.PreviewAttempts[id] = identity
}

func (s *CodeState) NeedsPreview() bool {
	if s.Inventory == nil {
		return false
	}
	id := s.Preference.Selected
	entry := s.Inventory.Entry(id)
	return entry != nil && entry.Renderable() && s.PreviewAttempts[id] != s.PreviewIdentity(id)
}

func (s *CodeState) PreviewFailure(id string) string {
	if s.Inventory == nil || s.PreviewAttempts[id] != s.PreviewIdentity(id) {
		return ""
	}
	return s.Failures[id]
}
