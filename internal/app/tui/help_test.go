package tui

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	ai "github.com/simjay/plous/internal/ai/model"
	appchat "github.com/simjay/plous/internal/app/chat"
	"github.com/simjay/plous/internal/app/monitor"
	appsettings "github.com/simjay/plous/internal/app/settings"
	gitops "github.com/simjay/plous/internal/integrations/gitops/common"
	helmtool "github.com/simjay/plous/internal/integrations/helm/monitor"
	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	diffview "github.com/simjay/plous/internal/ui/diff"
	docedit "github.com/simjay/plous/internal/ui/editor"
	helpview "github.com/simjay/plous/internal/ui/help"
	textview "github.com/simjay/plous/internal/ui/text"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHelpOpensGloballyAndRestoresEachPanel(t *testing.T) {
	for focus := 0; focus < 3; focus++ {
		for _, command := range []string{"h", "help"} {
			t.Run(fmt.Sprintf("%d/%s", focus, command), func(t *testing.T) {
				m, _ := testModel(t)
				createWorkspace(t, m, "Payments")
				mainCommand(t, m, "3")
				press(t, m, "enter")
				detail, active := m.detail, m.active
				focusPanel(t, m, focus)
				typeText(m, ":"+command)
				press(t, m, "enter")
				if m.help == nil || m.focus != focus || m.detail != detail {
					t.Fatal("help did not preserve underlying view and focus")
				}
				press(t, m, "tab")
				press(t, m, "down")
				press(t, m, "esc")
				if m.help != nil || m.focus != focus || m.detail != detail || m.active != active || m.mode != 2 {
					t.Fatal("closing help lost the previous view")
				}
			})
		}
	}
}

func TestHelpPreservesFormAndIgnoresPaste(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unsaved")
	form := m.form
	press(t, m, "enter")
	typeText(m, ":h")
	press(t, m, "enter")
	m.Update(tea.PasteMsg{Content: ":quit\n"})
	press(t, m, "enter")
	if m.help == nil || m.command != nil || m.form != form {
		t.Fatal("help accepted paste or submitted the form")
	}
	typeText(m, ":s")
	press(t, m, "enter")
	if m.form != form || m.help == nil || !strings.Contains(m.status, "Finish or cancel") {
		t.Fatal("global settings discarded an unfinished form")
	}
	press(t, m, "esc")
	focusPanel(t, m, 1)
	if m.form != form || form.Fields[0].Value.Text != "Payments unsaved" || m.current().Name != "Payments" {
		t.Fatal("help changed form edits or saved them")
	}
}

func TestGlobalCommandsAndChatTypingAreSeparate(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	focusPanel(t, m, 2)
	typeText(m, "ignored")
	m.Update(tea.PasteMsg{Content: ":q\n"})
	if m.draft().Text != "" || m.chatEditing || m.command != nil {
		t.Fatal("chat navigation accepted text or paste")
	}
	for _, command := range []string{"c", "chat"} {
		typeText(m, ":"+command)
		press(t, m, "enter")
		if !m.chatEditing || m.focus != 2 {
			t.Fatal("chat command did not enter typing")
		}
		typeText(m, ":help")
		m.Update(tea.PasteMsg{Content: " :settings"})
		press(t, m, "ctrl+n")
		press(t, m, "ctrl+o")
		if m.help != nil || m.command != nil || m.chat().History || !strings.Contains(m.draft().Text, ":help :settings") {
			t.Fatal("typing or retired Ctrl shortcuts invoked navigation")
		}
		press(t, m, "esc")
	}
	draft := m.draft().Text
	typeText(m, ":h")
	press(t, m, "enter")
	if m.help == nil || m.draft().Text != draft {
		t.Fatal("Esc followed by help lost the draft")
	}
	press(t, m, "esc")
	for _, command := range []string{"s", "settings"} {
		focusPanel(t, m, 2)
		typeText(m, ":"+command)
		press(t, m, "enter")
		if m.page != "settings" || m.focus != 1 || m.draft().Text != draft {
			t.Fatal("global settings failed")
		}
	}
}

func TestChatActionAliasesAndGlobalStop(t *testing.T) {
	for _, short := range []bool{true, false} {
		m, _ := testModel(t)
		createWorkspace(t, m, "Payments")
		id := selectChatConnection(t, m)
		conversation, err := m.history.NewConversation(id, "Saved chat")
		if err != nil {
			t.Fatal(err)
		}
		turn, err := m.history.StartTurn(id, conversation.ID, history.Request{Prompt: "hello", Agent: ai.DefaultConfig()})
		if err != nil {
			t.Fatal(err)
		}
		c := m.chat()
		c.Conversation, c.Turns = conversation, []history.Turn{turn}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		m.run = &appchat.ChatRun{Chat: c, Cancel: cancel}
		startChat(t, m)
		typeText(m, "Keep this draft")
		press(t, m, "esc")
		if ctx.Err() != nil {
			t.Fatal("leaving input stopped a response")
		}
		commands := []string{"stopchat", "newchat", "openchat"}
		if short {
			commands = []string{"X", "C", "H"}
		}
		focusPanel(t, m, 0)
		typeText(m, ":"+commands[0])
		press(t, m, "enter")
		if ctx.Err() == nil || m.run == nil || !m.run.Stopping {
			t.Fatal("global stop did not cancel the response while retaining its producer")
		}
		m.Update(appchat.ChatEvent{Run: m.run, Done: true, Err: context.Canceled})
		m.drainPersistence()
		saved, err := m.history.Turns(id, conversation.ID)
		if err != nil || saved[0].Response.Status != "cancelled" {
			t.Fatal("stop did not persist the response", err)
		}
		typeText(m, ":"+commands[1])
		press(t, m, "down")
		press(t, m, "enter")
		finishChatRestore(t, m)
		if m.chat().Conversation.ID != "" || !m.chatEditing || m.focus != 2 || m.draft().Text != "Keep this draft" {
			t.Fatal("new chat did not preserve the draft and enter input")
		}
		press(t, m, "esc")
		typeText(m, ":"+commands[2])
		press(t, m, "enter")
		if !m.chat().History || m.chatEditing || len(m.chat().Choices) != 2 {
			t.Fatal("open chat did not show history")
		}
		press(t, m, "down")
		press(t, m, "enter")
		finishChatRestore(t, m)
		if m.chat().Conversation.ID != conversation.ID || m.chatEditing {
			t.Fatal("history selection did not reopen the conversation in navigation")
		}
	}
}

func TestHelpFourSectionsFitAndCanBeRead(t *testing.T) {
	m, _ := testModel(t)
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		mainCommand(t, m, "help")
		for _, editingCommand := range []bool{false, true} {
			if editingCommand {
				typeText(m, ":s")
			}
			view := ansi.Strip(m.render())
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Fatalf("help height %d at %v", len(lines), size)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != size[0] {
					t.Fatalf("help row overflow at %v: %q", size, line)
				}
			}
			for _, label := range []string{"Workspace", "AI chat", "Global", ":quit", "bb", "ii", "11", "22", "33"} {
				if !strings.Contains(view, label) {
					t.Fatalf("missing %q at %v", label, size)
				}
			}
			window := strings.Split(ansi.Strip(m.helpPanels()), "\n")
			width, height := m.helpDimensions()
			if width >= size[0] || height > size[1]-2 || len(window) != height {
				t.Fatalf("Help must leave visible space around its overlay at %v", size)
			}
			if strings.Count(window[len(window)-1], "╰") != 1 || strings.Count(window[0], "╭") != 3 {
				t.Fatalf("Help must retain three upper sections and one full-width global box at %v: first %q, last %q", size, window[0], window[len(window)-1])
			}
			footerLabel := "Help"
			if editingCommand {
				footerLabel = "Command"
			}
			if !strings.Contains(lines[0], "plous") || !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), footerLabel+"  ") {
				t.Fatal("Help must preserve the application header and single footer")
			}
			if editingCommand && !strings.Contains(view, ":s▏") {
				t.Fatal("command editor was hidden by Help")
			}
			if editingCommand {
				press(t, m, "esc")
			}
		}
		for focus := 0; focus < 3; focus++ {
			for m.help.section != focus {
				press(t, m, "right")
			}
			press(t, m, "end")
			if m.help.scroll[focus] < 0 {
				t.Fatal("invalid scroll offset")
			}
			press(t, m, "home")
			if m.help.scroll[focus] != 0 {
				t.Fatal("help cannot return to the first line")
			}
		}
		press(t, m, "esc")
	}
}

func TestHelpOverlayReturnsToSettingsAndPreservesBackground(t *testing.T) {
	m, _ := testModel(t)
	m.openSettings()
	m.beginAppSettings()
	typeText(m, " unsaved")
	m.form.Index = len(m.form.Fields)
	before := ansi.Strip(m.render())
	form, settings := m.form, m.settingsOverlay
	typeText(m, "hh")
	if m.help == nil || m.form != form || m.settingsOverlay != settings {
		t.Fatal("hh must open Help over Settings without changing its edits")
	}
	view := strings.Split(ansi.Strip(m.render()), "\n")
	background := strings.Split(before, "\n")
	width, height := m.helpDimensions()
	x, y := (m.width-width)/2, (m.height-height)/2
	for row := 0; row < m.height-1; row++ {
		if row < y || row >= y+height {
			if view[row] != background[row] {
				t.Fatalf("Help replaced the underlying view outside the overlay at row %d", row)
			}
		} else if ansi.Cut(view[row], 0, x) != ansi.Cut(background[row], 0, x) || ansi.Cut(view[row], x+width, m.width) != ansi.Cut(background[row], x+width, m.width) {
			t.Fatalf("Help replaced the background beside the overlay at row %d", row)
		}
	}
	press(t, m, "tab")
	press(t, m, "down")
	help := m.help
	typeText(m, "hh")
	if m.help != help {
		t.Fatal("reopening Help must preserve its original return view")
	}
	press(t, m, "esc")
	if m.help != nil || m.form != form || m.settingsOverlay != settings || m.focus != 1 || ansi.Strip(m.render()) != before {
		t.Fatal("closing Help must return to the same Settings overlay and edits")
	}
}

func helpEntries(s helpview.Section) map[string]helpview.Entry {
	entries := map[string]helpview.Entry{}
	for _, g := range s.Groups {
		for _, e := range g.Entries {
			entries[e.ID] = e
		}
	}
	return entries
}

func TestContextualHelpResourceActionsMatchRegistry(t *testing.T) {
	for _, resource := range []kubecommon.Resource{
		{Name: "pods", Kind: "Pod"}, {Group: "apps", Name: "deployments", Kind: "Deployment"},
		{Name: "namespaces", Kind: "Namespace"}, {Name: "secrets", Kind: "Secret"}, {Name: "configmaps", Kind: "ConfigMap"},
		{Name: "events", Kind: "Event"}, {Group: "apps", Name: "controllerrevisions", Kind: "ControllerRevision"},
		{Group: "example.test", Name: "widgets", Kind: "Widget"},
	} {
		t.Run(resource.Key(), func(t *testing.T) {
			e := &monitor.ExploreState{Resource: resource, Detail: &unstructured.Unstructured{Object: map[string]any{"kind": resource.Kind}}}
			expected := map[string]monitor.ResourceAction{}
			for _, action := range monitor.ActionsFor(resource, e.Detail) {
				expected[action.Command] = action
			}
			entries := helpEntries(e.Help())
			for _, action := range monitor.ResourceActionDefinitions {
				got, present := entries[action.Command]
				want, applicable := expected[action.Command]
				if present != applicable || present && (got.Key != want.Alias || got.Label != want.Label) {
					t.Fatalf("%s help does not match resource action: %+v, applicable %t", action.Command, got, applicable)
				}
			}
			e.Tab = 1
			_, editable := helpEntries(e.Help())["edit"]
			_, wantEditable := expected["edit"]
			if editable != wantEditable {
				t.Fatal("YAML bypassed resource edit applicability")
			}
			e.Detail = nil
			entries = helpEntries(e.Help())
			if _, present := entries["scale"]; present {
				t.Fatal("table advertises detail-only Scale")
			}
			if entries["sort"].Key != "S" {
				t.Fatal("table lost Sort")
			}
		})
	}
}

func TestContextualHelpSnapshotAndPurity(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	m.focus = 1
	c := m.selectedConnection()
	c.Target, c.TargetCaptured = nil, false
	expanded := maps.Clone(m.expanded)
	originalFocus, originalTool, originalConnection := m.focus, m.tool, m.connection
	e := m.explore
	action := &monitor.ResourceActionState{Stage: "menu", Items: monitor.ActionsFor(e.Resource, e.Detail), Reasons: map[string]string{"delete": "Access denied"}}
	e.Action = action
	m.openHelp()
	if c.TargetCaptured || c.Target != nil || !reflect.DeepEqual(expanded, m.expanded) || m.focus != originalFocus || m.tool != originalTool || m.connection != originalConnection {
		t.Fatalf("Help changed state: target captured=%v, target present=%v, expanded changed=%v, focus=%d/%d, tool=%s/%s, connection=%s/%s", c.TargetCaptured, c.Target != nil, !reflect.DeepEqual(expanded, m.expanded), m.focus, originalFocus, m.tool, originalTool, m.connection, originalConnection)
	}
	if got := helpEntries(m.help.sections[1])["delete"].Reason; got != "Access denied" {
		t.Fatalf("lost known denial: %q", got)
	}
	before := m.help.sections[1].Text()
	action.Reasons["delete"] = "changed during background refresh"
	e.Action = nil
	m.help.scroll[1] = 2
	m.updateHelp("right")
	m.openHelp()
	if m.help.sections[1].Text() != before || m.help.scroll[1] != 2 || m.focus != originalFocus {
		t.Fatal("Help reference or originating focus changed while reading")
	}
	m.closeHelp()
	m.openHelp()
	if m.help.sections[1].Text() == before {
		t.Fatal("reopening Help did not refresh context")
	}
	m.closeHelp()
	m.explore = nil
	m.helpSections()
	if m.explore != nil {
		t.Fatal("Help initialized a lazy Kubernetes view")
	}
}

func TestContextualHelpPanelAndOverlayPrecedence(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	title := m.mainHelp().Title
	rows := m.treeRowsRead()
	for i, row := range rows {
		if row.kind == "settings" {
			m.treeCursor = i
			break
		}
	}
	if m.mainHelp().Title != title || !strings.Contains(m.workspaceHelp().Title, "Settings") {
		t.Fatal("tree highlight redirected main Help")
	}
	m.form = &appsettings.Form{Title: "Unfinished setup", Kind: "connection"}
	if m.mainHelp().Title != "Unfinished setup" {
		t.Fatal("resource overrode setup Help")
	}
	m.form = nil
	m.mode = 0
	if !strings.HasPrefix(m.mainHelp().Title, "Overview") {
		t.Fatal("resource overrode Overview")
	}
	m.mode = 2
	if !strings.Contains(m.mainHelp().Title, "Activity") {
		t.Fatal("resource overrode Activity")
	}
}

func TestContextualHelpHelmAndGitOpsRouting(t *testing.T) {
	m, h := helmModel(t)
	h.Preference.Section = "Charts"
	if m.mainHelp().Title != "Helm · Charts" {
		t.Fatal(m.mainHelp().Title)
	}
	h.Document = docedit.New("replicas: 2\n")
	h.Comparison = diffview.New("replicas: 1\n", h.Document.Text(), "draft", 0, false)
	entries := helpEntries(m.mainHelp())
	if entries["save"].Label != "Save to draft values" {
		t.Fatal("Helm values review lost its save target")
	}
	h.Document = nil
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("read-only Helm comparison offers Save")
	}
	h.Comparison = nil
	h.Timeline = &monitor.HelmTimeline{Release: helmtool.Release{Revision: 2}, Revisions: []helmtool.Revision{{Revision: 2}, {Revision: 1}}}
	if _, rollback := helpEntries(m.mainHelp())["rollback"]; rollback {
		t.Fatal("latest revision offers rollback key")
	}
	if strings.Contains(m.mainHelp().Text(), "Preview rollback") {
		t.Fatal("latest revision offers a rollback menu route")
	}
	h.Timeline.Cursor = 1
	if _, rollback := helpEntries(m.mainHelp())["rollback"]; !rollback {
		t.Fatal("older revision lost rollback")
	}
	g, v := gitOpsModel(t)
	if !strings.Contains(g.mainHelp().Title, "Argo") {
		t.Fatal(g.mainHelp().Title)
	}
	v.State.ApplyDetail(v.State.Snapshot.Items[0])
	if _, linked := helpEntries(g.mainHelp())["link-Kubernetes resource"]; !linked {
		t.Fatal("Argo resource link missing")
	}
	g.tool = string(gitops.Flux)
	v = g.gitOpsCurrent()
	if !strings.Contains(g.mainHelp().Title, "Flux") || strings.Contains(g.mainHelp().Text(), "rollback") {
		t.Fatal(g.mainHelp().Text())
	}
	v.Namespaces = &monitor.NamespacePicker{}
	if !strings.Contains(g.mainHelp().Title, "Namespace") {
		t.Fatal("Flux namespace picker lost precedence")
	}
}

func TestContextualHelpCommandsAndScreenFrames(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(0)
	for _, section := range m.helpSections() {
		for _, e := range helpEntries(section) {
			if e.Command == "" {
				continue
			}
			found := false
			for _, binding := range append([]string{e.Key}, e.Alternatives...) {
				if !strings.HasPrefix(binding, ":") {
					continue
				}
				command := canonicalCommand(strings.TrimPrefix(binding, ":"))
				if command == e.Command || actionCommand(command) == e.Command {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s has no registered command spelling: %+v", e.ID, e)
			}
		}
	}
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		m.execute("help")
		view := ansi.Strip(m.render())
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) != size[0] {
				t.Fatalf("Help overflow at %v", size)
			}
		}
		if strings.Contains(view, "Current Workspace\nLaunch") || strings.Contains(view, "Click controls to activate") {
			t.Fatal("Help still contains introductory essays")
		}
		if dir := os.Getenv("PLOUS_REVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("help-%d.txt", size[0])), []byte(view), 0600); err != nil {
				t.Fatal(err)
			}
		}
		m.updateHelp("end")
		widths, height, _ := m.helpSizes()
		if m.help.scroll[1] != max(0, len(helpLines(m.help.sections[1], widths[1]-4))-m.helpBodyHeight(1, height)) {
			t.Fatal("last Help controls are unreachable")
		}
		m.closeHelp()
	}
}

func TestContextualHelpInteractionLayers(t *testing.T) {
	m, a := documentModel(t)
	a.Document.Paste("draft\n")
	a.Document.Key("v", "v")
	a.Document.Key("right", "")
	before := a.Document.Text()
	m.openHelp()
	if !strings.Contains(m.panelHelpText(1), "Save to Kubernetes") {
		t.Fatal("wrong save target")
	}
	m.closeHelp()
	if a.Document.Text() != before || a.Document.Mode != docedit.Select {
		t.Fatal("Help altered document selection")
	}
	a.Diagnostic = &textview.Viewer{}
	if m.mainHelp().Title != "Error details" {
		t.Fatal("editor overrode diagnostic")
	}
	a.Diagnostic.Searching = true
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("search inherited Save")
	}
	a.Diagnostic = nil
	a.Discard = true
	if m.mainHelp().Title != "Discard draft" {
		t.Fatal("editor overrode discard confirmation")
	}
	a.Discard, a.Busy = false, true
	if _, save := helpEntries(m.mainHelp())["save"]; save {
		t.Fatal("pending write advertises another Save")
	}
	a.Busy, a.Stage = false, "scale"
	if strings.Contains(m.mainHelp().Text(), "PgUp") {
		t.Fatal("replica input advertises list keys")
	}
	c := m.chat()
	m.chats[m.conversationScope()] = c
	c.Models = true
	if m.chatHelp().Title != "AI chat · Models" {
		t.Fatal("model picker lost precedence")
	}
	c.Models, c.History = false, true
	if m.chatHelp().Title != "AI chat · History" {
		t.Fatal("chat history lost precedence")
	}
	c.History = false
	m.chatEditing = true
	entries := helpEntries(m.chatHelp())
	if entries["send"].Key != "Enter" || len(entries["newline"].Alternatives) != 1 {
		t.Fatal("composer lost newline exception")
	}
	if _, stop := entries["stopchat"]; stop {
		t.Fatal("idle chat advertises Stop")
	}
}

func TestContextualHelpPickersDescribeControlsWithoutRepeatingData(t *testing.T) {
	e := &monitor.ExploreState{Picker: "sort"}
	text := e.Help().Text()
	if !strings.Contains(text, "reverse current sort") || strings.Contains(text, "Type to") {
		t.Fatal(text)
	}
	h := &monitor.HelmState{MenuKind: "release", Menu: monitor.HelmMenuItems([]string{"private-release-name"})}
	if text := h.Help().Text(); strings.Contains(text, "private-release-name") || !strings.Contains(text, "PgUp") {
		t.Fatal(text)
	}
	h.MenuKind = "actions"
	h.Menu = []monitor.HelmMenuItem{monitor.HelmPreviewChartChanges.Item()}
	if !strings.Contains(h.Help().Text(), h.Menu[0].Label) {
		t.Fatal("action menu lost its commands")
	}
}

func (m *model) panelHelpText(panel int) string { return m.panelHelpSection(panel).Text() }

func (m *model) viewHelp() string { s := m.mainHelp(); m.resolveHelpCommands(&s); return s.Text() }

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
