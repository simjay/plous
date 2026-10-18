package code

func (s *CodeState) SelectedUnsupported() bool {
	if s.Inventory == nil {
		return false
	}
	entry := s.Inventory.Entry(s.Preference.Selected)
	return entry != nil && !entry.Component && !entry.Renderable()
}
