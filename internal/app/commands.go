package app

import "strings"

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
	{"X", "stopchat"},
	{"p", "restoreprompt"},
	{"k", "kinds"},
	{"K", "connections"},
	{"N", "namespace"},
	{"g", "refresh"},
	{"y", "yaml"},
	{"d", "describe"},
	{"z", "zoom"},
	{"L", "linenumbers"},
	{"v", "review"},
	{"a", "actions"},
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
