package agent

import "context"

type Kind string

const (
	Plous      Kind = "plous"
	ClaudeCode Kind = "claude-code"
	Codex      Kind = "codex"
)

func (k Kind) Effective() Kind {
	if k == "" {
		return Plous
	}
	return k
}
func (k Kind) Valid() bool {
	switch k.Effective() {
	case Plous, ClaudeCode, Codex:
		return true
	}
	return false
}
func (k Kind) Label() string {
	switch k.Effective() {
	case Plous:
		return "Plous"
	case ClaudeCode:
		return "Claude Code"
	case Codex:
		return "Codex"
	}
	return string(k)
}

type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
type Turn struct {
	ID, System, Prompt  string
	Context             []Message
	MaxCalls, MaxTokens int
}
type Usage struct {
	Reported bool  `json:"reported"`
	Input    int64 `json:"input_tokens"`
	Output   int64 `json:"output_tokens"`
	Total    int64 `json:"total_tokens"`
}
type Event struct {
	TurnID, Text, Progress    string
	ContextUsed, ContextLimit int
}
type Result struct {
	Text, Status string
	Usage        Usage
}
type Emit func(Event) error

type Backend interface {
	// Events carry full text snapshots or progress/context updates for this turn.
	// A failed emission stops execution. Success requires a completed terminal result.
	Run(context.Context, Turn, Tools, Emit) (Result, error)
}
