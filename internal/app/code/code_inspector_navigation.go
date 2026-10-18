package code

import (
	"github.com/simjay/plous/internal/source"

	"slices"
	"strings"
)

func (s *CodeState) NavigationEntries() []source.Entry {
	copy := *s
	copy.Filter = ""
	copy.RenderRoot = ""
	entries := copy.Entries()
	slices.SortStableFunc(entries, func(a, b source.Entry) int { return strings.Compare(a.Path, b.Path) })
	return entries
}

func (s *CodeState) SelectField(index int) bool {
	fields := s.CurrentFields()
	if index < 0 || index >= len(fields) {
		return false
	}
	s.FieldCursor = index
	s.Preference.Field = fields[index].Pointer
	s.RememberSelection()
	return true
}
