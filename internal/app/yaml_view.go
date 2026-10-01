package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	docedit "github.com/simjay/plous/internal/editor"
	"github.com/simjay/plous/internal/kubernetes"
	"github.com/simjay/plous/internal/textview"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type yamlDocument struct {
	document       *docedit.Model
	identity, text string
	revealed       bool
	source         *unstructured.Unstructured
	sourceVersion  string
}

func (m *model) yamlDocumentActive() bool {
	e := m.explore
	if e != nil && !e.detailValid && e.detailRun == nil && (e.detailError != "" || e.detailText != "") {
		return false
	}
	return m.exploreVisible() && e != nil && e.detail != nil && e.tab == 1 && e.action == nil && e.terminal == nil && e.input == nil && e.picker == ""
}

func (m *model) yamlDocumentSearching() bool {
	return m.yamlDocumentActive() && m.syncYAMLDocument().document.Searching
}

func (m *model) syncYAMLDocument() *yamlDocument {
	e := m.explorer()
	if e.detail == nil {
		return nil
	}
	context := ""
	if connection := m.currentConnection(); connection != nil {
		context = connection.Settings["context"]
	}
	identity := strings.Join([]string{m.active, m.connection, context, e.resource.Key(), e.detail.GetAPIVersion(), e.detail.GetKind(), e.detail.GetNamespace(), e.detail.GetName(), string(e.detail.GetUID())}, "\x00")
	d := e.yamlDocument
	// API and watch updates replace the captured object. Focus and resize reuse it.
	version := e.detail.GetResourceVersion()
	if d != nil && d.identity == identity && d.source == e.detail && d.sourceVersion == version && d.revealed == e.reveal {
		return d
	}
	text := kubernetes.YAML(*e.detail, e.reveal)
	if d == nil || d.identity != identity {
		d = &yamlDocument{identity: identity, document: docedit.New(text)}
		d.document.ReadOnly, d.document.Syntax = true, "yaml"
		d.document.Name = "YAML · " + e.detail.GetName()
		e.yamlDocument = d
	}
	if d.text != text || d.revealed != e.reveal {
		d.document.RefreshReadOnly(text)
		if d.text != "" {
			d.document.Notice = "YAML refreshed; search and selection cleared."
			if d.revealed != e.reveal {
				d.document.Notice = "Value visibility changed; search and selection cleared."
			}
		}
		d.text, d.revealed = text, e.reveal
	}
	d.source, d.sourceVersion = e.detail, version
	d.document.Target = context + " · " + e.detail.GetNamespace()
	if e.resource.Key() == "secrets" {
		if e.reveal {
			d.document.Target += " · Values revealed"
		} else {
			d.document.Target += " · Values hidden"
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
	if m.explore.resource.Key() == "secrets" {
		label := "Reveal values"
		if d.revealed {
			label = "Hide values"
		}
		actions = append(actions, textview.Action{Key: "v", Label: label})
	}
	d.document.Actions = []textview.ActionGroup{{Label: "Read", Actions: actions}, m.documentDisplayActions(d.document)}
	if m.explore.resource.Key() == "secrets" && height > 1 {
		visibility := "Values hidden"
		if d.revealed {
			visibility = "Values revealed"
		}
		return muted.Render(visibility) + "\n" + d.document.ViewEmbedded(width, height-1, workspaceTheme())
	}
	return d.document.ViewEmbedded(width, height, workspaceTheme())
}

func (m *model) yamlDocumentKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.yamlDocumentActive() {
		return false, nil
	}
	d := m.syncYAMLDocument().document
	key := msg.String()
	if !d.Searching {
		switch key {
		case "v":
			if m.explore.resource.Key() == "secrets" {
				m.explore.reveal = !m.explore.reveal
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
	m.syncYAMLDocument().document.Paste(text)
	return true
}

func (m *model) returnFromExploreYAML() bool {
	e := m.explorer()
	if e.tab != 1 || e.yamlOrigin == nil {
		return false
	}
	origin := e.yamlOrigin
	e.tab, e.scroll = origin.tab, origin.scroll
	e.relatedCursor = origin.relatedCursor
	e.yamlOrigin, e.yamlDocument, e.reveal = nil, nil, false
	m.rememberExplore()
	return true
}
