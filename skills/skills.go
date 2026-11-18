// Package skills embeds the agent workflows distributed with Plous.
package skills

import (
	_ "embed"
	"strings"
)

//go:embed plous-debug/SKILL.md
var DebugSkill string

//go:embed plous-monitor/SKILL.md
var MonitorSkill string

func DebugInstructions() string {
	_, body, _ := strings.Cut(strings.TrimPrefix(DebugSkill, "---\n"), "\n---\n")
	return body
}
