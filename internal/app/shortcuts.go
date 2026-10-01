package app

import tea "charm.land/bubbletea/v2"

func navigationShortcut(key string) (command, label string) {
	switch key {
	case "1", "2", "3":
		return key, "open " + modes[int(key[0]-'1')]
	case "b":
		return "sidebar", "toggle sidebar"
	case "i":
		return "aiinteraction", "toggle AI panel"
	case "s":
		return "settings", "open Settings"
	case "c":
		return "chat", "type in chat"
	case "h":
		return "help", "open Help"
	}
	return "", ""
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
