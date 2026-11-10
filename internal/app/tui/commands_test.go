package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/ai/history"
	"github.com/simjay/plous/internal/app/monitor"
	textview "github.com/simjay/plous/internal/ui/text"
)

func TestNumericModeCommandsFromEveryPanel(t *testing.T) {
	for mode, command := range []string{"1", "2", "3"} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("%s/from%d", command, focus), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				press(t, m, "esc")
				m.mode, m.page, m.detail = (mode+1)%len(modes), "help", "previous detail"
				focusPanel(t, m, focus)
				typeText(m, ":"+command)
				if cmd := press(t, m, "enter"); cmd != nil {
					if _, quit := cmd().(tea.QuitMsg); quit {
						t.Fatal("switching modes must not quit")
					}
				}
				if m.mode != mode || m.page != "" || m.detail != "" || m.active != id || m.focus != 1 || m.command != nil {
					t.Fatalf(":%s did not select %s and preserve workspace scope", command, modes[mode])
				}
			})
		}
	}
}

func TestMainCommandAliases(t *testing.T) {
	for _, tc := range []struct {
		aliases []string
		page    string
		form    bool
	}{{[]string{"n", "new"}, "", true}, {[]string{"s", "settings"}, "settings", false}} {
		for _, alias := range tc.aliases {
			t.Run(alias, func(t *testing.T) {
				m, _ := testModel(t)
				createWorkspace(t, m, "Payments")
				mainCommand(t, m, "1")
				mainCommand(t, m, alias)
				if (m.form != nil) != tc.form || (!tc.form && m.page != tc.page) || m.command != nil {
					t.Fatalf(":%s opened wrong screen: page=%q form=%#v", alias, m.page, m.form)
				}
			})
		}
	}
}

func TestQuitAliasesSaveDraftBeforeExit(t *testing.T) {
	for _, alias := range []string{"q", "quit"} {
		t.Run(alias, func(t *testing.T) {
			m, _ := testModel(t)
			id := createWorkspace(t, m, "Payments")
			startChat(t, m)
			typeText(m, "unsaved chat draft :q")
			mainCommand(t, m, "3")
			mainCommand(t, m, alias)
			m.drainPersistence()
			cmd := m.syncQuit()
			if cmd == nil {
				t.Fatalf(":%s must request exit", alias)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf(":%s did not return a quit message", alias)
			}
			data, err := m.store.ReadSession()
			if err != nil {
				t.Fatal(err)
			}
			var saved session
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Drafts[m.conversationScope()] != "unsaved chat draft :q" || saved.Mode != 2 || saved.Workspace != id {
				t.Fatalf(":%s must save the latest draft and navigation before exit: %#v", alias, saved)
			}
		})
	}
}

func TestMainCommandAliasesRequireExactMatches(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	for _, text := range []string{"qu", "over", "resource", "set", "ne", "he", "q extra", "config", "ch", "st", "o", "overview", "res", "resources", "e", "explore", "activity", "4", "44", "left", "right", "side", "ai", "bb", "ii", "ss", "cc", "z", "zoom", "zz", "ff"} {
		if cmd := mainCommand(t, m, text); cmd != nil {
			t.Fatalf("partial or extended command %q must not quit", text)
		}
		if !strings.Contains(m.status, "Unknown command") || m.mode != 0 || m.page != "" || m.form != nil || m.leftHidden || m.rightHidden {
			t.Fatalf("partial or extended command %q must not invoke any action: %q", text, m.status)
		}
	}
}

func TestCommandDraftSurvivesFocusChangesAndCancels(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	startChat(t, m)
	mainCommand(t, m, "1")
	typeText(m, ":")
	press(t, m, "alt+right")
	typeText(m, ":q")
	press(t, m, "alt+right")
	typeText(m, "a")
	if m.command == nil || m.command.Text != "" || m.mode != 0 || m.draft().Text != ":q" {
		t.Fatal("typing in chat/tree must leave the main command draft and mode alone")
	}
	press(t, m, "alt+right")
	typeText(m, "2")
	press(t, m, "enter")
	if m.mode != 1 || m.command != nil {
		t.Fatal("returning to main should resume and execute its command draft")
	}
	typeText(m, ":q")
	if cmd := press(t, m, "esc"); cmd != nil || m.command != nil || m.mode != 1 {
		t.Fatal("Esc must close the command box without executing the command")
	}
	mainCommand(t, m, "n")
	typeText(m, ":q :4 :settings")
	if m.form == nil || m.form.Fields[0].Value.Text != ":q :4 :settings" || m.command != nil || m.mode != 1 {
		t.Fatal("aliases must remain literal text inside forms")
	}
}

func TestChatCommandFromFormActionPreservesEdits(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unsaved")
	f := m.form
	press(t, m, "enter")
	typeText(m, ":c")
	press(t, m, "enter")
	if m.focus != 2 || !m.chatEditing || m.form != f || m.current().Name != "Payments" {
		t.Fatal("chat command from a form action must focus chat without submitting the form")
	}
	if f.Fields[0].Value.Text != "Payments unsaved" {
		t.Fatal("opening chat lost unfinished form edits")
	}
}

func TestFreshChatCommandReplacesPendingCommandFromAnotherPanel(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	for _, origin := range []int{0, 2} {
		mainCommand(t, m, "1")
		typeText(m, ":s")
		focusPanel(t, m, origin)
		typeText(m, ":c")
		press(t, m, "enter")
		if m.focus != 2 || !m.chatEditing || m.command != nil {
			t.Fatal("fresh :c appended to a pending command instead of entering chat")
		}
		press(t, m, "esc")
		mainCommand(t, m, "1")
		typeText(m, ":s")
		focusPanel(t, m, origin)
		typeText(m, ":c")
		press(t, m, "esc")
		if m.focus != origin || m.command != nil {
			t.Fatal("cancelling a fresh command did not restore its originating panel")
		}
	}
}

func TestSmallTerminalAllowsQuitAliasOnly(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	mainCommand(t, m, "n")
	if m.form != nil || len(m.workspaces) != 1 {
		t.Fatal("a small terminal must not create an invisible workspace form")
	}
	mainCommand(t, m, "q")
	m.drainPersistence()
	cmd := m.syncQuit()
	if cmd == nil {
		t.Fatal("the short quit command must still work in a small terminal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("the short quit command did not request exit")
	}
}

func TestCommandBoxStaysAtMainPanelBottom(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	for _, size := range [][2]int{{80, 16}, {120, 30}, {200, 60}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			left := min(26, max(18, size[0]/6))
			right := min(48, max(24, size[0]*3/10))
			width, height := size[0]-left-right, size[1]-2
			before := strings.Split(ansi.Strip(m.mainPanel(width, height)), "\n")
			typeText(m, ":q")
			lines := strings.Split(ansi.Strip(m.mainPanel(width, height)), "\n")
			if len(lines) != height || lines[1] != before[1] {
				t.Fatal("opening command input must preserve panel height and mode tabs")
			}
			if !strings.Contains(strings.Join(lines, "\n"), "revision 1") {
				t.Fatal("current view content must remain above the command input")
			}
			for _, row := range []int{height - 4, height - 3, height - 2} {
				if ansi.StringWidth(lines[row]) != width || !strings.HasPrefix(lines[row], "│ ") || !strings.HasSuffix(lines[row], " │") {
					t.Fatalf("command box exceeds main body at row %d: %q", row, lines[row])
				}
			}
			if !strings.HasPrefix(lines[height-4], "│ ╭") || !strings.HasSuffix(lines[height-4], "╮ │") ||
				!strings.Contains(lines[height-3], ":q▏") || !strings.HasPrefix(lines[height-2], "│ ╰") || !strings.HasSuffix(lines[height-2], "╯ │") {
				t.Fatalf("command must use a three-row rounded box directly above panel bottom:\n%s", strings.Join(lines[height-4:], "\n"))
			}
			full := strings.Split(ansi.Strip(m.render()), "\n")
			if len(full) != size[1] || strings.Count(full[len(full)-2], "╰") != 3 || !strings.Contains(full[len(full)-1], "Enter Run") {
				t.Fatal("command box must stay within the main panel and preserve one footer")
			}
			press(t, m, "esc")
		})
	}
}

func TestCommandBoxKeepsWelcomeInPlace(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	m.closeSettings()
	m.mode = 1
	for _, size := range [][2]int{{80, 16}, {100, 19}, {120, 30}, {200, 60}} {
		for focus := range 3 {
			t.Run(fmt.Sprintf("%dx%d/focus%d", size[0], size[1], focus), func(t *testing.T) {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m.command = nil
				m.focus = focus
				width, height := m.panelWidths()[1], m.height-2
				render := func() string { return ansi.Strip(m.mainPanel(width, height)) }
				before := render()
				if !strings.Contains(before, "█") {
					t.Fatal("expected welcome artwork before opening the command box")
				}
				for _, input := range []string{":", "help"} {
					typeText(m, input)
					if m.command == nil {
						t.Fatal("command box did not open")
					}
					lines := strings.Split(render(), "\n")
					if len(lines) != height || strings.Join(lines[:height-4], "\n") != strings.Join(strings.Split(before, "\n")[:height-4], "\n") {
						t.Fatal("opening or typing a command moved the welcome content")
					}
				}
				press(t, m, "esc")
				if m.focus != focus || render() != before {
					t.Fatal("closing the command box did not restore the same view and focus")
				}
			})
		}
	}
}

func TestCommandBoxScrollsToCursor(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "1")
	typeText(m, ":start-"+strings.Repeat("界x", 100)+"-end")
	for _, width := range []int{38, 60, 124} {
		press(t, m, "end")
		lines := strings.Split(ansi.Strip(m.mainPanel(width, 14)), "\n")
		if !strings.Contains(lines[11], "-end▏") || strings.Contains(lines[11], "start-") || ansi.StringWidth(lines[11]) != width {
			t.Fatalf("command input must reveal the end cursor without overflowing: %q", lines[11])
		}
		press(t, m, "home")
		lines = strings.Split(ansi.Strip(m.mainPanel(width, 14)), "\n")
		if !strings.Contains(lines[11], ":▏start-") || strings.Contains(lines[11], "-end") || ansi.StringWidth(lines[11]) != width {
			t.Fatalf("command input must reveal the start cursor without overflowing: %q", lines[11])
		}
	}
}

func TestRetiredFirstCutCommandsDoNotRouteOrChangeScope(t *testing.T) {
	for _, command := range []string{"k", "kinds", "t", "build", "T", "activitytype", "M", "recovercontext"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			before, connection := m.explore.Preference, m.connection
			for _, focus := range []int{0, 1, 2} {
				m.focus = focus
				m.execute(command)
				if !strings.Contains(m.status, "Unknown command") || m.focus != focus || m.connection != connection || m.explore.Preference != before || m.explore.Picker != "" {
					t.Fatal("retired command changed navigation or scope", command, m.status)
				}
			}
		})
	}
}

func TestActivityTypeUsesArrowsInsteadOfLetterShortcut(t *testing.T) {
	m := activityTimelineModel(t)
	before := m.activityUI.Filter
	typeText(m, "T")
	if m.activityUI.Filter != before {
		t.Fatal("retired Activity shortcut still cycles types")
	}
	press(t, m, "right")
	if m.activityUI.Filter == before {
		t.Fatal("Activity arrows lost type navigation")
	}
}

func TestConnectionPickerRemainsVisibleAfterResourcePickerRemoval(t *testing.T) {
	m := exploreModel(t)
	m.execute("K")
	choices := m.exploreConnections()
	view := m.explorePickerBody(100, 20)
	if m.explore.Picker != "connections" || len(choices) == 0 || !strings.Contains(view, "Choose Kubernetes connection") || !strings.Contains(view, choices[0].Label) {
		t.Fatal("connection picker lost its visible choices", view)
	}
	press(t, m, "enter")
	if m.explore.Picker != "" || m.connection != choices[0].Connection.ID {
		t.Fatal("connection picker did not open its selected connection")
	}
}

func TestResourceRefreshUsesPlainKeyOrCommand(t *testing.T) {
	for _, command := range []string{"key g", "g", "refresh"} {
		t.Run(command, func(t *testing.T) {
			m := exploreModel(t)
			e := m.explore
			watch := e.Stream.Ctx
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if watch.Err() != nil {
				t.Fatal("retired Ctrl+R refreshed the resource table")
			}
			if command == "key g" {
				typeText(m, "g")
			} else {
				mainCommand(t, m, command)
			}
			if watch.Err() == nil || m.explore.Preference.Namespace != "shop" || m.explore.Preference.Resource != "pods" {
				t.Fatal("refresh did not replace the request while retaining scope")
			}
		})
	}
}

func TestRestorePromptCommandRetainsDraftAndDocument(t *testing.T) {
	for _, command := range []string{"p", "restoreprompt"} {
		t.Run(command, func(t *testing.T) {
			m, a := documentModel(t)
			a.Document.Paste("# unsaved\n")
			document := a.Document.Text()
			m.chat().Turns = []history.Turn{{Request: history.Request{Prompt: "/model"}}}
			m.draft().Insert("unfinished", true)
			mainCommand(t, m, command)
			if m.draft().Text != "unfinished" || !strings.Contains(m.status, "Clear the draft") {
				t.Fatal("restore replaced an unfinished draft")
			}
			*m.draft() = textview.NewField("")
			m.focusChat(true)
			m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if m.draft().Text != "" {
				t.Fatal("retired Ctrl+R restored a prompt")
			}
			press(t, m, "esc")
			m.rightHidden = true
			mainCommand(t, m, command)
			if m.focus != 2 || !m.chatEditing || m.rightHidden || m.draft().Text != "/model" || !m.draft().Literal {
				t.Fatal("restore should open a literal draft without sending")
			}
			if len(m.chat().Turns) != 1 || m.run != nil || m.explore.Action != a || a.Document.Text() != document || a.Pending != "" {
				t.Fatal("restoring a prompt sent a request or changed the document")
			}
		})
	}
	t.Run("empty history", func(t *testing.T) {
		m, _ := testModel(t)
		m.execute("p")
		if m.draft().Text != "" || !strings.Contains(m.status, "No previous message") {
			t.Fatal("empty history should explain why there is nothing to restore")
		}
	})
}

func TestEditorValidationUsesSaveCommand(t *testing.T) {
	m, a := documentModel(t)
	before := a.Document.Text()
	for _, insert := range []bool{false, true} {
		if insert {
			typeText(m, "i")
		}
		m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if a.Pending != "" || a.Busy || a.Document.Text() != before {
			t.Fatal("retired Ctrl+S changed or validated the draft")
		}
	}
	press(t, m, "esc")
	a.Document.Paste("# changed\n")
	before = a.Document.Text()
	m.execute("w")
	if a.Pending != "validate" || a.Document.Text() != before {
		t.Fatal(":w should request validation without changing the document")
	}
}

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

func TestDoubleKeyActionsFromEveryPanel(t *testing.T) {
	for _, key := range []string{"b", "i", "s", "c", "h", "f", "1", "2", "3"} {
		for focus := 0; focus < 3; focus++ {
			t.Run(fmt.Sprintf("%s%s/from%d", key, key, focus), func(t *testing.T) {
				m, _ := testModel(t)
				id := createWorkspace(t, m, "Payments")
				mainCommand(t, m, "1")
				beforeMode, afterMode := 0, 0
				if key >= "1" && key <= "3" {
					afterMode = int(key[0] - '1')
					beforeMode = (afterMode + 1) % len(modes)
					m.mode = beforeMode
				}
				focusPanel(t, m, focus)
				typeText(m, key)
				if m.shortcutPrefix != key || m.focus != focus || m.leftHidden || m.rightHidden || m.chatEditing || m.help != nil || m.page != "" || m.mode != beforeMode || m.viewExpansion != nil {
					t.Fatal("the first key executed an action")
				}
				lines := strings.Split(ansi.Strip(m.render()), "\n")
				if len(lines) != m.height || !strings.Contains(lines[len(lines)-1], key+" → "+key) {
					t.Fatal("the pending shortcut must appear in the single footer")
				}
				m.Update(autosaveMsg{})
				typeText(m, key)
				if m.shortcutPrefix != "" || m.command != nil || m.active != id || m.mode != afterMode {
					t.Fatal("completed shortcut changed scope or left a pending command")
				}
				switch key {
				case "f":
					if !m.viewExpanded() || m.focus != 1 || m.panelWidths() != [3]int{0, m.width, 0} {
						t.Fatal("ff did not expand and focus main")
					}
				case "1", "2", "3":
					if m.focus != 1 || m.page != "" {
						t.Fatal("mode shortcut did not focus the main view")
					}
				case "b":
					if !m.leftHidden || m.focus == 0 {
						t.Fatal("bb did not hide the sidebar and repair focus")
					}
				case "i":
					if m.rightHidden || m.assistantSize != assistantDouble || m.focus != focus {
						t.Fatal("ii did not expand AI while preserving focus")
					}
				case "s":
					if m.page != "settings" || m.focus != 1 {
						t.Fatal("ss did not open Settings")
					}
				case "h":
					if m.help == nil || m.focus != focus {
						t.Fatal("hh did not open Help while preserving focus")
					}
				case "c":
					if m.focus != 2 || !m.chatEditing {
						t.Fatal("cc did not enter the composer")
					}
					typeText(m, "bb ii ss cc hh 11 22 33 44")
					if m.draft().Text != "bb ii ss cc hh 11 22 33 44" || m.leftHidden || m.rightHidden {
						t.Fatal("shortcut letters stopped being literal after entering chat")
					}
				}
			})
		}
	}
}

func TestRetiredFourthModeShortcut(t *testing.T) {
	for focus := 0; focus < 3; focus++ {
		m, _ := testModel(t)
		mainCommand(t, m, "2")
		focusPanel(t, m, focus)
		typeText(m, "44")
		if m.mode != 1 || m.focus != focus || m.shortcutPrefix != "" {
			t.Fatal("44 must not change mode or begin a shortcut")
		}
	}
}

func TestDoubleKeyPrefixClearsOnInterveningInput(t *testing.T) {
	for _, event := range []string{"esc", "down", "x", "tab", "shift+tab", "paste", "resize", "blur", "colon"} {
		t.Run(event, func(t *testing.T) {
			m, _ := testModel(t)
			mainCommand(t, m, "s")
			typeText(m, "b")
			switch event {
			case "paste":
				m.Update(tea.PasteMsg{Content: "bb ii ss cc hh 11 22 33 44"})
			case "resize":
				m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
			case "blur":
				m.Update(tea.BlurMsg{})
			case "colon":
				typeText(m, ":")
				press(t, m, "esc")
			case "x":
				typeText(m, "x")
			default:
				press(t, m, event)
			}
			if m.shortcutPrefix != "" || m.page != "settings" {
				t.Fatal("intervening input did not clear the prefix, or Esc also closed Settings")
			}
			typeText(m, "b")
			if m.leftHidden || m.shortcutPrefix != "b" {
				t.Fatal("a key completed a sequence from before the interruption")
			}
		})
	}
	m, _ := testModel(t)
	typeText(m, "bii")
	if m.leftHidden || m.assistantSize != assistantDouble || m.shortcutPrefix != "" {
		t.Fatal("a different shortcut key did not begin a fresh sequence")
	}
	typeText(m, "b")
	m.Update(tea.KeyPressMsg{Code: 'b', Text: "b", IsRepeat: true})
	if m.leftHidden {
		t.Fatal("a reported key repeat completed a shortcut")
	}
	typeText(m, "b")
	if !m.leftHidden {
		t.Fatal("a separate keypress did not complete the shortcut")
	}
}

func TestDoubleKeysStayLiteralInEditors(t *testing.T) {
	for _, editor := range []string{"form", "chat", "command"} {
		t.Run(editor, func(t *testing.T) {
			m, _ := testModel(t)
			createWorkspace(t, m, "Payments")
			mainCommand(t, m, "1")
			switch editor {
			case "form":
				mainCommand(t, m, "new")
			case "chat":
				typeText(m, "cc")
			case "command":
				typeText(m, ":")
			}
			typeText(m, "bb ii ss cc hh 11 22 33 44")
			if m.shortcutPrefix != "" || m.leftHidden || m.rightHidden || m.page != "" || m.help != nil || m.mode != 0 {
				t.Fatal("editor input triggered a navigation action")
			}
			var value string
			switch editor {
			case "form":
				value = m.form.Fields[0].Value.Text
			case "chat":
				value = m.draft().Text
			case "command":
				value = m.command.Text
			}
			if value != "bb ii ss cc hh 11 22 33 44" {
				t.Fatalf("editor changed literal text: %q", value)
			}
		})
	}
}

func TestDoubleKeyCommandsPreserveFormsAndReplacePendingCommands(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	createWorkspace(t, m, "Payments")
	openSettingsItem(t, m, "Rename Workspace")
	typeText(m, " unsaved")
	f := m.form
	press(t, m, "enter")
	for _, sequence := range []string{"ss", "11", "22", "33"} {
		typeText(m, sequence)
		if m.form != f || !strings.Contains(m.status, "Finish or cancel") || m.mode != mode {
			t.Fatalf("%s bypassed the unfinished-form guard", sequence)
		}
	}
	typeText(m, "b")
	press(t, m, "esc")
	if m.form != f {
		t.Fatal("cancelling the prefix cancelled the form")
	}
	typeText(m, "iicc")
	if m.form != f || m.current().Name != "Payments" || m.rightHidden || m.focus != 2 || !m.chatEditing {
		t.Fatal("form actions failed to reveal chat while preserving edits")
	}
	press(t, m, "esc")
	focusPanel(t, m, 1)
	press(t, m, "esc")
	press(t, m, "esc")
	typeText(m, ":q")
	focusPanel(t, m, 0)
	typeText(m, "ss")
	if m.command != nil || m.page != "settings" || m.focus != 1 {
		t.Fatal("completed shortcut retained a stale command instead of opening Settings")
	}
}

func TestDoubleKeysInHelpAndHistory(t *testing.T) {
	m, _ := testModel(t)
	createWorkspace(t, m, "Payments")
	mainCommand(t, m, "help")
	typeText(m, "b")
	press(t, m, "esc")
	if m.help == nil {
		t.Fatal("prefix cancellation closed Help")
	}
	typeText(m, "bbiiii")
	if m.help == nil || !m.leftHidden || !m.rightHidden {
		t.Fatal("help did not apply visibility changes to the underlying layout")
	}
	typeText(m, "cc")
	if m.help != nil || m.rightHidden || m.focus != 2 || !m.chatEditing {
		t.Fatal("cc from Help failed to open chat")
	}
	press(t, m, "esc")
	mainCommand(t, m, "H")
	typeText(m, "cc")
	if m.chat().History || !m.chatEditing {
		t.Fatal("cc from history failed to enter the composer")
	}
}

func TestDoubleKeyPrefixDoesNotSurviveRestartOrSmallTerminal(t *testing.T) {
	m, _ := testModel(t)
	mode := m.mode
	typeText(m, "b")
	if err := m.saveSession(); err != nil {
		t.Fatal(err)
	}
	restored, err := newModel(m.store, m.history, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if restored.shortcutPrefix != "" {
		t.Fatal("restart restored an incomplete shortcut")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	typeText(m, "bb iiss cc hh 11 22 33 44")
	if m.shortcutPrefix != "" || m.leftHidden || m.rightHidden || m.page != "" || m.chatEditing || m.mode != mode {
		t.Fatal("small-terminal fallback executed hidden actions")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	typeText(m, "b")
	assertCtrlCQuit(t, m)
}
