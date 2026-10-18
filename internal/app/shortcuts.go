package app

import (
	tea "charm.land/bubbletea/v2"
)

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
