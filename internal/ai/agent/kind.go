package agent

import "slices"

type Kind string

const (
	Plous       Kind = "plous"
	ClaudeCode  Kind = "claude-code"
	Codex       Kind = "codex"
	OpenCode    Kind = "opencode"
	GrokBuild   Kind = "grok"
	Antigravity Kind = "agy"
	Cursor      Kind = "cursor"
	GeminiCLI   Kind = "gemini"
)

func (k Kind) Effective() Kind {
	if k == "" {
		return Plous
	}
	return k
}

var labels = map[Kind]string{
	Plous:       "Plous",
	ClaudeCode:  "Claude Code",
	Codex:       "Codex",
	OpenCode:    "OpenCode",
	GrokBuild:   "Grok Build",
	Antigravity: "Antigravity CLI",
	Cursor:      "Cursor Agent",
	GeminiCLI:   "Gemini CLI",
}

var panelKinds = [...]Kind{Plous, ClaudeCode, Codex}

// PanelKinds returns the supported assistant choices in display order.
func PanelKinds() []Kind { return slices.Clone(panelKinds[:]) }

func (k Kind) Valid() bool {
	_, ok := labels[k.Effective()]
	return ok
}

func (k Kind) Label() string {
	if label, ok := labels[k.Effective()]; ok {
		return label
	}
	return string(k)
}

// SupportsPanel reports whether Plous can run this agent in its assistant panel.
func (k Kind) SupportsPanel() bool {
	return slices.Contains(panelKinds[:], k.Effective())
}
