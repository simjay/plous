package activity

import helpview "github.com/simjay/plous/internal/ui/help"

type HelpContext struct{ Detail, Compare, OpenRelease, Observations, LoadError bool }

func (c HelpContext) Help() helpview.Section {
	s := helpview.New("Activity timeline", helpview.Key("open", "Enter", "Details"), helpview.Key("type", "←→", "Previous / next type"), helpview.Key("filter", "/", "Search"), helpview.Command("refresh", "g", "Reload local history"), helpview.Key("back", "Esc", "Clear search / sidebar"))
	if c.Detail {
		s = helpview.New("Activity detail", helpview.Key("back", "Esc", "Activity"))
		if c.Compare {
			s.Add("Record", helpview.Command("review", "d", "Compare snapshots"))
		}
		if c.OpenRelease {
			s.Add("Release", helpview.Key("open", "Enter", "Open recorded release"))
		}
		if c.Observations {
			s.Add("Evidence", helpview.Command("observations", "O", "Show / hide saved observations"))
		}
		s.Add("Read", helpview.Find()...)
	} else if c.LoadError {
		s.Add("Error", helpview.Command("describe", "d", "Read full load error"))
	}
	if c.Detail {
		s.Add("Navigation", helpview.Scroll()...)
	} else {
		s.Add("Navigation", helpview.List()...)
	}
	return s
}
