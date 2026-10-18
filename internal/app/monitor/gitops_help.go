package monitor

import (
	helpview "github.com/simjay/plous/internal/ui/help"
	"strings"
)

func (s *GitOpsState) Help() helpview.Section {
	if s.Input != nil {
		return helpview.Search(s.Title()+" · Filter", "Open match")
	}
	if s.Find != nil && s.Find.Searching {
		return helpview.Search(s.Title()+" · Find", "Finish find")
	}
	title := s.Title() + " · " + string(s.Section)
	if s.Detail != nil {
		title = s.Title() + " · " + s.Detail.Ref.Kind
	}
	h := helpview.New(title)
	if s.Detail == nil {
		h.Add("Browse", helpview.Key("open", "Enter", "Open details"), helpview.Key("filter", "/", "Filter list"), helpview.Key("clear-filter", "Backspace", "Clear text filter"), helpview.Command("namespace", "n", "Controller namespace"))
	} else {
		h.Add("Links", helpview.Key("open", "Enter", "Open selected link / toggle configuration"))
		seen := map[string]bool{}
		for _, action := range s.Actions() {
			var label string
			switch {
			case action.Kind == "configuration":
				label = "Configuration & revisions"
			case action.Kind == "resource":
				label = "Kubernetes resource"
			case strings.HasPrefix(action.Label, "Dependency:"):
				label = "Dependency"
			case strings.HasPrefix(action.Label, "Source:"):
				label = "Source object"
			default:
				label = "Related object"
			}
			if seen[label+action.Disabled] {
				continue
			}
			seen[label+action.Disabled] = true
			e := helpview.Key("link-"+label, "Enter", label)
			e.Reason = action.Disabled
			h.Add("", e)
		}
		h.Add("Read", helpview.Find()...)
	}
	h.Add("Navigation", helpview.List()...)
	h.Add("View", helpview.Command("refresh", "g", "Refresh"), helpview.Key("back", "Esc", "Clear search / back"))
	return h
}
