package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	textview "github.com/simjay/plous/internal/ui/text"
)

type commandDefinition struct {
	key, name string
}

var applicationCommands = []commandDefinition{
	{"n", "new"},
	{"s", "settings"},
	{"h", "help"},
	{"b", "sidebar"},
	{"i", "aiinteraction"},
	{"q", "quit"},
	{"c", "chat"},
	{"C", "newchat"},
	{"H", "openchat"},
	{"A", "readreply"},
	{"X", "stopchat"},
	{"p", "restoreprompt"},
	{"K", "connections"},
	{"N", "namespace"},
	{"g", "refresh"},
	{"y", "yaml"},
	{"d", "describe"},
	{"f", "fullscreen"},
	{"e", "expandsection"},
	{"L", "linenumbers"},
	{"v", "review"},
	{"a", "actions"},
	{"B", "rollback"},
	{"J", "release"},
	{"F", "revision"},
	{"O", "observations"},
	{"I", "charts"},
	{"W", "chartrange"},
	{"P", "chartlatest"},
}

// Resolve exact, case-sensitive aliases without interpreting resource names as prefixes.
func canonicalCommand(command string) string {
	name, argument, hasArgument := strings.Cut(command, " ")
	for _, definition := range applicationCommands {
		if name == definition.key {
			if hasArgument {
				return definition.name + " " + argument
			}
			return definition.name
		}
	}
	return command
}

func (m *model) beginCommand() {
	m.mouse.commandPaused = false
	// Panel travel resumes a command draft; a fresh colon starts a new command.
	m.unparkOverlays()
	e := textview.NewField("")
	m.command = &e
	m.commandFocus = m.focus
	m.focus = 1
	m.status = ""
}

func (m *model) execute(command string) bool {
	m.shortcutPrefix = ""
	if m.chatSourcesVisible() {
		switch canonicalCommand(command) {
		case "help", "quit", "stopchat", "readreply":
		case "chat", "newchat", "openchat", "settings", "sidebar", "aiinteraction", "1", "2", "3":
			m.closeChatSources()
		default:
			m.status = "Close Sources before using this command."
			return false
		}
	}
	if m.presentedDocumentVisible() && m.focus == 1 && !m.helpVisible() && m.settingsOverlay == nil {
		switch command {
		case "q", "q!":
			m.closePresentedDocument()
			return false
		case "w", "wq", "v":
			m.status = "This document is read-only."
			return false
		}
	}

	if m.helmDocumentCommand(command) {
		return false
	}
	if m.editorLocalCommand(command) {
		return false
	}
	command = canonicalCommand(command)
	if command == "refresh" && m.currentRoot && m.current() == nil && m.focus != 0 {
		m.requestCurrent("", true)
		return false
	}
	if command == "refresh" && m.focus == 0 && !m.helpVisible() {
		m.refreshTree()
		return false
	}
	if m.overviewCommand(command) {
		return false
	}

	if m.executeGitOps(command) {
		return false
	}
	if m.executeHelm(command) {
		return false
	}
	if m.chartCommand(command) {
		return false
	}
	if m.viewCommand(command) {
		return false
	}
	if m.activityCommand(command) {
		return false
	}
	if m.explore != nil && m.explore.Action != nil && !slices.Contains([]string{"quit", "help", "chat", "newchat", "openchat", "readreply", "stopchat", "restoreprompt", "sidebar", "aiinteraction"}, command) {
		m.status = "Finish or cancel the open resource action first."
		return false
	}
	if (m.form != nil || m.removeForm != nil) && (command == "new" || command == "settings" || command == "1" || command == "2" || command == "3") {
		m.status = "Finish or cancel the open form in the main panel first."
		return false
	}
	switch command {
	case "1", "2", "3":
		m.closeHelp()
		m.closeSettings()
		m.focus = 1
		m.mode = int(command[0] - '1')
		if m.mode == 2 && m.activityUI.Error != "" {
			m.loadHistory()
		}
		m.page = ""
		m.detail = ""
		if m.mode == 2 {
			m.restoreActivitySearch()
		}
		m.status = ""
	case "review":
		if m.form != nil || m.settingsOverlay != nil || !m.openActivityReview() {
			m.status = "Open an Activity entry to view its changes."
		} else {
			m.closeHelp()
			m.focus = 1
			m.status = ""
		}
	case "new":
		m.closeHelp()
		m.beginWorkspace()
	case "settings":
		m.closeHelp()
		m.openSettings()
	case "help":
		m.openHelp()
		m.status = ""
	case "aiinteraction":
		m.cycleAssistantSize()
	case "sidebar":
		m.restoreView()
		hidden, name := &m.leftHidden, "Sidebar"
		*hidden = !*hidden
		m.ensureVisibleFocus()
		m.status = name + " shown."
		if *hidden {
			m.status = name + " hidden."
		}
	case "chat", "newchat", "openchat":
		m.chatCommand(command)
	case "readreply":
		m.openChatReply()
	case "restoreprompt":
		m.restoreChatPrompt()
	case "stopchat":
		if m.nativeAgent != nil && m.nativeAgent.SaveErr != nil {
			m.retryNativeSave()
		} else if m.nativeAgent != nil && !m.nativeAgent.Finished {
			m.stopNativeAgent()
			m.status = "Closing native agent session."
		} else if m.run == nil {
			m.status = "No AI response is running."
		} else if m.run.Finished {
			m.status = "Reply is awaiting durable save. Received text is retained."
		} else {
			m.stopChat()
			m.status = "Stopping AI response. Received text is being saved."
		}
	case "quit":
		if m.quitting {
			return false
		}
		m.quitting, m.persistenceErr = true, nil
		m.stopNativeAgent()
		m.retryNativeSave()
		m.persist()
		for _, c := range m.chats {
			if c.Dirty && c.SaveFailed {
				m.queueChatSave(c)
			}
			if c.Restore != nil {
				c.Restore.Cancel()
				c.Restore = nil
			}
		}
		m.searches[2].cancelChatFind()
		m.stopChat()
		m.status = "Saving before exit…"

	default:
		if !m.executeExplore(command) {
			m.status = "Unknown command. Use :h for available commands."
		}
	}
	return false
}

var navigationShortcuts = []struct{ key, command, label string }{
	{"b", "sidebar", "toggle sidebar"},
	{"i", "aiinteraction", "resize or hide AI panel"},
	{"s", "settings", "open Settings"},
	{"c", "chat", "type in chat"},
	{"h", "help", "open Help"},
	{"f", "fullscreen", "fullscreen or restore layout"},
}

func navigationShortcut(key string) (command, label string) {
	if key == "1" || key == "2" || key == "3" {
		return key, "open " + modes[int(key[0]-'1')]
	}
	for _, shortcut := range navigationShortcuts {
		if shortcut.key == key {
			return shortcut.command, shortcut.label
		}
	}
	return "", ""
}

func navigationBinding(command string) string {
	for _, shortcut := range navigationShortcuts {
		if shortcut.command == command {
			return shortcut.key + shortcut.key
		}
	}
	return ""
}

// Called only in navigation, after text editors have handled their input.
func (m *model) updateShortcut(k tea.KeyPressMsg) bool {
	key := k.String()
	command, _ := navigationShortcut(key)

	if k.IsRepeat && command != "" {
		return true
	}
	previous := m.shortcutPrefix
	m.shortcutPrefix = ""
	if key == "esc" && previous != "" {
		return true
	}
	if command == "" {
		return false
	}
	m.status = ""
	if previous == key {
		// A completed shortcut replaces a command left pending in another panel.
		m.command = nil
		m.execute(command)
	} else {
		m.shortcutPrefix = key
	}
	return true
}
