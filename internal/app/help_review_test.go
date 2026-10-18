package app

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/monitor"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestHelpMatchesComparisonInputOwnership(t *testing.T) {
	d := diffview.New("before", "after", "local file", 1, true)
	d.Reader.SetLines([]textview.Line{{Text: "changed line"}})
	entries := helpEntries(d.Help("Review", "local file", "Back"))
	if entries["horizontal"].Label != "Move text cursor" || entries["activate"].Key != "" || entries["finish-find"].Key != "" {
		t.Fatal("comparison reading advertises action or search submission")
	}
	d.Key("right", "")
	if d.Reader.Cursor.Col != 1 || d.Actions {
		t.Fatal("reading no longer owns horizontal movement")
	}
	if decision, _ := d.Key("enter", ""); decision.Action != "" {
		t.Fatal("reading Enter submits a decision")
	}
	d.Key("a", "a")
	entries = helpEntries(d.Help("Review", "local file", "Back"))
	if entries["choose"].Key != "←→" || entries["activate"].Key != "Enter" || entries["change"].Key != "" {
		t.Fatal("action Help includes reading-only keys")
	}
	d.Key("right", "")
	if decision, _ := d.Key("enter", ""); decision.Action != "confirm" || decision.ID != d.ID {
		t.Fatal("advertised focused action does not produce the matching decision")
	}
	d.Key("a", "a")
	d.Key("v", "v")
	entries = helpEntries(d.Help("Review", "local file", "Back"))
	if entries["actions"].Key != "" || entries["change"].Key != "" || entries["back"].Label != "Clear selection" {
		t.Fatal("selection Help advertises intercepted comparison commands")
	}
	d.Key("esc", "")
	d.Key("/", "/")
	entries = helpEntries(d.Help("Review", "local file", "Back"))
	if entries["finish"].Key != "Enter" || entries["save"].Key != "" {
		t.Fatal("search Help includes save or omits finishing the query")
	}
	readonly := diffview.New("before", "after", "saved record", 1, false)
	readonly.Key("a", "a")
	entries = helpEntries(readonly.Help("Comparison", "", "Back"))
	if entries["choose"].Key != "" || entries["save"].Key != "" {
		t.Fatal("read-only comparison advertises a second action or save")
	}
}

func TestHelpMatchesTreeSearchAndManualNamespace(t *testing.T) {
	m := exploreModel(t)
	m.focus = 0
	if !m.beginPanelSearch() {
		t.Fatal("tree search did not open")
	}
	entries := helpEntries(m.workspaceHelp())
	if entries["finish"].Key != "Enter" || entries["select"].Key != "↑↓" || entries["refresh"].Key != "" || entries["ends"].Key != "" {
		t.Fatal("tree search exposes commands or caret keys as navigation")
	}
	typeText(m, "g")
	if m.searches[0].input.Text != "g" {
		t.Fatal("tree search shortcut did not stay literal")
	}
	p := &monitor.NamespacePicker{Manual: true}
	field := textview.NewField("team")
	input, cursor := &field, 0
	for _, title := range []string{"Kubernetes", "Helm", "Argo", "Flux"} {
		entries = helpEntries(p.Help(title))
		if entries["finish"].Key != "Enter" || entries["select"].Key != "" || entries["page"].Key != "" {
			t.Fatalf("%s manual namespace advertises list navigation", title)
		}
	}
	value, apply, close, status := monitor.UpdateNamespacePicker(p, &input, &cursor, "default", localMouseKey("enter"))
	if value != "team" || !apply || close || status != "" {
		t.Fatal("manual namespace does not match advertised Enter behavior")
	}
	monitor.UpdateNamespacePicker(p, &input, &cursor, "default", localMouseKey("esc"))
	entries = helpEntries(p.Help("Namespace"))
	if p.Manual || entries["ends"].Key != "Home / End" || entries["page"].Key == "" {
		t.Fatal("returning to namespace list did not restore its controls")
	}
}

func TestHelpMatchesYAMLAndEditorSelection(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(1)
	m.explore.DetailValid = true
	d := m.syncYAMLDocument().Document
	entries := helpEntries(m.mainHelp())
	if entries["document"].Key != "Home / End" || entries["line"].Key != "" || entries["select"].Key != "z" || entries["move"].Alternatives[0] != "j / k" {
		t.Fatal("YAML Help includes unsupported editor movement")
	}
	m.yamlDocumentKey(localMouseKey("end"))
	if d.Cursor != utf8.RuneCountInString(d.Text()) {
		t.Fatal("YAML End did not reach the document end")
	}
	m.yamlDocumentKey(localMouseKey("home"))
	if d.Cursor != 0 {
		t.Fatal("YAML Home did not reach the document start")
	}
	m, a := documentModel(t)
	baseline := a.Document.Text()
	a.Document.Paste("# unsaved\n")
	typeText(m, "vll")
	entries = helpEntries(m.mainHelp())
	if entries["cut"].Key != "d" || entries["copy"].Key != "y" || entries["select"].Label != "Leave selection" {
		t.Fatal("Select mode Help still describes Normal mode actions")
	}
	text, selection, revision := a.Document.Text(), a.Document.SelectedText(), a.Document.Revision
	m.openHelp()
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.render()
		m.updateHelp("end")
	}
	m.closeHelp()
	if a.Document.Mode != docedit.Select || a.Document.Text() != text || a.Document.SelectedText() != selection || a.Document.Revision != revision {
		t.Fatal("rendering or resizing Help changed the draft or selection")
	}
	a.Document.Undo()
	if a.Document.Text() != baseline {
		t.Fatal("rendering Help lost undo history")
	}
}

func TestHelpMatchesHelmInspectorAndCodeDetails(t *testing.T) {
	m, h := helmModel(t)
	h.Detail = &helmtool.Snapshot{Name: "shop", Version: 1}
	h.Tab = 2
	entries := helpEntries(m.mainHelp())
	if entries["scroll"].Key != "↑↓" || len(entries["scroll"].Alternatives) != 0 {
		t.Fatal("Helm inspector advertises unsupported j/k navigation")
	}
	m.updateHelm("down")
	if h.Subcursor != 1 {
		t.Fatal("Helm inspector does not scroll with the advertised key")
	}
	m, s := codeModel(t)
	m.tool, s.View, s.BrowserDetail = "code-kustomize", "code-kustomize", true
	entries = helpEntries(m.mainHelp())
	if entries["scroll"].Key != "↑↓" || entries["open"].Key != "" {
		t.Fatal("configuration details still advertise opening details or selecting a card")
	}
	m.codeBrowserKey("down")
	if s.DetailOffset != 1 {
		t.Fatal("configuration details do not scroll with the advertised key")
	}
	s.Menu = "more-tools"
	if title := m.mainHelp().Title; title != "Code · More Tools" {
		t.Fatal("Help exposes an internal menu name:", title)
	}
	global := ansi.Strip(m.globalHelpBody())
	for _, command := range []string{":1 / 11", ":2 / 22", ":3 / 33"} {
		if !strings.Contains(global, command) {
			t.Fatal("global Help is missing", command)
		}
	}
}

func TestHelpEditorActionsAndLiteralErrorKey(t *testing.T) {
	m, a := documentModel(t)
	a.Err = "Validation failed"
	a.Document.Controls = true
	section := m.mainHelp()
	escapes := 0
	for _, group := range section.Groups {
		for _, entry := range group.Entries {
			if entry.Key == "Esc" {
				escapes++
				if entry.Label != "Request Close" {
					t.Fatal("editor actions advertise the document's Escape behavior")
				}
			}
		}
	}
	if escapes != 1 || helpEntries(section)["error"].Key != "!" {
		t.Fatal("editor actions have conflicting Escape entries or omit the error")
	}
	a.Document.Controls, a.Document.Mode = false, docedit.Insert
	if helpEntries(m.mainHelp())["error"].Key != "" {
		t.Fatal("Insert mode advertises literal ! as a command")
	}
	a.Document.Mode, a.Document.Searching = docedit.Normal, true
	if helpEntries(m.mainHelp())["error"].Key != "" {
		t.Fatal("search advertises literal ! as a command")
	}
	a.Document.Searching = false
	if helpEntries(m.mainHelp())["error"].Key != "!" {
		t.Fatal("Normal mode lost the full error command")
	}
	a.Document.Paste("# unsaved\n")
	a.ShowDiagnostic(a.Err)
	m.editorLocalCommand("q")
	if !a.Discard || m.mainHelp().Title != "Discard draft" {
		t.Fatal("error reader hides Help for its discard confirmation")
	}
}
