package logs

import helpview "github.com/simjay/plous/internal/ui/help"

func (m *Model) Help() helpview.Section {
	if m != nil && m.Reader.Searching {
		return helpview.Search("Logs · Find", "Finish search")
	}
	s := helpview.New("Pod / workload logs", helpview.Key("back", "Esc", "Clear search / selection, then back"))
	if m == nil || !m.Reader.Selecting {
		s.Add("Live", helpview.Key("follow", "Space", "Pause / jump live"))
	}
	s.Add("Read", helpview.Find()...)
	s.Add("Text", helpview.Key("select", "v", "Select / finish selection"), helpview.Key("copy", "y", "Copy selection / line"), helpview.Key("horizontal", "←→", "Move text cursor"))
	s.Add("Navigation", helpview.Scroll()...)
	s.Note("New logs keep collecting while reading.")
	return s
}
