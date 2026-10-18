package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/simjay/plous/internal/app/monitor"
	kubernetes "github.com/simjay/plous/internal/integrations/kubernetes/monitor"
	"github.com/simjay/plous/internal/ui"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func (m *model) yamlDocumentActive() bool {
	e := m.explore
	if e != nil && !e.DetailValid && e.DetailRun == nil && (e.DetailError != "" || e.DetailText != "") {
		return false
	}
	return m.exploreVisible() && e != nil && e.Detail != nil && e.Tab == 1 && e.Action == nil && e.Terminal == nil && e.Input == nil && e.Picker == ""
}

func (m *model) yamlDocumentSearching() bool {
	return m.yamlDocumentActive() && m.syncYAMLDocument().Document.Searching
}

func (m *model) syncYAMLDocument() *monitor.YamlDocument {
	e := m.explorer()
	if e.Detail == nil {
		return nil
	}
	context := ""
	if connection := m.currentConnection(); connection != nil {
		context = connection.Settings["context"]
	}
	identity := strings.Join([]string{m.active, m.connection, context, e.Resource.Key(), e.Detail.GetAPIVersion(), e.Detail.GetKind(), e.Detail.GetNamespace(), e.Detail.GetName(), string(e.Detail.GetUID())}, "\x00")
	d := e.YamlDocument
	// API and watch updates replace the captured object. Focus and resize reuse it.
	version := e.Detail.GetResourceVersion()
	if d != nil && d.Identity == identity && d.Source == e.Detail && d.SourceVersion == version && d.Revealed == e.Reveal {
		return d
	}
	text := kubernetes.YAML(*e.Detail, e.Reveal)
	if d == nil || d.Identity != identity {
		d = &monitor.YamlDocument{Identity: identity, Document: docedit.New(text)}
		d.Document.ReadOnly, d.Document.Syntax = true, "yaml"
		d.Document.Name = "YAML · " + e.Detail.GetName()
		e.YamlDocument = d
	}
	if d.Text != text || d.Revealed != e.Reveal {
		d.Document.RefreshReadOnly(text)
		if d.Text != "" {
			d.Document.Notice = "YAML refreshed; search and selection cleared."
			if d.Revealed != e.Reveal {
				d.Document.Notice = "Value visibility changed; search and selection cleared."
			}
		}
		d.Text, d.Revealed = text, e.Reveal
	}
	d.Source, d.SourceVersion = e.Detail, version
	d.Document.Target = context + " · " + e.Detail.GetNamespace()
	if e.Resource.Key() == "secrets" {
		if e.Reveal {
			d.Document.Target += " · Values revealed"
		} else {
			d.Document.Target += " · Values hidden"
		}
	}
	return d
}

func (m *model) yamlDocumentBody(width, height int) string {
	d := m.syncYAMLDocument()
	if d == nil {
		return ""
	}
	actions := []textview.Action{{Key: "/", Label: "Find"}, {Key: "z", Label: "Select"}, {Key: "y", Label: "Copy"}, {Key: "E", Label: "Edit"}}
	if m.explore.Resource.Key() == "secrets" {
		label := "Reveal values"
		if d.Revealed {
			label = "Hide values"
		}
		actions = append(actions, textview.Action{Key: "v", Label: label})
	}
	d.Document.Actions = []textview.ActionGroup{{Label: "Read", Actions: actions}, m.documentDisplayActions(d.Document)}
	if m.explore.Resource.Key() == "secrets" && height > 1 {
		visibility := "Values hidden"
		if d.Revealed {
			visibility = "Values revealed"
		}
		return ui.Muted.Render(visibility) + "\n" + d.Document.ViewEmbedded(width, height-1, ui.WorkspaceTheme())
	}
	return d.Document.ViewEmbedded(width, height, ui.WorkspaceTheme())
}

func (m *model) yamlDocumentKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.yamlDocumentActive() {
		return false, nil
	}
	d := m.syncYAMLDocument().Document
	key := msg.String()
	if !d.Searching {
		switch key {
		case "v":
			if m.explore.Resource.Key() == "secrets" {
				m.explore.Reveal = !m.explore.Reveal
				m.syncYAMLDocument()
				return true, nil
			}
		case "z":
			key = "v"
		case "home":
			d.ClearPrefix()
			d.Key("g", "g")
			key = "g"
		case "end":
			key = "G"
		case "up", "down", "left", "right", "j", "k", "pgup", "pgdown", "/", "n", "N", "y", "esc":
		default:
			return false, nil
		}
	}
	event := d.Key(key, msg.Text)
	return event.Action != "back", m.copyWorkspaceText(event.Copy)
}

func (m *model) yamlDocumentPaste(text string) bool {
	if !m.yamlDocumentActive() {
		return false
	}
	m.syncYAMLDocument().Document.Paste(text)
	return true
}

func (m *model) returnFromExploreYAML() bool {
	e := m.explorer()
	if e.Tab != 1 || e.YamlOrigin == nil {
		return false
	}
	origin := e.YamlOrigin
	e.Tab, e.Scroll = origin.Tab, origin.Scroll
	e.RelatedCursor = origin.RelatedCursor
	e.YamlOrigin, e.YamlDocument, e.Reveal = nil, nil, false
	m.rememberExplore()
	return true
}
