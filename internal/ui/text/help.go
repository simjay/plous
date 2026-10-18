package text

import helpview "github.com/simjay/plous/internal/ui/help"

func (m *Viewer) Help(title, back string) helpview.Section {
	if m != nil && m.Searching {
		return helpview.Search(title+" · Find", "Finish search")
	}
	if m != nil {
		back = m.EscapeLabel(back)
	}
	s := helpview.New(title, helpview.Key("back", "Esc", back))
	s.Add("Read", helpview.Find()...)
	s.Add("Text", helpview.Key("select", "v", "Select / finish selection"), helpview.Key("copy", "y", "Copy selection / line"), helpview.Key("horizontal", "←→", "Move text cursor"))
	s.Add("Navigation", helpview.Scroll()...)
	return s
}
