package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	textview "github.com/simjay/plous/internal/ui/text"
	"hash/fnv"
	"strings"
)

func MouseTarget(text, target string) string { return textview.Mouse(text, target) }

func MouseCommand(text, command string) string { return MouseTarget(text, "command/"+command) }

func MouseAction(text, key string) string { return MouseTarget(text, "action/"+key) }

func MouseChoice(text string, index int, identity string) string {
	h := fnv.New64a()
	h.Write([]byte(ansi.Strip(identity)))
	return MouseTarget(text, fmt.Sprintf("choice/%d/%x", index, h.Sum64()))
}

func MouseFooterKey(key string) bool {
	if strings.HasPrefix(key, ":") {
		return true
	}
	switch key {
	case "Enter", "Esc", "Space", "Backspace", "hh", "cc", "bb", "ii", "ss", "ff", "/":
		return true
	}
	return len([]rune(key)) == 1 && strings.Contains("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!", key)
}
