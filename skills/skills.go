// Package skills embeds the debugging workflow distributed with Plous.
package skills

import (
	_ "embed"
	"strings"
)

//go:embed plous-debug/SKILL.md
var DebugSkill string

func DebugInstructions() string {
	_, body, _ := strings.Cut(strings.TrimPrefix(DebugSkill, "---\n"), "\n---\n")
	return body
}
