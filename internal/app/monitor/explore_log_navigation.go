package monitor

func (e *ExploreState) Location() ExploreLocation {
	return ExploreLocation{
		YamlOrigin: e.YamlOrigin, Preference: e.Preference, Resource: e.Resource, Selected: e.Selected,
		Object: e.Detail.DeepCopy(), Tab: e.Tab, Scroll: e.Scroll,
		RelatedCursor: e.RelatedCursor, Container: e.Container,
	}
}

func (e *ExploreState) RestoreLocation(location ExploreLocation) {
	e.YamlOrigin = location.YamlOrigin
	e.Preference, e.Selected, e.Detail, e.Tab = location.Preference, location.Selected, location.Object, location.Tab
	e.Scroll, e.RelatedCursor, e.Container = location.Scroll, location.RelatedCursor, location.Container
	if location.Resource.Name != "" {
		e.Resource = location.Resource
	} else if resource, ok := ExactResource(e.Resources, e.Preference.Resource); ok {
		e.Resource = resource
	}
	e.DetailValid = false
}
