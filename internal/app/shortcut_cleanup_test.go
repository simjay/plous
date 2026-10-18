package app

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/monitor"
	textview "github.com/simjay/plous/internal/ui/text"

	"strings"
	"testing"
)

func TestRetiredCommandAliasesAndLiteralAllNamespace(t *testing.T) {
	m := exploreModel(t)
	for _, command := range []string{"setting", "ac", "lg", "ex", "sl", "rr", "ed", "dl", "ci", "cv", "xy", "un", "nc", "oc", "rp", "rf", "cn", "ya", "de", "ln", "vr", "vd", "ud", "rd", "be", "ce", "ap", "backtoeditor", "closeeditor", "applychange", "validatereview", "viewchanges", "nsp", "stc"} {
		m.execute(command)
		if !strings.Contains(m.status, "Unknown command") || m.explore.Action != nil || m.settingsOverlay != nil {
			t.Fatalf("retired alias %q still handled: %s", command, m.status)
		}
	}
	for _, command := range []string{"N all", "pods all"} {
		m.execute(command)
		if m.explore.Preference.Namespace != "all" {
			t.Fatalf("%s did not select literal namespace all", command)
		}
	}
	m.execute("N *")
	if m.explore.Preference.Namespace != "" {
		t.Fatal("wildcard did not select all namespaces")
	}
	for _, action := range monitor.ResourceActionDefinitions {
		if actionCommand(action.Alias) != action.Id || actionCommand(action.Command) != action.Id {
			t.Fatalf("lost action %s", action.Id)
		}
	}
}

func TestLogsInterveningInputClearsShortcutPrefix(t *testing.T) {
	m := exploreModel(t)
	m.openExploreSelection(4)
	m.explore.DetailValid = true
	m.podLogViewer().Append("one\ntwo\nthree\n")
	for _, key := range []string{"down", "pgup", "v", "/"} {
		typeText(m, "b")
		switch key {
		case "pgup":
			m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
		case "v", "/":
			typeText(m, key)
		default:
			press(t, m, key)
		}
		if m.shortcutPrefix != "" {
			t.Fatalf("%s retained prefix", key)
		}
		if key == "/" {
			press(t, m, "esc")
		}
		typeText(m, "b")
		if m.leftHidden {
			t.Fatalf("b %s b toggled sidebar", key)
		}
		press(t, m, "esc")
	}
	typeText(m, "/bbii33:settings!")
	m.Update(tea.PasteMsg{Content: ":quit"})
	if m.podLogViewer().Reader.Query != "bbii33:settings!:quit" || m.leftHidden || m.rightHidden || m.mode != 1 {
		t.Fatal("log search dispatched literal text")
	}
}

func TestActionAndReaderShortcutsPersist(t *testing.T) {
	for _, view := range []string{"editor actions", "exec review", "action picker", "logs"} {
		t.Run(view, func(t *testing.T) {
			m, a := documentModel(t)
			switch view {
			case "editor actions":
				a.Document.Controls = true
			case "exec review":
				m.explore.Action = nil
				tm := reviewTerminal(t, 40, 8)
				tm.BeginReview()
				m.explore.Terminal = tm
			case "action picker":
				a.Stage = "menu"
			case "logs":
				m.explore.Action = nil
				m.explore.Tab = 4
			}
			if err := m.saveSession(); err != nil {
				t.Fatal(err)
			}
			typeText(m, "b")
			m.Update(autosaveMsg{})
			m.drainPersistence()
			typeText(m, "b")
			if !m.leftHidden || !m.dirty {
				t.Fatal("completed shortcut did not schedule autosave")
			}
			m.Update(autosaveMsg{})
			m.drainPersistence()
			data, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			var saved session
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if !saved.LeftHidden {
				t.Fatal("autosave lost hidden sidebar")
			}
		})
	}
}

func TestCompletedExecEditReplacesShellShortcut(t *testing.T) {
	m, a := documentModel(t)
	a.Stage, a.Input = "execcommand", textview.NewField("/bin/sh")
	tm := reviewTerminal(t, 40, 8)
	tm.Done, tm.Launch = true, a
	tm.BeginReview()
	m.explore.Action, m.explore.Terminal = nil, tm
	typeText(m, "s")
	if m.explore.Action != nil || m.shortcutPrefix != "s" {
		t.Fatal("s bypassed reserved Settings prefix")
	}
	press(t, m, "esc")
	typeText(m, "e")
	if m.explore.Action != a || !a.ExecEditing || a.PreviousTerminal != tm {
		t.Fatal("e did not open editable launcher retaining output")
	}
	press(t, m, "esc")
	if a.ExecEditing {
		t.Fatal("Esc did not leave command input")
	}
	typeText(m, "hh")
	if m.panelHelpSection(1).Title != "Exec · Shell / command" || !strings.Contains(m.panelHelpText(1), "Start") {
		t.Fatal("launcher help not contextual")
	}
}

func TestFinishedPanelFindNavigatesWithoutCapturingCommands(t *testing.T) {
	m, _ := testModel(t)
	m.focus = 2
	m.chat().Turns = []history.Turn{{Request: history.Request{Prompt: "needle"}, Response: history.Response{Text: strings.Repeat("line\n", 50) + "needle", Status: "completed"}}}
	typeText(m, "/needle")
	m.chatTranscript(50, 14)
	press(t, m, "enter")
	s := m.searches[2]
	if !s.finished || s.match != 0 {
		t.Fatal("Enter should finish query without advancing")
	}
	typeText(m, "n")
	m.chatTranscript(50, 14)
	if s.match != 1 {
		t.Fatal("n did not advance match")
	}
	typeText(m, "N")
	m.chatTranscript(50, 14)
	if s.match != 0 {
		t.Fatal("N did not return to first match")
	}
	before := m.chat().Scroll
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.chatTranscript(50, 14)
	if m.chat().Scroll <= before {
		t.Fatal("retained query pinned the viewport instead of scrolling")
	}
	m.Update(tea.PasteMsg{Content: "ignored"})
	if s.input.Text != "needle" {
		t.Fatal("navigation paste edited finished query")
	}
	typeText(m, "bb")
	if !m.leftHidden || m.searches[2] != s {
		t.Fatal("retained search captured global shortcut")
	}
	press(t, m, "esc")
	if m.searches[2] != nil || m.chatEditing {
		t.Fatal("Esc did not clear search locally")
	}
}

func TestHelpUsesUnderlyingViewControls(t *testing.T) {
	m := exploreModel(t)
	m.execute("help")
	text := m.panelHelpText(1)
	if !strings.Contains(text, "Resource table") || strings.Contains(text, "Interrupt") || strings.Contains(text, "Apply reviewed") {
		t.Fatal("table help includes unrelated controls")
	}
	m.closeHelp()
	m.openExploreSelection(4)
	m.execute("help")
	text = m.panelHelpText(1)
	if !strings.Contains(text, "Pod / workload logs") || strings.Contains(text, "R Restart") {
		t.Fatal("logs help includes resource mutations")
	}
	m.closeHelp()
	tm := reviewTerminal(t, 40, 8)
	tm.Done = true
	tm.Launch = &monitor.ResourceActionState{Stage: "execcommand"}
	tm.BeginReview()
	m.explore.Terminal = tm
	m.execute("help")
	text = m.panelHelpText(1)
	if !strings.Contains(text, "e Edit command") || strings.Contains(text, "r Resume") || strings.Contains(text, "s Shell") {
		t.Fatalf("ended Exec help inaccurate: %s", text)
	}
}
