package app

import (
	"strings"
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
	{"G", "graph"},
	{"y", "yaml"},
	{"d", "describe"},
	{"f", "fullscreen"},
	{"e", "expandsection"},
	{"L", "linenumbers"},
	{"v", "review"},
	{"a", "actions"},
	{"B", "rollback"},
	{"r", "render"},
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
